-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000030: KYC review workflow — screening attestation + encryption
-- (Phase 6, manual admin KYC)
--
-- Three changes, one migration:
--   1. bvn/nin widen to TEXT: the values are now AES-GCM ciphertext
--      ("v1:" + base64), which does not fit VARCHAR(11). The plaintext in
--      existing dev rows is migrated lazily by the repository on next read.
--   2. screening columns: the admin must record a sanctions/PEP check
--      (source, result, who, when) BEFORE approval. NULL result = not yet
--      screened; approval is refused until it says 'clear'.
--   3. evidence jsonb: free-form review evidence (document references,
--      admin notes) attached to the submission row.
-- ═══════════════════════════════════════════════════════════════════════════

ALTER TABLE kyc_submissions ALTER COLUMN bvn TYPE TEXT;
ALTER TABLE kyc_submissions ALTER COLUMN nin TYPE TEXT;

ALTER TABLE kyc_submissions
    ADD COLUMN IF NOT EXISTS screening_source  VARCHAR(120),
    ADD COLUMN IF NOT EXISTS screening_result  VARCHAR(20)
        CHECK (screening_result IN ('clear','hit')),
    ADD COLUMN IF NOT EXISTS screening_notes   TEXT,
    ADD COLUMN IF NOT EXISTS screening_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS screened_by       UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS evidence          JSONB;

COMMENT ON COLUMN kyc_submissions.bvn IS
    'AES-GCM ciphertext ("v1:"+base64). Plaintext exists only in process memory. Never logged; never returned except to admin detail view.';
COMMENT ON COLUMN kyc_submissions.nin IS
    'AES-GCM ciphertext, same handling as BVN.';
COMMENT ON COLUMN kyc_submissions.screening_result IS
    'Admin''s manual sanctions/PEP check: NULL = not screened, clear = approve allowed, hit = approval blocked (reject/escalate).';
