package totp

import (
	"fmt"

	"github.com/pquerna/otp/totp"
)

// TOTPManager implements identity.TOTPManager using standard RFC 6238 TOTP algorithms.
type TOTPManager struct {
	issuer string
}

// NewTOTPManager creates a new TOTP manager.
func NewTOTPManager() *TOTPManager {
	return &TOTPManager{issuer: "Aurora Virtualization"}
}

// GenerateSecret creates a new cryptographically random base32 TOTP secret and QR URL.
func (m *TOTPManager) GenerateSecret(accountName string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      m.issuer,
		AccountName: accountName,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate totp secret: %w", err)
	}

	return key.Secret(), key.URL(), nil
}

// ValidateCode verifies if the provided 6-digit code is valid for the given secret.
func (m *TOTPManager) ValidateCode(secret, code string) bool {
	if secret == "" || code == "" {
		return false
	}
	return totp.Validate(code, secret)
}
