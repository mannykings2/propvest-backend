-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Rollback Idempotency Constraints
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000015_add_idempotency_constraints (DOWN)
-- Description: Removes idempotency constraints
-- ═══════════════════════════════════════════════════════════════════════════

-- Remove the unique constraint on pending withdrawals per user
DROP INDEX IF EXISTS idx_one_pending_withdrawal_per_user;

-- Revert the status column comment to original
COMMENT ON COLUMN wallet_transactions.status IS 
    'Transaction status: pending, completed, failed';

-- ═══════════════════════════════════════════════════════════════════════════
-- NOTES
-- ═══════════════════════════════════════════════════════════════════════════
-- After rollback:
-- - The race condition for duplicate pending withdrawals returns
-- - Application-level check becomes the only protection (weaker guarantee)
-- - 'processing' status can still be used by application but has no special DB support
-- ═══════════════════════════════════════════════════════════════════════════
