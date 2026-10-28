-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000023: Extend Audit Logs for Admin Actions
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Extend existing audit_logs table (from migration 000011) with
--          additional fields for comprehensive admin action tracking
-- Author: Milestone 7 - Administration Module
-- Date: 2026-10-06
-- Reference: MILESTONE_7_ADMINISTRATION_TASKS.md (Phase 1)
-- ═══════════════════════════════════════════════════════════════════════════
--
-- CONTEXT:
-- Migration 000011 created basic audit_logs table with:
--   - id, actor_id, action, target_type, target_id
--   - ip_address, user_agent, request_id, metadata, created_at
--
-- This migration adds fields required for admin action auditing:
--   - actor_type: Distinguish admin, user, system actions
--   - actor_role: Role snapshot at time of action (admin, super_admin)
--   - old_values: Before-state snapshot (redacted, allowlisted)
--   - new_values: After-state snapshot (redacted, allowlisted)
--   - reason: Required for consequential actions (suspension, role change)
--   - idempotency_key: For financial/high-risk operations
--   - description: Human-readable action summary
--
-- ATOMICITY GUARANTEE:
-- Audit logs are written in the SAME database transaction as the business
-- change. If the business operation fails, the audit log is rolled back.
-- If the audit log insert fails, the business operation is rolled back.
-- This ensures audit integrity - no lost audit records, no false claims.
-- ═══════════════════════════════════════════════════════════════════════════

-- ───────────────────────────────────────────────────────────────────────────
-- Add new columns to existing audit_logs table
-- ───────────────────────────────────────────────────────────────────────────

-- ActorType distinguishes who performed the action (admin vs user vs system)
-- Used for filtering audit logs by actor category
ALTER TABLE audit_logs
ADD COLUMN IF NOT EXISTS actor_type VARCHAR(32) NOT NULL DEFAULT 'user'
    CHECK (actor_type IN ('admin', 'user', 'system'));

-- ActorRole is a snapshot of the user's role at the time of the action
-- Critical for investigating privilege escalation or role abuse
-- Nullable because system actions have no role
ALTER TABLE audit_logs
ADD COLUMN IF NOT EXISTS actor_role VARCHAR(32) NULL;

-- OldValues stores the before-state as JSONB
-- Redacted/allowlisted to prevent storing passwords, tokens, secrets
-- Example: {"status": "active", "role": "user"}
ALTER TABLE audit_logs
ADD COLUMN IF NOT EXISTS old_values JSONB NULL;

-- NewValues stores the after-state as JSONB
-- Redacted/allowlisted to prevent storing passwords, tokens, secrets
-- Example: {"status": "suspended", "role": "admin"}
ALTER TABLE audit_logs
ADD COLUMN IF NOT EXISTS new_values JSONB NULL;

-- Reason is required for consequential admin actions
-- Examples: "Policy violation", "Requested by user", "Fraud detected"
-- Nullable for routine/read-only actions that don't need justification
ALTER TABLE audit_logs
ADD COLUMN IF NOT EXISTS reason TEXT NULL;

-- IdempotencyKey ensures financial/high-risk operations are not duplicated
-- Unique per actor to prevent concurrent duplicate requests
-- Example: "withdraw-approval-550e8400-e29b-41d4-a716-446655440000"
ALTER TABLE audit_logs
ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(128) NULL;

-- Description is a human-readable summary of the action
-- Used for audit log displays and reports
-- Example: "Admin suspended user account for policy violation"
ALTER TABLE audit_logs
ADD COLUMN IF NOT EXISTS description TEXT NULL;

-- ───────────────────────────────────────────────────────────────────────────
-- Update existing rows to have actor_type (backward compatibility)
-- ───────────────────────────────────────────────────────────────────────────

-- Set default actor_type for existing audit logs
-- Assume 'user' for historical records (safe default)
UPDATE audit_logs
SET actor_type = 'user'
WHERE actor_type IS NULL;

-- ───────────────────────────────────────────────────────────────────────────
-- Indexes for performance
-- ───────────────────────────────────────────────────────────────────────────

-- Index on actor_type for filtering admin vs user actions
-- Partial index excludes system actions (less common)
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_type_created
    ON audit_logs(actor_type, created_at DESC)
    WHERE actor_type IN ('admin', 'user');

-- Unique constraint on (actor_id, idempotency_key) to prevent duplicate operations
-- Partial index only applies when idempotency_key is present
-- This prevents an admin from accidentally performing the same high-risk action twice
CREATE UNIQUE INDEX IF NOT EXISTS uq_audit_logs_actor_idempotency
    ON audit_logs(actor_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Index on actor_role for investigating role-based actions
-- Useful for queries like "show all actions performed by super_admins"
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_role
    ON audit_logs(actor_role)
    WHERE actor_role IS NOT NULL;

-- GIN index on old_values for JSONB queries (optional, add if needed)
-- Allows efficient queries like: WHERE old_values @> '{"status": "active"}'
-- CREATE INDEX IF NOT EXISTS idx_audit_logs_old_values ON audit_logs USING GIN (old_values);

-- GIN index on new_values for JSONB queries (optional, add if needed)
-- CREATE INDEX IF NOT EXISTS idx_audit_logs_new_values ON audit_logs USING GIN (new_values);

-- ───────────────────────────────────────────────────────────────────────────
-- Comments (documentation in the database)
-- ───────────────────────────────────────────────────────────────────────────

COMMENT ON COLUMN audit_logs.actor_type IS
    'Type of actor: admin (admin user action), user (regular user action), system (automated action)';

COMMENT ON COLUMN audit_logs.actor_role IS
    'Snapshot of actor''s role at action time (admin, super_admin, etc.). Used for privilege escalation investigation.';

COMMENT ON COLUMN audit_logs.old_values IS
    'Before-state snapshot as JSONB. Redacted/allowlisted - does NOT contain passwords, tokens, or secrets.';

COMMENT ON COLUMN audit_logs.new_values IS
    'After-state snapshot as JSONB. Redacted/allowlisted - does NOT contain passwords, tokens, or secrets.';

COMMENT ON COLUMN audit_logs.reason IS
    'Human-readable justification for consequential actions (suspension, role change, deletion). Required for high-risk operations.';

COMMENT ON COLUMN audit_logs.idempotency_key IS
    'Unique key for idempotent operations. Prevents duplicate financial/high-risk actions. Unique per actor.';

COMMENT ON COLUMN audit_logs.description IS
    'Human-readable action summary for audit log displays and reports.';

-- ═══════════════════════════════════════════════════════════════════════════
-- Usage Examples
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Example 1: User suspension with reason
--   INSERT INTO audit_logs (
--       actor_id, actor_type, actor_role, action,
--       target_type, target_id,
--       old_values, new_values, reason,
--       ip_address, user_agent, request_id, description
--   ) VALUES (
--       '550e8400-e29b-41d4-a716-446655440000',  -- Admin user ID
--       'admin',
--       'admin',
--       'user.suspended',
--       'user',
--       '660e8400-e29b-41d4-a716-446655440000',  -- Suspended user ID
--       '{"status": "active", "is_active": true}'::jsonb,
--       '{"status": "suspended", "is_active": false}'::jsonb,
--       'Policy violation: Multiple failed payment attempts',
--       '192.168.1.100',
--       'Mozilla/5.0...',
--       'req-abc123',
--       'Admin suspended user account for policy violation'
--   );
--
-- Example 2: Role change with idempotency
--   INSERT INTO audit_logs (
--       actor_id, actor_type, actor_role, action,
--       target_type, target_id,
--       old_values, new_values, reason, idempotency_key,
--       description
--   ) VALUES (
--       '550e8400-e29b-41d4-a716-446655440000',  -- Super admin ID
--       'admin',
--       'super_admin',
--       'user.role_changed',
--       'user',
--       '770e8400-e29b-41d4-a716-446655440000',  -- Target user ID
--       '{"role": "user"}'::jsonb,
--       '{"role": "admin"}'::jsonb,
--       'Promoted to admin role per approval ticket ADMIN-1234',
--       'role-change-770e8400-20261006',  -- Idempotency key
--       'Super admin promoted user to admin role'
--   );
--
-- Example 3: Query audit logs by admin actions in date range
--   SELECT 
--       created_at,
--       actor_role,
--       action,
--       target_type,
--       reason,
--       description
--   FROM audit_logs
--   WHERE actor_type = 'admin'
--     AND created_at BETWEEN '2026-10-01' AND '2026-10-31'
--   ORDER BY created_at DESC;
--
-- Example 4: Query high-risk actions (those with idempotency keys)
--   SELECT 
--       created_at,
--       actor_role,
--       action,
--       target_type,
--       idempotency_key,
--       reason
--   FROM audit_logs
--   WHERE idempotency_key IS NOT NULL
--   ORDER BY created_at DESC;
--
-- ═══════════════════════════════════════════════════════════════════════════
-- Verification
-- ═══════════════════════════════════════════════════════════════════════════

DO $$
DECLARE
    new_columns_count INTEGER;
    new_indexes_count INTEGER;
BEGIN
    -- Count new columns added
    SELECT COUNT(*) INTO new_columns_count
    FROM information_schema.columns
    WHERE table_name = 'audit_logs'
      AND column_name IN (
          'actor_type', 'actor_role', 'old_values', 'new_values',
          'reason', 'idempotency_key', 'description'
      );
    
    -- Count new indexes created
    SELECT COUNT(*) INTO new_indexes_count
    FROM pg_indexes
    WHERE tablename = 'audit_logs'
      AND indexname IN (
          'idx_audit_logs_actor_type_created',
          'uq_audit_logs_actor_idempotency',
          'idx_audit_logs_actor_role'
      );
    
    RAISE NOTICE 'Migration 000023 complete: Added % columns, created % indexes',
        new_columns_count, new_indexes_count;
    
    IF new_columns_count < 7 THEN
        RAISE WARNING 'Expected 7 new columns, found only %. Check migration execution.', new_columns_count;
    END IF;
    
    IF new_indexes_count < 3 THEN
        RAISE WARNING 'Expected 3 new indexes, found only %. Check migration execution.', new_indexes_count;
    END IF;
END $$;

-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
