-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Idempotency Constraints Migration
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000015_add_idempotency_constraints
-- Description: Adds database-level constraints to enforce idempotency
-- Author: Outbox Pattern Implementation
-- Date: 2026-10-06
-- Reference: AUDIT_03_WALLET_WITHDRAWAL.md
-- ═══════════════════════════════════════════════════════════════════════════
--
-- PURPOSE:
-- Enforce idempotency guarantees at the database level to prevent race conditions
-- and duplicate operations.
--
-- CONSTRAINTS ADDED:
-- 1. One pending withdrawal per user (prevents concurrent withdrawal race condition)
-- 2. Processing status for atomic state transitions
--
-- WHY DATABASE CONSTRAINTS?
-- Application-level checks can have race conditions between READ and WRITE.
-- Database constraints are atomic and provide stronger guarantees.
-- ═══════════════════════════════════════════════════════════════════════════

-- ───────────────────────────────────────────────────────────────────────────
-- CONSTRAINT 1: One Pending Withdrawal Per User
-- ───────────────────────────────────────────────────────────────────────────
-- 
-- PROBLEM:
-- Race condition exists in current code:
--   Request A: Check pending → None found
--   Request B: Check pending → None found
--   Request A: Create pending withdrawal
--   Request B: Create pending withdrawal ← DUPLICATE!
--
-- SOLUTION:
-- Partial unique index prevents multiple pending withdrawals per user.
-- Only applies to pending status (allows multiple completed withdrawals).
--
-- BEHAVIOR:
-- First request succeeds, second request gets constraint violation error.
-- Application can catch this error and return user-friendly message.
--
CREATE UNIQUE INDEX IF NOT EXISTS idx_one_pending_withdrawal_per_user
    ON wallet_transactions(user_id)
    WHERE type = 'withdrawal' AND status = 'pending';

COMMENT ON INDEX idx_one_pending_withdrawal_per_user IS 
    'Prevents users from having multiple pending withdrawals simultaneously (race condition protection)';

-- ───────────────────────────────────────────────────────────────────────────
-- ENHANCEMENT: Add 'processing' status support
-- ───────────────────────────────────────────────────────────────────────────
--
-- CURRENT STATE MACHINE:
--   pending → completed
--   pending → failed
--
-- IMPROVED STATE MACHINE:
--   pending → processing → completed
--   pending → processing → failed
--
-- BENEFIT:
-- Worker can atomically claim a withdrawal by updating status from 'pending'
-- to 'processing'. Second worker trying to claim gets 0 rows affected.
--
-- UPDATE: UPDATE wallet_transactions
--         SET status = 'processing'
--         WHERE id = ? AND status = 'pending'
--         -- If RowsAffected = 0, already claimed by another worker
--
-- NOTE: This migration doesn't alter the status column type (VARCHAR is flexible).
-- The application code will use the new 'processing' status value.
-- No schema change needed, just documenting the enhancement.

COMMENT ON COLUMN wallet_transactions.status IS 
    'Transaction status: pending (created), processing (worker claimed), completed (success), failed (error)';

-- ═══════════════════════════════════════════════════════════════════════════
-- TESTING THE CONSTRAINT
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Test 1: Create first pending withdrawal (should succeed)
-- INSERT INTO wallet_transactions (user_id, type, status, ...)
-- VALUES ('user-uuid', 'withdrawal', 'pending', ...);
-- → SUCCESS
--
-- Test 2: Create second pending withdrawal for same user (should fail)
-- INSERT INTO wallet_transactions (user_id, type, status, ...)
-- VALUES ('user-uuid', 'withdrawal', 'pending', ...);
-- → ERROR: duplicate key value violates unique constraint
--
-- Test 3: Create pending withdrawal after first completes (should succeed)
-- UPDATE wallet_transactions SET status = 'completed' WHERE ...;
-- INSERT INTO wallet_transactions (user_id, type, status, ...)
-- VALUES ('user-uuid', 'withdrawal', 'pending', ...);
-- → SUCCESS (constraint only applies to pending status)
--
-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
