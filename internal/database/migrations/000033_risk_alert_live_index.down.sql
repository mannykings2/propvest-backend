-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000033 (down): restore the 000032 predicate (open/in_review).
-- ═══════════════════════════════════════════════════════════════════════════
DROP INDEX IF EXISTS uq_risk_alerts_live;

CREATE UNIQUE INDEX uq_risk_alerts_live
    ON risk_alerts (dedupe_key)
    WHERE status IN ('open', 'in_review');