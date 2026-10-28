-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000011: notifications, property_documents, audit_logs
-- ═══════════════════════════════════════════════════════════════════════════
-- Three supporting tables introduced together:
--   notifications      - in-app messages (email/SMS handled async by the worker)
--   property_documents - external file URLs attached to a property
--   audit_logs         - append-only security/compliance trail
-- ═══════════════════════════════════════════════════════════════════════════

-- NOTE: notifications is intentionally NOT created here. It is owned by
-- migration 000022_create_notifications, which defines the canonical schema
-- used by the application (VARCHAR(100) type + CHECK constraints on type/
-- title/body). An earlier version of this migration created a conflicting bare
-- table (VARCHAR(64), no constraints); because the later CREATE TABLE in 000022
-- is not IF NOT EXISTS, the collision aborted the chain. The block is removed
-- here so 000022 owns the table.

-- NOTE: property_documents is intentionally NOT created here. It is owned by
-- migration 000018_create_property_documents, which defines the canonical
-- schema used by the PropertyDocument model (document_type, public_id,
-- mime_type, file_size, is_public, ...). An earlier version of this migration
-- created a conflicting bare table (legacy `type` column); because
-- CREATE TABLE IF NOT EXISTS no-ops on a name collision, it shadowed 000018
-- and aborted the chain on the COMMENT ON COLUMN document_type statement.
-- The block is removed here so 000018 owns the table.

-- ── audit_logs (append-only) ────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID,
    action VARCHAR(64) NOT NULL,
    target_type VARCHAR(64),
    target_id UUID,
    ip_address VARCHAR(64),
    user_agent TEXT,
    request_id VARCHAR(64),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);

COMMENT ON TABLE audit_logs IS 'Append-only security/compliance trail (never update/delete)';
