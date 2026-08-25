-- Aurora Rollback Migration: 000006_monitoring_and_telemetry.down.sql

DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS alert_thresholds;
DROP TABLE IF EXISTS metric_samples;
