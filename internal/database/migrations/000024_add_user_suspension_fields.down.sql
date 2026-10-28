-- Rollback Migration 000024: Remove User Suspension Fields
-- Milestone 7 - Phase 4: User Management

-- Drop indexes first
DROP INDEX IF EXISTS idx_users_suspended_by;
DROP INDEX IF EXISTS idx_users_suspended_at;

-- Drop suspension tracking columns
ALTER TABLE users DROP COLUMN IF EXISTS suspended_reason;
ALTER TABLE users DROP COLUMN IF EXISTS suspended_by;
ALTER TABLE users DROP COLUMN IF EXISTS suspended_at;
