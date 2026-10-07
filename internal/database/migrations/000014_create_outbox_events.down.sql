-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Rollback Outbox Events Table
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000014_create_outbox_events (DOWN)
-- Description: Safely removes the outbox_events table and related objects
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop the table (CASCADE removes dependent objects like indexes and triggers)
DROP TABLE IF EXISTS outbox_events CASCADE;

-- ═══════════════════════════════════════════════════════════════════════════
-- NOTES
-- ═══════════════════════════════════════════════════════════════════════════
-- This rollback is SAFE because:
-- 1. The outbox_events table is independent (no foreign keys from other tables)
-- 2. Dropping it doesn't affect existing business data
-- 3. The application should gracefully handle the missing table (with appropriate
--    error handling in the outbox repository)
--
-- IMPORTANT: If rolling back in production with pending events:
-- - Any pending outbox events will be lost
-- - The system will fall back to the old behavior (direct RabbitMQ publish)
-- - Reconciliation will be the primary safety mechanism again
-- - Consider extracting pending events before rollback if needed
-- ═══════════════════════════════════════════════════════════════════════════
