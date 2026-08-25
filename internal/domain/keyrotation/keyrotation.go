package keyrotation

import (
	"context"
	"errors"
	"time"
)

var (
	ErrKeyNotFound           = errors.New("cryptographic key record not found")
	ErrKeyAlreadyRevoked     = errors.New("cryptographic key is already revoked")
	ErrRotationInProgress    = errors.New("key rotation is already in progress")
	ErrInvalidGracePeriod    = errors.New("invalid grace period duration for key rotation")
)

// KeyType specifies the category of security credentials being rotated.
type KeyType string

const (
	TypeJWTSigning       KeyType = "jwt_signing"
	TypeWebhookSecret    KeyType = "webhook_secret"
	TypeNodeMTLSCert     KeyType = "node_mtls_cert"
	TypeDBCredential     KeyType = "db_credential"
	TypeBackupEncryption KeyType = "backup_encryption"
)

// KeyStatus represents the lifecycle state of a cryptographic key.
type KeyStatus string

const (
	StatusActive      KeyStatus = "active"
	StatusGracePeriod KeyStatus = "grace_period" // Dual valid period during rotation
	StatusRetired     KeyStatus = "retired"
	StatusRevoked     KeyStatus = "revoked"
)

// Record tracks cryptographic key versioning and retirement windows.
type Record struct {
	ID                   string     `json:"id"`
	Type                 KeyType    `json:"type"`
	KeyID                string     `json:"keyId"`
	Status               KeyStatus  `json:"status"`
	Version              int        `json:"version"`
	Algorithm            string     `json:"algorithm"` // e.g. "RS256", "Ed25519", "HMAC-SHA256", "AES-GCM-256"
	Description          string     `json:"description,omitempty"`
	RotatedBy            string     `json:"rotatedBy"` // Actor ID
	GracePeriodExpiresAt *time.Time `json:"gracePeriodExpiresAt,omitempty"`
	RevokedAt            *time.Time `json:"revokedAt,omitempty"`
	RevocationReason     string     `json:"revocationReason,omitempty"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

// Repository defines storage for key rotation audit records.
type Repository interface {
	Save(ctx context.Context, r *Record) error
	GetByID(ctx context.Context, id string) (*Record, error)
	GetActive(ctx context.Context, keyType KeyType) (*Record, error)
	List(ctx context.Context, keyType KeyType, limit, offset int) ([]*Record, int, error)
	UpdateStatus(ctx context.Context, id string, status KeyStatus, reason string) error
}
