package keystore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// KeyStore manages secure on-disk cryptographic keys and certificates for the Node Agent.
type KeyStore struct {
	stateDir string
}

// NewKeyStore creates a KeyStore for the specified state directory.
func NewKeyStore(stateDir string) (*KeyStore, error) {
	if stateDir == "" {
		return nil, errors.New("state directory path cannot be empty")
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create keystore directory: %w", err)
	}
	return &KeyStore{stateDir: stateDir}, nil
}

func (k *KeyStore) KeyPath() string  { return filepath.Join(k.stateDir, "node.key") }
func (k *KeyStore) CertPath() string { return filepath.Join(k.stateDir, "node.crt") }
func (k *KeyStore) CAPath() string   { return filepath.Join(k.stateDir, "ca.crt") }

// HasCertificates checks if all required mTLS files exist.
func (k *KeyStore) HasCertificates() bool {
	_, errK := os.Stat(k.KeyPath())
	_, errC := os.Stat(k.CertPath())
	_, errCA := os.Stat(k.CAPath())
	return errK == nil && errC == nil && errCA == nil
}

// GenerateKeyAndCSR creates a local ECDSA P-256 private key with 0600 permissions and generates a CSR.
func (k *KeyStore) GenerateKeyAndCSR(nodeName, fqdn string) (csrPEM []byte, err error) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate node private key: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	// Write with strict 0600 file permissions
	if err := os.WriteFile(k.KeyPath(), keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("failed to write node private key to disk: %w", err)
	}

	// Create PKCS#10 CSR
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Project Aurora Node"},
			CommonName:   nodeName,
		},
	}

	if fqdn != "" {
		if ip := net.ParseIP(fqdn); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, fqdn)
		}
	}
	if nodeName != "" && nodeName != fqdn {
		if ip := net.ParseIP(nodeName); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, nodeName)
		}
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate request: %w", err)
	}

	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	return csrPEM, nil
}

// SaveCertificates persists the signed client certificate and Root CA certificate.
func (k *KeyStore) SaveCertificates(caCertPEM, nodeCertPEM []byte) error {
	if err := os.WriteFile(k.CAPath(), caCertPEM, 0644); err != nil {
		return fmt.Errorf("failed to save CA certificate: %w", err)
	}
	if err := os.WriteFile(k.CertPath(), nodeCertPEM, 0644); err != nil {
		return fmt.Errorf("failed to save node certificate: %w", err)
	}
	return nil
}

// LoadClientTLSConfig loads the certificate chain and builds an mTLS 1.3 client tls.Config.
func (k *KeyStore) LoadClientTLSConfig() (*tls.Config, error) {
	if !k.HasCertificates() {
		return nil, errors.New("keystore missing certificate or private key files")
	}

	clientCert, err := tls.LoadX509KeyPair(k.CertPath(), k.KeyPath())
	if err != nil {
		return nil, fmt.Errorf("failed to load client key pair: %w", err)
	}

	caCertPEM, err := os.ReadFile(k.CAPath())
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caCertPEM) {
		return nil, errors.New("failed to parse CA certificate into cert pool")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
