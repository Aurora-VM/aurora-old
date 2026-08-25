package identity

import "time"

// RefreshSession represents a stateful user session tied to a refresh token family.
type RefreshSession struct {
	ID                string     `json:"id"`
	UserID            string     `json:"userId"`
	TokenHash         string     `json:"-"` // SHA-256 hash of refresh token; never stored plaintext
	FamilyID          string     `json:"familyId"`
	UserAgent         string     `json:"userAgent"`
	IPAddress         string     `json:"ipAddress"`
	ExpiresAt         time.Time  `json:"expiresAt"`
	CreatedAt         time.Time  `json:"createdAt"`
	IsRevoked         bool       `json:"isRevoked"`
	RevokedAt         *time.Time `json:"revokedAt,omitempty"`
	ReplacedByTokenID *string    `json:"replacedByTokenId,omitempty"`
}

// IsValid checks whether the session is unexpired and unrevoked.
func (s *RefreshSession) IsValid() bool {
	if s.IsRevoked {
		return false
	}
	return time.Now().Before(s.ExpiresAt)
}
