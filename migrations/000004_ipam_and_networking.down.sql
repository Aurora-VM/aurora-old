-- Aurora Migration: 000004_ipam_and_networking.down.sql

DROP TABLE IF EXISTS firewall_rules;
DROP TABLE IF EXISTS ip_allocations;
DROP TABLE IF EXISTS ip_pools;
