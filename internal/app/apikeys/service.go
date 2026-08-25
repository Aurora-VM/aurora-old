package apikeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/google/uuid"
)

// CreateAPIKeyRequest contains parameters for creating a new API key.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// CreatedAPIKeyResponse contains metadata and the plaintext token returned ONLY once.
type CreatedAPIKeyResponse struct {
	APIKey       *identity.APIKey `json:"apiKey"`
	PlaintextKey string           `json:"plaintextKey"` // Displayed only upon creation
}

// Service manages scoped API keys and token authentication.
type Service struct {
	keyRepo   identity.APIKeyRepository
	userRepo  identity.UserRepository
	roleRepo  identity.RoleRepository
	auditRepo audit.Repository
}

// NewService creates a new API key application service.
func NewService(
	keyRepo identity.APIKeyRepository,
	userRepo identity.UserRepository,
	roleRepo identity.RoleRepository,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		keyRepo:   keyRepo,
		userRepo:  userRepo,
		roleRepo:  roleRepo,
		auditRepo: auditRepo,
	}
}

// CreateAPIKey generates a high-entropy API key with specified permission scopes.
func (s *Service) CreateAPIKey(ctx context.Context, userID string, req CreateAPIKeyRequest) (*CreatedAPIKeyResponse, error) {
	if req.Name == "" {
		return nil, errors.New("api key name is required")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, identity.ErrUserNotFound
	}

	// Generate 32 bytes of cryptographic randomness
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	randomStr := hex.EncodeToString(rawBytes)
	plaintextKey := fmt.Sprintf("aur_live_%s", randomStr)
	prefix := fmt.Sprintf("aur_live_%s...", randomStr[:6])

	// Calculate SHA-256 hash for storage
	h := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(h[:])

	apiKey := &identity.APIKey{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Name:      req.Name,
		KeyHash:   keyHash,
		Prefix:    prefix,
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
		CreatedAt: time.Now().UTC(),
		IsRevoked: false,
	}

	if err := s.keyRepo.Create(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("failed to save api key: %w", err)
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &userID,
		Action:       "apikey.created",
		ResourceType: "apikey",
		ResourceID:   &apiKey.ID,
		StatusCode:   201,
		Details:      map[string]interface{}{"name": apiKey.Name, "prefix": apiKey.Prefix, "scopes": apiKey.Scopes},
		CreatedAt:    time.Now().UTC(),
	})

	return &CreatedAPIKeyResponse{
		APIKey:       apiKey,
		PlaintextKey: plaintextKey,
	}, nil
}

// ListAPIKeys retrieves all active and revoked API keys for the specified user.
func (s *Service) ListAPIKeys(ctx context.Context, userID string) ([]*identity.APIKey, error) {
	return s.keyRepo.ListByUser(ctx, userID)
}

// RevokeAPIKey revokes a specific API key.
func (s *Service) RevokeAPIKey(ctx context.Context, userID, keyID string) error {
	key, err := s.keyRepo.GetByID(ctx, keyID)
	if err != nil {
		return identity.ErrAPIKeyNotFound
	}

	if key.UserID != userID {
		return identity.ErrResourceForbidden
	}

	if err := s.keyRepo.Revoke(ctx, keyID); err != nil {
		return err
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &userID,
		Action:       "apikey.revoked",
		ResourceType: "apikey",
		ResourceID:   &keyID,
		StatusCode:   200,
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// AuthenticateAPIKey verifies a plaintext API key and returns an authenticated Subject.
func (s *Service) AuthenticateAPIKey(ctx context.Context, plaintextKey string) (*identity.Subject, error) {
	h := sha256.Sum256([]byte(plaintextKey))
	keyHash := hex.EncodeToString(h[:])

	apiKey, err := s.keyRepo.GetByKeyHash(ctx, keyHash)
	if err != nil {
		return nil, identity.ErrAPIKeyInvalid
	}

	if !apiKey.IsValid() {
		if apiKey.IsRevoked {
			return nil, identity.ErrAPIKeyRevoked
		}
		return nil, identity.ErrAPIKeyExpired
	}

	user, err := s.userRepo.GetByID(ctx, apiKey.UserID)
	if err != nil || !user.IsActive {
		return nil, identity.ErrAccountDisabled
	}

	perms, err := s.roleRepo.GetUserPermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	go func(id string) {
		_ = s.keyRepo.UpdateLastUsed(context.Background(), id)
	}(apiKey.ID)

	return &identity.Subject{
		Type:        "api_key",
		ID:          apiKey.ID,
		UserID:      user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Permissions: perms,
		Scopes:      apiKey.Scopes,
	}, nil
}
