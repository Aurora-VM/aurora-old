-- Aurora Rollback Migration: 000005_storage_and_volumes.down.sql

DROP TABLE IF EXISTS volume_snapshots;
DROP TABLE IF EXISTS volumes;
DROP TABLE IF EXISTS storage_pools;
