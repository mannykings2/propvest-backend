-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000032 (down): drop the Risk layer.
-- Alerts go first because screening_results and str_reports reference them.
-- ═══════════════════════════════════════════════════════════════════════════
DROP TABLE IF EXISTS ctr_reports;
DROP TABLE IF EXISTS str_reports;
DROP TABLE IF EXISTS screening_results;
DROP TABLE IF EXISTS risk_alerts;
DROP TABLE IF EXISTS risk_rules;

ALTER TABLE users
    DROP COLUMN IF EXISTS risk_flagged,
    DROP COLUMN IF EXISTS risk_flag_reason,
    DROP COLUMN IF EXISTS risk_flagged_at,
    DROP COLUMN IF EXISTS risk_flagged_by;