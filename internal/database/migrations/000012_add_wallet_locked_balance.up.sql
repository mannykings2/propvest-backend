-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000012: Add locked_balance to wallets
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Implement proper fund reservation for pending withdrawals
--
-- WHY LOCKED BALANCE?
-- When a user initiates a withdrawal, we need to:
--   1. Prevent them from spending that money elsewhere (investments, other withdrawals)
--   2. NOT permanently debit their balance (reversal needed if transfer fails)
--
-- Solution: Split balance into two parts:
--   - available_balance: can be spent on investments/withdrawals
--   - locked_balance: reserved for pending withdrawals
--   - total_balance: main_balance (available + locked)
--
-- Example flow:
--   User has ₦100,000 in main_balance
--   User requests withdrawal of ₦50,000
--   → main_balance stays ₦100,000 (unchanged for now)
--   → locked_balance increases to ₦50,000
--   → available = ₦100,000 - ₦50,000 = ₦50,000 (computed)
--
--   When withdrawal succeeds:
--   → main_balance -= ₦50,000 (now ₦50,000)
--   → locked_balance -= ₦50,000 (now ₦0)
--
--   When withdrawal fails:
--   → locked_balance -= ₦50,000 (now ₦0)
--   → main_balance unchanged (still ₦100,000)
--
-- This prevents:
--   - Double spending during pending withdrawals
--   - Race conditions in concurrent operations
--   - Incorrect balance after failed withdrawals
-- ═══════════════════════════════════════════════════════════════════════════

-- Add locked_balance column to wallets table
ALTER TABLE wallets
    ADD COLUMN IF NOT EXISTS locked_balance BIGINT NOT NULL DEFAULT 0;

-- Add check constraint: locked_balance cannot be negative
DO $$ BEGIN
    ALTER TABLE wallets
        ADD CONSTRAINT chk_wallets_locked_balance_non_negative
        CHECK (locked_balance >= 0);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Add check constraint: locked_balance cannot exceed main_balance
-- This prevents locking more money than the user has
DO $$ BEGIN
    ALTER TABLE wallets
        ADD CONSTRAINT chk_wallets_locked_not_exceeds_main
        CHECK (locked_balance <= main_balance);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Create index for queries that filter by locked balance
-- Useful for: "find all wallets with pending withdrawals"
CREATE INDEX IF NOT EXISTS idx_wallets_locked_balance
    ON wallets(locked_balance)
    WHERE locked_balance > 0;

-- Add comment for documentation
COMMENT ON COLUMN wallets.locked_balance IS 'Amount reserved for pending withdrawals (kobo). Deducted from available balance but not yet permanently debited.';

-- ═══════════════════════════════════════════════════════════════════════════
-- COMPUTED COLUMN EXPLANATION (for future reference)
-- ═══════════════════════════════════════════════════════════════════════════
-- We don't create an 'available_balance' column because it would be redundant.
-- Instead, calculate it in application code:
--
--   available_balance = main_balance - locked_balance
--
-- Why not a database computed column?
--   1. PostgreSQL doesn't support true computed columns until v12+ GENERATED ALWAYS
--   2. Adds complexity without much benefit
--   3. Application-level computation is fast (simple subtraction)
--   4. Keeps schema simpler
--
-- In Go code:
--   type Wallet struct {
--       MainBalance   int64
--       LockedBalance int64
--   }
--
--   func (w *Wallet) AvailableBalance() int64 {
--       return w.MainBalance - w.LockedBalance
--   }
-- ═══════════════════════════════════════════════════════════════════════════
