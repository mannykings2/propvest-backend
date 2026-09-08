# Requirements Document

## Introduction

The PropVest wallet/payment processing system currently processes deposits and withdrawals synchronously within webhook handlers. This creates timeout risks, lacks retry mechanisms, and couples multiple concerns (wallet updates, notifications, receipt generation) into single request handlers. This feature implements a production-grade asynchronous message queue architecture using RabbitMQ to decouple webhook handlers from background processing, enabling reliable processing with idempotency guarantees, dead-letter queue handling, and comprehensive observability.

The system will transform the current architecture from:
- **Synchronous**: Webhook → Wallet Update + Notifications + Receipts (all in <3s response window)

To an asynchronous pattern:
- **Asynchronous**: Thin Webhook (<50ms) → Queue → Multiple Specialized Consumers (with retries, DLQs, and monitoring)

## Glossary

- **Webhook_Handler**: HTTP endpoint receiving payment provider notifications (Paystack/Flutterwave)
- **Event_Store**: PostgreSQL table recording all payment events with idempotency keys to prevent duplicate processing
- **Message_Queue**: RabbitMQ broker managing event routing between producers and consumers
- **Queue_Consumer**: Worker process consuming messages from specific queues and executing business logic
- **Dead_Letter_Queue** (DLQ): Fallback queue for messages that fail after maximum retry attempts
- **Idempotency_Key**: Unique identifier (payment reference + event type) ensuring each event is processed exactly once
- **RabbitMQ_Connection**: AMQP connection to RabbitMQ broker with automatic reconnection logic
- **Deposit_Queue**: Queue for deposit confirmation events
- **Withdrawal_Queue**: Queue for withdrawal processing events
- **Notification_Queue**: Queue for user notification events
- **Receipt_Queue**: Queue for receipt generation and email delivery
- **Retry_Policy**: Exponential backoff strategy with configurable maximum attempts
- **Reconciliation_Worker**: Background job detecting and reprocessing missed or stuck events
- **Observability_Dashboard**: Monitoring interface displaying queue metrics, processing rates, and error rates
- **Queue_Depth**: Number of unprocessed messages in a queue
- **Processing_Rate**: Messages consumed per second/minute by consumers
- **Error_Rate**: Percentage of messages failing validation or processing

## Requirements

### Requirement 1: Thin Webhook Handlers

**User Story:** As a system architect, I want webhook handlers to respond within 50ms, so that payment providers don't timeout and retry webhooks unnecessarily.

#### Acceptance Criteria

1. WHEN a webhook request is received, THE Webhook_Handler SHALL verify the signature and return HTTP 200 within 50ms
2. THE Webhook_Handler SHALL NOT perform wallet updates, notifications, or external API calls before responding
3. WHEN signature verification succeeds, THE Webhook_Handler SHALL write to Event_Store and publish to Message_Queue before responding
4. WHEN signature verification fails, THE Webhook_Handler SHALL return HTTP 401 without writing to Event_Store
5. THE Webhook_Handler SHALL include request_id in all published events for distributed tracing
6. WHEN Event_Store write fails, THE Webhook_Handler SHALL return HTTP 500 to trigger provider retry
7. WHEN Message_Queue publish fails, THE Webhook_Handler SHALL log the error but still return HTTP 200 (event exists in Event_Store for reconciliation)

### Requirement 2: Event Store for Idempotency

**User Story:** As a developer, I want all payment events stored with idempotency keys, so that duplicate webhooks don't cause double-processing of deposits or withdrawals.

#### Acceptance Criteria

1. THE Event_Store SHALL have a unique constraint on (payment_reference, event_type, provider)
2. WHEN a webhook event is received, THE Webhook_Handler SHALL attempt to insert into Event_Store with status "pending"
3. IF Event_Store insert fails due to duplicate key, THEN THE Webhook_Handler SHALL return HTTP 200 immediately (already processed)
4. THE Event_Store SHALL record raw_payload (JSONB), received_at (timestamp), processed_at (nullable timestamp), and status (enum: pending, processing, completed, failed)
5. WHEN a Queue_Consumer begins processing, THE Queue_Consumer SHALL update Event_Store status to "processing"
6. WHEN processing completes successfully, THE Queue_Consumer SHALL update status to "completed" and set processed_at
7. WHEN processing fails after all retries, THE Queue_Consumer SHALL update status to "failed" and store failure_reason
8. THE Event_Store SHALL be queryable by payment_reference for reconciliation and debugging

### Requirement 3: Separate Queues with Dead-Letter Queues

**User Story:** As a DevOps engineer, I want separate queues for different event types with DLQs, so that failures in one queue don't block others and failed messages can be manually investigated.

#### Acceptance Criteria

1. THE Message_Queue SHALL create four primary queues: Deposit_Queue, Withdrawal_Queue, Notification_Queue, Receipt_Queue
2. THE Message_Queue SHALL create corresponding DLQs: Deposit_DLQ, Withdrawal_DLQ, Notification_DLQ, Receipt_DLQ
3. WHEN a message is published, THE Webhook_Handler SHALL route to the appropriate queue based on event_type
4. THE Message_Queue SHALL configure message TTL of 48 hours for all primary queues
5. WHEN a message fails processing and retry count < max_retries, THE Queue_Consumer SHALL requeue with exponential backoff delay
6. WHEN retry count >= max_retries (default 3), THE Queue_Consumer SHALL route message to corresponding DLQ
7. THE DLQ SHALL preserve original message payload, headers, and failure metadata (error_message, retry_count, last_attempted_at)
8. THE Message_Queue SHALL set DLQ message expiry to 7 days for manual investigation

### Requirement 4: Queue Consumers with Retry Logic

**User Story:** As a developer, I want consumers to retry transient failures with exponential backoff, so that temporary issues (database locks, API rate limits) don't cause permanent failures.

#### Acceptance Criteria

1. THE Queue_Consumer SHALL implement exponential backoff with delays: 5s, 15s, 45s for retries 1, 2, 3
2. WHEN a transient error occurs (database connection, timeout, rate limit), THE Queue_Consumer SHALL increment retry_count and requeue with delay
3. WHEN a permanent error occurs (invalid data, business rule violation), THE Queue_Consumer SHALL route to DLQ immediately without retrying
4. THE Queue_Consumer SHALL use manual acknowledgment mode (not auto-ack) to prevent message loss
5. WHEN processing succeeds, THE Queue_Consumer SHALL acknowledge (ACK) the message to remove from queue
6. WHEN processing fails and should retry, THE Queue_Consumer SHALL negative-acknowledge (NACK) with requeue=true
7. WHEN routing to DLQ, THE Queue_Consumer SHALL acknowledge original message and publish to DLQ with failure metadata
8. THE Queue_Consumer SHALL set consumer prefetch count to 10 for optimal throughput without overloading

### Requirement 5: Dead-Letter Queue Monitoring

**User Story:** As an operations engineer, I want alerts when messages land in DLQs, so that I can investigate and resolve issues before users are impacted.

#### Acceptance Criteria

1. THE Observability_Dashboard SHALL display Queue_Depth for each DLQ in real-time
2. WHEN Deposit_DLQ depth > 5, THE Observability_Dashboard SHALL trigger "high" severity alert to engineering team
3. WHEN Withdrawal_DLQ depth > 1, THE Observability_Dashboard SHALL trigger "critical" severity alert immediately (money is locked)
4. THE Observability_Dashboard SHALL display per-DLQ error reasons aggregated by error_message
5. THE Observability_Dashboard SHALL provide a UI to view individual DLQ messages with payload and retry history
6. THE Observability_Dashboard SHALL provide a "requeue" button to move messages from DLQ back to primary queue for reprocessing
7. WHEN DLQ message age > 24 hours, THE Observability_Dashboard SHALL highlight in red for urgent attention

### Requirement 6: RabbitMQ Connection Management

**User Story:** As a reliability engineer, I want automatic reconnection to RabbitMQ, so that temporary broker outages don't require manual restarts.

#### Acceptance Criteria

1. THE RabbitMQ_Connection SHALL implement exponential backoff reconnection with max delay of 60s
2. WHEN RabbitMQ becomes unreachable, THE RabbitMQ_Connection SHALL log error and attempt reconnection
3. WHILE reconnecting, THE Webhook_Handler SHALL continue accepting requests and writing to Event_Store (messages will be reconciled later)
4. WHEN RabbitMQ_Connection is restored, THE Queue_Consumer SHALL resume consuming from last acknowledged offset
5. THE RabbitMQ_Connection SHALL use connection heartbeat of 30s to detect network issues
6. WHEN heartbeat fails, THE RabbitMQ_Connection SHALL close connection and initiate reconnection
7. THE RabbitMQ_Connection SHALL publish connection status events to application logs (connected, disconnected, reconnecting)

### Requirement 7: Reconciliation Worker

**User Story:** As a developer, I want a reconciliation job that detects missed events, so that RabbitMQ failures don't cause permanent data inconsistencies.

#### Acceptance Criteria

1. THE Reconciliation_Worker SHALL run every 5 minutes scanning Event_Store for pending events
2. WHEN Event_Store contains events with status "pending" AND received_at older than 10 minutes, THE Reconciliation_Worker SHALL republish to Message_Queue
3. WHEN Event_Store contains events with status "processing" AND processing started more than 30 minutes ago, THE Reconciliation_Worker SHALL reset status to "pending" and republish
4. THE Reconciliation_Worker SHALL batch-process reconciliation in chunks of 100 events to avoid database lock contention
5. THE Reconciliation_Worker SHALL log reconciliation actions (event_id, reason, republished_at) for audit trail
6. THE Reconciliation_Worker SHALL skip events already routed to DLQ (status "failed")
7. THE Reconciliation_Worker SHALL emit metrics: events_reconciled_count, events_pending_count, events_stuck_count

### Requirement 8: Idempotency in Consumers

**User Story:** As a developer, I want consumers to check Event_Store before processing, so that RabbitMQ duplicate deliveries don't cause double-credits or double-notifications.

#### Acceptance Criteria

1. WHEN Queue_Consumer receives a message, THE Queue_Consumer SHALL query Event_Store by Idempotency_Key
2. IF Event_Store status is "completed", THEN THE Queue_Consumer SHALL acknowledge message without processing (already done)
3. IF Event_Store status is "processing" AND processing started < 5 minutes ago, THEN THE Queue_Consumer SHALL requeue for later (another consumer is handling it)
4. IF Event_Store status is "processing" AND processing started > 5 minutes ago, THEN THE Queue_Consumer SHALL proceed (previous consumer crashed)
5. THE Queue_Consumer SHALL use database transactions combining Event_Store update and business logic (wallet credit, etc.)
6. WHEN business logic succeeds, THE Queue_Consumer SHALL update Event_Store to "completed" in same transaction
7. WHEN database transaction fails, THE Queue_Consumer SHALL rollback and requeue message for retry

### Requirement 9: Observability Metrics

**User Story:** As an operations engineer, I want real-time metrics on queue health, so that I can detect bottlenecks and performance degradation.

#### Acceptance Criteria

1. THE Observability_Dashboard SHALL display Queue_Depth for each primary queue updated every 10 seconds
2. THE Observability_Dashboard SHALL display Processing_Rate (messages/minute) per queue over 1min, 5min, 15min windows
3. THE Observability_Dashboard SHALL display Error_Rate percentage per queue over 1min, 5min, 15min windows
4. THE Observability_Dashboard SHALL display average processing time per message type (p50, p95, p99)
5. THE Observability_Dashboard SHALL display consumer lag: time between message published and consumed
6. WHEN Deposit_Queue depth > 100, THE Observability_Dashboard SHALL trigger "warning" alert (backlog forming)
7. WHEN consumer lag > 5 minutes for any queue, THE Observability_Dashboard SHALL trigger "high" alert (consumers overloaded or stuck)
8. THE Observability_Dashboard SHALL display RabbitMQ connection status per worker instance

### Requirement 10: Graceful Degradation

**User Story:** As a system architect, I want the system to degrade gracefully when RabbitMQ is down, so that webhooks are still accepted and can be processed later.

#### Acceptance Criteria

1. WHEN RabbitMQ_Connection is unavailable, THE Webhook_Handler SHALL still accept webhooks and write to Event_Store
2. WHILE RabbitMQ_Connection is unavailable, THE Webhook_Handler SHALL log warnings but return HTTP 200 (event persisted for reconciliation)
3. WHEN RabbitMQ_Connection is restored, THE Reconciliation_Worker SHALL republish all pending events from Event_Store
4. THE Webhook_Handler SHALL implement circuit breaker pattern: after 10 consecutive publish failures, stop attempting publish for 60s
5. WHEN circuit breaker is open, THE Webhook_Handler SHALL log state and skip Message_Queue publish (rely on reconciliation)
6. WHEN circuit breaker transitions to half-open, THE Webhook_Handler SHALL attempt one publish to test connection health
7. IF test publish succeeds, THE circuit breaker SHALL close and resume normal operation

### Requirement 11: Message Serialization and Validation

**User Story:** As a developer, I want strongly-typed message schemas validated on publish and consume, so that invalid messages are rejected early and don't cause consumer crashes.

#### Acceptance Criteria

1. THE Webhook_Handler SHALL define JSON schemas for each event type: DepositEvent, WithdrawalEvent, NotificationEvent, ReceiptEvent
2. WHEN publishing a message, THE Webhook_Handler SHALL validate against schema before sending to Message_Queue
3. IF schema validation fails, THEN THE Webhook_Handler SHALL log error with validation details and return HTTP 400
4. THE Queue_Consumer SHALL validate incoming message against schema before processing
5. IF consumer validation fails, THEN THE Queue_Consumer SHALL route directly to DLQ with error "schema_validation_failed"
6. THE JSON schema SHALL enforce required fields: event_id (UUID), event_type (enum), payment_reference (string), user_id (UUID), timestamp (RFC3339)
7. THE JSON schema SHALL validate amount_kobo as positive integer for financial events

### Requirement 12: Queue Configuration

**User Story:** As a DevOps engineer, I want queue configurations in code with sane defaults, so that queues are created consistently across environments.

#### Acceptance Criteria

1. THE Message_Queue SHALL declare queues with durable=true (survive broker restart)
2. THE Message_Queue SHALL configure max-length=10000 for primary queues (apply backpressure if consumers can't keep up)
3. WHEN primary queue reaches max-length, THE Message_Queue SHALL reject new messages with error (prevents memory exhaustion)
4. THE Message_Queue SHALL configure message priority: withdrawal events priority=10, deposit priority=5, notifications priority=1
5. THE Message_Queue SHALL set queue auto-delete=false (don't delete when all consumers disconnect)
6. THE Message_Queue SHALL configure prefetch=10 per consumer (balance throughput vs memory usage)
7. THE Message_Queue SHALL enable publisher confirms to guarantee messages are persisted before ACK to webhook handler

### Requirement 13: Concurrency and Scaling

**User Story:** As a platform engineer, I want multiple consumer instances processing messages concurrently, so that the system can handle traffic spikes.

#### Acceptance Criteria

1. THE Queue_Consumer SHALL support horizontal scaling: multiple worker instances consuming from same queue
2. WHEN multiple consumers are running, THE Message_Queue SHALL distribute messages round-robin across consumers
3. THE Queue_Consumer SHALL process messages concurrently within a single instance using goroutines (default: 10 concurrent handlers)
4. THE Queue_Consumer SHALL use database row-level locking (SELECT FOR UPDATE) to prevent concurrent processing of same wallet
5. WHEN two consumers attempt to process events for same wallet simultaneously, THE database lock SHALL serialize access
6. THE Queue_Consumer SHALL set connection pool size to min(20, concurrency * 2) for database connections
7. THE Observability_Dashboard SHALL display active consumer count per queue

### Requirement 14: Environment-Specific Configuration

**User Story:** As a DevOps engineer, I want queue behavior configurable per environment, so that development uses different settings than production.

#### Acceptance Criteria

1. THE Message_Queue SHALL read configuration from environment variables: RABBITMQ_URL, QUEUE_MAX_RETRIES, RETRY_DELAY_BASE, CONSUMER_CONCURRENCY
2. THE Message_Queue SHALL set development defaults: max_retries=1, consumer_concurrency=2, queue_max_length=100
3. THE Message_Queue SHALL set production defaults: max_retries=3, consumer_concurrency=10, queue_max_length=10000
4. THE Message_Queue SHALL validate configuration on startup and fail-fast if invalid (negative retry count, invalid URL)
5. THE Message_Queue SHALL log active configuration on startup for debugging
6. WHEN RABBITMQ_URL is empty, THE Queue_Consumer SHALL not start but application SHALL still run (webhook handler works, reconciliation handles backlog later)

### Requirement 15: Audit Trail and Debugging

**User Story:** As a support engineer, I want to trace a payment event from webhook to completion, so that I can debug customer issues efficiently.

#### Acceptance Criteria

1. THE Event_Store SHALL record request_id from webhook for correlation across services
2. THE Event_Store SHALL record message_id assigned by RabbitMQ for queue debugging
3. THE Queue_Consumer SHALL log structured events: message_received, processing_started, processing_completed, processing_failed with request_id
4. THE Observability_Dashboard SHALL provide search by payment_reference showing: webhook received_at, queued_at, processing_started_at, completed_at
5. THE Observability_Dashboard SHALL display processing timeline: webhook → queue → consumer → completion with duration at each stage
6. THE Event_Store SHALL preserve error stacktraces for failed events (limited to 10KB)
7. WHEN support searches by user_id, THE Observability_Dashboard SHALL return all events (deposits, withdrawals, notifications) for that user ordered by timestamp

### Requirement 16: Future Extensibility

**User Story:** As a product manager, I want the queue architecture to support future event types, so that we can add SMS, push notifications, and analytics without refactoring.

#### Acceptance Criteria

1. THE Message_Queue SHALL use topic-based routing: events published to "payments.deposit", "payments.withdrawal", "notifications.email", "notifications.sms"
2. THE Queue_Consumer SHALL bind queues to topics using wildcards: Notification_Queue binds to "notifications.*"
3. WHEN a new event type is added, THE system SHALL only require adding a new consumer (no changes to Webhook_Handler or Event_Store schema)
4. THE Event_Store event_type column SHALL be VARCHAR(100) to accommodate future types
5. THE JSON schema validation SHALL allow optional "metadata" field (JSONB) for event-specific data
6. THE Message_Queue SHALL support exchange fanout: one event published to multiple queues (e.g., deposit triggers wallet update AND analytics)

## Parser and Serializer Requirements

### Requirement 17: Event Serialization

**User Story:** As a developer, I want a parser and pretty-printer for event messages, so that round-trip serialization preserves event data accurately.

#### Acceptance Criteria

1. THE Event_Parser SHALL parse JSON event messages into strongly-typed Go structs (DepositEvent, WithdrawalEvent, etc.)
2. WHEN a valid JSON event is provided, THE Event_Parser SHALL return a populated struct with all fields
3. WHEN an invalid JSON event is provided, THE Event_Parser SHALL return a descriptive error with field name and validation failure
4. THE Event_PrettyPrinter SHALL format event structs back into valid JSON messages
5. FOR ALL valid event structs, parsing then printing then parsing SHALL produce an equivalent struct (round-trip property)
6. THE Event_Parser SHALL validate required fields during parsing: event_id, event_type, payment_reference
7. THE Event_Parser SHALL validate amount_kobo > 0 for financial events

---

## Summary

This requirements document defines a production-grade asynchronous message queue system using RabbitMQ for the PropVest wallet/payment system. The design prioritizes:

- **Reliability**: Idempotency, retries, DLQs, reconciliation
- **Performance**: <50ms webhook responses, concurrent processing, horizontal scaling
- **Observability**: Metrics, alerts, audit trails, debugging tools
- **Resilience**: Graceful degradation, circuit breakers, automatic reconnection
- **Extensibility**: Topic-based routing, pluggable consumers, metadata support

The system transforms synchronous webhook processing into an asynchronous, fault-tolerant architecture capable of handling payment provider webhooks at scale while maintaining exactly-once processing semantics.
