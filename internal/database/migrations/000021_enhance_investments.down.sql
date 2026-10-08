-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000021: Rollback - Enhance Investments Table
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Rollback investment enhancements from migration 000021
-- WARNING: This will drop columns added in 000021. Data in these columns will be lost.
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop indexes (in reverse order of creation)
DROP INDEX IF EXISTS idx_investments_user_property;
DROP INDEX IF EXISTS idx_investments_currency;
DROP INDEX IF EXISTS idx_investments_idempotency_key;
DROP INDEX IF EXISTS idx_investments_user_idempotency;

-- Drop constraints
ALTER TABLE investments
DROP CONSTRAINT IF EXISTS chk_investments_currency_format;

ALTER TABLE investments
DROP CONSTRAINT IF EXISTS chk_investments_unit_price_positive;

-- Drop columns (in reverse order of addition)
ALTER TABLE investments
DROP COLUMN IF EXISTS refunded_at,
DROP COLUMN IF EXISTS completed_at,
DROP COLUMN IF EXISTS cancelled_at,
DROP COLUMN IF EXISTS idempotency_key,
DROP COLUMN IF EXISTS currency,
DROP COLUMN IF EXISTS unit_price_kobo;
