-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000009: payments (provider-side funding records)
-- ═══════════════════════════════════════════════════════════════════════════
-- Separate from wallet_transactions: `payments` is the gateway lifecycle record
-- (initialize -> verify/webhook), `wallet_transactions` is the internal ledger.
-- The wallet is credited (a ledger row created) ONLY after a payment succeeds.
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    provider VARCHAR(32) NOT NULL,
    reference VARCHAR(255) UNIQUE NOT NULL,
    provider_reference VARCHAR(255),
    amount_kobo BIGINT NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'NGN',
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    channel VARCHAR(16) NOT NULL,
    authorization_url TEXT,
    raw_payload JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_payments_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_payments_status CHECK (status IN ('pending', 'success', 'failed')),
    CONSTRAINT chk_payments_channel CHECK (channel IN ('deposit', 'withdrawal')),
    CONSTRAINT chk_payments_amount_positive CHECK (amount_kobo > 0)
);

CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_provider_reference ON payments(provider_reference);

CREATE OR REPLACE TRIGGER update_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE payments IS 'Payment provider lifecycle records (deposits/withdrawals). Ledger credit happens only on success.';
COMMENT ON COLUMN payments.raw_payload IS 'Raw provider webhook/verify body for reconciliation and forensics';
