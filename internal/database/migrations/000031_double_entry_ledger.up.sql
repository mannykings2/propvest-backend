-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000031: Double-entry ledger, fee rules, settlement & reconciliation
-- (Phase 7 — Money layer)
--
-- One rule: every naira has exactly one home. Every money mutation writes a
-- balanced journal (sum(debits) = sum(credits)) inside the SAME database
-- transaction as the wallet change it describes.
--
-- Account model note: 2000 "User funds custody" is the CONTROL account — the
-- ring-fenced total. Postings go to the per-user sub-ledger accounts (2010
-- wallets, 2100 withdrawal reserve, 2020 earnings payable) so that
--   sum(user account entries)  ==  sum(wallets.balance columns)
-- for every user, which is what makes the nightly reconciliation provable.
-- 2000 is reported as the computed control total, never posted to directly.
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS ledger_accounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(8)  NOT NULL UNIQUE,
    name        VARCHAR(120) NOT NULL,
    type        VARCHAR(20) NOT NULL
        CHECK (type IN ('asset','liability','equity','revenue','income','expense')),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_id   UUID NOT NULL,                 -- groups one balanced posting
    account_id   UUID NOT NULL REFERENCES ledger_accounts(id),
    user_id      UUID REFERENCES users(id),     -- sub-ledger owner (2010/2100/2020/2200)
    wallet_txn_id UUID,                         -- wallet_transactions.id when applicable
    debit        BIGINT NOT NULL DEFAULT 0 CHECK (debit >= 0),
    credit       BIGINT NOT NULL DEFAULT 0 CHECK (credit >= 0),
    currency     VARCHAR(3) NOT NULL DEFAULT 'NGN',
    description  TEXT,
    ref          VARCHAR(160),                  -- business reference (payment/txn ref)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A row is a debit OR a credit, never both.
    CONSTRAINT ledger_entry_one_side CHECK (debit = 0 OR credit = 0)
);

CREATE INDEX IF NOT EXISTS idx_ledger_entries_journal ON ledger_entries (journal_id);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_account ON ledger_entries (account_id);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_user    ON ledger_entries (user_id);
CREATE INDEX IF NOT EXISTS idx_ledger_entries_ref     ON ledger_entries (ref);

-- Immutability: the journal is evidence. No UPDATE, no DELETE — corrections
-- are compensating journals (new rows), never edits.
CREATE OR REPLACE FUNCTION ledger_entries_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries is append-only: post a reversing journal instead of editing';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ledger_entries_immutable ON ledger_entries;
CREATE TRIGGER trg_ledger_entries_immutable
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_immutable();

-- ── Fee rules: the ONLY place fee maths is defined ─────────────────────────
-- Exactly two rules are seeded (7.4): a 1.0% withdrawal fee capped at ₦2,000,
-- and a performance fee per product inside the 10–15% band. Any withdrawal
-- path that charges without an active rule here fails.
CREATE TABLE IF NOT EXISTS fee_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(60) NOT NULL UNIQUE,
    name        VARCHAR(120) NOT NULL,
    fee_type    VARCHAR(20) NOT NULL CHECK (fee_type IN ('transaction','performance')),
    basis       VARCHAR(40) NOT NULL,           -- 'withdrawal' | 'distributed_profit'
    rate_bp     INT NOT NULL CHECK (rate_bp >= 0),   -- basis points: 100 = 1%
    cap_kobo    BIGINT,                          -- NULL = uncapped
    min_kobo    BIGINT NOT NULL DEFAULT 0,
    applies_to  VARCHAR(60),                     -- product code for performance fees
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Settlement: Paystack payout batches vs what we booked ──────────────────
CREATE TABLE IF NOT EXISTS settlement_batches (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider     VARCHAR(40) NOT NULL DEFAULT 'paystack',
    external_ref VARCHAR(160) NOT NULL UNIQUE,
    batch_date   DATE,
    gross_kobo   BIGINT NOT NULL DEFAULT 0,
    fees_kobo    BIGINT NOT NULL DEFAULT 0,
    net_kobo     BIGINT NOT NULL DEFAULT 0,
    status       VARCHAR(20) NOT NULL DEFAULT 'expected'
        CHECK (status IN ('expected','received','matched','disputed')),
    alert        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS settlement_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id     UUID NOT NULL REFERENCES settlement_batches(id) ON DELETE CASCADE,
    external_ref VARCHAR(160) NOT NULL,
    amount_kobo  BIGINT NOT NULL,
    matched      BOOLEAN NOT NULL DEFAULT FALSE,
    payment_id   UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_settlement_items_batch ON settlement_items (batch_id);
CREATE INDEX IF NOT EXISTS idx_settlement_items_ref   ON settlement_items (external_ref);

-- ── Reconciliation runs: the nightly trust checks ──────────────────────────
CREATE TABLE IF NOT EXISTS reconciliation_runs (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind                  VARCHAR(40) NOT NULL DEFAULT 'nightly',
    run_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status                VARCHAR(20) NOT NULL CHECK (status IN ('pass','fail')),
    total_debits_kobo     BIGINT NOT NULL DEFAULT 0,
    total_credits_kobo    BIGINT NOT NULL DEFAULT 0,
    user_liability_kobo   BIGINT NOT NULL DEFAULT 0,
    wallet_balance_kobo   BIGINT NOT NULL DEFAULT 0,
    custody_assets_kobo   BIGINT NOT NULL DEFAULT 0,
    suspense_kobo         BIGINT NOT NULL DEFAULT 0,
    discrepancy_kobo      BIGINT NOT NULL DEFAULT 0,
    details               JSONB,
    alerted               BOOLEAN NOT NULL DEFAULT FALSE
);

-- ── The withdrawal fee is booked when the money actually leaves ────────────
-- fee_kobo/net_kobo are frozen at initiation so a fee-rule change mid-flight
-- cannot alter an in-flight withdrawal's maths.
ALTER TABLE wallet_transactions
    ADD COLUMN IF NOT EXISTS fee_kobo BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS net_kobo BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN ledger_accounts.code IS
    'Chart-of-accounts code (1000 operating cash, 1010 clearing, 2010 wallets, 2100 reserve, 2020 earnings payable, 2200 holdings, 3000/3100 fee revenue, 4000 rental income, 9000 suspense).';
COMMENT ON COLUMN ledger_entries.journal_id IS
    'UUID shared by every leg of one balanced posting; sum(debits)=sum(credits) per journal is enforced in the posting service.';
