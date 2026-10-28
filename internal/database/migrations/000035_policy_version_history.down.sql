-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000035 (down): restore the single-row-per-policy constraint
-- ═══════════════════════════════════════════════════════════════════════════
-- Fails (correctly) if more than one version row per policy exists.
ALTER TABLE policy_versions
    ADD CONSTRAINT policy_versions_policy_key UNIQUE (policy);