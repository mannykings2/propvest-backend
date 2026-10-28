-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000030 (down): KYC review screening columns
-- ═══════════════════════════════════════════════════════════════════════════
-- bvn/nin stay TEXT on purpose: narrowing them back to VARCHAR(11) would
-- truncate encrypted values if any rows exist. Harmless to leave widened.
ALTER TABLE kyc_submissions
    DROP COLUMN IF EXISTS screening_source,
    DROP COLUMN IF EXISTS screening_result,
    DROP COLUMN IF EXISTS screening_notes,
    DROP COLUMN IF EXISTS screening_at,
    DROP COLUMN IF EXISTS screened_by,
    DROP COLUMN IF EXISTS evidence;
