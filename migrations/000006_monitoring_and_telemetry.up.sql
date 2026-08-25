-- Aurora Migration: 000006_monitoring_and_telemetry.up.sql
-- Tables: metric_samples, alert_thresholds, alert_events

CREATE TABLE IF NOT EXISTS metric_samples (
    id BIGSERIAL,
    resource_type VARCHAR(32) NOT NULL, -- 'node' or 'instance'
    resource_id UUID NOT NULL,
    metric_name VARCHAR(64) NOT NULL,   -- 'cpu_percent', 'memory_used_bytes', 'disk_used_bytes', 'net_rx_bytes', 'net_tx_bytes', 'load_1m', 'load_5m', 'load_15m'
    value DOUBLE PRECISION NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (resource_type, resource_id, metric_name, timestamp)
);

CREATE INDEX IF NOT EXISTS idx_metric_samples_query 
ON metric_samples (resource_type, resource_id, metric_name, timestamp DESC);

CREATE TABLE IF NOT EXISTS alert_thresholds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    resource_type VARCHAR(32) NOT NULL, -- 'node' or 'instance'
    resource_id UUID NOT NULL,
    metric_name VARCHAR(64) NOT NULL,
    operator VARCHAR(8) NOT NULL,       -- 'gt', 'gte', 'lt', 'lte', 'eq'
    threshold_value DOUBLE PRECISION NOT NULL,
    duration_seconds INT NOT NULL DEFAULT 60,
    severity VARCHAR(16) NOT NULL DEFAULT 'warning', -- 'info', 'warning', 'critical'
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_thresholds_resource 
ON alert_thresholds (resource_type, resource_id);

CREATE TABLE IF NOT EXISTS alert_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    threshold_id UUID REFERENCES alert_thresholds(id) ON DELETE CASCADE,
    resource_type VARCHAR(32) NOT NULL,
    resource_id UUID NOT NULL,
    triggered_value DOUBLE PRECISION NOT NULL,
    severity VARCHAR(16) NOT NULL,
    message TEXT NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'firing', -- 'firing', 'acknowledged', 'resolved'
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alert_events_lookup 
ON alert_events (resource_type, resource_id, state);
