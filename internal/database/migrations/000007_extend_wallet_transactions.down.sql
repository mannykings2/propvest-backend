-- Rollback 000007: remove the ledger extension columns.
DROP INDEX IF EXISTS idx_wallet_transactions_idempotency_key;
DROP INDEX IF EXISTS idx_wallet_transactions_external_reference;
DROP INDEX IF EXISTS idx_wallet_transactions_user_id;

ALTER TABLE wallet_transactions
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS external_reference,
    DROP COLUMN IF EXISTS user_id;
