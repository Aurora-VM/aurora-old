-- Aurora Migration: 000005_storage_and_volumes.up.sql
-- Tables: storage_pools, volumes, volume_snapshots

CREATE TABLE IF NOT EXISTS storage_pools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    driver VARCHAR(32) NOT NULL DEFAULT 'dir', -- 'zfs', 'btrfs', 'lvm', 'ceph', 'dir'
    total_space_bytes BIGINT NOT NULL DEFAULT 0,
    used_space_bytes BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'online', -- 'online', 'degraded', 'offline'
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_node_pool_name UNIQUE(node_id, name)
);

CREATE TABLE IF NOT EXISTS volumes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pool_id UUID NOT NULL REFERENCES storage_pools(id) ON DELETE RESTRICT,
    instance_id UUID REFERENCES instances(id) ON DELETE SET NULL,
    name VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 10737418240, -- 10 GiB default
    content_type VARCHAR(32) NOT NULL DEFAULT 'filesystem', -- 'filesystem' or 'block'
    mount_path VARCHAR(255) DEFAULT '/mnt/data',
    read_only BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(32) NOT NULL DEFAULT 'ready', -- 'creating', 'ready', 'attached', 'deleting'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_pool_volume_name UNIQUE(pool_id, name)
);

CREATE TABLE IF NOT EXISTS volume_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    volume_id UUID NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_volume_snapshot_name UNIQUE(volume_id, name)
);

CREATE INDEX IF NOT EXISTS idx_storage_pools_node ON storage_pools(node_id);
CREATE INDEX IF NOT EXISTS idx_volumes_user ON volumes(user_id);
CREATE INDEX IF NOT EXISTS idx_volumes_instance ON volumes(instance_id);
CREATE INDEX IF NOT EXISTS idx_volumes_pool ON volumes(pool_id);
CREATE INDEX IF NOT EXISTS idx_volume_snapshots_volume ON volume_snapshots(volume_id);
