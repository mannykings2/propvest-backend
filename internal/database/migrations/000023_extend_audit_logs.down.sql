-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000023 ROLLBACK: Remove Audit Log Extensions
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Reverse migration 000023 - remove extended audit log fields
-- Date: 2026-10-06
-- ═══════════════════════════════════════════════════════════════════════════
--
-- WARNING: This rollback will DROP columns that contain audit data.
-- If audit logs have been written with these fields, that data will be LOST.
-- Only run this rollback if:
--   1. Migration 000023 was applied incorrectly and needs to be redone
--   2. Milestone 7 deployment is being fully rolled back
--   3. No production audit data exists in these columns yet
--
-- RECOMMENDATION: Archive audit_logs table before running this rollback.
-- ═══════════════════════════════════════════════════════════════════════════

-- ───────────────────────────────────────────────────────────────────────────
-- Drop indexes (must be dropped before columns)
-- ───────────────────────────────────────────────────────────────────────────

DROP INDEX IF EXISTS idx_audit_logs_actor_type_created;
DROP INDEX IF EXISTS uq_audit_logs_actor_idempotency;
DROP INDEX IF EXISTS idx_audit_logs_actor_role;

-- ───────────────────────────────────────────────────────────────────────────
-- Drop columns
-- ───────────────────────────────────────────────────────────────────────────

ALTER TABLE audit_logs DROP COLUMN IF EXISTS description;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS idempotency_key;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS reason;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS new_values;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS old_values;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor_role;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor_type;

-- ───────────────────────────────────────────────────────────────────────────
-- Verification
-- ───────────────────────────────────────────────────────────────────────────

DO $$
DECLARE
    remaining_columns INTEGER;
BEGIN
    -- Verify extended columns are removed
    SELECT COUNT(*) INTO remaining_columns
    FROM information_schema.columns
    WHERE table_name = 'audit_logs'
      AND column_name IN (
          'actor_type', 'actor_role', 'old_values', 'new_values',
          'reason', 'idempotency_key', 'description'
      );
    
    IF remaining_columns > 0 THEN
        RAISE WARNING 'Migration 000023 rollback incomplete: % extended columns still exist', remaining_columns;
    ELSE
        RAISE NOTICE 'Migration 000023 rollback complete: All extended columns removed';
    END IF;
END $$;

-- ═══════════════════════════════════════════════════════════════════════════
-- END OF ROLLBACK
-- ═══════════════════════════════════════════════════════════════════════════
