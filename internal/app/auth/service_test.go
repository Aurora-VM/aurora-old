package auth

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	pquernaTotp "github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAuthService(t *testing.T) (*Service, *memory.MemoryStore, identity.SecretProtector) {
	memStore := memory.NewMemoryStore()
	hasher := crypto.NewArgon2Hasher(nil)
	protector, err := secrets.NewAESGCMProtector("test-master-key-32-characters-long!")
	require.NoError(t, err)
	tokenMgr, err := jwt.NewTokenManager("test-jwt-secret-key-32-characters-long!")
	require.NoError(t, err)
	totpMgr := totp.NewTOTPManager()

	svc := NewService(
		memStore.Users(),
		memStore.Roles(),
		memStore.Sessions(),
		hasher,
		protector,
		tokenMgr,
		totpMgr,
		memStore.Audit(),
	)

	return svc, memStore, protector
}

func TestAuthService_RegisterAndLogin_FirstUserIsSuperadmin(t *testing.T) {
	svc, _, _ := setupTestAuthService(t)
	ctx := context.Background()

	// 1. Register first user -> Superadmin
	user1, err := svc.Register(ctx, RegisterRequest{
		Username: "admin",
		Email:    "admin@aurora.local",
		Password: "SecurePassword123!",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, user1.ID)

	// 2. Register second user -> Customer
	user2, err := svc.Register(ctx, RegisterRequest{
		Username: "customer1",
		Email:    "customer1@example.com",
		Password: "SecurePassword123!",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, user2.ID)

	// 3. Login as admin
	res, err := svc.Login(ctx, LoginRequest{
		UsernameOrEmail: "admin",
		Password:        "SecurePassword123!",
	})
	require.NoError(t, err)
	assert.False(t, res.Requires2FA)
	assert.NotEmpty(t, res.Tokens.AccessToken)
	assert.NotEmpty(t, res.Tokens.RefreshToken)
	assert.Contains(t, res.User.Roles, "superadmin")

	// 4. Login with invalid password
	_, err = svc.Login(ctx, LoginRequest{
		UsernameOrEmail: "admin",
		Password:        "WrongPassword!",
	})
	assert.ErrorIs(t, err, identity.ErrInvalidCredentials)
}

func TestAuthService_RefreshTokenRotation_AndReuseDetection(t *testing.T) {
	svc, _, _ := setupTestAuthService(t)
	ctx := context.Background()

	_, err := svc.Register(ctx, RegisterRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "Password12345!",
	})
	require.NoError(t, err)

	// Login
	loginRes, err := svc.Login(ctx, LoginRequest{
		UsernameOrEmail: "alice",
		Password:        "Password12345!",
	})
	require.NoError(t, err)
	originalRefresh := loginRes.Tokens.RefreshToken

	// 1. Successful Refresh -> New token pair issued, original rotated
	rotatedPair, err := svc.RefreshSession(ctx, originalRefresh, "127.0.0.1", "TestAgent")
	require.NoError(t, err)
	assert.NotEmpty(t, rotatedPair.AccessToken)
	assert.NotEmpty(t, rotatedPair.RefreshToken)
	assert.NotEqual(t, originalRefresh, rotatedPair.RefreshToken)

	// 2. Refresh Token Reuse Attempt (Attacker uses old rotated refresh token)
	_, err = svc.RefreshSession(ctx, originalRefresh, "127.0.0.1", "AttackerAgent")
	assert.ErrorIs(t, err, identity.ErrRefreshTokenReused)

	// 3. Subsequent refresh with the new token should now also fail because family was revoked
	_, err = svc.RefreshSession(ctx, rotatedPair.RefreshToken, "127.0.0.1", "TestAgent")
	assert.ErrorIs(t, err, identity.ErrRefreshTokenReused)
}

func TestAuthService_2FAWorkflow(t *testing.T) {
	svc, memStore, protector := setupTestAuthService(t)
	ctx := context.Background()

	u, err := svc.Register(ctx, RegisterRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "Password12345!",
	})
	require.NoError(t, err)

	// Setup and enable 2FA directly on user
	plainSecret := "JBSWY3DPEHPK3PXP"
	encSecret, err := protector.Encrypt(ctx, []byte(plainSecret))
	require.NoError(t, err)
	err = memStore.Users().Update2FA(ctx, u.ID, true, string(encSecret))
	require.NoError(t, err)

	// Login -> Must return 2FA challenge
	loginRes, err := svc.Login(ctx, LoginRequest{
		UsernameOrEmail: "bob",
		Password:        "Password12345!",
	})
	require.NoError(t, err)
	assert.True(t, loginRes.Requires2FA)
	assert.NotEmpty(t, loginRes.ChallengeTemp)
	assert.Nil(t, loginRes.Tokens)

	// Verify with invalid code
	_, err = svc.VerifyTOTP(ctx, loginRes.ChallengeTemp, "000000", "127.0.0.1", "agent")
	assert.ErrorIs(t, err, identity.ErrTOTPInvalid)

	// Verify with valid code
	validCode, err := pquernaTotp.GenerateCode(plainSecret, time.Now())
	require.NoError(t, err)

	verifiedRes, err := svc.VerifyTOTP(ctx, loginRes.ChallengeTemp, validCode, "127.0.0.1", "agent")
	require.NoError(t, err)
	assert.False(t, verifiedRes.Requires2FA)
	assert.NotEmpty(t, verifiedRes.Tokens.AccessToken)
}
