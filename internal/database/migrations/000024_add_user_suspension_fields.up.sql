-- Migration 000024: Add User Suspension Fields
-- Milestone 7 - Phase 4: User Management
-- 
-- Adds fields to track user account suspension by admins:
--   - suspended_at: When the suspension occurred
--   - suspended_by: Which admin performed the suspension
--   - suspended_reason: Why the account was suspended
--
-- These fields enable admin user management with full audit trail.

-- Add suspension tracking fields to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMP NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended_by UUID NULL REFERENCES users(id);
ALTER TABLE users ADD COLUMN IF NOT EXISTS suspended_reason TEXT NULL;

-- Add index for querying suspended users (admin dashboard needs this)
CREATE INDEX IF NOT EXISTS idx_users_suspended_at ON users(suspended_at) WHERE suspended_at IS NOT NULL;

-- Add index for suspended_by (to find all suspensions by a specific admin)
CREATE INDEX IF NOT EXISTS idx_users_suspended_by ON users(suspended_by) WHERE suspended_by IS NOT NULL;

-- Verification query (optional - can be run manually to verify)
-- SELECT 
--     column_name, 
--     data_type, 
--     is_nullable,
--     column_default
-- FROM information_schema.columns 
-- WHERE table_name = 'users' 
--   AND column_name IN ('suspended_at', 'suspended_by', 'suspended_reason')
-- ORDER BY column_name;
