package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var (
	ErrProhibitedScheme   = errors.New("prohibited URL scheme: only http and https are allowed")
	ErrProhibitedHostname = errors.New("prohibited hostname: local or internal hostnames are blocked")
	ErrProhibitedIP       = errors.New("prohibited destination: private, loopback, or link-local IP addresses are blocked")
	ErrDNSResolution      = errors.New("failed to resolve webhook destination hostname")
)

// Prohibited IP networks (CIDR blocks)
var prohibitedCIDRs = []*net.IPNet{
	// IPv4 Loopback
	mustParseCIDR("127.0.0.0/8"),
	// IPv4 RFC1918 Private Networks
	mustParseCIDR("10.0.0.0/8"),
	mustParseCIDR("172.16.0.0/12"),
	mustParseCIDR("192.168.0.0/16"),
	// IPv4 Link-Local / Cloud Metadata (169.254.169.254)
	mustParseCIDR("169.254.0.0/16"),
	// IPv4 Broadcast / Multicast / Unspecified
	mustParseCIDR("0.0.0.0/8"),
	mustParseCIDR("224.0.0.0/4"),
	mustParseCIDR("240.0.0.0/4"),
	mustParseCIDR("255.255.255.255/32"),

	// IPv6 Loopback / Unspecified
	mustParseCIDR("::1/128"),
	mustParseCIDR("::/128"),
	// IPv6 Link-Local
	mustParseCIDR("fe80::/10"),
	// IPv6 Unique Local (RFC4193)
	mustParseCIDR("fc00::/7"),
	// IPv6 Multicast
	mustParseCIDR("ff00::/8"),
	// IPv6 IPv4-mapped IPv4 private ranges
	mustParseCIDR("::ffff:127.0.0.0/104"),
	mustParseCIDR("::ffff:10.0.0.0/104"),
	mustParseCIDR("::ffff:172.16.0.0/108"),
	mustParseCIDR("::ffff:192.168.0.0/112"),
	mustParseCIDR("::ffff:169.254.0.0/112"),
}

func mustParseCIDR(cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR literal: %s", cidr))
	}
	return ipNet
}

// Prohibited hostnames and suffixes
var prohibitedHostnames = []string{
	"localhost",
	"localhost.localdomain",
	"127.0.0.1",
	"::1",
	"metadata.google.internal",
	"metadata",
	"instance-data",
}

var prohibitedSuffixes = []string{
	".local",
	".internal",
	".corp",
	".home",
	".lan",
	".arpa",
}

// ValidateURL performs static and DNS verification against SSRF targets.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL syntax: %w", err)
	}

	// 1. Validate Scheme
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrProhibitedScheme
	}

	hostname := strings.ToLower(u.Hostname())
	if hostname == "" {
		return ErrProhibitedHostname
	}

	// 2. Validate Hostname Strings
	for _, forbidden := range prohibitedHostnames {
		if hostname == forbidden {
			return ErrProhibitedHostname
		}
	}
	for _, suffix := range prohibitedSuffixes {
		if strings.HasSuffix(hostname, suffix) {
			return ErrProhibitedHostname
		}
	}

	// 3. Resolve IP & Validate against Prohibited Networks
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrDNSResolution, err.Error())
	}
	if len(ips) == 0 {
		return ErrDNSResolution
	}

	for _, ip := range ips {
		if IsProhibitedIP(ip) {
			return fmt.Errorf("%w: %s resolves to %s", ErrProhibitedIP, hostname, ip.String())
		}
	}

	return nil
}

// IsProhibitedIP checks if an IP belongs to loopback, private, link-local, or cloud metadata ranges.
func IsProhibitedIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}

	for _, block := range prohibitedCIDRs {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

// NewSafeHTTPClient returns an http.Client with a custom DialContext preventing DNS rebinding.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			return nil
		},
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Validate and resolve
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrDNSResolution, err)
			}
			if len(ips) == 0 {
				return nil, ErrDNSResolution
			}

			// Verify all returned IPs
			var targetIP net.IP
			for _, ip := range ips {
				if IsProhibitedIP(ip) {
					return nil, fmt.Errorf("%w: target %s resolves to prohibited IP %s", ErrProhibitedIP, host, ip.String())
				}
				if targetIP == nil {
					targetIP = ip
				}
			}

			// Dial resolved safe IP directly
			targetAddr := net.JoinHostPort(targetIP.String(), port)
			return dialer.DialContext(ctx, network, targetAddr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
