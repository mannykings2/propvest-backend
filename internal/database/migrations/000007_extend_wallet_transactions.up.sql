-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000007: Extend wallet_transactions into a full ledger
-- ═══════════════════════════════════════════════════════════════════════════
-- Description: Adds user_id, external_reference, idempotency_key, metadata to the
--   wallet_transactions ledger so it satisfies the Transaction Model doc (2.3)
--   and supports idempotent deposits/withdrawals. We EXTEND rather than rename
--   the table (DECISION D5) so existing code and data keep working.
-- ═══════════════════════════════════════════════════════════════════════════

ALTER TABLE wallet_transactions
    ADD COLUMN IF NOT EXISTS user_id UUID,
    ADD COLUMN IF NOT EXISTS external_reference VARCHAR(255),
    ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255),
    ADD COLUMN IF NOT EXISTS metadata JSONB;

-- Backfill user_id for any pre-existing rows by joining through the wallet.
UPDATE wallet_transactions wt
SET user_id = w.user_id
FROM wallets w
WHERE wt.wallet_id = w.id AND wt.user_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_id ON wallet_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_external_reference ON wallet_transactions(external_reference);
-- idempotency_key must be unique when present (NULLs are allowed and not unique).
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_transactions_idempotency_key
    ON wallet_transactions(idempotency_key)
    WHERE idempotency_key IS NOT NULL;

COMMENT ON COLUMN wallet_transactions.user_id IS 'Denormalized owner for direct per-user history queries';
COMMENT ON COLUMN wallet_transactions.external_reference IS 'Payment provider reference (Paystack), when applicable';
COMMENT ON COLUMN wallet_transactions.idempotency_key IS 'Client idempotency key; unique when present to prevent duplicate ledger rows';
COMMENT ON COLUMN wallet_transactions.metadata IS 'Free-form JSON context (provider payloads, extra fields)';
