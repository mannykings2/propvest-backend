-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Add Refresh Tokens Timestamps
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000006_add_refresh_tokens_timestamps
-- Description: Adds updated_at and deleted_at columns to refresh_tokens table
-- Author: Backend Engineering Team
-- Date: 2026-08-05
-- Reason: GORM expects updated_at and deleted_at but they're missing from 000002
-- ═══════════════════════════════════════════════════════════════════════════

-- Add updated_at column for GORM's automatic timestamp tracking
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'refresh_tokens' 
        AND column_name = 'updated_at'
    ) THEN
        ALTER TABLE refresh_tokens 
        ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();
    END IF;
END $$;

-- Add deleted_at column for GORM's soft-delete functionality
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'refresh_tokens' 
        AND column_name = 'deleted_at'
    ) THEN
        ALTER TABLE refresh_tokens 
        ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;
    END IF;
END $$;

-- Create index on deleted_at for GORM's soft-delete queries
-- GORM automatically adds "WHERE deleted_at IS NULL" to all queries
-- This index makes those queries fast
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_deleted_at ON refresh_tokens(deleted_at);

-- Create trigger to auto-update updated_at column
-- This ensures updated_at changes whenever the row is modified
CREATE OR REPLACE TRIGGER update_refresh_tokens_updated_at
    BEFORE UPDATE ON refresh_tokens
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add documentation comments
COMMENT ON COLUMN refresh_tokens.updated_at IS 'Automatically updated by GORM on every UPDATE';
COMMENT ON COLUMN refresh_tokens.deleted_at IS 'GORM soft-delete timestamp - NULL means active, timestamp means deleted';

-- ═══════════════════════════════════════════════════════════════════════════
-- EXPLANATION FOR JUNIOR DEVELOPERS
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Q: What is updated_at used for?
-- A: GORM automatically sets this field to time.Now() every time a record is 
--    updated. This lets you track when a refresh token was last modified 
--    (e.g., when it was rotated or revoked).
--
-- Q: What is deleted_at used for?
-- A: GORM's "soft delete" feature. When you call db.Delete(&token), GORM 
--    doesn't actually DELETE the row. It sets deleted_at to time.Now() and 
--    automatically filters out "deleted" records in all future queries.
--
-- Q: Why use soft-delete instead of hard-delete?
-- A: Security and audit purposes:
--    1. Can recover accidentally deleted tokens
--    2. Maintain complete audit trail for compliance
--    3. Investigate security incidents (which tokens were used when)
--    4. Forensics if account is compromised
--
-- Q: Why do we need the trigger?
-- A: PostgreSQL doesn't automatically update updated_at. The trigger calls 
--    update_updated_at_column() which was created in migration 000001.
--    This ensures updated_at always reflects the last modification time.
--
-- Q: What does GORM.DeletedAt do differently from *time.Time?
-- A: gorm.DeletedAt is a special type that:
--    1. Automatically adds "WHERE deleted_at IS NULL" to SELECT queries
--    2. Changes DELETE to UPDATE (sets deleted_at instead of removing row)
--    3. Provides Unscoped() method to query deleted records if needed
--
-- Q: Can I still permanently delete a record?
-- A: Yes, use db.Unscoped().Delete(&token) to bypass soft-delete and 
--    permanently remove the row from the database.
--
-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
