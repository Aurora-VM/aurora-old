package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

// InternalCA manages the Aurora root certificate authority, CSR signing, and mTLS certificate verification.
type InternalCA struct {
	caCert     *x509.Certificate
	caCertPEM  []byte
	caKey      *ecdsa.PrivateKey
	caKeyPEM   []byte
	certPool   *x509.CertPool
}

// NewInternalCA initializes a self-signed Root CA or restores from provided PEM blocks.
func NewInternalCA(existingCertPEM, existingKeyPEM []byte) (*InternalCA, error) {
	if len(existingCertPEM) > 0 && len(existingKeyPEM) > 0 {
		return loadExistingCA(existingCertPEM, existingKeyPEM)
	}
	return generateNewCA()
}

func generateNewCA() (*InternalCA, error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"Project Aurora Virtualization"},
			CommonName:    "Aurora Internal Root CA",
			Country:       []string{"US"},
		},
		NotBefore:             now.Add(-10 * time.Minute),
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour), // 10 years
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create self-signed CA certificate: %w", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated CA cert: %w", err)
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CA private key: %w", err)
	}
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certPool := x509.NewCertPool()
	certPool.AddCert(caCert)

	return &InternalCA{
		caCert:    caCert,
		caCertPEM: caCertPEM,
		caKey:     privKey,
		caKeyPEM:  caKeyPEM,
		certPool:  certPool,
	}, nil
}

func loadExistingCA(certPEM, keyPEM []byte) (*InternalCA, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CA certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("invalid CA private key PEM")
	}

	var privKey *ecdsa.PrivateKey
	if keyBlock.Type == "EC PRIVATE KEY" {
		privKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	} else if keyBlock.Type == "PRIVATE KEY" {
		key, parseErr := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if parseErr == nil {
			var ok bool
			privKey, ok = key.(*ecdsa.PrivateKey)
			if !ok {
				err = errors.New("PKCS8 private key is not ECDSA")
			}
		} else {
			err = parseErr
		}
	} else {
		err = fmt.Errorf("unsupported key PEM type: %s", keyBlock.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(cert)

	return &InternalCA{
		caCert:    cert,
		caCertPEM: certPEM,
		caKey:     privKey,
		caKeyPEM:  keyPEM,
		certPool:  certPool,
	}, nil
}

// GetCACertificatePEM returns the Root CA certificate in PEM format.
func (ca *InternalCA) GetCACertificatePEM() []byte {
	return ca.caCertPEM
}

// GetCAKeyPEM returns the Root CA private key in PEM format.
func (ca *InternalCA) GetCAKeyPEM() []byte {
	return ca.caKeyPEM
}

// SignNodeCSR parses a PKCS#10 CSR, signs a 90-day client certificate, and returns the certificate PEM and SHA-256 fingerprint.
func (ca *InternalCA) SignNodeCSR(csrPEM []byte, nodeID, nodeName string, ttl time.Duration) ([]byte, string, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || (block.Type != "CERTIFICATE REQUEST" && block.Type != "NEW CERTIFICATE REQUEST") {
		return nil, "", errors.New("invalid or unparseable CSR PEM block")
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse certificate request: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return nil, "", fmt.Errorf("invalid CSR signature: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate certificate serial number: %w", err)
	}

	if ttl <= 0 {
		ttl = 90 * 24 * time.Hour // Default 90 days
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Project Aurora Virtualization"},
			CommonName:   fmt.Sprintf("node:%s", nodeID),
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Add DNS and IP SANs
	if nodeName != "" {
		template.DNSNames = append(template.DNSNames, nodeName)
	}
	if ip := net.ParseIP(nodeName); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.caCert, csr.PublicKey, ca.caKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to sign node certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	fingerprint := ca.computeFingerprintFromDER(certDER)

	return certPEM, fingerprint, nil
}

// GenerateServerCertificate generates and signs an mTLS server certificate for the Control Plane listener.
func (ca *InternalCA) GenerateServerCertificate(hosts []string, ttl time.Duration) (certPEM, keyPEM []byte, err error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate server private key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, err
	}

	if ttl <= 0 {
		ttl = 365 * 24 * time.Hour // 1 year
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Project Aurora Virtualization"},
			CommonName:   "aurora-control-plane",
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.caCert, &privKey.PublicKey, ca.caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign server certificate: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(privKey)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// VerifyCertificate verifies a PEM certificate against the CA pool and extracts the node ID and SHA-256 fingerprint.
func (ca *InternalCA) VerifyCertificate(certPEM []byte) (string, string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", "", errors.New("invalid certificate PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	opts := x509.VerifyOptions{
		Roots:     ca.certPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	if _, err := cert.Verify(opts); err != nil {
		return "", "", fmt.Errorf("client certificate verification failed: %w", err)
	}

	fingerprint := ca.computeFingerprintFromDER(cert.Raw)

	// Extract Node ID from Subject CN: "node:<node_id>"
	cn := cert.Subject.CommonName
	if !strings.HasPrefix(cn, "node:") {
		return "", fingerprint, errors.New("certificate common name does not conform to node:<node_id> format")
	}

	nodeID := strings.TrimPrefix(cn, "node:")
	return nodeID, fingerprint, nil
}

// ComputeFingerprint computes the SHA-256 hex string fingerprint of a certificate PEM.
func (ca *InternalCA) ComputeFingerprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("invalid certificate PEM block")
	}
	return ca.computeFingerprintFromDER(block.Bytes), nil
}

func (ca *InternalCA) computeFingerprintFromDER(der []byte) string {
	h := sha256.Sum256(der)
	return hex.EncodeToString(h[:])
}

// BuildServerTLSConfig constructs a server TLS configuration enforcing mTLS 1.3.
func (ca *InternalCA) BuildServerTLSConfig(serverCertPEM, serverKeyPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid server key pair: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    ca.certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// BuildClientTLSConfig constructs a client TLS configuration for node agents connecting over mTLS 1.3.
func (ca *InternalCA) BuildClientTLSConfig(clientCertPEM, clientKeyPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid client key pair: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      ca.certPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
