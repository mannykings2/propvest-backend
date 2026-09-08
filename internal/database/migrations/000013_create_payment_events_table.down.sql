-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000013 Rollback: Drop payment_events table
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop indexes (automatically dropped with table, but explicit for clarity)
DROP INDEX IF EXISTS idx_payment_events_reconciliation;
DROP INDEX IF EXISTS idx_payment_events_event_type;
DROP INDEX IF EXISTS idx_payment_events_request_id;
DROP INDEX IF EXISTS idx_payment_events_payment_ref;
DROP INDEX IF EXISTS idx_payment_events_received_at;
DROP INDEX IF EXISTS idx_payment_events_status;

-- Drop table (cascade will drop constraints automatically)
DROP TABLE IF EXISTS payment_events;
