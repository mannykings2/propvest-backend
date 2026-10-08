-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000021: Enhance Investments Table
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Add fields required for Investment Module (Milestone 5)
-- - unit_price_kobo: Price snapshot at time of purchase (immutable)
-- - currency: Currency tracking (defaults to NGN)
-- - idempotency_key: Prevent duplicate investments on retries
-- - Lifecycle timestamps: cancelled_at, completed_at, refunded_at
-- ═══════════════════════════════════════════════════════════════════════════

-- Add unit_price_kobo (CRITICAL: price snapshot)
-- This is the property.unit_price at the moment of investment
-- NEVER recalculate from amount/slots - that breaks on property price changes
ALTER TABLE investments
ADD COLUMN unit_price_kobo BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN investments.unit_price_kobo IS 'Price per slot at time of purchase (immutable snapshot)';

-- Add currency field (defaults to NGN)
ALTER TABLE investments
ADD COLUMN currency VARCHAR(3) NOT NULL DEFAULT 'NGN';

COMMENT ON COLUMN investments.currency IS 'Currency of the investment (NGN, USD, etc.)';

-- Add idempotency_key (nullable, unique per user)
-- Prevents duplicate investments when client retries
ALTER TABLE investments
ADD COLUMN idempotency_key VARCHAR(255);

COMMENT ON COLUMN investments.idempotency_key IS 'Client-supplied key to prevent duplicate investments on retries';

-- Add lifecycle timestamps
ALTER TABLE investments
ADD COLUMN cancelled_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN completed_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN refunded_at TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN investments.cancelled_at IS 'Timestamp when investment was cancelled';
COMMENT ON COLUMN investments.completed_at IS 'Timestamp when investment reached completed state';
COMMENT ON COLUMN investments.refunded_at IS 'Timestamp when investment was refunded';

-- ═══════════════════════════════════════════════════════════════════════════
-- Constraints
-- ═══════════════════════════════════════════════════════════════════════════

-- Idempotency: Same user cannot use the same key twice
-- Uses partial unique index (only when idempotency_key IS NOT NULL)
CREATE UNIQUE INDEX idx_investments_user_idempotency 
ON investments(user_id, idempotency_key) 
WHERE idempotency_key IS NOT NULL;

-- Unit price must be positive
ALTER TABLE investments
ADD CONSTRAINT chk_investments_unit_price_positive 
CHECK (unit_price_kobo > 0);

-- Currency must be valid 3-letter code
ALTER TABLE investments
ADD CONSTRAINT chk_investments_currency_format 
CHECK (currency ~ '^[A-Z]{3}$');

-- ═══════════════════════════════════════════════════════════════════════════
-- Indexes for Performance
-- ═══════════════════════════════════════════════════════════════════════════

-- Index for idempotency lookups
CREATE INDEX idx_investments_idempotency_key 
ON investments(idempotency_key) 
WHERE idempotency_key IS NOT NULL;

-- Index for currency filtering
CREATE INDEX idx_investments_currency ON investments(currency);

-- Composite index for user + property queries (check existing investments)
CREATE INDEX idx_investments_user_property ON investments(user_id, property_id);

-- ═══════════════════════════════════════════════════════════════════════════
-- Data Migration: Set unit_price_kobo from amount_kobo / slots
-- ═══════════════════════════════════════════════════════════════════════════
-- For existing investments, calculate unit_price from amount/slots
-- This is safe because existing investments haven't had price changes
UPDATE investments
SET unit_price_kobo = amount_kobo / slots
WHERE unit_price_kobo = 0;

-- ═══════════════════════════════════════════════════════════════════════════
-- Verification
-- ═══════════════════════════════════════════════════════════════════════════
-- Ensure no investments have zero unit_price_kobo after migration
DO $$
DECLARE
    zero_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO zero_count FROM investments WHERE unit_price_kobo = 0;
    IF zero_count > 0 THEN
        RAISE EXCEPTION 'Migration failed: % investments still have unit_price_kobo = 0', zero_count;
    END IF;
END $$;
