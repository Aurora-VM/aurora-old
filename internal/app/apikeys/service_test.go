package apikeys

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyService_Create_List_Authenticate_Revoke(t *testing.T) {
	memStore := memory.NewMemoryStore()
	svc := NewService(memStore.APIKeys(), memStore.Users(), memStore.Roles(), memStore.Audit())
	ctx := context.Background()

	user := &identity.User{
		ID:       "usr_dev1",
		Username: "developer",
		Email:    "dev@aurora.local",
		IsActive: true,
	}
	require.NoError(t, memStore.Users().Create(ctx, user))

	// 1. Create API key
	exp := time.Now().Add(30 * 24 * time.Hour)
	createRes, err := svc.CreateAPIKey(ctx, user.ID, CreateAPIKeyRequest{
		Name:      "CI Deployer",
		Scopes:    []string{"instance:read", "instance:power"},
		ExpiresAt: &exp,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, createRes.PlaintextKey)
	assert.NotEmpty(t, createRes.APIKey.ID)
	assert.NotEmpty(t, createRes.APIKey.Prefix)

	// 2. List API keys
	keys, err := svc.ListAPIKeys(ctx, user.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "CI Deployer", keys[0].Name)

	// 3. Authenticate with valid plaintext key
	subject, err := svc.AuthenticateAPIKey(ctx, createRes.PlaintextKey)
	require.NoError(t, err)
	assert.Equal(t, "api_key", subject.Type)
	assert.Equal(t, user.ID, subject.UserID)
	assert.Equal(t, []string{"instance:read", "instance:power"}, subject.Scopes)

	// 4. Authenticate with invalid key
	_, err = svc.AuthenticateAPIKey(ctx, "aur_live_invalid_random_string")
	assert.ErrorIs(t, err, identity.ErrAPIKeyInvalid)

	// 5. Revoke API key
	err = svc.RevokeAPIKey(ctx, user.ID, createRes.APIKey.ID)
	require.NoError(t, err)

	// 6. Authenticate with revoked key
	_, err = svc.AuthenticateAPIKey(ctx, createRes.PlaintextKey)
	assert.ErrorIs(t, err, identity.ErrAPIKeyRevoked)
}
