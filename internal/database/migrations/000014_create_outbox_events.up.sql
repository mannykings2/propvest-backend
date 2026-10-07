-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Outbox Events Table Migration
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000014_create_outbox_events
-- Description: Creates transactional outbox table for reliable async messaging
-- Author: Outbox Pattern Implementation
-- Date: 2026-10-06
-- Reference: IMPLEMENTATION_PLAN.md, AUDIT_SUMMARY.md
-- ═══════════════════════════════════════════════════════════════════════════
--
-- PURPOSE:
-- The outbox pattern ensures that business state changes and corresponding
-- async events are committed atomically in a single database transaction.
-- This eliminates the reliability gap where messages could be lost if RabbitMQ
-- is unavailable or the API crashes after committing database changes.
--
-- FLOW:
-- 1. Business operation (e.g., withdrawal) creates outbox event in same txn
-- 2. Both business state + outbox event commit atomically
-- 3. Separate dispatcher process polls outbox table
-- 4. Dispatcher publishes events to RabbitMQ with retry logic
-- 5. Successfully published events marked as 'published'
--
-- This guarantees at-least-once delivery of async events.
-- ═══════════════════════════════════════════════════════════════════════════

-- ───────────────────────────────────────────────────────────────────────────
-- OUTBOX_EVENTS TABLE
-- ───────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS outbox_events (
    -- Primary identifier
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Event classification
    event_type VARCHAR(100) NOT NULL,        -- e.g., 'withdrawal.process', 'email.dispatch'
    aggregate_type VARCHAR(100) NOT NULL,    -- e.g., 'wallet_transaction', 'user'
    aggregate_id UUID NOT NULL,              -- ID of the business entity
    
    -- Event payload (the message to publish)
    payload JSONB NOT NULL,
    
    -- Processing state
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    
    -- Scheduling and claiming
    available_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMP WITH TIME ZONE,
    claimed_by VARCHAR(255),
    
    -- Success tracking
    published_at TIMESTAMP WITH TIME ZONE,
    
    -- Failure tracking
    last_error TEXT,
    
    -- Audit timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT chk_outbox_status CHECK (
        status IN ('pending', 'claimed', 'published', 'failed')
    ),
    CONSTRAINT chk_outbox_attempts CHECK (attempts >= 0)
);

-- ───────────────────────────────────────────────────────────────────────────
-- INDEXES
-- ───────────────────────────────────────────────────────────────────────────

-- Most important index: Used by dispatcher to claim pending events
-- Uses partial index to only index relevant rows (status IN pending/claimed)
-- Ordered by available_at for delayed retry support
CREATE INDEX IF NOT EXISTS idx_outbox_events_claim
    ON outbox_events(status, available_at)
    WHERE status IN ('pending', 'claimed');

-- Query events by business entity (useful for debugging/auditing)
CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
    ON outbox_events(aggregate_type, aggregate_id);

-- Chronological queries and monitoring
CREATE INDEX IF NOT EXISTS idx_outbox_events_created_at
    ON outbox_events(created_at DESC);

-- Query by event type (useful for monitoring specific event types)
CREATE INDEX IF NOT EXISTS idx_outbox_events_type
    ON outbox_events(event_type);

-- ───────────────────────────────────────────────────────────────────────────
-- TRIGGERS
-- ───────────────────────────────────────────────────────────────────────────

-- Automatically update updated_at column on every UPDATE
CREATE TRIGGER update_outbox_events_updated_at
    BEFORE UPDATE ON outbox_events
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ───────────────────────────────────────────────────────────────────────────
-- COMMENTS (Documentation in the database)
-- ───────────────────────────────────────────────────────────────────────────

COMMENT ON TABLE outbox_events IS 'Transactional outbox for reliable async message delivery. Events created atomically with business operations, then published by dispatcher.';

COMMENT ON COLUMN outbox_events.event_type IS 'Type of event (e.g., withdrawal.process, email.dispatch)';
COMMENT ON COLUMN outbox_events.aggregate_type IS 'Type of business entity this event relates to';
COMMENT ON COLUMN outbox_events.aggregate_id IS 'ID of the specific business entity';
COMMENT ON COLUMN outbox_events.payload IS 'JSON message payload to publish to message queue';
COMMENT ON COLUMN outbox_events.status IS 'Processing status: pending (not yet claimed), claimed (being processed), published (successfully delivered), failed (max retries exceeded)';
COMMENT ON COLUMN outbox_events.attempts IS 'Number of publish attempts (for retry tracking)';
COMMENT ON COLUMN outbox_events.available_at IS 'When event becomes eligible for processing (supports delayed retry with exponential backoff)';
COMMENT ON COLUMN outbox_events.claimed_at IS 'When event was claimed by dispatcher instance';
COMMENT ON COLUMN outbox_events.claimed_by IS 'Dispatcher instance ID that claimed this event (for debugging)';
COMMENT ON COLUMN outbox_events.published_at IS 'When event was successfully published to message queue';
COMMENT ON COLUMN outbox_events.last_error IS 'Most recent error message from failed publish attempt';

-- ═══════════════════════════════════════════════════════════════════════════
-- USAGE EXAMPLE
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Creating an outbox event (within a business transaction):
--
-- BEGIN;
--   -- Business operation
--   INSERT INTO wallet_transactions (...) VALUES (...);
--   
--   -- Outbox event (atomic with business operation)
--   INSERT INTO outbox_events (
--       event_type, aggregate_type, aggregate_id, payload
--   ) VALUES (
--       'withdrawal.process',
--       'wallet_transaction',
--       '550e8400-e29b-41d4-a716-446655440000',
--       '{"transaction_id": "...", "amount": 50000, ...}'::jsonb
--   );
-- COMMIT;
--
-- Claiming events (by dispatcher):
--
-- UPDATE outbox_events
-- SET status = 'claimed',
--     claimed_at = NOW(),
--     claimed_by = 'dispatcher-instance-1',
--     updated_at = NOW()
-- WHERE id IN (
--     SELECT id
--     FROM outbox_events
--     WHERE status = 'pending'
--       AND available_at <= NOW()
--     ORDER BY created_at
--     LIMIT 10
--     FOR UPDATE SKIP LOCKED
-- )
-- RETURNING *;
--
-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
