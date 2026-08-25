-- Aurora Migration: 000007_audit_and_compliance.up.sql
-- Alter audit_logs to support tamper-evident hash chaining and severity.
-- Create siem_destinations table.

ALTER TABLE audit_logs 
ADD COLUMN IF NOT EXISTS severity VARCHAR(24) NOT NULL DEFAULT 'info',
ADD COLUMN IF NOT EXISTS prev_hash VARCHAR(64) DEFAULT '',
ADD COLUMN IF NOT EXISTS tamper_proof_hash VARCHAR(64) DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_action_time 
ON audit_logs (actor_id, action, created_at DESC);

CREATE TABLE IF NOT EXISTS siem_destinations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL,
    type VARCHAR(32) NOT NULL,            -- 'webhook', 'syslog_tcp', 'syslog_udp'
    target VARCHAR(512) NOT NULL,         -- URL or host:port
    auth_token VARCHAR(512) DEFAULT '',   -- Optional bearer token / secret
    format VARCHAR(32) NOT NULL DEFAULT 'json', -- 'json', 'cef', 'rfc5424'
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
