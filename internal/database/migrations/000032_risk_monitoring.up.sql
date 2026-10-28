-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000032: Risk layer — AML rules, alerts, screening, STR/CTR
-- (Phase 8, monitoring & reporting)
--
-- Phase 8 adds the layer that *watches* the transaction stream:
--   1. risk_rules     — every detection rule with its mode (blocking denies
--                       synchronously on the transaction path; monitoring
--                       raises alerts from a background scan) and an
--                       enabled flag the admin can toggle from the UI.
--   2. risk_alerts    — the triage queue: open → in_review → escalated|cleared,
--                       assignee, resolution notes, deduped per pattern window.
--   3. screening_results — every manual sanctions/PEP attestation (Phase 6.3
--                       wrote these onto kyc_submissions only); hits link to
--                       the escalated alert they produced.
--   4. str_reports    — suspicious-transaction reports to NFIU, 24h deadline
--                       from escalation (MLPPA 2022), goAML reference on filing.
--   5. ctr_reports    — daily cash-threshold scan rows (₦5M individual /
--                       ₦10M corporate), 7-day filing clock.
--   6. users risk-flag — manual freeze marker; a flagged user's withdrawals
--                       are blocked (8.4) and screening hits set it.
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS risk_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(60) UNIQUE NOT NULL,
    name        VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    mode        VARCHAR(20) NOT NULL CHECK (mode IN ('blocking','monitoring')),
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    params      JSONB NOT NULL DEFAULT '{}',
    updated_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS risk_alerts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    rule_code        VARCHAR(60) NOT NULL REFERENCES risk_rules(code),
    severity         VARCHAR(20) NOT NULL DEFAULT 'medium'
                     CHECK (severity IN ('low','medium','high','critical')),
    subject_type     VARCHAR(30) NOT NULL DEFAULT 'user',
    subject_id       UUID,
    -- dedupe_key is "<rule>:<user>:<window>" — one LIVE alert per pattern
    -- occurrence; once resolved the same pattern raises a fresh alert.
    dedupe_key       TEXT NOT NULL,
    details          JSONB NOT NULL DEFAULT '{}',
    status           VARCHAR(20) NOT NULL DEFAULT 'open'
                     CHECK (status IN ('open','in_review','escalated','cleared')),
    assignee_id      UUID REFERENCES users(id),
    resolution_notes TEXT,
    resolved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Exactly one open/in_review alert per pattern window (8.1 acceptance:
-- "seeding a rule breach creates exactly one alert").
CREATE UNIQUE INDEX IF NOT EXISTS uq_risk_alerts_live
    ON risk_alerts (dedupe_key)
    WHERE status IN ('open','in_review');
CREATE INDEX IF NOT EXISTS idx_risk_alerts_status ON risk_alerts (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_risk_alerts_user   ON risk_alerts (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_risk_alerts_rule   ON risk_alerts (rule_code, created_at DESC);

CREATE TABLE IF NOT EXISTS screening_results (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    kind        VARCHAR(20) NOT NULL DEFAULT 'initial'
                CHECK (kind IN ('initial','rescreen','ad_hoc')),
    source      VARCHAR(120) NOT NULL,
    result      VARCHAR(20) NOT NULL CHECK (result IN ('clear','hit')),
    notes       TEXT,
    screened_by UUID REFERENCES users(id),
    screened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- A 'hit' immediately raises an escalated alert; the row points at it.
    alert_id    UUID REFERENCES risk_alerts(id)
);
CREATE INDEX IF NOT EXISTS idx_screening_user ON screening_results (user_id, screened_at DESC);
CREATE INDEX IF NOT EXISTS idx_screening_result ON screening_results (result, screened_at DESC);

CREATE TABLE IF NOT EXISTS str_reports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id    UUID UNIQUE NOT NULL REFERENCES risk_alerts(id),
    narrative   TEXT NOT NULL DEFAULT '',
    prepared_by UUID REFERENCES users(id),
    status      VARCHAR(20) NOT NULL DEFAULT 'draft'
                CHECK (status IN ('draft','filed')),
    -- 24 hours from escalation: STR goes to NFIU within 24h of determination.
    deadline_at TIMESTAMPTZ NOT NULL,
    filed_at    TIMESTAMPTZ,
    goaml_ref   VARCHAR(120),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_str_status ON str_reports (status, deadline_at);

CREATE TABLE IF NOT EXISTS ctr_reports (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id),
    window_date    DATE NOT NULL,
    amount_kobo    BIGINT NOT NULL,
    txn_count      INTEGER NOT NULL DEFAULT 1,
    threshold_kobo BIGINT NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'open'
                   CHECK (status IN ('open','filed')),
    -- 7 days from the transaction day (CTR filing window).
    deadline_at    TIMESTAMPTZ NOT NULL,
    filed_at       TIMESTAMPTZ,
    notes          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Idempotent daily scan: one row per user per threshold per day.
    UNIQUE (user_id, window_date, threshold_kobo)
);
CREATE INDEX IF NOT EXISTS idx_ctr_status ON ctr_reports (status, window_date DESC);

-- Manual risk flag (8.4): freeze-review marker. Blocked withdrawals read it.
ALTER TABLE users ADD COLUMN IF NOT EXISTS risk_flagged     BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS risk_flag_reason TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS risk_flagged_at  TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS risk_flagged_by  UUID REFERENCES users(id);

COMMENT ON COLUMN users.risk_flagged IS
    'Manual AML freeze: when true withdrawals are blocked pending review (Phase 8.4). Set by admin flag or a screening hit.';

-- ── Seed rules (8.2 v1) ─────────────────────────────────────────────────────
-- Params are data, not code: the admin UI edits them through
-- PATCH /admin/risk/rules/:code. Defaults below match the Phase 8 plan.
INSERT INTO risk_rules (code, name, description, mode, enabled, params) VALUES
    ('withdraw_velocity_24h',
     'Withdrawal velocity (24h)',
     'More than 5 withdrawals in any rolling 24 hours.',
     'blocking', TRUE,
     '{"max_count":5,"window_hours":24}'),

    ('risk_flagged_user',
     'Risk-flagged user withdrawal',
     'Withdrawals by a user flagged for risk review (manual flag or screening hit) are blocked until cleared.',
     'blocking', TRUE,
     '{}'),

    ('new_account_high_value_withdrawal',
     'New-account high-value withdrawal',
     'Withdrawal within 7 days of signup at or above the high-value threshold — classic mule/drain pattern.',
     'blocking', TRUE,
     '{"age_days":7,"amount_kobo":100000000}'),

    ('deposit_velocity_10m',
     'Deposit velocity (10min)',
     'More than 3 deposits in any rolling 10 minutes.',
     'monitoring', TRUE,
     '{"max_count":3,"window_minutes":10}'),

    ('deposit_cap_24h',
     'Cumulative deposit cap (24h)',
     'Deposits totalling more than the cap in any rolling 24 hours.',
     'monitoring', TRUE,
     '{"cap_kobo":500000000}'),

    ('structuring_7d',
     'Structuring near limit (7d)',
     '3+ transactions in 7 days each at 90–99% of a reporting threshold — smurfing pattern.',
     'monitoring', TRUE,
     '{"min_count":3,"window_days":7,"lower_pct":90,"upper_pct":99,"reference_kobo":500000000}'),

    ('round_amount_cycling',
     'Round-amount cycling (7d)',
     'The same round amount moved 3+ times in 7 days.',
     'monitoring', TRUE,
     '{"min_count":3,"window_days":7,"granularity_kobo":100000000}'),

    ('rapid_in_out',
     'Rapid in-and-out (15min)',
     'Deposit followed by a withdrawal within 15 minutes — funds pass through rather than invest.',
     'monitoring', TRUE,
     '{"minutes":15}'),

    ('duplicate_account_funding',
     'Duplicate account funding',
     'One funding source (Paystack customer email) moneying two or more distinct wallets in 7 days.',
     'monitoring', TRUE,
     '{"min_users":2,"window_days":7}'),

    ('kyc_name_mismatch',
     'KYC name vs payout account mismatch',
     'Bank-verified withdrawal account name does not match the user''s KYC name beyond tolerance. Evaluated at withdrawal initiation (the bank name only exists there); alerts, never blocks.',
     'monitoring', TRUE,
     '{}'),

    ('screening_hit',
     'Sanctions/PEP screening hit',
     'Raised directly by the manual screening flow when an admin records a HIT: freeze-review, escalated alert, STR drafted. Not scan-evaluated.',
     'monitoring', TRUE,
     '{}')

ON CONFLICT (code) DO NOTHING;

-- Backfill: Phase 6.3 attestations already recorded on kyc_submissions become
-- the first screening_results rows, so the re-screen reminder has a baseline.
INSERT INTO screening_results (user_id, kind, source, result, notes, screened_by, screened_at)
SELECT k.user_id,
       'initial',
       COALESCE(k.screening_source, 'kyc_submission backfill'),
       k.screening_result,
       k.screening_notes,
       k.screened_by,
       COALESCE(k.screening_at, k.updated_at)
FROM kyc_submissions k
WHERE k.screening_result IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM screening_results s
      WHERE s.user_id = k.user_id AND s.screened_at = COALESCE(k.screening_at, k.updated_at)
  );