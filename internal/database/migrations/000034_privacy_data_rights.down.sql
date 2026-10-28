-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000034 (down): drop the Phase 9 privacy/data-rights tables.
-- Consent evidence and deletion history are legal records — dropping them is
-- a last resort, not a routine rollback.
-- ═══════════════════════════════════════════════════════════════════════════
DROP TABLE IF EXISTS account_deletions;
DROP TABLE IF EXISTS data_export_jobs;
DROP TABLE IF EXISTS marketing_suppressions;
DROP TABLE IF EXISTS user_consents;
DROP TABLE IF EXISTS policy_versions;