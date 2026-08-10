-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000011: notifications, property_documents, audit_logs
-- ═══════════════════════════════════════════════════════════════════════════
-- Three supporting tables introduced together:
--   notifications      - in-app messages (email/SMS handled async by the worker)
--   property_documents - external file URLs attached to a property
--   audit_logs         - append-only security/compliance trail
-- ═══════════════════════════════════════════════════════════════════════════

-- ── notifications ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    type VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMP WITH TIME ZONE,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_notifications_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(read);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at DESC);

-- ── property_documents ──────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS property_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id UUID NOT NULL,
    type VARCHAR(32) NOT NULL,
    url TEXT NOT NULL,
    name VARCHAR(255),
    uploaded_by UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_property_documents_property FOREIGN KEY (property_id) REFERENCES properties(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_property_documents_property_id ON property_documents(property_id);

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

COMMENT ON TABLE notifications IS 'In-app notifications; durable copy + unread badge source';
COMMENT ON TABLE property_documents IS 'External file URLs (Cloudinary) attached to a property';
COMMENT ON TABLE audit_logs IS 'Append-only security/compliance trail (never update/delete)';
