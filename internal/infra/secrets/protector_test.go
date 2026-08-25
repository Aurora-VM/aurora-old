package secrets

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAESGCMProtector_EncryptDecrypt(t *testing.T) {
	ctx := context.Background()
	masterKey := "super-secure-master-key-32-chars!"
	protector, err := NewAESGCMProtector(masterKey)
	require.NoError(t, err)

	originalSecret := "JBSWY3DPEHPK3PXP" // TOTP secret example

	encrypted, err := protector.Encrypt(ctx, []byte(originalSecret))
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)
	assert.True(t, strings.HasPrefix(string(encrypted), "v1:aes-gcm:"))
	assert.NotContains(t, string(encrypted), originalSecret)

	decrypted, err := protector.Decrypt(ctx, encrypted)
	require.NoError(t, err)
	assert.Equal(t, originalSecret, string(decrypted))
}

func TestAESGCMProtector_TamperDetection(t *testing.T) {
	ctx := context.Background()
	protector, err := NewAESGCMProtector("valid-master-key-for-aurora-vault!")
	require.NoError(t, err)

	encrypted, err := protector.Encrypt(ctx, []byte("sensitive-api-token"))
	require.NoError(t, err)

	// Tamper with ciphertext payload
	tampered := string(encrypted) + "tampered"

	_, err = protector.Decrypt(ctx, []byte(tampered))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tampered")
}
