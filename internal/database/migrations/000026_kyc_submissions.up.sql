-- Migration 000026: KYC submissions queue
--
-- Profile.jsx POSTs {bvn, nin, docType} to /api/v1/me/kyc/submit. Previously
-- there was nowhere to put it: users.kyc_status is a single string and admin
-- DecideKYC flipped it with no record of what was submitted.
--
-- One row per user (UNIQUE user_id) — a re-submit updates the existing row,
-- which keeps the queue deduplicated and the audit trail simple.

CREATE TABLE IF NOT EXISTS kyc_submissions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id),
    bvn           VARCHAR(11) NOT NULL,
    nin           VARCHAR(11) NOT NULL,
    doc_type      VARCHAR(60) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','verified','rejected')),
    reason        TEXT,
    submitted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at    TIMESTAMPTZ,
    decided_by    UUID REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id)
);

CREATE INDEX IF NOT EXISTS idx_kyc_submissions_status ON kyc_submissions(status);

COMMENT ON TABLE kyc_submissions IS
    'KYC documents submitted by users, awaiting admin review. One row per user; re-submit updates it.';
COMMENT ON COLUMN kyc_submissions.bvn IS
    'BVN is PII. Never logged, never returned in list endpoints (only bvn_last4). Encryption at rest is a documented follow-up.';
COMMENT ON COLUMN kyc_submissions.nin IS
    'NIN is PII. Same handling as BVN.';
