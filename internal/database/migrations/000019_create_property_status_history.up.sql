-- Migration 000019: Create property_status_history table
-- This table provides an audit trail for property lifecycle status changes.
-- Essential for compliance, debugging, and understanding property history.

CREATE TABLE IF NOT EXISTS property_status_history (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Foreign Key to properties
    property_id UUID NOT NULL
        REFERENCES properties(id)
        ON DELETE CASCADE,

    -- Status Transition
    from_status VARCHAR(20),
    to_status VARCHAR(20) NOT NULL,

    -- Audit Information
    changed_by UUID
        REFERENCES users(id),

    reason TEXT,

    -- Timestamp
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- ═══════════════════════════════════════════════════════════════════════════
    -- CONSTRAINTS
    -- ═══════════════════════════════════════════════════════════════════════════

    -- from_status can be NULL for initial status (property creation)
    -- Otherwise must be a valid status
    CONSTRAINT chk_property_status_history_from
        CHECK (
            from_status IS NULL
            OR from_status IN ('draft', 'active', 'funded', 'completed')
        ),

    -- to_status must always be a valid status
    CONSTRAINT chk_property_status_history_to
        CHECK (
            to_status IN ('draft', 'active', 'funded', 'completed')
        )
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

-- Composite index for fetching status history for a property (most recent first)
-- This is the most common query pattern for audit trails
CREATE INDEX IF NOT EXISTS idx_property_status_history_property
    ON property_status_history(property_id, created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════════
-- COMMENTS
-- ═══════════════════════════════════════════════════════════════════════════

COMMENT ON TABLE property_status_history IS
    'Audit trail for property lifecycle status changes. '
    'Records who changed the status, when, and why. '
    'Essential for compliance and debugging.';

COMMENT ON COLUMN property_status_history.from_status IS
    'Previous status. NULL for initial status (property creation). '
    'Example transitions: NULL→draft, draft→active, active→funded, funded→completed';

COMMENT ON COLUMN property_status_history.to_status IS
    'New status after the change. Must be one of: draft, active, funded, completed';

COMMENT ON COLUMN property_status_history.changed_by IS
    'Admin user who made the status change. '
    'NULL for system-initiated changes (e.g., automatic funding status update). '
    'References users.id.';

COMMENT ON COLUMN property_status_history.reason IS
    'Optional reason for the status change. '
    'Example: "Property fully funded", "Admin published property", etc.';
