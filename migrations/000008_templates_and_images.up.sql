-- 000008_templates_and_images.up.sql
-- Stage 10: Image Management, OS Template Registry & Cloud-Init Schema

CREATE TABLE IF NOT EXISTS os_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL,
    slug VARCHAR(64) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    distribution VARCHAR(64) NOT NULL,
    version VARCHAR(32) NOT NULL,
    release VARCHAR(64) NOT NULL DEFAULT '',
    supported_architectures JSONB NOT NULL DEFAULT '["x86_64"]'::jsonb,
    supported_instance_types JSONB NOT NULL DEFAULT '["container", "virtual-machine"]'::jsonb,
    min_disk_bytes BIGINT NOT NULL DEFAULT 5368709120, -- 5GB default
    min_memory_bytes BIGINT NOT NULL DEFAULT 536870912, -- 512MB default
    cloud_init_supported BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(32) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deprecated', 'retired')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_os_templates_slug ON os_templates(slug);
CREATE INDEX IF NOT EXISTS idx_os_templates_distribution ON os_templates(distribution);
CREATE INDEX IF NOT EXISTS idx_os_templates_status ON os_templates(status);

CREATE TABLE IF NOT EXISTS image_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES os_templates(id) ON DELETE CASCADE,
    architecture VARCHAR(32) NOT NULL DEFAULT 'x86_64',
    instance_type VARCHAR(32) NOT NULL DEFAULT 'container' CHECK (instance_type IN ('container', 'virtual-machine')),
    incus_fingerprint VARCHAR(64) NOT NULL,
    image_alias VARCHAR(128) NOT NULL DEFAULT '',
    source_remote VARCHAR(64) NOT NULL DEFAULT 'images',
    source_url TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'available' CHECK (status IN ('queued', 'syncing', 'verifying', 'available', 'verification_failed', 'sync_failed', 'retired')),
    error_message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_image_artifacts_template ON image_artifacts(template_id);
CREATE INDEX IF NOT EXISTS idx_image_artifacts_fingerprint ON image_artifacts(incus_fingerprint);
CREATE INDEX IF NOT EXISTS idx_image_artifacts_lookup ON image_artifacts(template_id, architecture, instance_type, status);

CREATE TABLE IF NOT EXISTS node_image_availability (
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    artifact_id UUID NOT NULL REFERENCES image_artifacts(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT 'available' CHECK (status IN ('available', 'syncing', 'failed')),
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (node_id, artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_node_image_avail_node ON node_image_availability(node_id);
CREATE INDEX IF NOT EXISTS idx_node_image_avail_artifact ON node_image_availability(artifact_id);
