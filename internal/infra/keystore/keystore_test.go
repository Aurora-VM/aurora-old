package keystore

import (
	"os"
	"testing"

	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyStore_GenerateCSR_SaveCertificates_LoadTLS(t *testing.T) {
	tempDir := t.TempDir()

	ks, err := NewKeyStore(tempDir)
	require.NoError(t, err)
	assert.False(t, ks.HasCertificates())

	// 1. Generate local key and CSR
	csrPEM, err := ks.GenerateKeyAndCSR("node-east-01", "node-east-01.aurora.local")
	require.NoError(t, err)
	assert.NotEmpty(t, csrPEM)

	// Check that node.key exists with 0600 permissions
	fi, err := os.Stat(ks.KeyPath())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())

	// 2. Mock CA signing the CSR
	ca, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)

	certPEM, _, err := ca.SignNodeCSR(csrPEM, "node-12345", "node-east-01.aurora.local", 0)
	require.NoError(t, err)

	// 3. Save certificates into KeyStore
	err = ks.SaveCertificates(ca.GetCACertificatePEM(), certPEM)
	require.NoError(t, err)
	assert.True(t, ks.HasCertificates())

	// 4. Load client TLS configuration
	tlsConfig, err := ks.LoadClientTLSConfig()
	require.NoError(t, err)
	assert.NotNil(t, tlsConfig)
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.NotNil(t, tlsConfig.RootCAs)
}
