package ssrf_test

import (
	"net"
	"testing"

	"github.com/aurora-vm/aurora/internal/infra/ssrf"
)

func TestSSRF_ValidateURL(t *testing.T) {
	tests := []struct {
		url     string
		blocked bool
	}{
		{"http://localhost/webhook", true},
		{"http://127.0.0.1:8080/hook", true},
		{"http://10.0.0.1/notify", true},
		{"http://172.16.0.1/notify", true},
		{"http://192.168.1.1/notify", true},
		{"http://169.254.169.254/latest/meta-data", true}, // Cloud metadata
		{"http://[::1]/webhook", true},
		{"file:///etc/passwd", true},
		{"gopher://127.0.0.1:70", true},
		{"http://server.local/hook", true},
		{"http://internal.corp/hook", true},
		{"https://api.github.com/webhook", false},
		{"https://webhook.site/test-123", false},
	}

	for _, tt := range tests {
		err := ssrf.ValidateURL(tt.url)
		if tt.blocked && err == nil {
			t.Errorf("expected URL %s to be blocked by SSRF filter, but it passed", tt.url)
		} else if !tt.blocked && err != nil {
			t.Errorf("expected public URL %s to be allowed, but got error: %v", tt.url, err)
		}
	}
}

func TestSSRF_IsProhibitedIP(t *testing.T) {
	prohibitedIPs := []string{
		"127.0.0.1",
		"10.200.0.5",
		"172.20.10.1",
		"192.168.0.100",
		"169.254.169.254",
		"0.0.0.0",
		"224.0.0.1",
		"::1",
		"fe80::1",
		"fc00::1",
		"::ffff:127.0.0.1",
		"::ffff:10.0.0.1",
	}

	for _, ipStr := range prohibitedIPs {
		ip := net.ParseIP(ipStr)
		if !ssrf.IsProhibitedIP(ip) {
			t.Errorf("expected IP %s to be recognized as prohibited", ipStr)
		}
	}

	allowedIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"140.82.121.4",
		"2606:4700:4700::1111",
	}

	for _, ipStr := range allowedIPs {
		ip := net.ParseIP(ipStr)
		if ssrf.IsProhibitedIP(ip) {
			t.Errorf("expected public IP %s to be allowed, but was blocked", ipStr)
		}
	}
}
