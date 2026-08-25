package authz

import (
	"context"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// DynamicAuthorizer implements identity.Authorizer with role permissions, API key scoping, and multi-tenant resource ownership.
type DynamicAuthorizer struct {
	roleRepo identity.RoleRepository
}

// NewAuthorizer creates a new dynamic authorizer.
func NewAuthorizer(roleRepo identity.RoleRepository) *DynamicAuthorizer {
	return &DynamicAuthorizer{roleRepo: roleRepo}
}

// Authorize evaluates subject permissions and resource boundaries.
func (a *DynamicAuthorizer) Authorize(ctx context.Context, subject *identity.Subject, action string, resource *identity.Resource) error {
	if subject == nil {
		return identity.ErrUnauthorized
	}

	// 1. Superadmin wildcard bypass
	for _, r := range subject.Roles {
		if r == "superadmin" {
			return nil
		}
	}

	// 2. Action permission check
	if !subject.HasPermission(action) {
		return identity.ErrInsufficientPermission
	}

	// 3. API Key Scope check
	if len(subject.Scopes) > 0 {
		hasScope := false
		for _, sc := range subject.Scopes {
			if sc == action || sc == "*" {
				hasScope = true
				break
			}
		}
		if !hasScope {
			return identity.ErrInsufficientPermission
		}
	}

	// 4. Resource multi-tenant ownership check
	if resource != nil && resource.OwnerID != "" {
		// If caller is an admin or operator, allow cross-tenant access for administrative actions
		isAdminOrOperator := false
		for _, r := range subject.Roles {
			if r == "admin" || r == "operator" {
				isAdminOrOperator = true
				break
			}
		}

		if !isAdminOrOperator && resource.OwnerID != subject.UserID {
			return identity.ErrResourceForbidden
		}
	}

	return nil
}
