package identity

import "time"

// APIKey represents a machine or developer credential with explicit permission scopes.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	KeyHash    string     `json:"-"`      // SHA-256 hash of plaintext token; never exposed
	Prefix     string     `json:"prefix"` // Non-sensitive prefix for UI display (e.g. aur_live_9f8a)
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	IsRevoked  bool       `json:"isRevoked"`
}

// IsValid checks whether the API key is active and unexpired.
func (k *APIKey) IsValid() bool {
	if k.IsRevoked {
		return false
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return false
	}
	return true
}
