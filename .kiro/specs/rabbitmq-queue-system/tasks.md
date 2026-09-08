# Implementation Plan: RabbitMQ Queue System

## Overview

This plan implements a production-grade asynchronous message queue architecture using RabbitMQ to decouple webhook handlers from background processing. The implementation transforms synchronous webhook processing into a fault-tolerant system with idempotency guarantees, retry mechanisms, dead-letter queues, and comprehensive observability.

## Tasks

### 1. Database Foundation

- [ ] 1.1 Create payment_events table migration
  - Create migration file `000008_create_payment_events_table.up.sql`
  - Define table schema with idempotency constraint on (payment_reference, event_type, provider)
  - Add indexes for status, received_at, payment_reference, and reconciliation queries
  - Create corresponding down migration
  - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.7, 15.1, 15.2_

- [ ] 1.2 Create PaymentEvent model
  - Define `PaymentEvent` struct in `internal/models/payment_event.go`
  - Implement `EventStatus` enum (pending, processing, completed, failed)
  - Add GORM tags for field mapping and constraints
  - Include timestamp fields for lifecycle tracking
  - _Requirements: 2.1, 2.2, 2.4, 15.1_

- [ ] 1.3 Implement PaymentEventRepository interface
  - Define repository interface in `internal/repositories/payment_event_repository.go`
  - Include methods: Create, GetByID, GetByReference, UpdateStatus, MarkProcessing, MarkCompleted, MarkFailed
  - Add reconciliation methods: FindPendingEvents, FindStuckProcessing
  - Include helper methods: IncrementRetryCount, IsCompleted, GetEventsByUser
  - _Requirements: 2.1, 2.2, 2.3, 2.5, 2.6, 2.7, 7.1, 7.2, 7.3, 15.4_

- [ ] 1.4 Implement PaymentEventRepository
  - Create implementation in `internal/repositories/payment_event_repository_impl.go`
  - Implement Create with duplicate key detection (return ErrDuplicateEvent)
  - Implement status update methods with timestamp tracking
  - Implement reconciliation queries with time-based filters
  - Use GORM for database operations with context support
  - _Requirements: 2.1, 2.2, 2.3, 2.5, 2.6, 2.7, 7.1, 7.2, 7.3_

- [ ]* 1.5 Write unit tests for PaymentEventRepository
  - Test idempotency: duplicate insert returns ErrDuplicateEvent
  - Test status transitions: pending → processing → completed
  - Test FindPendingEvents with time filtering
  - Test FindStuckProcessing for crash recovery
  - Test IsCompleted check
  - Use test database fixtures and cleanup
  - _Requirements: 2.1, 2.2, 2.3, 7.1, 7.2, 7.3_

### 2. Queue Infrastructure

- [ ] 2.1 Create queue configuration
  - Create `internal/queue/config.go` with QueueConfig struct
  - Define configurations for deposit, withdrawal, notification, and receipt queues
  - Set queue parameters: durable, max-length, TTL, priority, DLQ routing
  - Set consumer parameters: prefetch count, concurrency
  - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6, 12.7, 14.1, 14.2, 14.3_

- [ ] 2.2 Implement circuit breaker
  - Create `internal/queue/circuit_breaker.go`
  - Implement CircuitBreaker struct with states: Closed, Open, HalfOpen
  - Implement CanAttempt method with timeout-based state transitions
  - Implement RecordSuccess and RecordFailure methods
  - Add configurable failure threshold and reset timeout
  - _Requirements: 10.4, 10.5, 10.6_

- [ ] 2.3 Implement RabbitMQ client - connection management
  - Create `internal/queue/client.go` with Client struct
  - Implement NewClient with connection initialization
  - Implement connect method with channel creation and publisher confirms
  - Implement declareTopology for exchanges and queues
  - Add circuit breaker integration
  - _Requirements: 6.1, 6.2, 6.5, 6.7, 12.7, 14.6_

- [ ] 2.4 Implement RabbitMQ client - publishing
  - Implement Publish method with context timeout (5s)
  - Add publisher confirms with confirmation waiting
  - Implement circuit breaker checks before publishing
  - Add message serialization to JSON
  - Set message properties: ContentType, DeliveryMode, Priority, Timestamp, MessageId
  - _Requirements: 1.3, 1.5, 1.7, 11.2, 11.3, 12.7_

- [ ] 2.5 Implement RabbitMQ client - consuming
  - Implement Consume method with QoS (prefetch) configuration
  - Create consumeWorker with concurrent goroutines (configurable concurrency)
  - Implement manual acknowledgment: Ack on success, Nack with requeue on retry
  - Parse retry count from message headers (x-retry-count)
  - Calculate exponential backoff delays (5s, 15s, 45s)
  - Route to DLQ after max retries (3 attempts)
  - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_

- [ ] 2.6 Implement RabbitMQ client - reconnection
  - Implement startReconnectMonitor goroutine
  - Listen for connection close events
  - Implement reconnect with exponential backoff (max 60s)
  - Update connection enabled flag on connect/disconnect
  - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.6, 6.7_

- [ ] 2.7 Implement queue topology setup
  - Declare main exchange "propvest.events" (topic type)
  - Declare dead-letter exchange "propvest.dlx" (topic type)
  - Declare primary queues with DLX arguments
  - Declare DLQ queues (7-day TTL)
  - Bind primary queues to main exchange
  - Bind DLQ queues to dead-letter exchange
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.8, 12.1, 12.2, 12.3_

- [ ]* 2.8 Write unit tests for circuit breaker
  - Test state transitions: Closed → Open → HalfOpen → Closed
  - Test failure threshold triggering (10 failures)
  - Test reset timeout (60s)
  - Test CanAttempt behavior in each state
  - _Requirements: 10.4, 10.5, 10.6_

### 3. Message Schemas and Validation

- [ ] 3.1 Define message event structs
  - Create `internal/queue/messages.go`
  - Define BaseEvent with common fields: EventID, EventType, PaymentReference, Timestamp, RequestID
  - Define DepositEvent with UserID, AmountKobo, Currency, Provider
  - Define WithdrawalEvent with TransactionID, bank details
  - Define NotificationEvent with NotificationType, Template, Data
  - Define ReceiptEvent with TransactionID, TransactionType
  - Define event type constants
  - _Requirements: 11.1, 11.6, 11.7, 16.4, 16.5_

- [ ] 3.2 Implement JSON schema validation
  - Create `internal/queue/validation.go`
  - Define JSON schema for DepositEvent (required fields, amount > 0, enums)
  - Implement ValidateDepositEvent function using gojsonschema
  - Define and implement schemas for WithdrawalEvent, NotificationEvent, ReceiptEvent
  - Return descriptive validation errors with field names
  - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 11.7_

- [ ]* 3.3 Write unit tests for message validation
  - Test valid DepositEvent passes validation
  - Test missing required fields triggers validation error
  - Test invalid amount_kobo (zero or negative) triggers error
  - Test invalid enum values trigger error
  - Test invalid UUID format triggers error
  - _Requirements: 11.2, 11.3, 11.4, 11.5, 11.6, 11.7_

### 4. Error Handling and Retry Logic

- [ ] 4.1 Define error types
  - Create `internal/queue/errors.go`
  - Define transient errors: ErrDatabaseConnection, ErrDatabaseTimeout, ErrProviderTimeout, ErrProviderRateLimit
  - Define permanent errors: ErrInvalidEvent, ErrInvalidAmount, ErrUserNotFound, ErrInsufficientBalance
  - Implement IsTransient and IsPermanent helper functions
  - _Requirements: 4.2, 4.3, 11.4, 11.5_

- [ ] 4.2 Implement retry policy
  - Create `internal/queue/retry.go`
  - Define RetryPolicy struct with MaxRetries, InitialBackoff, MaxBackoff, Multiplier
  - Implement GetDelay method with exponential backoff calculation
  - Define DefaultRetryPolicy (3 retries, 5s initial, 45s max, 3x multiplier)
  - _Requirements: 4.1, 4.2, 4.3, 14.1, 14.2, 14.3_

- [ ]* 4.3 Write unit tests for retry logic
  - Test GetDelay calculation: retry 1 = 5s, retry 2 = 15s, retry 3 = 45s
  - Test max backoff capping
  - Test IsTransient for various error types
  - Test IsPermanent for various error types
  - _Requirements: 4.1, 4.2, 4.3_

### 5. Webhook Handler Refactoring

- [ ] 5.1 Refactor webhook handler to thin handler
  - Modify existing webhook handler in `internal/handlers/wallet.go`
  - Remove synchronous wallet update logic
  - Remove synchronous notification logic
  - Keep only: signature verification, event store insert, queue publish
  - Ensure response time < 50ms
  - _Requirements: 1.1, 1.2, 1.3, 1.6, 10.1, 10.2_

- [ ] 5.2 Add idempotency check in webhook handler
  - Attempt insert to event_store with status "pending"
  - On duplicate key error, return HTTP 200 immediately
  - On insert success, proceed to queue publish
  - On insert failure (non-duplicate), return HTTP 500
  - _Requirements: 1.3, 2.1, 2.2, 2.3, 2.6_

- [ ] 5.3 Add queue publishing with circuit breaker
  - Check if RabbitMQ client is enabled
  - Check circuit breaker state before publishing
  - Publish event to appropriate queue based on event type
  - On publish failure, log error but still return HTTP 200 (graceful degradation)
  - Include request_id in event for distributed tracing
  - _Requirements: 1.3, 1.5, 1.7, 10.1, 10.2, 10.4, 10.5, 15.1_

- [ ]* 5.4 Write integration tests for thin webhook handler
  - Test webhook response time < 50ms
  - Test successful webhook: signature valid, event stored, queue published
  - Test invalid signature returns HTTP 401
  - Test duplicate webhook returns HTTP 200 without re-processing
  - Test RabbitMQ down: webhook succeeds, event stored (graceful degradation)
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.6, 1.7, 2.1, 2.2, 2.3, 10.1, 10.2_

### 6. Queue Consumers

- [ ] 6.1 Implement deposit consumer
  - Create deposit consumer in `cmd/worker/consumers/deposit_consumer.go`
  - Implement handler function: parse DepositEvent, validate schema
  - Check idempotency: query event store, skip if completed
  - Mark event as "processing" in event store
  - Begin database transaction
  - Credit user wallet with amount_kobo
  - Create transaction ledger entry (type: deposit, status: completed)
  - Mark event as "completed" in event store
  - Commit transaction
  - Publish NotificationEvent to notification queue
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 11.4_

- [ ] 6.2 Implement withdrawal consumer
  - Create withdrawal consumer in `cmd/worker/consumers/withdrawal_consumer.go`
  - Implement handler function: parse WithdrawalEvent, validate schema
  - Check idempotency: query event store, skip if completed
  - Mark event as "processing" in event store
  - Initiate bank transfer via Paystack API
  - Store transfer_code from Paystack response
  - Mark event as "completed" in event store
  - Publish NotificationEvent (withdrawal initiated)
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 11.4_

- [ ] 6.3 Implement notification consumer
  - Create notification consumer in `cmd/worker/consumers/notification_consumer.go`
  - Implement handler function: parse NotificationEvent, validate schema
  - Route based on notification_type: email, sms, push
  - Send email notification using existing email service
  - Send SMS notification using existing SMS service
  - Log notification sent with request_id for tracing
  - _Requirements: 8.1, 8.2, 11.4, 15.3_

- [ ] 6.4 Implement receipt consumer
  - Create receipt consumer in `cmd/worker/consumers/receipt_consumer.go`
  - Implement handler function: parse ReceiptEvent, validate schema
  - Generate PDF receipt using transaction data
  - Email receipt to user
  - Log receipt generation with request_id
  - _Requirements: 8.1, 8.2, 11.4, 15.3_

- [ ] 6.5 Add consumer error handling
  - In each consumer, wrap handler with error classification
  - On permanent error, return error to route to DLQ immediately
  - On transient error, return error to trigger retry with backoff
  - Log all errors with context (queue name, event_id, retry_count)
  - _Requirements: 4.2, 4.3, 11.5_

- [ ]* 6.6 Write integration tests for deposit consumer
  - Test end-to-end deposit flow: consume message → credit wallet → create transaction → complete event
  - Test idempotency: process same message twice, wallet credited only once
  - Test transient error: database timeout triggers retry
  - Test permanent error: invalid user_id routes to DLQ
  - Test notification queuing after successful deposit
  - Use testcontainers for PostgreSQL and RabbitMQ
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7, 4.2, 4.3_

- [ ]* 6.7 Write integration tests for withdrawal consumer
  - Test end-to-end withdrawal flow: consume message → initiate transfer → complete event
  - Test idempotency: process same message twice, transfer initiated only once
  - Test Paystack API timeout triggers retry
  - Test notification queuing after withdrawal initiated
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6, 8.7_

### 7. Reconciliation Worker

- [ ] 7.1 Implement reconciliation worker
  - Create `cmd/worker/reconciler/reconciler.go`
  - Implement RunReconciliation function with 5-minute ticker
  - Query event_store for pending events older than 10 minutes
  - Query event_store for processing events older than 30 minutes (stuck/crashed consumers)
  - Batch-process in chunks of 100 events
  - Republish events to RabbitMQ
  - Update processing events back to pending before republishing
  - Skip events with status "failed" (already in DLQ)
  - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.6, 10.3_

- [ ] 7.2 Add reconciliation metrics
  - Log reconciliation start and completion
  - Emit metrics: events_reconciled_count, events_pending_count, events_stuck_count
  - Log each republished event with event_id, reason, republished_at
  - _Requirements: 7.5, 7.7, 15.3_

- [ ]* 7.3 Write integration tests for reconciliation
  - Test pending event detection: create event received 15 minutes ago, verify republished
  - Test stuck processing detection: create processing event started 40 minutes ago, verify reset to pending and republished
  - Test batch processing: create 150 pending events, verify processed in batches of 100
  - Test failed events skipped: create failed event, verify not republished
  - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.6_

### 8. Worker Process Setup

- [ ] 8.1 Create worker main entry point
  - Create or modify `cmd/worker/main.go`
  - Load configuration from environment variables
  - Initialize database connection
  - Initialize RabbitMQ client with reconnection
  - Validate configuration on startup (fail-fast)
  - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5, 14.6_

- [ ] 8.2 Start queue consumers
  - Start deposit consumer with DepositQueueConfig
  - Start withdrawal consumer with WithdrawalQueueConfig
  - Start notification consumer with NotificationQueueConfig
  - Start receipt consumer with ReceiptQueueConfig
  - Each consumer runs in separate goroutine
  - Log active configuration for each consumer
  - _Requirements: 13.1, 13.2, 13.3, 14.5_

- [ ] 8.3 Start reconciliation worker
  - Start reconciliation goroutine with configurable interval
  - Log reconciliation schedule on startup
  - _Requirements: 7.1, 14.1_

- [ ] 8.4 Implement graceful shutdown
  - Listen for SIGINT and SIGTERM signals
  - Stop accepting new messages from queues
  - Wait for in-flight messages to complete (with timeout)
  - Close RabbitMQ connection
  - Close database connection
  - Log shutdown completion
  - _Requirements: 6.1, 6.2, 6.3, 6.4_

### 9. Configuration Management

- [ ] 9.1 Create queue configuration loader
  - Create `internal/config/queue_config.go`
  - Define QueueConfig struct with all configurable parameters
  - Load from environment variables with defaults
  - Set development defaults: max_retries=1, concurrency=2, max_length=100
  - Set production defaults: max_retries=3, concurrency=10, max_length=10000
  - _Requirements: 14.1, 14.2, 14.3_

- [ ] 9.2 Add configuration validation
  - Implement Validate method on QueueConfig
  - Validate max_retries in range [0, 10]
  - Validate consumer_concurrency in range [1, 100]
  - Validate reconciliation_interval >= 1 minute
  - Return descriptive errors for invalid values
  - _Requirements: 14.4_

- [ ] 9.3 Update .env.example with queue variables
  - Add RABBITMQ_URL with example value
  - Add QUEUE_MAX_RETRIES, QUEUE_RETRY_DELAY_BASE, QUEUE_CONSUMER_CONCURRENCY
  - Add per-queue concurrency overrides
  - Add reconciliation configuration
  - Add circuit breaker configuration
  - _Requirements: 14.1, 14.2, 14.3_

### 10. Observability and Monitoring

- [ ] 10.1 Create metrics collection
  - Create `internal/metrics/queue_metrics.go`
  - Define Prometheus metrics: QueueDepth (gauge), MessagesProcessed (counter), ProcessingDuration (histogram)
  - Define DLQ metrics: DLQDepth (gauge)
  - Define circuit breaker metric: CircuitBreakerState (gauge)
  - Define reconciliation metrics: ReconciliationRuns, ReconciliationEventsRepublished
  - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 7.7_

- [ ] 10.2 Add metrics to consumers
  - Wrap each consumer handler with metrics collection
  - Record processing duration using histogram
  - Increment MessagesProcessed counter by status (success, retry, failure)
  - Update QueueDepth gauge periodically (every 10 seconds)
  - _Requirements: 9.1, 9.2, 9.3, 9.4_

- [ ] 10.3 Add structured logging
  - Create queue logger in `internal/logger/structured.go`
  - Use slog with JSON handler for structured output
  - Implement LogMessageProcessed helper function
  - Include fields: queue, message_id, event_id, payment_reference, duration_ms, request_id, error
  - _Requirements: 15.3, 15.6_

- [ ] 10.4 Add DLQ monitoring
  - Implement periodic DLQ depth checking (every 10 seconds)
  - Update DLQDepth metric for each DLQ
  - Emit DLQ depth to metrics endpoint
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 9.1_

- [ ] 10.5 Expose Prometheus metrics endpoint
  - Add `/metrics` endpoint to worker process HTTP server
  - Use prometheus HTTP handler
  - Document example Prometheus queries in code comments
  - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6, 9.7, 9.8_

### 11. Documentation and Deployment

- [ ] 11.1 Create RabbitMQ setup guide
  - Create `docs/RABBITMQ_SETUP.md`
  - Document RabbitMQ installation (Docker Compose example)
  - Document queue topology (exchanges, queues, bindings)
  - Document management UI access and basic operations
  - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6, 12.7_

- [ ] 11.2 Update docker-compose.yml
  - Add RabbitMQ service with management plugin
  - Set resource limits (memory, CPU)
  - Configure persistent volume for message durability
  - Expose ports: 5672 (AMQP), 15672 (management UI)
  - _Requirements: 12.1, 14.6_

- [ ] 11.3 Create migration checklist
  - Create `docs/QUEUE_MIGRATION_CHECKLIST.md`
  - Document pre-migration steps: backup database, test RabbitMQ connection
  - Document migration steps: run database migration, deploy worker process, deploy webhook changes
  - Document post-migration verification: check metrics, monitor DLQs, test webhook flow
  - Document rollback procedure
  - _Requirements: All requirements_

- [ ] 11.4 Update README with queue architecture
  - Add section describing asynchronous queue system
  - Add architecture diagram (ASCII or reference to image)
  - Document environment variables for queue configuration
  - Document worker process startup command
  - _Requirements: All requirements_

- [ ] 11.5 Create observability dashboard guide
  - Create `docs/QUEUE_OBSERVABILITY.md`
  - Document Prometheus metrics and example queries
  - Document alert rules for DLQ depth, queue backlog, consumer lag
  - Document structured log format and querying
  - Document debugging procedures using event store
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 9.1, 9.2, 9.3, 9.4, 9.5, 9.6, 9.7, 9.8, 15.3, 15.4, 15.5, 15.6_

### 12. Final Integration and Testing

- [ ] 12.1 End-to-end system test
  - Set up test environment with PostgreSQL and RabbitMQ (testcontainers)
  - Start API process and worker process
  - Send deposit webhook to API
  - Verify webhook responds < 50ms
  - Verify event stored in event_store
  - Verify message published to deposit queue
  - Verify consumer processes message
  - Verify wallet credited
  - Verify transaction created
  - Verify notification queued
  - Verify event marked completed
  - _Requirements: All requirements in category 1, 2, 8, 11_

- [ ] 12.2 Test duplicate webhook handling
  - Send same deposit webhook twice
  - Verify first webhook stores event and publishes to queue
  - Verify second webhook returns 200 but doesn't create duplicate event
  - Verify wallet credited only once
  - _Requirements: 2.1, 2.2, 2.3, 8.1, 8.2_

- [ ] 12.3 Test retry and DLQ flow
  - Mock transient error in consumer (database timeout)
  - Send deposit webhook
  - Verify message retried with exponential backoff (5s, 15s, 45s)
  - Mock permanent error in consumer (invalid user)
  - Verify message routed to DLQ after max retries
  - Verify DLQDepth metric increases
  - _Requirements: 3.4, 3.5, 3.6, 3.7, 4.1, 4.2, 4.3, 4.5, 4.6, 4.7, 5.1, 5.2_

- [ ] 12.4 Test reconciliation worker
  - Create pending event in database (received 15 minutes ago)
  - Wait for reconciliation cycle (or trigger manually)
  - Verify event republished to queue
  - Verify consumer processes event
  - Verify event marked completed
  - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.7_

- [ ] 12.5 Test graceful degradation
  - Stop RabbitMQ broker
  - Send deposit webhook
  - Verify webhook responds 200 (graceful degradation)
  - Verify event stored in event_store
  - Verify circuit breaker opens after 10 failures
  - Restart RabbitMQ
  - Verify worker reconnects automatically
  - Verify reconciliation republishes pending events
  - _Requirements: 6.1, 6.2, 6.3, 6.4, 10.1, 10.2, 10.3, 10.4, 10.5, 10.6_

- [ ] 12.6 Checkpoint - Review and validate
  - Ensure all tests pass
  - Review code for Go best practices and project conventions
  - Verify metrics are exposed and queryable
  - Verify structured logging is consistent
  - Check for proper error handling and context propagation
  - Ask the user if questions arise

## Notes

- Tasks marked with `*` are optional testing tasks that can be skipped for faster MVP delivery
- Each task references specific requirements from the requirements document for traceability
- The implementation follows Go best practices: dependency injection, error wrapping, context propagation, structured logging
- Database migrations must be run before deploying the API and worker processes
- RabbitMQ must be running and accessible before starting the worker process
- Testing strategy focuses on integration tests (not property-based tests) due to infrastructure-heavy nature
- Checkpoints ensure incremental validation and early detection of issues
- The system supports horizontal scaling: multiple worker instances can consume from the same queues
- Circuit breaker and reconciliation provide resilience against RabbitMQ failures
- DLQ monitoring enables manual investigation and reprocessing of failed messages

## Task Dependency Graph

```json
{
  "waves": [
    {
      "id": 0,
      "tasks": ["1.1", "2.1", "3.1", "4.1", "9.1"]
    },
    {
      "id": 1,
      "tasks": ["1.2", "2.2", "3.2", "4.2", "9.2"]
    },
    {
      "id": 2,
      "tasks": ["1.3", "2.3", "3.3", "4.3", "9.3"]
    },
    {
      "id": 3,
      "tasks": ["1.4", "2.4"]
    },
    {
      "id": 4,
      "tasks": ["1.5", "2.5", "2.8"]
    },
    {
      "id": 5,
      "tasks": ["2.6", "5.1", "10.1"]
    },
    {
      "id": 6,
      "tasks": ["2.7", "5.2", "10.3"]
    },
    {
      "id": 7,
      "tasks": ["5.3", "6.1", "6.2", "6.3", "6.4"]
    },
    {
      "id": 8,
      "tasks": ["5.4", "6.5", "6.6", "6.7", "10.2"]
    },
    {
      "id": 9,
      "tasks": ["7.1", "8.1", "10.4"]
    },
    {
      "id": 10,
      "tasks": ["7.2", "8.2", "10.5"]
    },
    {
      "id": 11,
      "tasks": ["7.3", "8.3", "11.1", "11.2"]
    },
    {
      "id": 12,
      "tasks": ["8.4", "11.3", "11.4", "11.5"]
    },
    {
      "id": 13,
      "tasks": ["12.1", "12.2", "12.3", "12.4", "12.5"]
    },
    {
      "id": 14,
      "tasks": ["12.6"]
    }
  ]
}
```
