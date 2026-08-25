-- Project Aurora: Phase 15 - Production Operations, High Availability & Self-Healing Migration

-- 1. Extend nodes table for self-healing & maintenance
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS drain_mode BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS unhealthy_reason TEXT;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS last_state_change_at TIMESTAMPTZ DEFAULT NOW();

-- 2. Durable Asynchronous Jobs Table
CREATE TABLE IF NOT EXISTS jobs (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    type VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64),
    resource_id VARCHAR(64),
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    payload JSONB,
    result JSONB,
    error TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ,
    locked_by_worker VARCHAR(128),
    locked_until TIMESTAMPTZ,
    progress_percent INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_next_retry ON jobs(status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_jobs_tenant_created ON jobs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_worker_lease ON jobs(locked_by_worker, locked_until);
CREATE INDEX IF NOT EXISTS idx_jobs_resource ON jobs(resource_type, resource_id);

-- 3. Job Execution Attempts Log
CREATE TABLE IF NOT EXISTS job_attempts (
    id VARCHAR(64) PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_number INT NOT NULL,
    worker_id VARCHAR(128) NOT NULL,
    error TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_job_attempts_job_id ON job_attempts(job_id, attempt_number ASC);

-- 4. Worker Leases & Distributed Control Plane Registrations
CREATE TABLE IF NOT EXISTS worker_leases (
    worker_id VARCHAR(128) PRIMARY KEY,
    hostname VARCHAR(255) NOT NULL,
    pid INT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_worker_leases_expires ON worker_leases(expires_at);

-- 5. Workload Migrations Table
CREATE TABLE IF NOT EXISTS workload_migrations (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    instance_id VARCHAR(64) NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    source_node_id VARCHAR(64) NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    dest_node_id VARCHAR(64) NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL DEFAULT 'live',
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    preflight_data JSONB,
    progress_percent INT NOT NULL DEFAULT 0,
    bytes_transferred BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    error TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_migrations_instance ON workload_migrations(instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_migrations_tenant ON workload_migrations(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_migrations_status ON workload_migrations(status);

-- 6. Rate Limit Distributed Buckets
CREATE TABLE IF NOT EXISTS rate_limit_buckets (
    bucket_key VARCHAR(255) PRIMARY KEY,
    tokens INT NOT NULL,
    max_tokens INT NOT NULL,
    last_refill_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_expires ON rate_limit_buckets(expires_at);
