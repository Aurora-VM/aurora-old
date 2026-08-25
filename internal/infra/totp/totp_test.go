package totp

import (
	"testing"
	"time"

	pquernaTotp "github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTOTPManager_GenerateAndValidate(t *testing.T) {
	mgr := NewTOTPManager()

	secret, qrURL, err := mgr.GenerateSecret("alice@example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, secret)
	assert.Contains(t, qrURL, "otpauth://totp/")

	// Generate valid code
	validCode, err := pquernaTotp.GenerateCode(secret, time.Now())
	require.NoError(t, err)

	assert.True(t, mgr.ValidateCode(secret, validCode))
	assert.False(t, mgr.ValidateCode(secret, "000000"))
	assert.False(t, mgr.ValidateCode(secret, "invalid"))
	assert.False(t, mgr.ValidateCode("", validCode))
}
