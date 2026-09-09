-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000013: Create payment_events table for RabbitMQ queue system
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Implement event store for idempotent webhook processing and
--          asynchronous message queue architecture
--
-- WHY EVENT STORE?
-- Payment webhooks from Paystack/Flutterwave can arrive multiple times due to:
--   1. Network retries when webhook response is delayed
--   2. Provider's internal retry logic
--   3. Our own reconciliation system republishing events
--
-- Solution: Store every webhook event with unique constraint to prevent duplicates
--
-- Event Lifecycle:
--   1. Webhook arrives → Insert with status='pending'
--   2. Queue consumer picks up → Update status='processing'
--   3. Business logic completes → Update status='completed'
--   4. Processing fails permanently → Update status='failed'
--
-- This enables:
--   - Exactly-once processing semantics
--   - Audit trail for debugging
--   - Reconciliation of missed/stuck events
--   - Distributed tracing across services
-- ═══════════════════════════════════════════════════════════════════════════

-- Create payment_events table
CREATE TABLE payment_events (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- ═══════════════════════════════════════════════════════════════════════
    -- IDEMPOTENCY KEY (prevents duplicate processing)
    -- ═══════════════════════════════════════════════════════════════════════
    -- Combination of (payment_reference, event_type, provider) is unique
    -- 
    -- Example:
    --   payment_reference: "PAYSTACK_12345"
    --   event_type: "deposit.confirmed"
    --   provider: "paystack"
    --
    -- If same webhook arrives twice, INSERT will fail with unique violation
    -- ═══════════════════════════════════════════════════════════════════════
    payment_reference     VARCHAR(255) NOT NULL,
    event_type           VARCHAR(100) NOT NULL,  -- deposit.confirmed, withdrawal.requested
    provider             VARCHAR(50) NOT NULL,   -- paystack, flutterwave
    
    -- ═══════════════════════════════════════════════════════════════════════
    -- EVENT LIFECYCLE (tracks processing status)
    -- ═══════════════════════════════════════════════════════════════════════
    -- Status values:
    --   pending: Event stored, waiting for queue consumer
    --   processing: Consumer picked up, executing business logic
    --   completed: Successfully processed (wallet updated, etc.)
    --   failed: Failed after all retries, sent to DLQ
    -- ═══════════════════════════════════════════════════════════════════════
    status               VARCHAR(20) NOT NULL DEFAULT 'pending',
    received_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    processing_started_at TIMESTAMP,
    processed_at         TIMESTAMP,
    
    -- ═══════════════════════════════════════════════════════════════════════
    -- PAYLOAD AND CONTEXT (preserve original webhook data)
    -- ═══════════════════════════════════════════════════════════════════════
    -- raw_payload: Complete webhook JSON from Paystack/Flutterwave
    -- metadata: Additional context (request headers, IP address, etc.)
    -- ═══════════════════════════════════════════════════════════════════════
    raw_payload          JSONB NOT NULL,
    metadata             JSONB,
    
    -- ═══════════════════════════════════════════════════════════════════════
    -- ERROR TRACKING (debugging and retry logic)
    -- ═══════════════════════════════════════════════════════════════════════
    -- failure_reason: Error message/stacktrace when processing fails
    -- retry_count: How many times consumer attempted to process
    -- ═══════════════════════════════════════════════════════════════════════
    failure_reason       TEXT,
    retry_count          INT NOT NULL DEFAULT 0,
    
    -- ═══════════════════════════════════════════════════════════════════════
    -- DISTRIBUTED TRACING (correlate events across services)
    -- ═══════════════════════════════════════════════════════════════════════
    -- request_id: Unique ID for entire request flow (webhook → queue → consumer)
    -- message_id: RabbitMQ message identifier
    -- ═══════════════════════════════════════════════════════════════════════
    request_id           VARCHAR(100),
    message_id           VARCHAR(100),
    
    -- ═══════════════════════════════════════════════════════════════════════
    -- AUDIT TIMESTAMPS
    -- ═══════════════════════════════════════════════════════════════════════
    created_at           TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- ═══════════════════════════════════════════════════════════════════════
    -- CONSTRAINTS
    -- ═══════════════════════════════════════════════════════════════════════
    CONSTRAINT unique_payment_event 
        UNIQUE (payment_reference, event_type, provider),
    
    CONSTRAINT chk_payment_events_status 
        CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    
    CONSTRAINT chk_payment_events_retry_count 
        CHECK (retry_count >= 0)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES (optimize common query patterns)
-- ═══════════════════════════════════════════════════════════════════════════

-- Index for filtering by status (consumer queries)
-- Used by: SELECT * FROM payment_events WHERE status = 'pending'
CREATE INDEX idx_payment_events_status 
    ON payment_events(status);

-- Index for time-based queries (monitoring dashboards)
-- Used by: SELECT * FROM payment_events WHERE received_at > NOW() - INTERVAL '1 hour'
CREATE INDEX idx_payment_events_received_at 
    ON payment_events(received_at DESC);

-- Index for lookup by payment reference (customer support, debugging)
-- Used by: SELECT * FROM payment_events WHERE payment_reference = 'PAYSTACK_12345'
CREATE INDEX idx_payment_events_payment_ref 
    ON payment_events(payment_reference);

-- Index for distributed tracing (correlate logs across services)
-- Used by: SELECT * FROM payment_events WHERE request_id = 'req-abc-123'
CREATE INDEX idx_payment_events_request_id 
    ON payment_events(request_id);

-- Index for event type filtering (analytics, reporting)
-- Used by: SELECT COUNT(*) FROM payment_events WHERE event_type = 'deposit.confirmed'
CREATE INDEX idx_payment_events_event_type 
    ON payment_events(event_type);

-- ═══════════════════════════════════════════════════════════════════════════
-- RECONCILIATION INDEX (critical for detecting stuck/missed events)
-- ═══════════════════════════════════════════════════════════════════════════
-- Partial index for reconciliation worker queries
-- 
-- Reconciliation worker runs every 5 minutes looking for:
--   1. Events stuck in 'pending' (RabbitMQ failed to deliver)
--   2. Events stuck in 'processing' (consumer crashed mid-processing)
--
-- Query pattern:
--   SELECT * FROM payment_events 
--   WHERE status IN ('pending', 'processing') 
--     AND received_at < NOW() - INTERVAL '10 minutes'
--   LIMIT 100
--
-- Partial index only stores rows where status is pending or processing
-- This keeps index small and queries fast
-- ═══════════════════════════════════════════════════════════════════════════
CREATE INDEX idx_payment_events_reconciliation 
    ON payment_events(status, received_at) 
    WHERE status IN ('pending', 'processing');

-- ═══════════════════════════════════════════════════════════════════════════
-- COMMENTS (documentation for future developers)
-- ═══════════════════════════════════════════════════════════════════════════
COMMENT ON TABLE payment_events IS 
    'Event store for idempotent webhook processing. Each webhook event is stored exactly once using (payment_reference, event_type, provider) as idempotency key.';

COMMENT ON COLUMN payment_events.payment_reference IS 
    'Unique payment identifier from provider (e.g., Paystack reference)';

COMMENT ON COLUMN payment_events.event_type IS 
    'Type of payment event: deposit.confirmed, withdrawal.requested, etc.';

COMMENT ON COLUMN payment_events.provider IS 
    'Payment provider: paystack, flutterwave';

COMMENT ON COLUMN payment_events.status IS 
    'Processing status: pending (queued), processing (consumer working), completed (done), failed (sent to DLQ)';

COMMENT ON COLUMN payment_events.raw_payload IS 
    'Complete webhook JSON payload from payment provider for debugging and reprocessing';

COMMENT ON COLUMN payment_events.metadata IS 
    'Additional context: request headers, IP address, webhook signature verification details';

COMMENT ON COLUMN payment_events.failure_reason IS 
    'Error message and stacktrace when processing fails permanently';

COMMENT ON COLUMN payment_events.retry_count IS 
    'Number of times queue consumer attempted to process this event';

COMMENT ON COLUMN payment_events.request_id IS 
    'Distributed tracing ID to correlate logs across webhook handler, queue, and consumers';

COMMENT ON COLUMN payment_events.message_id IS 
    'RabbitMQ message identifier for queue debugging';

COMMENT ON CONSTRAINT unique_payment_event ON payment_events IS 
    'Idempotency constraint: prevents duplicate processing of same webhook event';

-- ═══════════════════════════════════════════════════════════════════════════
-- USAGE EXAMPLES
-- ═══════════════════════════════════════════════════════════════════════════
-- 
-- 1. Webhook handler receives event (idempotent insert):
--    INSERT INTO payment_events (payment_reference, event_type, provider, raw_payload)
--    VALUES ('PAYSTACK_12345', 'deposit.confirmed', 'paystack', '{"amount": 50000}')
--    ON CONFLICT (payment_reference, event_type, provider) DO NOTHING;
--    -- Returns 0 rows if duplicate (already processed)
--
-- 2. Queue consumer marks event as processing:
--    UPDATE payment_events 
--    SET status = 'processing', processing_started_at = NOW()
--    WHERE id = '...' AND status = 'pending';
--
-- 3. Queue consumer completes processing:
--    UPDATE payment_events 
--    SET status = 'completed', processed_at = NOW()
--    WHERE id = '...';
--
-- 4. Reconciliation worker finds stuck events:
--    SELECT * FROM payment_events 
--    WHERE status = 'pending' 
--      AND received_at < NOW() - INTERVAL '10 minutes'
--    LIMIT 100;
--
-- 5. Support looks up customer payment:
--    SELECT * FROM payment_events 
--    WHERE payment_reference = 'PAYSTACK_12345'
--    ORDER BY received_at DESC;
-- ═══════════════════════════════════════════════════════════════════════════
