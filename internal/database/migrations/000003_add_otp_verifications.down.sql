-- Rollback migration: Remove OTP verification table
-- This drops the otp_verifications table and all its data
-- Use this if you need to undo the phone verification feature

-- ═══════════════════════════════════════════════════════════════════════════
-- DROP TABLE
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop all indexes first (optional, but explicit is good)
-- PostgreSQL automatically drops indexes when table is dropped,
-- but being explicit helps with documentation
DROP INDEX IF EXISTS idx_otp_verifications_user_phone;
DROP INDEX IF EXISTS idx_otp_verifications_expires_at;
DROP INDEX IF EXISTS idx_otp_verifications_code_hash;
DROP INDEX IF EXISTS idx_otp_verifications_phone;
DROP INDEX IF EXISTS idx_otp_verifications_user_id;

-- Drop the table
-- CASCADE means: also drop anything that depends on this table
-- In this case, there are no foreign keys referencing this table,
-- but CASCADE is defensive programming
DROP TABLE IF EXISTS otp_verifications CASCADE;

-- ═══════════════════════════════════════════════════════════════════════════
-- NOTES
-- ═══════════════════════════════════════════════════════════════════════════
--
-- What happens when you run this migration?
--   1. All OTP verification records are permanently deleted
--   2. Any pending phone change verifications are lost
--   3. Users will need to request new OTP codes
--   4. No other tables are affected (no foreign key dependencies)
--
-- When to use this rollback?
--   - Testing/development: resetting database state
--   - Removing phone verification feature entirely
--   - Replacing with different verification mechanism
--
-- ⚠️ WARNING: This is a destructive operation
-- In production, consider:
--   - Backing up data first
--   - Notifying users about pending verifications
--   - Coordinating with frontend to handle missing feature
--
-- ═══════════════════════════════════════════════════════════════════════════
