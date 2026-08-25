package identity

import "time"

// User represents a primary identity account in Aurora.
type User struct {
	ID                 string                 `json:"id"`
	Username           string                 `json:"username"`
	Email              string                 `json:"email"`
	PasswordHash       string                 `json:"-"` // Never expose in JSON
	IsActive           bool                   `json:"isActive"`
	TwoFactorSecretEnc string                 `json:"-"` // Encrypted at rest, never serialized
	TwoFactorEnabled   bool                   `json:"twoFactorEnabled"`
	Preferences        map[string]interface{} `json:"preferences"`
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          time.Time              `json:"updatedAt"`
	LastLoginAt        *time.Time             `json:"lastLoginAt,omitempty"`
}

// Subject represents an authenticated caller (user, API key, service identity) with associated authorization claims.
type Subject struct {
	Type        string                 `json:"type"` // "user", "api_key", "service"
	ID          string                 `json:"id"`   // User ID or API Key ID
	UserID      string                 `json:"userId"`
	Username    string                 `json:"username"`
	Email       string                 `json:"email"`
	Roles       []string               `json:"roles"`
	Permissions []string               `json:"permissions"`
	Scopes      []string               `json:"scopes,omitempty"` // For API keys
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// HasPermission checks if the subject holds a specific permission code.
func (s *Subject) HasPermission(code string) bool {
	// Superadmin wildcard check
	for _, role := range s.Roles {
		if role == "superadmin" {
			return true
		}
	}

	for _, p := range s.Permissions {
		if p == code || p == "*" {
			// If scopes are constrained (e.g. API key), verify scope allowance
			if len(s.Scopes) > 0 {
				for _, sc := range s.Scopes {
					if sc == code || sc == "*" {
						return true
					}
				}
				return false
			}
			return true
		}
	}
	return false
}

// HasRole checks if the subject holds a specific role name.
func (s *Subject) HasRole(role string) bool {
	for _, r := range s.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsSuperadmin returns true if the subject has the superadmin role.
func (s *Subject) IsSuperadmin() bool {
	return s.HasRole("superadmin")
}

// Resource represents any target infrastructure entity being acted upon for ownership and scope checks.
type Resource struct {
	Type       string // e.g. "instance", "node", "ipam", "user", "location"
	ID         string
	OwnerID    string
	LocationID string
}
