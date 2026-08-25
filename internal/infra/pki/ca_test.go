package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInternalCA_GenerateAndSignCSR(t *testing.T) {
	ca, err := NewInternalCA(nil, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, ca.GetCACertificatePEM())
	assert.NotEmpty(t, ca.GetCAKeyPEM())

	// Generate Client Key & CSR on Node Agent side
	nodePrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Project Aurora Node"},
			CommonName:   "hypervisor-01.us-east.local",
		},
		DNSNames: []string{"hypervisor-01.us-east.local"},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, nodePrivKey)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Sign CSR via CA
	nodeID := "node-uuid-12345"
	certPEM, fingerprint, err := ca.SignNodeCSR(csrPEM, nodeID, "hypervisor-01.us-east.local", 90*24*time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, certPEM)
	assert.NotEmpty(t, fingerprint)

	// Verify signed certificate against CA
	verifiedNodeID, verifiedFingerprint, err := ca.VerifyCertificate(certPEM)
	require.NoError(t, err)
	assert.Equal(t, nodeID, verifiedNodeID)
	assert.Equal(t, fingerprint, verifiedFingerprint)
}

func TestInternalCA_RejectUntrustedCert(t *testing.T) {
	ca1, err := NewInternalCA(nil, nil)
	require.NoError(t, err)

	ca2, err := NewInternalCA(nil, nil)
	require.NoError(t, err)

	// Sign certificate with CA2
	nodeKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "node:untrusted"},
	}, nodeKey)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	certPEM, _, err := ca2.SignNodeCSR(csrPEM, "untrusted", "fake-node", 1*time.Hour)
	require.NoError(t, err)

	// Verify against CA1 -> Must fail!
	_, _, err = ca1.VerifyCertificate(certPEM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client certificate verification failed")
}
