-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000033: live-alert dedupe index must include 'escalated'
--
-- The 000032 index freed the dedupe slot for open/in_review only, but an
-- ESCALATED alert (screening hit, analyst escalation) is still live — its STR
-- is being prepared. Without it in the predicate, a repeat hit would open a
-- second alert and a second STR draft for the same pattern window.
--
-- Also: the repository's insert uses bare `ON CONFLICT DO NOTHING` (gorm
-- cannot emit the index predicate after DO NOTHING), so the index predicate
-- itself is what decides which rows collide. Rewriting it here on one
-- database-agnostic statement.
-- ═══════════════════════════════════════════════════════════════════════════

DROP INDEX IF EXISTS uq_risk_alerts_live;

CREATE UNIQUE INDEX uq_risk_alerts_live
    ON risk_alerts (dedupe_key)
    WHERE status IN ('open', 'in_review', 'escalated');

COMMENT ON INDEX uq_risk_alerts_live IS
    'One LIVE alert per pattern window (rule:user:window). Cleared alerts release the slot so a recurring pattern re-raises.';