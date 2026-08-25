package account

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	pquernaTotp "github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountService_2FASetup_Enable_Disable(t *testing.T) {
	memStore := memory.NewMemoryStore()
	hasher := crypto.NewArgon2Hasher(nil)
	protector, err := secrets.NewAESGCMProtector("test-master-key-32-characters-long!")
	require.NoError(t, err)
	totpMgr := totp.NewTOTPManager()

	svc := NewService(memStore.Users(), hasher, protector, totpMgr, memStore.Audit())
	ctx := context.Background()

	// Create user
	passHash, _ := hasher.Hash("StrongPassword123!")
	user := &identity.User{
		ID:           "usr_alice",
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: passHash,
		IsActive:     true,
	}
	require.NoError(t, memStore.Users().Create(ctx, user))

	// 1. Setup 2FA
	enrollment, err := svc.Setup2FA(ctx, user.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, enrollment.Secret)
	assert.NotEmpty(t, enrollment.QRCodeURL)

	// 2. Enable 2FA with invalid code -> failure
	err = svc.Enable2FA(ctx, user.ID, enrollment.Secret, "000000")
	assert.ErrorIs(t, err, identity.ErrTOTPInvalid)

	// 3. Enable 2FA with valid code -> success & encrypted at rest
	validCode, err := pquernaTotp.GenerateCode(enrollment.Secret, time.Now())
	require.NoError(t, err)
	err = svc.Enable2FA(ctx, user.ID, enrollment.Secret, validCode)
	require.NoError(t, err)

	updatedUser, err := memStore.Users().GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.True(t, updatedUser.TwoFactorEnabled)
	assert.NotEmpty(t, updatedUser.TwoFactorSecretEnc)
	assert.NotEqual(t, enrollment.Secret, updatedUser.TwoFactorSecretEnc) // Verify encrypted at rest!

	// 4. Disable 2FA with wrong password -> failure
	err = svc.Disable2FA(ctx, user.ID, "WrongPassword", validCode)
	assert.ErrorIs(t, err, identity.ErrInvalidCredentials)

	// 5. Disable 2FA with correct password and valid code -> success
	err = svc.Disable2FA(ctx, user.ID, "StrongPassword123!", validCode)
	require.NoError(t, err)

	disabledUser, err := memStore.Users().GetByID(ctx, user.ID)
	require.NoError(t, err)
	assert.False(t, disabledUser.TwoFactorEnabled)
	assert.Empty(t, disabledUser.TwoFactorSecretEnc)
}
