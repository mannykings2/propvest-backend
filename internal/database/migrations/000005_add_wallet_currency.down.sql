-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Rollback Wallet Currency Column
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000005_add_wallet_currency (DOWN)
-- Description: Removes currency column from wallets table
-- Author: Backend Engineering Team
-- Date: 2026-08-04
-- ═══════════════════════════════════════════════════════════════════════════

-- Remove currency column from wallets table
-- Uses IF EXISTS for safety (won't fail if column doesn't exist)
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'wallets' 
        AND column_name = 'currency'
    ) THEN
        ALTER TABLE wallets 
        DROP COLUMN currency;
    END IF;
END $$;

-- ═══════════════════════════════════════════════════════════════════════════
-- EXPLANATION FOR JUNIOR DEVELOPERS
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Q: What is a "down" migration?
-- A: It's the reverse of an "up" migration. If "up" adds a column, "down" 
--    removes it. This allows you to rollback changes if needed.
--
-- Q: When would I use this?
-- A: If you discover a bug in the currency feature and need to quickly 
--    rollback to the previous version. You'd run:
--    migrate -path internal/database/migrations -database "..." down 1
--
-- Q: Is it safe to drop a column?
-- A: ⚠️ WARNING: This is DESTRUCTIVE! All currency data will be lost.
--    Only use in development or if you're absolutely sure.
--
-- Q: Why do we still need IF EXISTS?
-- A: Defense in depth. If the column doesn't exist (maybe it was manually 
--    removed), the migration won't fail with an error.
--
-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
