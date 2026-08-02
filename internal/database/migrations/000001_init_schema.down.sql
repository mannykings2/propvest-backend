-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Rollback Initial Schema Migration
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000001_init_schema (DOWN)
-- Description: Rolls back all tables created in the init_schema migration
-- Author: Principal Software Architect
-- Date: 2026-07-26
-- ═══════════════════════════════════════════════════════════════════════════
-- WARNING: This will DELETE ALL DATA in these tables!
-- Only run this in development or with proper backups
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop tables in reverse dependency order
-- (dependent tables first, then their dependencies)

DROP TABLE IF EXISTS wallet_transactions CASCADE;
DROP TABLE IF EXISTS properties CASCADE;
DROP TABLE IF EXISTS wallets CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Drop the trigger function
DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;

-- Note: We don't drop the uuid-ossp extension because:
-- 1. It might be used by other applications sharing this database
-- 2. PostgreSQL 13+ has gen_random_uuid() built-in anyway
-- 3. It's harmless to leave installed

-- ═══════════════════════════════════════════════════════════════════════════
-- END OF ROLLBACK
-- ═══════════════════════════════════════════════════════════════════════════
