-- Project Aurora: Phase 15 - Operations, HA & Jobs Migration Down

DROP TABLE IF EXISTS rate_limit_buckets;
DROP TABLE IF EXISTS workload_migrations;
DROP TABLE IF EXISTS worker_leases;
DROP TABLE IF EXISTS job_attempts;
DROP TABLE IF EXISTS jobs;

ALTER TABLE nodes DROP COLUMN IF EXISTS last_state_change_at;
ALTER TABLE nodes DROP COLUMN IF EXISTS unhealthy_reason;
ALTER TABLE nodes DROP COLUMN IF EXISTS drain_mode;
