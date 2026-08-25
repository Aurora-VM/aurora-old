package postgres

import (
	"context"
	"fmt"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RoleRepository implements identity.RoleRepository & identity.PermissionRepository using PostgreSQL.
type RoleRepository struct {
	pool *pgxpool.Pool
}

// NewRoleRepository creates a new PostgreSQL role and permission repository.
func NewRoleRepository(pool *pgxpool.Pool) *RoleRepository {
	return &RoleRepository{pool: pool}
}

func (r *RoleRepository) GetByID(ctx context.Context, id string) (*identity.Role, error) {
	query := `SELECT id, name, description, is_system, created_at FROM roles WHERE id = $1;`
	var role identity.Role
	err := r.pool.QueryRow(ctx, query, id).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt)
	if err != nil {
		return nil, identity.ErrRoleNotFound
	}
	return &role, nil
}

func (r *RoleRepository) GetByName(ctx context.Context, name string) (*identity.Role, error) {
	query := `SELECT id, name, description, is_system, created_at FROM roles WHERE name = $1;`
	var role identity.Role
	err := r.pool.QueryRow(ctx, query, name).Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt)
	if err != nil {
		return nil, identity.ErrRoleNotFound
	}
	return &role, nil
}

func (r *RoleRepository) List(ctx context.Context) ([]*identity.Role, error) {
	query := `SELECT id, name, description, is_system, created_at FROM roles ORDER BY name;`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*identity.Role
	for rows.Next() {
		var role identity.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, &role)
	}
	return roles, nil
}

func (r *RoleRepository) GetGrantsForUser(ctx context.Context, userID string) ([]*identity.UserRoleGrant, error) {
	query := `
	SELECT g.id, g.user_id, g.role_id, r.name, g.scope_type, g.scope_id, g.granted_by, g.created_at
	FROM user_role_grants g
	JOIN roles r ON g.role_id = r.id
	WHERE g.user_id = $1;
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grants []*identity.UserRoleGrant
	for rows.Next() {
		var g identity.UserRoleGrant
		var scopeID, grantedBy *string
		if err := rows.Scan(&g.ID, &g.UserID, &g.RoleID, &g.RoleName, &g.ScopeType, &scopeID, &grantedBy, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.ScopeID = scopeID
		g.GrantedBy = grantedBy
		grants = append(grants, &g)
	}
	return grants, nil
}

func (r *RoleRepository) AssignRoleToUser(ctx context.Context, grant *identity.UserRoleGrant) error {
	query := `
	INSERT INTO user_role_grants (id, user_id, role_id, scope_type, scope_id, granted_by, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (user_id, role_id, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'::uuid)) DO NOTHING;
	`
	_, err := r.pool.Exec(ctx, query, grant.ID, grant.UserID, grant.RoleID, grant.ScopeType, grant.ScopeID, grant.GrantedBy, grant.CreatedAt)
	return err
}

func (r *RoleRepository) RevokeRoleFromUser(ctx context.Context, userID, roleID string, scopeType string, scopeID *string) error {
	query := `DELETE FROM user_role_grants WHERE user_id = $1 AND role_id = $2 AND scope_type = $3 AND scope_id IS NOT DISTINCT FROM $4;`
	_, err := r.pool.Exec(ctx, query, userID, roleID, scopeType, scopeID)
	return err
}

func (r *RoleRepository) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	query := `
	SELECT DISTINCT rp.permission_code
	FROM user_role_grants urg
	JOIN role_permissions rp ON urg.role_id = rp.role_id
	WHERE urg.user_id = $1;
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user permissions: %w", err)
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}
	return permissions, nil
}

func (r *RoleRepository) ListPermissions(ctx context.Context) ([]*identity.Permission, error) {
	query := `SELECT code, description, category FROM permissions ORDER BY category, code;`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []*identity.Permission
	for rows.Next() {
		var p identity.Permission
		if err := rows.Scan(&p.Code, &p.Description, &p.Category); err != nil {
			return nil, err
		}
		perms = append(perms, &p)
	}
	return perms, nil
}
