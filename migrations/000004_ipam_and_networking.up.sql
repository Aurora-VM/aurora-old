-- Aurora Migration: 000004_ipam_and_networking.up.sql
-- Tables: ip_pools, ip_allocations, firewall_rules

CREATE TABLE IF NOT EXISTS ip_pools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL,
    location_id VARCHAR(64) NOT NULL DEFAULT '',
    ip_version INT NOT NULL DEFAULT 4, -- 4 or 6
    cidr VARCHAR(64) NOT NULL UNIQUE,
    gateway VARCHAR(64) NOT NULL,
    dns_servers TEXT[] NOT NULL DEFAULT '{"1.1.1.1", "8.8.8.8"}',
    vlan_id INT DEFAULT NULL,
    is_private BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ip_allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pool_id UUID NOT NULL REFERENCES ip_pools(id) ON DELETE CASCADE,
    instance_id UUID REFERENCES instances(id) ON DELETE SET NULL,
    ip_address VARCHAR(64) NOT NULL UNIQUE,
    mac_address VARCHAR(32),
    interface_name VARCHAR(32) NOT NULL DEFAULT 'eth0',
    is_reserved BOOLEAN NOT NULL DEFAULT false,
    notes TEXT,
    allocated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS firewall_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    direction VARCHAR(16) NOT NULL DEFAULT 'inbound', -- 'inbound' or 'outbound'
    action VARCHAR(16) NOT NULL DEFAULT 'allow',       -- 'allow' or 'drop'
    protocol VARCHAR(16) NOT NULL DEFAULT 'tcp',     -- 'tcp', 'udp', 'icmp', 'all'
    port_range VARCHAR(32) NOT NULL DEFAULT '',        -- e.g. "80", "443", "1000-2000", "any"
    source_cidr VARCHAR(64) NOT NULL DEFAULT '0.0.0.0/0',
    dest_cidr VARCHAR(64) NOT NULL DEFAULT '0.0.0.0/0',
    priority INT NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ip_pools_location ON ip_pools(location_id);
CREATE INDEX IF NOT EXISTS idx_ip_allocations_pool ON ip_allocations(pool_id);
CREATE INDEX IF NOT EXISTS idx_ip_allocations_instance ON ip_allocations(instance_id);
CREATE INDEX IF NOT EXISTS idx_firewall_rules_instance ON firewall_rules(instance_id);
