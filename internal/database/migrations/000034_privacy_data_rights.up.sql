-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000034: Data rights, consent & suppression (Phase 9 — Regulation
-- layer: NDPA 2023 + GAID 2025, FCCPC opt-out baseline)
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Consent must be recordable, versioned and WITHDRAWABLE; marketing must be
-- opt-IN and separable from contract-required processing (KYC/terms); every
-- data-subject right must be implementable — access (export job), erasure
-- (deferred deletion that still respects statutory financial retention);
-- suppressions must be enforceable against bulk sends.

-- ── Policy versions: consent records pin the version they agreed to ────────
CREATE TABLE IF NOT EXISTS policy_versions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    policy       VARCHAR(40) NOT NULL UNIQUE,          -- privacy_policy | terms
    version      VARCHAR(20) NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by   UUID,                                 -- admin who bumped (NULL = seed)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (policy, version)
);

INSERT INTO policy_versions (policy, version) VALUES
    ('privacy_policy', '1.0'),
    ('terms', '1.0')
ON CONFLICT (policy) DO NOTHING;

-- ── User consents: append-only evidence ────────────────────────────────────
-- A withdrawal is a NEW row with granted=false (never an UPDATE) — the
-- timeline of what the user agreed to, when, from where, is the evidence.
CREATE TABLE IF NOT EXISTS user_consents (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose       VARCHAR(60) NOT NULL,                -- privacy_policy | terms | marketing
    granted       BOOLEAN NOT NULL,
    policy_version VARCHAR(20),                        -- NULL for marketing (no version)
    ip_address    VARCHAR(64),
    user_agent    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_consents_user
    ON user_consents (user_id, purpose, created_at DESC);

-- ── Marketing suppression list (NDPA objection-to-direct-marketing right) ──
CREATE TABLE IF NOT EXISTS marketing_suppressions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel    VARCHAR(20) NOT NULL DEFAULT 'email',   -- email | sms
    address    VARCHAR(255) NOT NULL,
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    reason     VARCHAR(40) NOT NULL DEFAULT 'unsubscribe', -- unsubscribe | admin
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (channel, address)
);

-- ── Data export jobs (NDPA access right) ───────────────────────────────────
-- The bundle is assembled asynchronously and kept IN THE DATABASE (no shared
-- filesystem), reachable only by a one-time-ish bearer token whose SHA-256 is
-- stored; the bundle row is wiped when the job expires (24h SLA window).
CREATE TABLE IF NOT EXISTS data_export_jobs (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status             VARCHAR(20) NOT NULL DEFAULT 'queued'
                       CHECK (status IN ('queued', 'ready', 'failed', 'expired')),
    download_token_hash VARCHAR(64),
    bundle             JSONB,
    error              TEXT,
    expires_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_data_export_jobs_user
    ON data_export_jobs (user_id, created_at DESC);

-- ── Account deletion requests (NDPA erasure with statutory exception) ──────
-- 30-day grace in which the user (or an admin) can cancel; on execution the
-- identity is anonymised but financial ledger rows are RETAINED — the legal
-- erasure exception for AML/tax record-keeping.
CREATE TABLE IF NOT EXISTS account_deletions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status        VARCHAR(20) NOT NULL DEFAULT 'requested'
                  CHECK (status IN ('requested', 'cancelled', 'executed')),
    reason        TEXT,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    execute_after TIMESTAMPTZ NOT NULL,
    cancelled_at  TIMESTAMPTZ,
    executed_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- At most one LIVE request per user (history rows stay after execution).
CREATE UNIQUE INDEX IF NOT EXISTS idx_account_deletions_live
    ON account_deletions (user_id) WHERE status = 'requested';