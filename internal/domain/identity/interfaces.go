package identity

import "context"

// UserRepository defines the persistence port for user accounts.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	UpdatePassword(ctx context.Context, id, passwordHash string) error
	Update2FA(ctx context.Context, id string, enabled bool, secretEnc string) error
	UpdateLastLogin(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
}

// RoleRepository defines persistence for roles and permissions.
type RoleRepository interface {
	GetByID(ctx context.Context, id string) (*Role, error)
	GetByName(ctx context.Context, name string) (*Role, error)
	List(ctx context.Context) ([]*Role, error)
	GetGrantsForUser(ctx context.Context, userID string) ([]*UserRoleGrant, error)
	AssignRoleToUser(ctx context.Context, grant *UserRoleGrant) error
	RevokeRoleFromUser(ctx context.Context, userID, roleID string, scopeType string, scopeID *string) error
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)
}

// PermissionRepository defines repository access for system permission codes.
type PermissionRepository interface {
	List(ctx context.Context) ([]*Permission, error)
	GetByCode(ctx context.Context, code string) (*Permission, error)
}

// APIKeyRepository defines persistence for API keys.
type APIKeyRepository interface {
	Create(ctx context.Context, key *APIKey) error
	GetByID(ctx context.Context, id string) (*APIKey, error)
	GetByKeyHash(ctx context.Context, keyHash string) (*APIKey, error)
	ListByUser(ctx context.Context, userID string) ([]*APIKey, error)
	UpdateLastUsed(ctx context.Context, id string) error
	Revoke(ctx context.Context, id string) error
}

// SessionRepository manages stateful refresh tokens and token family revocation.
type SessionRepository interface {
	Create(ctx context.Context, session *RefreshSession) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*RefreshSession, error)
	Revoke(ctx context.Context, id string, replacedByID *string) error
	RevokeFamily(ctx context.Context, familyID string) error
	RevokeAllForUser(ctx context.Context, userID string) error
}

// PasswordHasher defines the contract for hashing and verifying passwords securely.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) (bool, error)
}

// SecretProtector defines the contract for reversible envelope encryption at rest.
type SecretProtector interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// TokenManager handles issuing and parsing signed access tokens and refresh tokens.
type TokenManager interface {
	GenerateAccessToken(user *User, roles, permissions []string) (string, error)
	ValidateAccessToken(tokenString string) (*Subject, error)
	GenerateRefreshToken() (plaintextToken string, tokenHash string, err error)
	HashRefreshToken(plaintextToken string) string
}

// TOTPManager defines the contract for generating and validating Time-Based One-Time Passwords.
type TOTPManager interface {
	GenerateSecret(accountName string) (secret string, qrCodeURL string, err error)
	ValidateCode(secret, code string) bool
}

// Authorizer evaluates whether a subject possesses authorization to execute an action on a target resource.
type Authorizer interface {
	Authorize(ctx context.Context, subject *Subject, action string, resource *Resource) error
}
