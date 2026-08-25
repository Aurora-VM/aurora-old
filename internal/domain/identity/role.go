package identity

import "time"

// Permission represents a discrete, atomic action within the platform.
type Permission struct {
	Code        string `json:"code"` // e.g. "instance:power", "node:maintenance", "ipam:manage"
	Description string `json:"description"`
	Category    string `json:"category"`
}

// Role represents a collection of permissions.
type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	IsSystem    bool         `json:"isSystem"`
	Permissions []Permission `json:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
}

// UserRoleGrant represents an assignment of a role to a user within an optional resource scope.
type UserRoleGrant struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	RoleID    string    `json:"roleId"`
	RoleName  string    `json:"roleName,omitempty"`
	ScopeType string    `json:"scopeType"` // "global", "location", "instance"
	ScopeID   *string   `json:"scopeId,omitempty"`
	GrantedBy *string   `json:"grantedBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
