-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Rollback Refresh Tokens Timestamps
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000006_add_refresh_tokens_timestamps (DOWN)
-- Description: Removes updated_at and deleted_at columns from refresh_tokens
-- Author: Backend Engineering Team
-- Date: 2026-08-05
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop the trigger first (must drop trigger before dropping column)
DROP TRIGGER IF EXISTS update_refresh_tokens_updated_at ON refresh_tokens;

-- Drop the index on deleted_at
DROP INDEX IF EXISTS idx_refresh_tokens_deleted_at;

-- Remove deleted_at column
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'refresh_tokens' 
        AND column_name = 'deleted_at'
    ) THEN
        ALTER TABLE refresh_tokens 
        DROP COLUMN deleted_at;
    END IF;
END $$;

-- Remove updated_at column
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'refresh_tokens' 
        AND column_name = 'updated_at'
    ) THEN
        ALTER TABLE refresh_tokens 
        DROP COLUMN updated_at;
    END IF;
END $$;

-- ═══════════════════════════════════════════════════════════════════════════
-- EXPLANATION FOR JUNIOR DEVELOPERS
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Q: Why drop trigger before dropping column?
-- A: Because the trigger references the updated_at column. If you try to drop 
--    the column first, PostgreSQL will complain that the trigger depends on it.
--    Always drop dependencies before dropping the thing they depend on.
--
-- Q: Is this rollback safe?
-- A: ⚠️ WARNING: This is DESTRUCTIVE!
--    - All updated_at timestamps will be lost
--    - All soft-deleted records will become visible again (deleted_at removed)
--    - You cannot undo this without a backup
--    Only use in development or if you're absolutely certain.
--
-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
