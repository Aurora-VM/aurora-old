package authz

import (
	"context"
	"testing"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicAuthorizer_SuperadminBypass(t *testing.T) {
	authz := NewAuthorizer(nil)
	ctx := context.Background()

	superadmin := &identity.Subject{
		UserID: "usr_sa",
		Roles:  []string{"superadmin"},
	}

	err := authz.Authorize(ctx, superadmin, "instance:delete", &identity.Resource{OwnerID: "usr_other"})
	require.NoError(t, err)
}

func TestDynamicAuthorizer_CustomerPermissions(t *testing.T) {
	authz := NewAuthorizer(nil)
	ctx := context.Background()

	customer := &identity.Subject{
		UserID:      "usr_cust1",
		Roles:       []string{"customer"},
		Permissions: []string{"instance:read", "instance:power"},
	}

	// 1. Allowed action on own resource
	err := authz.Authorize(ctx, customer, "instance:read", &identity.Resource{OwnerID: "usr_cust1"})
	require.NoError(t, err)

	// 2. Disallowed action on own resource (e.g. node:maintenance)
	err = authz.Authorize(ctx, customer, "node:maintenance", &identity.Resource{OwnerID: "usr_cust1"})
	assert.ErrorIs(t, err, identity.ErrInsufficientPermission)

	// 3. Allowed action on OTHER customer's resource (Cross-tenant breach attempt)
	err = authz.Authorize(ctx, customer, "instance:read", &identity.Resource{OwnerID: "usr_cust2"})
	assert.ErrorIs(t, err, identity.ErrResourceForbidden)
}

func TestDynamicAuthorizer_APIKeyScopes(t *testing.T) {
	authz := NewAuthorizer(nil)
	ctx := context.Background()

	apiKeySubject := &identity.Subject{
		Type:        "api_key",
		UserID:      "usr_cust1",
		Permissions: []string{"instance:read", "instance:create", "instance:power"},
		Scopes:      []string{"instance:read"}, // Restricted scope
	}

	// Allowed by scope
	err := authz.Authorize(ctx, apiKeySubject, "instance:read", &identity.Resource{OwnerID: "usr_cust1"})
	require.NoError(t, err)

	// Denied because scope excludes instance:create even if user has permission
	err = authz.Authorize(ctx, apiKeySubject, "instance:create", nil)
	assert.ErrorIs(t, err, identity.ErrInsufficientPermission)
}
