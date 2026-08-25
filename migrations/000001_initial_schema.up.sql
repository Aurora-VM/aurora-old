-- Project Aurora: Initial Schema Migration (000001)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";

-- 1. Identity & Dynamic Scoped RBAC
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username CITEXT UNIQUE NOT NULL,
    email CITEXT UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    two_factor_secret_enc TEXT,
    two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    preferences JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(64) UNIQUE NOT NULL,
    description TEXT,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS permissions (
    code VARCHAR(128) PRIMARY KEY,
    description TEXT NOT NULL,
    category VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_code VARCHAR(128) NOT NULL REFERENCES permissions(code) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE IF NOT EXISTS user_role_grants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    scope_type VARCHAR(32) NOT NULL DEFAULT 'global',
    scope_id UUID,
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_role_scope UNIQUE (user_id, role_id, scope_type, scope_id)
);

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    prefix VARCHAR(12) NOT NULL,
    scopes JSONB NOT NULL DEFAULT '["instance:read"]'::jsonb,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Infrastructure Topology & Node Registry
CREATE TABLE IF NOT EXISTS locations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL UNIQUE,
    region VARCHAR(100) NOT NULL,
    country VARCHAR(2) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS nodes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id UUID NOT NULL REFERENCES locations(id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL UNIQUE,
    fqdn VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(50) NOT NULL DEFAULT 'enrolling',
    cert_fingerprint VARCHAR(64),
    cpu_cores INTEGER NOT NULL DEFAULT 0,
    memory_bytes BIGINT NOT NULL DEFAULT 0,
    storage_bytes BIGINT NOT NULL DEFAULT 0,
    cpu_overcommit_ratio NUMERIC(3,2) NOT NULL DEFAULT 1.00,
    memory_overcommit_ratio NUMERIC(3,2) NOT NULL DEFAULT 1.00,
    capabilities JSONB NOT NULL DEFAULT '{"incus": true, "kvm": true, "zfs": false}'::jsonb,
    maintenance_mode BOOLEAN NOT NULL DEFAULT FALSE,
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS node_enrollment_secrets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    secret_hash VARCHAR(64) NOT NULL UNIQUE,
    node_name_pattern VARCHAR(100),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    used_by_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Virtualization & Instances
CREATE TABLE IF NOT EXISTS os_templates (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    distribution VARCHAR(50) NOT NULL,
    version VARCHAR(50) NOT NULL,
    image_alias VARCHAR(255) NOT NULL,
    architecture VARCHAR(50) NOT NULL DEFAULT 'x86_64',
    is_cloud_init BOOLEAN NOT NULL DEFAULT TRUE,
    min_disk_bytes BIGINT NOT NULL DEFAULT 5368709120,
    min_memory_bytes BIGINT NOT NULL DEFAULT 536870912,
    icon_svg TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS instances (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    template_id UUID NOT NULL REFERENCES os_templates(id) ON DELETE RESTRICT,
    name VARCHAR(63) NOT NULL UNIQUE,
    hostname VARCHAR(255) NOT NULL,
    instance_type VARCHAR(20) NOT NULL DEFAULT 'container',
    desired_status VARCHAR(50) NOT NULL DEFAULT 'running',
    observed_status VARCHAR(50) NOT NULL DEFAULT 'unknown',
    vcpus INTEGER NOT NULL DEFAULT 1,
    memory_bytes BIGINT NOT NULL,
    swap_bytes BIGINT NOT NULL DEFAULT 0,
    storage_bytes BIGINT NOT NULL,
    io_limit_iops INTEGER DEFAULT 0,
    bandwidth_limit_mbps INTEGER DEFAULT 0,
    cloud_init_userdata TEXT,
    is_suspended BOOLEAN NOT NULL DEFAULT FALSE,
    suspension_reason TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_reconciled_at TIMESTAMPTZ
);

-- 4. Topology IPAM & Multi-Interface Networking
CREATE TABLE IF NOT EXISTS networks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id UUID NOT NULL REFERENCES locations(id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL,
    network_type VARCHAR(32) NOT NULL DEFAULT 'public_bridge',
    bridge_interface VARCHAR(32) NOT NULL DEFAULT 'br0',
    vlan_id INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ip_pools (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    network_id UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    ip_version VARCHAR(4) NOT NULL DEFAULT 'v4',
    cidr CIDR NOT NULL,
    gateway INET NOT NULL,
    dns_primary INET NOT NULL DEFAULT '1.1.1.1',
    dns_secondary INET NOT NULL DEFAULT '8.8.8.8',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS instance_network_interfaces (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    network_id UUID NOT NULL REFERENCES networks(id) ON DELETE RESTRICT,
    device_name VARCHAR(16) NOT NULL DEFAULT 'eth0',
    mac_address MACADDR NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_instance_device UNIQUE (instance_id, device_name),
    CONSTRAINT uq_instance_mac UNIQUE (mac_address)
);

CREATE TABLE IF NOT EXISTS ip_allocations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pool_id UUID NOT NULL REFERENCES ip_pools(id) ON DELETE RESTRICT,
    interface_id UUID REFERENCES instance_network_interfaces(id) ON DELETE SET NULL,
    address INET NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'available',
    allocated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS port_forwards (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    host_port INTEGER NOT NULL,
    guest_port INTEGER NOT NULL,
    protocol VARCHAR(10) NOT NULL DEFAULT 'tcp',
    description VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_node_host_port_protocol UNIQUE (node_id, host_port, protocol)
);

-- 5. Backups & Storage
CREATE TABLE IF NOT EXISTS backup_destinations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    bucket VARCHAR(100) NOT NULL,
    region VARCHAR(50) NOT NULL,
    access_key_enc TEXT NOT NULL,
    secret_key_enc TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS backups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    destination_id UUID NOT NULL REFERENCES backup_destinations(id) ON DELETE RESTRICT,
    storage_path VARCHAR(512) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 VARCHAR(64),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

-- 6. Jobs & Audit Logs
CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    target_resource_type VARCHAR(50) NOT NULL,
    target_resource_id UUID NOT NULL,
    type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    progress INTEGER NOT NULL DEFAULT 0,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT,
    idempotency_key VARCHAR(128) UNIQUE,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_ip INET,
    user_agent TEXT,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    request_id VARCHAR(64),
    status_code INTEGER,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_instances_user ON instances(user_id);
CREATE INDEX IF NOT EXISTS idx_instances_node ON instances(node_id);
CREATE INDEX IF NOT EXISTS idx_interfaces_instance ON instance_network_interfaces(instance_id);
CREATE INDEX IF NOT EXISTS idx_ip_allocations_pool ON ip_allocations(pool_id);
CREATE INDEX IF NOT EXISTS idx_ip_allocations_interface ON ip_allocations(interface_id);
CREATE INDEX IF NOT EXISTS idx_user_role_grants_user ON user_role_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
