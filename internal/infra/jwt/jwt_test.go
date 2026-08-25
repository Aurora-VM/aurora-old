package jwt

import (
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenManager_AccessAndRefreshTokens(t *testing.T) {
	secret := "a-very-long-secret-key-that-is-at-least-32-bytes-long!"
	tm, err := NewTokenManager(secret)
	require.NoError(t, err)

	user := &identity.User{
		ID:       "usr_123456",
		Username: "alice",
		Email:    "alice@example.com",
	}
	roles := []string{"customer"}
	permissions := []string{"instance:read", "instance:create"}

	// Generate and validate access token
	tokenStr, err := tm.GenerateAccessToken(user, roles, permissions)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	subject, err := tm.ValidateAccessToken(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, user.ID, subject.ID)
	assert.Equal(t, user.Username, subject.Username)
	assert.Equal(t, roles, subject.Roles)
	assert.Equal(t, permissions, subject.Permissions)
	assert.True(t, subject.HasPermission("instance:read"))
	assert.False(t, subject.HasPermission("node:create"))

	// Generate refresh token
	plain, hash, err := tm.GenerateRefreshToken()
	require.NoError(t, err)
	assert.NotEmpty(t, plain)
	assert.NotEmpty(t, hash)
	assert.Equal(t, hash, tm.HashRefreshToken(plain))
}

func TestTokenManager_ExpiredToken(t *testing.T) {
	secret := "a-very-long-secret-key-that-is-at-least-32-bytes-long!"
	tm, err := NewTokenManager(secret)
	require.NoError(t, err)
	tm.accessExpiry = -1 * time.Minute // Expired immediately

	user := &identity.User{ID: "usr_exp", Username: "bob", Email: "bob@example.com"}
	tokenStr, err := tm.GenerateAccessToken(user, []string{"customer"}, nil)
	require.NoError(t, err)

	_, err = tm.ValidateAccessToken(tokenStr)
	assert.ErrorIs(t, err, identity.ErrTokenExpired)
}
