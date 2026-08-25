-- Aurora Migration: 000003_instances_schema.up.sql
-- Table: instances

CREATE TABLE IF NOT EXISTS instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    name VARCHAR(64) NOT NULL UNIQUE,
    type VARCHAR(32) NOT NULL DEFAULT 'container', -- 'container' or 'virtual-machine'
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- 'pending', 'creating', 'running', 'stopped', 'restarting', 'frozen', 'error', 'deleted'
    cpu_cores INT NOT NULL DEFAULT 1,
    memory_bytes BIGINT NOT NULL DEFAULT 1073741824, -- 1 GB default
    storage_bytes BIGINT NOT NULL DEFAULT 10737418240, -- 10 GB default
    image VARCHAR(255) NOT NULL,
    ipv4_address VARCHAR(64),
    ipv6_address VARCHAR(128),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_instances_user_id ON instances(user_id);
CREATE INDEX IF NOT EXISTS idx_instances_node_id ON instances(node_id);
CREATE INDEX IF NOT EXISTS idx_instances_status ON instances(status);
CREATE INDEX IF NOT EXISTS idx_instances_name ON instances(name);
