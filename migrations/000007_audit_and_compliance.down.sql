-- Aurora Rollback Migration: 000007_audit_and_compliance.down.sql

DROP TABLE IF EXISTS siem_destinations;
ALTER TABLE audit_logs 
DROP COLUMN IF EXISTS tamper_proof_hash,
DROP COLUMN IF EXISTS prev_hash,
DROP COLUMN IF EXISTS severity;
