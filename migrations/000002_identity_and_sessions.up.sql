-- Project Aurora: Identity, Refresh Sessions, and Standard RBAC Seed (000002)

CREATE TABLE IF NOT EXISTS refresh_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    family_id VARCHAR(64) NOT NULL,
    user_agent TEXT,
    ip_address INET,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMPTZ,
    replaced_by_token_id VARCHAR(64)
);

CREATE INDEX IF NOT EXISTS idx_refresh_sessions_user ON refresh_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_sessions_family ON refresh_sessions(family_id);
CREATE INDEX IF NOT EXISTS idx_refresh_sessions_hash ON refresh_sessions(token_hash);

-- Seed System Permissions
INSERT INTO permissions (code, description, category) VALUES
    ('*', 'Unrestricted wildcard permission', 'system'),
    ('instance:read', 'View instances and instance details', 'instance'),
    ('instance:create', 'Create new virtual instances', 'instance'),
    ('instance:update', 'Update instance configuration', 'instance'),
    ('instance:delete', 'Delete instances', 'instance'),
    ('instance:power', 'Start, stop, restart, and pause instances', 'instance'),
    ('instance:console', 'Access web terminal and VNC console', 'instance'),
    ('instance:files:read', 'Read guest filesystem files', 'instance'),
    ('instance:files:write', 'Write guest filesystem files', 'instance'),
    ('node:read', 'View hypervisor nodes and status', 'node'),
    ('node:create', 'Enroll new hypervisor nodes', 'node'),
    ('node:update', 'Update hypervisor node settings', 'node'),
    ('node:maintenance', 'Toggle node maintenance mode', 'node'),
    ('ipam:read', 'View IP pools, subnets, and allocations', 'ipam'),
    ('ipam:manage', 'Manage IP pools and port forwarding rules', 'ipam'),
    ('user:read', 'View user accounts', 'user'),
    ('user:manage', 'Manage user accounts, roles, and grants', 'user'),
    ('audit:read', 'View system audit trails', 'audit')
ON CONFLICT (code) DO NOTHING;

-- Seed Default System Roles
INSERT INTO roles (id, name, description, is_system) VALUES
    ('00000000-0000-0000-0000-000000000001', 'superadmin', 'Unrestricted platform administrator', TRUE),
    ('00000000-0000-0000-0000-000000000002', 'admin', 'System administrator with user & infrastructure management', TRUE),
    ('00000000-0000-0000-0000-000000000003', 'operator', 'Infrastructure hypervisor operator', TRUE),
    ('00000000-0000-0000-0000-000000000004', 'customer', 'Standard tenant / VPS customer', TRUE)
ON CONFLICT (name) DO NOTHING;

-- Map Role Permissions
-- 1. Superadmin has wildcard '*'
INSERT INTO role_permissions (role_id, permission_code) VALUES
    ('00000000-0000-0000-0000-000000000001', '*')
ON CONFLICT DO NOTHING;

-- 2. Customer has instance permissions
INSERT INTO role_permissions (role_id, permission_code) VALUES
    ('00000000-0000-0000-0000-000000000004', 'instance:read'),
    ('00000000-0000-0000-0000-000000000004', 'instance:create'),
    ('00000000-0000-0000-0000-000000000004', 'instance:power'),
    ('00000000-0000-0000-0000-000000000004', 'instance:console'),
    ('00000000-0000-0000-0000-000000000004', 'instance:files:read'),
    ('00000000-0000-0000-0000-000000000004', 'instance:files:write')
ON CONFLICT DO NOTHING;

-- 3. Operator has node, instance read, and IPAM permissions
INSERT INTO role_permissions (role_id, permission_code) VALUES
    ('00000000-0000-0000-0000-000000000003', 'node:read'),
    ('00000000-0000-0000-0000-000000000003', 'node:maintenance'),
    ('00000000-0000-0000-0000-000000000003', 'instance:read'),
    ('00000000-0000-0000-0000-000000000003', 'ipam:read'),
    ('00000000-0000-0000-0000-000000000003', 'audit:read')
ON CONFLICT DO NOTHING;
