-- Project Aurora — Phase 16: Disaster Recovery, Backups & Security Hardening Schema

-- 1. Backup Records Table
CREATE TABLE IF NOT EXISTS backups (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    resource_type VARCHAR(32) NOT NULL, -- 'database', 'instance', 'volume', 'cluster'
    resource_id VARCHAR(64),
    type VARCHAR(32) NOT NULL, -- 'full', 'incremental', 'point_in_time'
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending', 'running', 'verified', 'failed', 'expired', 'deleted'
    storage_location VARCHAR(512) NOT NULL,
    checksum_sha256 VARCHAR(64) NOT NULL,
    encryption_key_version VARCHAR(64) NOT NULL DEFAULT 'v1',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    retention_expiry TIMESTAMPTZ,
    is_protected_point BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_backups_tenant ON backups(tenant_id);
CREATE INDEX IF NOT EXISTS idx_backups_status ON backups(status);
CREATE INDEX IF NOT EXISTS idx_backups_resource ON backups(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_backups_created ON backups(created_at DESC);

-- 2. Backup Policies Table
CREATE TABLE IF NOT EXISTS backup_policies (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    schedule_cron VARCHAR(64) NOT NULL,
    retention_days INT NOT NULL DEFAULT 30,
    max_backups INT NOT NULL DEFAULT 14,
    storage_target VARCHAR(64) NOT NULL DEFAULT 's3',
    encrypt BOOLEAN NOT NULL DEFAULT TRUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Restore & Disaster Recovery Plans Table
CREATE TABLE IF NOT EXISTS restore_plans (
    id VARCHAR(64) PRIMARY KEY,
    backup_id VARCHAR(64) NOT NULL REFERENCES backups(id) ON DELETE CASCADE,
    dry_run BOOLEAN NOT NULL DEFAULT FALSE,
    target_state VARCHAR(64) NOT NULL DEFAULT 'consistent',
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending', 'validating', 'restoring', 'verifying', 'completed', 'failed'
    actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    discrepancies_found INT NOT NULL DEFAULT 0,
    repairs_attempted INT NOT NULL DEFAULT 0,
    repairs_succeeded INT NOT NULL DEFAULT 0,
    audit_hash_verified BOOLEAN NOT NULL DEFAULT FALSE,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

-- 4. Reconciliation Reports Table
CREATE TABLE IF NOT EXISTS reconciliation_reports (
    id VARCHAR(64) PRIMARY KEY,
    trigger VARCHAR(64) NOT NULL,
    dry_run BOOLEAN NOT NULL DEFAULT FALSE,
    orphaned_instances_count INT NOT NULL DEFAULT 0,
    missing_nodes_count INT NOT NULL DEFAULT 0,
    stale_jobs_count INT NOT NULL DEFAULT 0,
    abandoned_migrations INT NOT NULL DEFAULT 0,
    inconsistent_quotas INT NOT NULL DEFAULT 0,
    total_discrepancies INT NOT NULL DEFAULT 0,
    repaired_count INT NOT NULL DEFAULT 0,
    unsafe_count INT NOT NULL DEFAULT 0,
    discrepancies JSONB NOT NULL DEFAULT '[]'::jsonb,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reconciliation_created ON reconciliation_reports(created_at DESC);

-- 5. Key Rotation Audit Table
CREATE TABLE IF NOT EXISTS key_rotations (
    id VARCHAR(64) PRIMARY KEY,
    type VARCHAR(32) NOT NULL, -- 'jwt_signing', 'webhook_secret', 'node_mtls_cert', 'db_credential', 'backup_encryption'
    key_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active', -- 'active', 'grace_period', 'retired', 'revoked'
    version INT NOT NULL DEFAULT 1,
    algorithm VARCHAR(32) NOT NULL,
    description TEXT,
    rotated_by VARCHAR(64) NOT NULL,
    grace_period_expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    revocation_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_key_rotations_type_status ON key_rotations(type, status);
