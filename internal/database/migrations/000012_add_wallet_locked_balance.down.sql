-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000012 Rollback: Remove locked_balance from wallets
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop index
DROP INDEX IF EXISTS idx_wallets_locked_balance;

-- Drop constraints
ALTER TABLE wallets DROP CONSTRAINT IF EXISTS chk_wallets_locked_not_exceeds_main;
ALTER TABLE wallets DROP CONSTRAINT IF EXISTS chk_wallets_locked_balance_non_negative;

-- Drop column
ALTER TABLE wallets DROP COLUMN IF EXISTS locked_balance;
