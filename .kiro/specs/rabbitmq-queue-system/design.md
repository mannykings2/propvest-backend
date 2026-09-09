# Design Document: RabbitMQ Queue System

## Overview

This document details the technical design for migrating PropVest's synchronous webhook processing architecture to an asynchronous, queue-based system using RabbitMQ. The system implements exactly-once processing semantics through idempotency guarantees, provides retry mechanisms with dead-letter queues, and ensures graceful degradation when RabbitMQ is unavailable.

### Current Architecture Problem

```
┌─────────────┐         ┌──────────────────────────────────────┐
│   Paystack  │────────▶│  Webhook Handler (Synchronous)       │
│  Webhook    │ 3s timeout │  1. Verify signature               │
└─────────────┘         │  2. Update wallet (+1-2s)            │
                        │  3. Create transaction (+500ms)       │
                        │  4. Send notifications (+800ms)       │
                        │  5. Generate receipt (+400ms)         │
                        │  TOTAL: 2.7s (RISKY)                 │
                        └──────────────────────────────────────┘
```

**Problems:**
- Timeout risk (Paystack/Flutterwave timeout after 3s)
- No retry mechanism if notification or receipt fails
- Webhook handler is a single point of failure
- Coupling multiple concerns in HTTP request context

### Target Architecture

```
┌─────────────┐    ┌──────────────┐    ┌────────────────┐    ┌──────────────────┐
│   Paystack  │───▶│ Thin Webhook │───▶│  Event Store   │───▶│    RabbitMQ      │
│  Webhook    │ 50ms│  Handler     │    │  (Postgres)    │    │  (Message Bus)   │
└─────────────┘    └──────────────┘    └────────────────┘    └──────────────────┘
                                                                      │
                    ┌─────────────────────────────────────────────────┤
                    │                     │                     │     │
                    ▼                     ▼                     ▼     ▼
            ┌────────────┐       ┌────────────┐       ┌────────────┐ ┌────────────┐
            │  Deposit   │       │ Withdrawal │       │Notification│ │  Receipt   │
            │  Consumer  │       │  Consumer  │       │  Consumer  │ │  Consumer  │
            └────────────┘       └────────────┘       └────────────┘ └────────────┘
                    │                     │                     │            │
                    ▼                     ▼                     ▼            ▼
            ┌────────────┐       ┌────────────┐       ┌────────────┐ ┌────────────┐
            │  Deposit   │       │ Withdrawal │       │Notification│ │  Receipt   │
            │    DLQ     │       │    DLQ     │       │    DLQ     │ │    DLQ     │
            └────────────┘       └────────────┘       └────────────┘ └────────────┘
```

**Benefits:**
- **Speed**: Webhook responds in <50ms (signature validation + DB write only)
- **Reliability**: Retry with exponential backoff, DLQ for manual investigation
- **Resilience**: Graceful degradation when RabbitMQ is down
- **Scalability**: Horizontal scaling of consumers
- **Observability**: Metrics on queue depth, processing rate, error rate

---

## Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           PROPVEST BACKEND                                  │
│                                                                             │
│  ┌───────────────┐                                                         │
│  │  API Process  │                                                         │
│  │  (cmd/api)    │                                                         │
│  ├───────────────┤                                                         │
│  │ Webhook       │──┐                                                      │
│  │ Handlers      │  │                                                      │
│  └───────────────┘  │                                                      │
│                     │                                                      │
│                     ▼                                                      │
│  ┌──────────────────────────────────────────────────────────┐            │
│  │             EVENT STORE (PostgreSQL)                      │            │
│  │  Table: payment_events                                    │            │
│  │  ┌───────────────────────────────────────────────────┐   │            │
│  │  │ id, payment_reference, event_type, provider,      │   │            │
│  │  │ status, raw_payload, received_at, processed_at    │   │            │
│  │  └───────────────────────────────────────────────────┘   │            │
│  └──────────────────────────────────────────────────────────┘            │
│                     │                                                      │
│                     ▼                                                      │
│  ┌──────────────────────────────────────────────────────────┐            │
│  │              RABBITMQ CONNECTION POOL                     │            │
│  │  • Auto-reconnect with exponential backoff               │            │
│  │  • Circuit breaker for degradation                       │            │
│  │  • Publisher confirms enabled                            │            │
│  └──────────────────────────────────────────────────────────┘            │
│                                                                             │
│  ┌───────────────┐                                                         │
│  │Worker Process │                                                         │
│  │ (cmd/worker)  │                                                         │
│  ├───────────────┤                                                         │
│  │ Consumers:    │                                                         │
│  │ • Deposit     │──▶ Credit wallet, send notification                    │
│  │ • Withdrawal  │──▶ Initiate bank transfer                              │
│  │ • Notification│──▶ Send email/SMS/push                                 │
│  │ • Receipt     │──▶ Generate PDF receipt, email                         │
│  │               │                                                         │
│  │ Background:   │                                                         │
│  │ • Reconciler  │──▶ Republish stuck/pending events every 5 min          │
│  └───────────────┘                                                         │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                            RABBITMQ BROKER                                  │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────┐            │
│  │  EXCHANGE: propvest.events (topic exchange)              │            │
│  └──────────────────────────────────────────────────────────┘            │
│           │                    │                   │            │          │
│           ▼                    ▼                   ▼            ▼          │
│  ┌──────────────┐    ┌──────────────┐   ┌──────────────┐ ┌──────────┐  │
│  │ deposit.     │    │ withdrawal.  │   │notification. │ │ receipt. │  │
│  │ confirmed    │    │ requested    │   │ dispatch     │ │ generate │  │
│  │              │    │              │   │              │ │          │  │
│  │ Queue depth: │    │ Queue depth: │   │ Queue depth: │ │Queue     │  │
│  │ max=10000    │    │ max=10000    │   │ max=10000    │ │depth:    │  │
│  │ TTL=48h      │    │ TTL=48h      │   │ TTL=48h      │ │max=10000 │  │
│  │ Priority:5   │    │ Priority:10  │   │ Priority:1   │ │TTL=48h   │  │
│  │ Durable:true │    │ Durable:true │   │ Durable:true │ │Pri:1     │  │
│  └──────────────┘    └──────────────┘   └──────────────┘ └──────────┘  │
│           │                    │                   │            │          │
│           │ (max 3 retries)    │ (max 3 retries)  │ (max 3)    │(max 3)  │
│           ▼                    ▼                   ▼            ▼          │
│  ┌──────────────┐    ┌──────────────┐   ┌──────────────┐ ┌──────────┐  │
│  │ deposit.dlq  │    │withdrawal.dlq│   │notification. │ │receipt.  │  │
│  │              │    │              │   │ dlq          │ │dlq       │  │
│  │ TTL=7d       │    │ TTL=7d       │   │ TTL=7d       │ │TTL=7d    │  │
│  └──────────────┘    └──────────────┘   └──────────────┘ └──────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Component Interactions

**1. Webhook Reception (API Process):**
```go
// internal/handlers/wallet.go
func (h *WalletHandler) HandleWebhook(c *gin.Context) {
    // 1. Read raw body for signature verification (<5ms)
    body := readBody(c)
    
    // 2. Verify signature (<10ms)
    if !h.verifySignature(body) {
        return HTTP 401
    }
    
    // 3. Parse webhook (<5ms)
    event := parseWebhook(body)
    
    // 4. Idempotent insert to event_store (<20ms)
    err := h.eventStore.Create(event)
    if err == ErrDuplicate {
        return HTTP 200 // Already processed
    }
    
    // 5. Publish to queue (best-effort, <10ms)
    h.queueClient.Publish(event)
    
    // TOTAL: ~50ms
    return HTTP 200
}
```

**2. Event Processing (Worker Process):**
```go
// cmd/worker/main.go
func startDepositConsumer(mq *queue.Client) {
    mq.Consume("propvest.deposit.confirmed", func(ctx context.Context, body []byte) error {
        event := unmarshal(body)
        
        // Check idempotency
        if h.eventStore.IsCompleted(event.ID) {
            return nil // Already processed
        }
        
        // Update status to "processing"
        h.eventStore.UpdateStatus(event.ID, "processing")
        
        // Process in transaction
        tx := db.Begin()
        defer tx.Rollback()
        
        // Credit wallet
        wallet.Credit(event.AmountKobo)
        
        // Create transaction ledger
        createTransaction(wallet, event)
        
        // Update event store status
        h.eventStore.UpdateStatus(event.ID, "completed")
        
        tx.Commit()
        
        // Queue notification (fire and forget)
        queueNotification(event)
        
        return nil
    })
}
```

**3. Reconciliation (Worker Process):**
```go
// cmd/worker/reconciler.go
func reconcilePendingEvents(ctx context.Context) {
    // Run every 5 minutes
    ticker := time.NewTicker(5 * time.Minute)
    
    for range ticker.C {
        // Find stuck events
        events := eventStore.FindPending(olderThan: 10 * time.Minute)
        
        for event := range events {
            log.Warn("Republishing stuck event", "event_id", event.ID)
            queueClient.Publish(event)
        }
        
        // Find processing events (consumer crashed)
        stuckProcessing := eventStore.FindProcessing(olderThan: 30 * time.Minute)
        
        for event := range stuckProcessing {
            log.Error("Consumer crash detected", "event_id", event.ID)
            eventStore.UpdateStatus(event.ID, "pending")
            queueClient.Publish(event)
        }
    }
}
```

---

## Components and Interfaces

### 1. Event Store (PostgreSQL)

#### Database Schema

```sql
-- Migration: 000008_create_payment_events_table.up.sql
CREATE TABLE payment_events (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Idempotency key (unique constraint prevents duplicates)
    payment_reference VARCHAR(255) NOT NULL,
    event_type        VARCHAR(50) NOT NULL,  -- deposit.confirmed, withdrawal.requested
    provider          VARCHAR(50) NOT NULL,   -- paystack, flutterwave
    
    -- Event lifecycle
    status            VARCHAR(20) NOT NULL DEFAULT 'pending', 
                      -- pending, processing, completed, failed
    received_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    processing_started_at TIMESTAMP,
    processed_at      TIMESTAMP,
    
    -- Payload and context
    raw_payload       JSONB NOT NULL,        -- Original webhook payload
    metadata          JSONB,                 -- Request ID, IP, headers
    
    -- Error tracking
    failure_reason    TEXT,
    retry_count       INT DEFAULT 0,
    
    -- Tracing
    request_id        VARCHAR(100),          -- Distributed tracing ID
    message_id        VARCHAR(100),          -- RabbitMQ message ID
    
    -- Audit
    created_at        TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Idempotency constraint
    CONSTRAINT unique_payment_event UNIQUE (payment_reference, event_type, provider)
);

-- Indexes for fast lookups
CREATE INDEX idx_payment_events_status ON payment_events(status);
CREATE INDEX idx_payment_events_received_at ON payment_events(received_at);
CREATE INDEX idx_payment_events_payment_ref ON payment_events(payment_reference);
CREATE INDEX idx_payment_events_request_id ON payment_events(request_id);
CREATE INDEX idx_payment_events_event_type ON payment_events(event_type);

-- Index for reconciliation queries
CREATE INDEX idx_payment_events_reconciliation 
    ON payment_events(status, received_at) 
    WHERE status IN ('pending', 'processing');
```

#### Go Model

```go
// internal/models/payment_event.go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/datatypes"
)

type PaymentEvent struct {
    ID                  uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    
    // Idempotency
    PaymentReference    string         `gorm:"not null;index"`
    EventType          string         `gorm:"not null;index"`
    Provider           string         `gorm:"not null"`
    
    // Lifecycle
    Status             EventStatus    `gorm:"not null;default:'pending';index"`
    ReceivedAt         time.Time      `gorm:"not null;default:now()"`
    ProcessingStartedAt *time.Time
    ProcessedAt        *time.Time
    
    // Payload
    RawPayload         datatypes.JSON `gorm:"type:jsonb;not null"`
    Metadata           datatypes.JSON `gorm:"type:jsonb"`
    
    // Error tracking
    FailureReason      *string
    RetryCount         int            `gorm:"default:0"`
    
    // Tracing
    RequestID          *string        `gorm:"index"`
    MessageID          *string
    
    CreatedAt          time.Time
    UpdatedAt          time.Time
}

type EventStatus string

const (
    EventStatusPending    EventStatus = "pending"
    EventStatusProcessing EventStatus = "processing"
    EventStatusCompleted  EventStatus = "completed"
    EventStatusFailed     EventStatus = "failed"
)

// TableName overrides the default table name
func (PaymentEvent) TableName() string {
    return "payment_events"
}
```

#### Repository Interface

```go
// internal/repositories/payment_event_repository.go
package repositories

import (
    "context"
    "time"
    "github.com/google/uuid"
    "github.com/mannykings2/propvest-backend/internal/models"
)

type PaymentEventRepository interface {
    // Create inserts a new payment event (idempotent)
    // Returns ErrDuplicateEvent if (payment_reference, event_type, provider) already exists
    Create(ctx context.Context, event *models.PaymentEvent) error
    
    // GetByID retrieves an event by ID
    GetByID(ctx context.Context, id uuid.UUID) (*models.PaymentEvent, error)
    
    // GetByReference retrieves an event by payment reference and type
    GetByReference(ctx context.Context, reference, eventType, provider string) (*models.PaymentEvent, error)
    
    // UpdateStatus updates event status atomically
    UpdateStatus(ctx context.Context, id uuid.UUID, status models.EventStatus) error
    
    // MarkProcessing updates status to processing and sets processing_started_at
    MarkProcessing(ctx context.Context, id uuid.UUID) error
    
    // MarkCompleted updates status to completed and sets processed_at
    MarkCompleted(ctx context.Context, id uuid.UUID) error
    
    // MarkFailed updates status to failed with failure reason
    MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
    
    // IncrementRetryCount increments the retry counter
    IncrementRetryCount(ctx context.Context, id uuid.UUID) error
    
    // FindPendingEvents finds events in pending status older than duration
    FindPendingEvents(ctx context.Context, olderThan time.Duration) ([]*models.PaymentEvent, error)
    
    // FindStuckProcessing finds events in processing status older than duration
    FindStuckProcessing(ctx context.Context, olderThan time.Duration) ([]*models.PaymentEvent, error)
    
    // IsCompleted checks if an event is already completed
    IsCompleted(ctx context.Context, id uuid.UUID) (bool, error)
    
    // GetEventsByUser retrieves all events for a user by payment references
    GetEventsByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.PaymentEvent, int64, error)
}
```

#### Repository Implementation (Excerpt)

```go
// internal/repositories/payment_event_repository_impl.go
package repositories

import (
    "context"
    "errors"
    "time"
    "gorm.io/gorm"
    "github.com/google/uuid"
    "github.com/mannykings2/propvest-backend/internal/models"
    "github.com/mannykings2/propvest-backend/internal/errors"
)

type paymentEventRepository struct {
    db *gorm.DB
}

func NewPaymentEventRepository(db *gorm.DB) PaymentEventRepository {
    return &paymentEventRepository{db: db}
}

func (r *paymentEventRepository) Create(ctx context.Context, event *models.PaymentEvent) error {
    result := r.db.WithContext(ctx).Create(event)
    
    if result.Error != nil {
        // Check for unique constraint violation
        if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
            return apperrors.ErrDuplicateEvent
        }
        return result.Error
    }
    
    return nil
}

func (r *paymentEventRepository) MarkProcessing(ctx context.Context, id uuid.UUID) error {
    now := time.Now()
    return r.db.WithContext(ctx).
        Model(&models.PaymentEvent{}).
        Where("id = ?", id).
        Updates(map[string]interface{}{
            "status": models.EventStatusProcessing,
            "processing_started_at": now,
            "updated_at": now,
        }).Error
}

func (r *paymentEventRepository) FindPendingEvents(ctx context.Context, olderThan time.Duration) ([]*models.PaymentEvent, error) {
    cutoff := time.Now().Add(-olderThan)
    var events []*models.PaymentEvent
    
    err := r.db.WithContext(ctx).
        Where("status = ? AND received_at < ?", models.EventStatusPending, cutoff).
        Order("received_at ASC").
        Limit(100). // Process in batches
        Find(&events).Error
    
    return events, err
}

func (r *paymentEventRepository) IsCompleted(ctx context.Context, id uuid.UUID) (bool, error) {
    var count int64
    err := r.db.WithContext(ctx).
        Model(&models.PaymentEvent{}).
        Where("id = ? AND status = ?", id, models.EventStatusCompleted).
        Count(&count).Error
    
    return count > 0, err
}
```

---

### 2. RabbitMQ Client

#### Queue Configuration

```go
// internal/queue/config.go
package queue

type QueueConfig struct {
    // Queue declaration
    Name       string
    Durable    bool  // Survive broker restart
    AutoDelete bool  // Delete when unused
    Exclusive  bool  // Used by only one connection
    
    // Queue arguments
    MaxLength  int   // Backpressure limit
    TTL        int64 // Message TTL in milliseconds
    Priority   uint8 // Queue priority (0-10)
    
    // DLQ configuration
    DLXName    string // Dead letter exchange
    DLXKey     string // Dead letter routing key
    
    // Consumer configuration
    PrefetchCount int // Messages to fetch at once
    Concurrency   int // Concurrent handlers per consumer
}

var (
    DepositQueueConfig = QueueConfig{
        Name:          "propvest.deposit.confirmed",
        Durable:       true,
        AutoDelete:    false,
        Exclusive:     false,
        MaxLength:     10000,
        TTL:           48 * 60 * 60 * 1000, // 48 hours
        Priority:      5,
        DLXName:       "propvest.dlx",
        DLXKey:        "deposit.dlq",
        PrefetchCount: 10,
        Concurrency:   10,
    }
    
    WithdrawalQueueConfig = QueueConfig{
        Name:          "propvest.withdrawal.requested",
        Durable:       true,
        AutoDelete:    false,
        Exclusive:     false,
        MaxLength:     10000,
        TTL:           48 * 60 * 60 * 1000,
        Priority:      10, // Higher priority (money locked)
        DLXName:       "propvest.dlx",
        DLXKey:        "withdrawal.dlq",
        PrefetchCount: 5,  // Lower concurrency (bank API rate limits)
        Concurrency:   5,
    }
    
    NotificationQueueConfig = QueueConfig{
        Name:          "propvest.notification.dispatch",
        Durable:       true,
        AutoDelete:    false,
        Exclusive:     false,
        MaxLength:     10000,
        TTL:           48 * 60 * 60 * 1000,
        Priority:      1,  // Lowest priority
        DLXName:       "propvest.dlx",
        DLXKey:        "notification.dlq",
        PrefetchCount: 20,
        Concurrency:   20,
    }
    
    ReceiptQueueConfig = QueueConfig{
        Name:          "propvest.receipt.generate",
        Durable:       true,
        AutoDelete:    false,
        Exclusive:     false,
        MaxLength:     10000,
        TTL:           48 * 60 * 60 * 1000,
        Priority:      1,
        DLXName:       "propvest.dlx",
        DLXKey:        "receipt.dlq",
        PrefetchCount: 10,
        Concurrency:   10,
    }
)
```

#### Enhanced Queue Client

```go
// internal/queue/client.go
package queue

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"
    
    amqp "github.com/rabbitmq/amqp091-go"
    "github.com/mannykings2/propvest-backend/internal/logger"
)

type Client struct {
    url           string
    conn          *amqp.Connection
    channel       *amqp.Channel
    publishCh     *amqp.Channel  // Separate channel for publishing
    enabled       bool
    closed        bool
    mu            sync.RWMutex
    
    // Circuit breaker
    circuitBreaker *CircuitBreaker
    
    // Reconnection
    reconnecting   bool
    reconnectMu    sync.Mutex
}

type CircuitBreaker struct {
    failureCount    int
    failureThreshold int
    resetTimeout    time.Duration
    state           CircuitState
    lastFailureTime time.Time
    mu              sync.RWMutex
}

type CircuitState int

const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)

func NewClient(url string, maxRetries int) *Client {
    c := &Client{
        url: url,
        circuitBreaker: &CircuitBreaker{
            failureThreshold: 10,
            resetTimeout:     60 * time.Second,
            state:            CircuitClosed,
        },
    }
    
    if url == "" {
        logger.Warn("RABBITMQ_URL not set; queue disabled")
        return c
    }
    
    if err := c.connect(); err != nil {
        logger.Warn("could not connect to RabbitMQ; running disabled", "error", err)
        return c
    }
    
    c.enabled = true
    c.startReconnectMonitor()
    
    return c
}

func (c *Client) connect() error {
    conn, err := amqp.Dial(c.url)
    if err != nil {
        return fmt.Errorf("dial failed: %w", err)
    }
    
    // Channel for consuming
    ch, err := conn.Channel()
    if err != nil {
        conn.Close()
        return fmt.Errorf("channel failed: %w", err)
    }
    
    // Separate channel for publishing (best practice)
    pubCh, err := conn.Channel()
    if err != nil {
        ch.Close()
        conn.Close()
        return fmt.Errorf("publish channel failed: %w", err)
    }
    
    // Enable publisher confirms
    if err := pubCh.Confirm(false); err != nil {
        pubCh.Close()
        ch.Close()
        conn.Close()
        return fmt.Errorf("confirm mode failed: %w", err)
    }
    
    // Declare exchanges
    if err := c.declareTopology(ch); err != nil {
        pubCh.Close()
        ch.Close()
        conn.Close()
        return fmt.Errorf("topology failed: %w", err)
    }
    
    c.mu.Lock()
    c.conn = conn
    c.channel = ch
    c.publishCh = pubCh
    c.mu.Unlock()
    
    logger.Info("connected to RabbitMQ")
    return nil
}

func (c *Client) declareTopology(ch *amqp.Channel) error {
    // Declare main exchange
    if err := ch.ExchangeDeclare(
        "propvest.events", // name
        "topic",           // type
        true,              // durable
        false,             // auto-deleted
        false,             // internal
        false,             // no-wait
        nil,               // arguments
    ); err != nil {
        return err
    }
    
    // Declare dead-letter exchange
    if err := ch.ExchangeDeclare(
        "propvest.dlx",
        "topic",
        true,
        false,
        false,
        false,
        nil,
    ); err != nil {
        return err
    }
    
    // Declare all queues
    configs := []QueueConfig{
        DepositQueueConfig,
        WithdrawalQueueConfig,
        NotificationQueueConfig,
        ReceiptQueueConfig,
    }
    
    for _, cfg := range configs {
        // Declare main queue
        args := amqp.Table{
            "x-max-length":               cfg.MaxLength,
            "x-message-ttl":              cfg.TTL,
            "x-max-priority":             10,
            "x-dead-letter-exchange":     cfg.DLXName,
            "x-dead-letter-routing-key":  cfg.DLXKey,
        }
        
        _, err := ch.QueueDeclare(
            cfg.Name,
            cfg.Durable,
            cfg.AutoDelete,
            cfg.Exclusive,
            false,
            args,
        )
        if err != nil {
            return fmt.Errorf("queue declare %s: %w", cfg.Name, err)
        }
        
        // Bind to exchange
        if err := ch.QueueBind(
            cfg.Name,
            cfg.Name, // routing key
            "propvest.events",
            false,
            nil,
        ); err != nil {
            return fmt.Errorf("queue bind %s: %w", cfg.Name, err)
        }
        
        // Declare DLQ
        dlqName := cfg.Name + ".dlq"
        _, err = ch.QueueDeclare(
            dlqName,
            true,  // durable
            false, // auto-delete
            false, // exclusive
            false, // no-wait
            amqp.Table{
                "x-message-ttl": int64(7 * 24 * 60 * 60 * 1000), // 7 days
            },
        )
        if err != nil {
            return fmt.Errorf("dlq declare %s: %w", dlqName, err)
        }
        
        // Bind DLQ to DLX
        if err := ch.QueueBind(
            dlqName,
            cfg.DLXKey,
            cfg.DLXName,
            false,
            nil,
        ); err != nil {
            return fmt.Errorf("dlq bind %s: %w", dlqName, err)
        }
    }
    
    return nil
}

func (c *Client) Publish(ctx context.Context, queueName string, event interface{}) error {
    // Check circuit breaker
    if !c.circuitBreaker.CanAttempt() {
        logger.Warn("circuit breaker open; skipping publish", "queue", queueName)
        return nil // Graceful degradation
    }
    
    c.mu.RLock()
    enabled, ch := c.enabled, c.publishCh
    c.mu.RUnlock()
    
    if !enabled || ch == nil {
        logger.Warn("queue disabled; dropping message", "queue", queueName)
        return nil
    }
    
    body, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal failed: %w", err)
    }
    
    pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
    
    err = ch.PublishWithContext(
        pctx,
        "propvest.events", // exchange
        queueName,         // routing key
        false,             // mandatory
        false,             // immediate
        amqp.Publishing{
            ContentType:  "application/json",
            Body:         body,
            DeliveryMode: amqp.Persistent,
            Priority:     5,
            Timestamp:    time.Now(),
            MessageId:    generateMessageID(),
        },
    )
    
    if err != nil {
        c.circuitBreaker.RecordFailure()
        return fmt.Errorf("publish failed: %w", err)
    }
    
    // Wait for confirmation
    select {
    case confirm := <-confirms:
        if !confirm.Ack {
            c.circuitBreaker.RecordFailure()
            return fmt.Errorf("publish not acknowledged")
        }
        c.circuitBreaker.RecordSuccess()
        return nil
    case <-pctx.Done():
        c.circuitBreaker.RecordFailure()
        return fmt.Errorf("publish timeout")
    }
}

func (c *Client) Consume(queueName string, config QueueConfig, handler func(context.Context, []byte) error) error {
    c.mu.RLock()
    enabled, ch := c.enabled, c.channel
    c.mu.RUnlock()
    
    if !enabled || ch == nil {
        logger.Warn("queue disabled; not consuming", "queue", queueName)
        return nil
    }
    
    // Set prefetch
    if err := ch.Qos(config.PrefetchCount, 0, false); err != nil {
        return fmt.Errorf("qos failed: %w", err)
    }
    
    deliveries, err := ch.Consume(
        queueName,
        "",    // consumer tag (auto-generated)
        false, // auto-ack = false (manual ack)
        false, // exclusive
        false, // no-local
        false, // no-wait
        nil,   // args
    )
    if err != nil {
        return fmt.Errorf("consume failed: %w", err)
    }
    
    // Start concurrent workers
    for i := 0; i < config.Concurrency; i++ {
        go c.consumeWorker(queueName, deliveries, handler)
    }
    
    return nil
}

func (c *Client) consumeWorker(queueName string, deliveries <-chan amqp.Delivery, handler func(context.Context, []byte) error) {
    logger.Info("consumer worker started", "queue", queueName)
    
    for d := range deliveries {
        ctx := context.Background()
        
        retryCount := 0
        if val, ok := d.Headers["x-retry-count"].(int32); ok {
            retryCount = int(val)
        }
        
        if err := handler(ctx, d.Body); err != nil {
            logger.Error("message handler failed",
                "queue", queueName,
                "retry_count", retryCount,
                "error", err)
            
            // Check retry limit
            if retryCount >= 3 {
                logger.Error("max retries reached; sending to DLQ",
                    "queue", queueName,
                    "message_id", d.MessageId)
                
                // Send to DLQ (already configured via x-dead-letter-exchange)
                d.Nack(false, false) // requeue=false sends to DLQ
            } else {
                // Retry with exponential backoff
                delay := calculateBackoff(retryCount)
                logger.Info("requeueing with backoff",
                    "queue", queueName,
                    "retry_count", retryCount + 1,
                    "delay", delay)
                
                // Publish with delay (using delayed message plugin or delay queue)
                c.publishWithDelay(queueName, d.Body, retryCount+1, delay)
                d.Ack(false) // Remove original message
            }
            continue
        }
        
        // Success
        d.Ack(false)
    }
    
    logger.Warn("consumer channel closed", "queue", queueName)
}

func calculateBackoff(retryCount int) time.Duration {
    delays := []time.Duration{
        5 * time.Second,
        15 * time.Second,
        45 * time.Second,
    }
    
    if retryCount < len(delays) {
        return delays[retryCount]
    }
    
    return delays[len(delays)-1]
}

func (c *Client) publishWithDelay(queueName string, body []byte, retryCount int, delay time.Duration) {
    // TODO: Implement delay using RabbitMQ delayed message plugin
    // or temporary delay queue
    time.Sleep(delay)
    
    ctx := context.Background()
    var event map[string]interface{}
    json.Unmarshal(body, &event)
    c.Publish(ctx, queueName, event)
}

func (c *Client) startReconnectMonitor() {
    go func() {
        for {
            c.mu.RLock()
            conn := c.conn
            c.mu.RUnlock()
            
            if conn == nil {
                time.Sleep(5 * time.Second)
                continue
            }
            
            // Wait for connection close
            connErr := <-conn.NotifyClose(make(chan *amqp.Error))
            
            logger.Error("RabbitMQ connection closed", "error", connErr)
            
            // Attempt reconnection
            c.reconnect()
        }
    }()
}

func (c *Client) reconnect() {
    c.reconnectMu.Lock()
    if c.reconnecting {
        c.reconnectMu.Unlock()
        return
    }
    c.reconnecting = true
    c.reconnectMu.Unlock()
    
    defer func() {
        c.reconnectMu.Lock()
        c.reconnecting = false
        c.reconnectMu.Unlock()
    }()
    
    backoff := 1 * time.Second
    maxBackoff := 60 * time.Second
    
    for {
        logger.Info("attempting reconnection...", "backoff", backoff)
        
        if err := c.connect(); err != nil {
            logger.Error("reconnection failed", "error", err, "retry_in", backoff)
            time.Sleep(backoff)
            
            // Exponential backoff
            backoff *= 2
            if backoff > maxBackoff {
                backoff = maxBackoff
            }
            continue
        }
        
        logger.Info("reconnected to RabbitMQ")
        c.enabled = true
        return
    }
}

func (c *Client) Close() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if c.closed {
        return
    }
    
    c.closed = true
    c.enabled = false
    
    if c.channel != nil {
        c.channel.Close()
    }
    if c.publishCh != nil {
        c.publishCh.Close()
    }
    if c.conn != nil {
        c.conn.Close()
    }
}

// Circuit breaker methods
func (cb *CircuitBreaker) CanAttempt() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    
    if cb.state == CircuitClosed {
        return true
    }
    
    if cb.state == CircuitOpen {
        if time.Since(cb.lastFailureTime) > cb.resetTimeout {
            cb.mu.RUnlock()
            cb.mu.Lock()
            cb.state = CircuitHalfOpen
            cb.mu.Unlock()
            cb.mu.RLock()
            return true
        }
        return false
    }
    
    return true // Half-open
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    cb.failureCount = 0
    cb.state = CircuitClosed
}

func (cb *CircuitBreaker) RecordFailure() {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    cb.failureCount++
    cb.lastFailureTime = time.Now()
    
    if cb.failureCount >= cb.failureThreshold {
        cb.state = CircuitOpen
        logger.Warn("circuit breaker opened", "failure_count", cb.failureCount)
    }
}

func generateMessageID() string {
    return fmt.Sprintf("%d-%d", time.Now().UnixNano(), randInt())
}

func randInt() int {
    // Use crypto/rand for production
    return int(time.Now().UnixNano() % 1000000)
}
```

---

## Data Models

### Message Schemas

```go
// internal/queue/messages.go
package queue

import (
    "time"
    "github.com/google/uuid"
)

// BaseEvent contains fields common to all event types
type BaseEvent struct {
    EventID          uuid.UUID  `json:"event_id"`
    EventType        string     `json:"event_type"`
    PaymentReference string     `json:"payment_reference"`
    Timestamp        time.Time  `json:"timestamp"`
    RequestID        string     `json:"request_id,omitempty"`
}

// DepositEvent is published when a deposit is confirmed
type DepositEvent struct {
    BaseEvent
    
    UserID       uuid.UUID `json:"user_id"`
    AmountKobo   int64     `json:"amount_kobo"`
    Currency     string    `json:"currency"`
    Provider     string    `json:"provider"`
    
    // Metadata from provider
    CustomerEmail string `json:"customer_email,omitempty"`
    Channel       string `json:"channel,omitempty"` // card, bank_transfer, ussd
}

// WithdrawalEvent is published when a withdrawal is requested
type WithdrawalEvent struct {
    BaseEvent
    
    UserID            uuid.UUID `json:"user_id"`
    TransactionID     uuid.UUID `json:"transaction_id"`
    AmountKobo        int64     `json:"amount_kobo"`
    Currency          string    `json:"currency"`
    
    // Bank details
    AccountNumber     string    `json:"account_number"`
    AccountName       string    `json:"account_name"`
    BankCode          string    `json:"bank_code"`
    BankName          string    `json:"bank_name"`
}

// NotificationEvent is published to trigger user notifications
type NotificationEvent struct {
    BaseEvent
    
    UserID         uuid.UUID           `json:"user_id"`
    NotificationType string            `json:"notification_type"` // email, sms, push
    Template       string              `json:"template"`
    Data           map[string]string   `json:"data"`
    
    // Contact details
    Email          string              `json:"email,omitempty"`
    PhoneNumber    string              `json:"phone_number,omitempty"`
    DeviceTokens   []string            `json:"device_tokens,omitempty"`
}

// ReceiptEvent is published to generate and email a receipt
type ReceiptEvent struct {
    BaseEvent
    
    UserID         uuid.UUID `json:"user_id"`
    TransactionID  uuid.UUID `json:"transaction_id"`
    TransactionType string   `json:"transaction_type"` // deposit, withdrawal
    AmountKobo     int64     `json:"amount_kobo"`
    Email          string    `json:"email"`
}

// Event type constants
const (
    EventTypeDepositConfirmed    = "deposit.confirmed"
    EventTypeWithdrawalRequested = "withdrawal.requested"
    EventTypeNotificationDispatch = "notification.dispatch"
    EventTypeReceiptGenerate     = "receipt.generate"
)
```

### JSON Schema Validation

```go
// internal/queue/validation.go
package queue

import (
    "encoding/json"
    "fmt"
    "github.com/xeipuuv/gojsonschema"
)

var depositEventSchema = `{
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "required": ["event_id", "event_type", "payment_reference", "user_id", "amount_kobo"],
    "properties": {
        "event_id": {"type": "string", "format": "uuid"},
        "event_type": {"type": "string", "enum": ["deposit.confirmed"]},
        "payment_reference": {"type": "string", "minLength": 1},
        "timestamp": {"type": "string", "format": "date-time"},
        "user_id": {"type": "string", "format": "uuid"},
        "amount_kobo": {"type": "integer", "minimum": 1},
        "currency": {"type": "string", "enum": ["NGN"]},
        "provider": {"type": "string", "enum": ["paystack", "flutterwave"]}
    }
}`

func ValidateDepositEvent(data []byte) error {
    schemaLoader := gojsonschema.NewStringLoader(depositEventSchema)
    documentLoader := gojsonschema.NewBytesLoader(data)
    
    result, err := gojsonschema.Validate(schemaLoader, documentLoader)
    if err != nil {
        return err
    }
    
    if !result.Valid() {
        var errs string
        for _, desc := range result.Errors() {
            errs += fmt.Sprintf("- %s\n", desc)
        }
        return fmt.Errorf("schema validation failed:\n%s", errs)
    }
    
    return nil
}

// Similar functions for WithdrawalEvent, NotificationEvent, ReceiptEvent
```

---

## Sequence Diagrams

### 1. Deposit Processing Flow

```
┌─────────┐     ┌─────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌─────────┐
│Paystack │     │ Webhook │     │  Event   │     │ RabbitMQ │     │ Deposit  │     │ Wallet  │
│         │     │ Handler │     │  Store   │     │          │     │ Consumer │     │ Service │
└────┬────┘     └────┬────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬────┘
     │               │               │                │                │                │
     │ POST /webhook │               │                │                │                │
     │ charge.success│               │                │                │                │
     ├──────────────▶│               │                │                │                │
     │               │               │                │                │                │
     │               │ 1. Verify     │                │                │                │
     │               │   signature   │                │                │                │
     │               │   (<10ms)     │                │                │                │
     │               │               │                │                │                │
     │               │ 2. INSERT INTO│                │                │                │
     │               │   payment_events              │                │                │
     │               │   (idempotent)│                │                │                │
     │               ├──────────────▶│                │                │                │
     │               │               │                │                │                │
     │               │◀──────────────┤                │                │                │
     │               │   OK / DUPLICATE               │                │                │
     │               │               │                │                │                │
     │               │ 3. Publish to │                │                │                │
     │               │   deposit queue                │                │                │
     │               ├───────────────┼───────────────▶│                │                │
     │               │               │                │                │                │
     │◀──────────────┤               │                │                │                │
     │  200 OK       │               │                │                │                │
     │  (<50ms)      │               │                │                │                │
     │               │               │                │                │                │
     │               │               │                │ 4. Consume msg │                │
     │               │               │                ├───────────────▶│                │
     │               │               │                │                │                │
     │               │               │                │                │ 5. Check       │
     │               │               │                │                │   idempotency  │
     │               │               │ IsCompleted()?│                │                │
     │               │               │◀───────────────┼────────────────┤                │
     │               │               │ false          │                │                │
     │               │               ├───────────────▶│                │                │
     │               │               │                │                │                │
     │               │               │MarkProcessing()                │                │
     │               │               │◀───────────────┼────────────────┤                │
     │               │               ├───────────────▶│                │                │
     │               │               │                │                │                │
     │               │               │                │                │ 6. BEGIN TX    │
     │               │               │                │                │                │
     │               │               │                │                │ 7. Credit      │
     │               │               │                │                │   wallet       │
     │               │               │                │                ├───────────────▶│
     │               │               │                │                │◀───────────────┤
     │               │               │                │                │                │
     │               │               │                │                │ 8. Create      │
     │               │               │                │                │   transaction  │
     │               │               │                │                │   ledger       │
     │               │               │                │                │                │
     │               │               │ MarkCompleted()                │                │
     │               │               │◀───────────────┼────────────────┤                │
     │               │               ├───────────────▶│                │                │
     │               │               │                │                │                │
     │               │               │                │                │ 9. COMMIT TX   │
     │               │               │                │                │                │
     │               │               │                │◀───────────────┤                │
     │               │               │                │   ACK          │                │
     │               │               │                │                │                │
     │               │               │                │                │10. Queue       │
     │               │               │                │                │   notification │
     │               │               │                │◀───────────────┤                │
     └───────────────┴───────────────┴────────────────┴────────────────┴────────────────┴────────
```

### 2. Withdrawal Processing Flow

```
┌─────────┐     ┌─────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌─────────┐
│  User   │     │   API   │     │  Wallet  │     │ RabbitMQ │     │Withdrawal│     │Paystack │
│         │     │ Handler │     │  Service │     │          │     │ Consumer │     │   API   │
└────┬────┘     └────┬────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬────┘
     │               │               │                │                │                │
     │ POST /wallet/ │               │                │                │                │
     │ withdraw      │               │                │                │                │
     ├──────────────▶│               │                │                │                │
     │               │               │                │                │                │
     │               │ 1. BEGIN TX   │                │                │                │
     │               │               │                │                │                │
     │               │ 2. Lock wallet│                │                │                │
     │               ├──────────────▶│                │                │                │
     │               │               │                │                │                │
     │               │ 3. Check      │                │                │                │
     │               │   balance     │                │                │                │
     │               │               │                │                │                │
     │               │ 4. Debit      │                │                │                │
     │               │   wallet      │                │                │                │
     │               │               │                │                │                │
     │               │ 5. Create     │                │                │                │
     │               │   pending txn │                │                │                │
     │               │               │                │                │                │
     │               │ 6. COMMIT TX  │                │                │                │
     │               │◀──────────────┤                │                │                │
     │               │               │                │                │                │
     │               │ 7. Queue      │                │                │                │
     │               │   withdrawal  │                │                │                │
     │               ├───────────────┼───────────────▶│                │                │
     │               │               │                │                │                │
     │◀──────────────┤               │                │                │                │
     │ 200 OK        │               │                │                │                │
     │ status:pending│               │                │                │                │
     │               │               │                │                │                │
     │               │               │                │ 8. Consume msg │                │
     │               │               │                ├───────────────▶│                │
     │               │               │                │                │                │
     │               │               │                │                │ 9. Initiate    │
     │               │               │                │                │   transfer     │
     │               │               │                │                ├───────────────▶│
     │               │               │                │                │                │
     │               │               │                │                │◀───────────────┤
     │               │               │                │                │ transfer_code  │
     │               │               │                │                │ status:pending │
     │               │               │                │                │                │
     │               │               │                │◀───────────────┤                │
     │               │               │                │   ACK          │                │
     │               │               │                │                │                │
     │               │               │                │                │                │
     │               │               │ (Later: Paystack webhook or reconciliation)     │
     │               │               │                │                │                │
     │               │ Webhook:      │                │                │                │
     │               │ transfer.success              │                │                │
     │               ├──────────────▶│                │                │                │
     │               │               │                │                │                │
     │               │               │10. Finalize    │                │                │
     │               │               │   withdrawal   │                │                │
     │               │               │   (completed)  │                │                │
     └───────────────┴───────────────┴────────────────┴────────────────┴────────────────┴────────
```

### 3. DLQ Handling Flow

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ Consumer │     │  Primary │     │   DLQ    │     │Monitoring│     │ Engineer │
│          │     │  Queue   │     │          │     │Dashboard │     │          │
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │                │
     │ 1. Consume msg │                │                │                │
     ├───────────────▶│                │                │                │
     │                │                │                │                │
     │ 2. Process     │                │                │                │
     │   (fails)      │                │                │                │
     │                │                │                │                │
     │ 3. NACK +      │                │                │                │
     │   requeue      │                │                │                │
     │   retry_count++│                │                │                │
     ├───────────────▶│                │                │                │
     │                │                │                │                │
     │ 4. Consume msg │                │                │                │
     │   (retry #2)   │                │                │                │
     ├───────────────▶│                │                │                │
     │                │                │                │                │
     │ 5. Process     │                │                │                │
     │   (fails again)│                │                │                │
     │                │                │                │                │
     │ 6. NACK +      │                │                │                │
     │   requeue      │                │                │                │
     │   retry_count++│                │                │                │
     ├───────────────▶│                │                │                │
     │                │                │                │                │
     │ 7. Consume msg │                │                │                │
     │   (retry #3)   │                │                │                │
     ├───────────────▶│                │                │                │
     │                │                │                │                │
     │ 8. Process     │                │                │                │
     │   (fails 3rd time)              │                │                │
     │                │                │                │                │
     │ 9. Max retries │                │                │                │
     │   reached      │                │                │                │
     │                │                │                │                │
     │10. NACK        │                │                │                │
     │   requeue=false│                │                │                │
     │   (send to DLQ)│                │                │                │
     ├───────────────▶│                │                │                │
     │                │                │                │                │
     │                │11. Route to DLQ                │                │
     │                │   via x-dead-letter-exchange   │                │
     │                ├───────────────▶│                │                │
     │                │                │                │                │
     │                │                │12. DLQ depth>5 │                │
     │                │                │   (alert)      │                │
     │                │                ├───────────────▶│                │
     │                │                │                │                │
     │                │                │                │13. Email alert │
     │                │                │                ├───────────────▶│
     │                │                │                │                │
     │                │                │                │14. Investigate │
     │                │                │                │   DLQ message  │
     │                │                │                │                │
     │                │                │15. Fix issue   │                │
     │                │                │   (deploy code)│                │
     │                │                │                │                │
     │                │                │16. Requeue from│                │
     │                │                │   DLQ via UI   │                │
     │                │◀───────────────┼────────────────┼────────────────┤
     │                │                │                │                │
     │17. Consume     │                │                │                │
     │   (retry after │                │                │                │
     │   fix)         │                │                │                │
     ├───────────────▶│                │                │                │
     │                │                │                │                │
     │18. Process     │                │                │                │
     │   SUCCESS ✓    │                │                │                │
     │                │                │                │                │
     │19. ACK         │                │                │                │
     ├───────────────▶│                │                │                │
     └────────────────┴────────────────┴────────────────┴────────────────┴────────
```

### 4. Reconciliation Flow

```
┌───────────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│ Reconciliation│     │  Event   │     │ RabbitMQ │     │ Consumer │
│    Worker     │     │  Store   │     │          │     │          │
└───────┬───────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
        │                  │                │                │
        │ (Every 5 min)    │                │                │
        │                  │                │                │
        │ 1. SELECT * FROM│                │                │
        │   payment_events│                │                │
        │   WHERE status='pending'         │                │
        │   AND received_at < NOW() - 10min               │
        ├─────────────────▶│                │                │
        │                  │                │                │
        │◀─────────────────┤                │                │
        │ [stuck events]   │                │                │
        │                  │                │                │
        │ 2. For each event:               │                │
        │   Republish to queue             │                │
        ├──────────────────┼───────────────▶│                │
        │                  │                │                │
        │                  │                │ 3. Consume     │
        │                  │                ├───────────────▶│
        │                  │                │                │
        │                  │                │                │
        │ 4. SELECT * FROM│                │                │
        │   payment_events│                │                │
        │   WHERE status='processing'      │                │
        │   AND processing_started_at <    │                │
        │   NOW() - 30min                  │                │
        ├─────────────────▶│                │                │
        │                  │                │                │
        │◀─────────────────┤                │                │
        │ [crashed consumer│                │                │
        │  events]         │                │                │
        │                  │                │                │
        │ 5. UPDATE status│                │                │
        │   = 'pending'   │                │                │
        ├─────────────────▶│                │                │
        │                  │                │                │
        │ 6. Republish    │                │                │
        ├──────────────────┼───────────────▶│                │
        │                  │                │                │
        │                  │                │ 7. Consume     │
        │                  │                ├───────────────▶│
        │                  │                │                │
        │ 8. Log metrics: │                │                │
        │   - events_reconciled_count      │                │
        │   - events_pending_count         │                │
        │   - events_stuck_count           │                │
        └──────────────────┴────────────────┴────────────────┴────────
```

---

## Error Handling

### Error Classification

```go
// internal/queue/errors.go
package queue

import "errors"

var (
    // Transient errors (should retry)
    ErrTransient = errors.New("transient error")
    ErrDatabaseConnection = errors.New("database connection failed")
    ErrDatabaseTimeout = errors.New("database query timeout")
    ErrProviderTimeout = errors.New("payment provider timeout")
    ErrProviderRateLimit = errors.New("payment provider rate limit")
    ErrQueuePublishFailed = errors.New("queue publish failed")
    
    // Permanent errors (should not retry, send to DLQ)
    ErrPermanent = errors.New("permanent error")
    ErrInvalidEvent = errors.New("invalid event schema")
    ErrInvalidAmount = errors.New("invalid amount")
    ErrUserNotFound = errors.New("user not found")
    ErrWalletNotFound = errors.New("wallet not found")
    ErrInsufficientBalance = errors.New("insufficient balance")
    ErrBusinessRule = errors.New("business rule violation")
)

func IsTransient(err error) bool {
    return errors.Is(err, ErrTransient) ||
           errors.Is(err, ErrDatabaseConnection) ||
           errors.Is(err, ErrDatabaseTimeout) ||
           errors.Is(err, ErrProviderTimeout) ||
           errors.Is(err, ErrProviderRateLimit) ||
           errors.Is(err, ErrQueuePublishFailed)
}

func IsPermanent(err error) bool {
    return errors.Is(err, ErrPermanent) ||
           errors.Is(err, ErrInvalidEvent) ||
           errors.Is(err, ErrInvalidAmount) ||
           errors.Is(err, ErrUserNotFound) ||
           errors.Is(err, ErrWalletNotFound) ||
           errors.Is(err, ErrInsufficientBalance) ||
           errors.Is(err, ErrBusinessRule)
}
```

### Retry Strategy

```go
// internal/queue/retry.go
package queue

import (
    "time"
    "math"
)

type RetryPolicy struct {
    MaxRetries     int
    InitialBackoff time.Duration
    MaxBackoff     time.Duration
    Multiplier     float64
}

var DefaultRetryPolicy = RetryPolicy{
    MaxRetries:     3,
    InitialBackoff: 5 * time.Second,
    MaxBackoff:     45 * time.Second,
    Multiplier:     3.0,
}

func (p *RetryPolicy) GetDelay(attemptNumber int) time.Duration {
    if attemptNumber <= 0 {
        return p.InitialBackoff
    }
    
    delay := float64(p.InitialBackoff) * math.Pow(p.Multiplier, float64(attemptNumber-1))
    
    if time.Duration(delay) > p.MaxBackoff {
        return p.MaxBackoff
    }
    
    return time.Duration(delay)
}

// Example: 
// Retry 1: 5s * 3^0 = 5s
// Retry 2: 5s * 3^1 = 15s
// Retry 3: 5s * 3^2 = 45s
```

### Circuit Breaker Implementation

```go
// internal/queue/circuit_breaker.go (already shown above in Client)

// State transitions:
// CLOSED --[10 failures]--> OPEN --[60s timeout]--> HALF_OPEN --[success]--> CLOSED
//                                                              --[failure]--> OPEN
```

---

## Testing Strategy

### Unit Tests

**Target Coverage**: 80%+

**Focus Areas**:
1. **Event Store Repository**:
   - Idempotency (duplicate insert returns ErrDuplicate)
   - Status transitions (pending → processing → completed)
   - Reconciliation queries (pending events, stuck processing)
   
2. **Queue Client**:
   - Message serialization/deserialization
   - Schema validation
   - Circuit breaker state transitions
   - Reconnection logic (mocked)
   
3. **Message Handlers**:
   - Deposit consumer logic
   - Withdrawal consumer logic
   - Idempotency checks
   - Error classification (transient vs permanent)

**Example Unit Test**:

```go
// internal/repositories/payment_event_repository_test.go
package repositories_test

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/mannykings2/propvest-backend/internal/models"
    "github.com/mannykings2/propvest-backend/internal/repositories"
    "github.com/mannykings2/propvest-backend/internal/errors"
)

func TestPaymentEventRepository_Create_Idempotency(t *testing.T) {
    db := setupTestDB(t)
    repo := repositories.NewPaymentEventRepository(db)
    
    ctx := context.Background()
    
    event := &models.PaymentEvent{
        PaymentReference: "DEP-12345",
        EventType:        "deposit.confirmed",
        Provider:         "paystack",
        Status:           models.EventStatusPending,
        RawPayload:       []byte(`{"amount": 10000}`),
    }
    
    // First insert should succeed
    err := repo.Create(ctx, event)
    require.NoError(t, err)
    
    // Second insert with same (reference, type, provider) should fail
    duplicate := &models.PaymentEvent{
        PaymentReference: "DEP-12345",
        EventType:        "deposit.confirmed",
        Provider:         "paystack",
        Status:           models.EventStatusPending,
        RawPayload:       []byte(`{"amount": 10000}`),
    }
    
    err = repo.Create(ctx, duplicate)
    assert.Error(t, err)
    assert.ErrorIs(t, err, apperrors.ErrDuplicateEvent)
}

func TestPaymentEventRepository_FindPendingEvents(t *testing.T) {
    db := setupTestDB(t)
    repo := repositories.NewPaymentEventRepository(db)
    
    ctx := context.Background()
    
    // Create old pending event
    old := &models.PaymentEvent{
        PaymentReference: "DEP-OLD",
        EventType:        "deposit.confirmed",
        Provider:         "paystack",
        Status:           models.EventStatusPending,
        ReceivedAt:       time.Now().Add(-15 * time.Minute),
        RawPayload:       []byte(`{}`),
    }
    repo.Create(ctx, old)
    
    // Create recent pending event
    recent := &models.PaymentEvent{
        PaymentReference: "DEP-RECENT",
        EventType:        "deposit.confirmed",
        Provider:         "paystack",
        Status:           models.EventStatusPending,
        ReceivedAt:       time.Now().Add(-5 * time.Minute),
        RawPayload:       []byte(`{}`),
    }
    repo.Create(ctx, recent)
    
    // Find events older than 10 minutes
    events, err := repo.FindPendingEvents(ctx, 10*time.Minute)
    require.NoError(t, err)
    
    // Should only return old event
    assert.Len(t, events, 1)
    assert.Equal(t, "DEP-OLD", events[0].PaymentReference)
}
```

### Integration Tests

**Target Coverage**: Critical flows

**Focus Areas**:
1. **End-to-End Deposit Flow**:
   - Webhook → Event Store → RabbitMQ → Consumer → Wallet Credit
   - Test with real PostgreSQL and RabbitMQ (testcontainers)
   
2. **Idempotency**:
   - Send duplicate webhook
   - Verify wallet credited only once
   
3. **Retry Logic**:
   - Simulate transient failure
   - Verify exponential backoff
   - Verify max retries → DLQ
   
4. **Reconciliation**:
   - Create stuck pending event
   - Run reconciliation
   - Verify republished to queue

**Example Integration Test**:

```go
// tests/integration/deposit_flow_test.go
package integration_test

import (
    "context"
    "testing"
    "time"
    "github.com/stretchr/testify/require"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

func TestDepositFlow_EndToEnd(t *testing.T) {
    // Start PostgreSQL container
    postgresC := startPostgresContainer(t)
    defer postgresC.Terminate(context.Background())
    
    // Start RabbitMQ container
    rabbitmqC := startRabbitMQContainer(t)
    defer rabbitmqC.Terminate(context.Background())
    
    // Initialize app with test containers
    app := setupTestApp(t, postgresC, rabbitmqC)
    
    // Create test user and wallet
    user := createTestUser(t, app.DB)
    wallet := createTestWallet(t, app.DB, user.ID)
    
    initialBalance := wallet.MainBalance
    
    // Send deposit webhook
    webhookPayload := map[string]interface{}{
        "event": "charge.success",
        "data": map[string]interface{}{
            "reference": "DEP-TEST-123",
            "amount":    150000, // ₦1,500.00
            "status":    "success",
            "customer": map[string]interface{}{
                "email": user.Email,
            },
        },
    }
    
    resp := sendWebhook(t, app, webhookPayload)
    require.Equal(t, 200, resp.StatusCode)
    
    // Wait for consumer to process (async)
    time.Sleep(2 * time.Second)
    
    // Verify wallet credited
    updatedWallet := getWallet(t, app.DB, wallet.ID)
    require.Equal(t, initialBalance+150000, updatedWallet.MainBalance)
    
    // Verify transaction created
    txn := getTransactionByReference(t, app.DB, "DEP-TEST-123")
    require.NotNil(t, txn)
    require.Equal(t, "deposit", txn.Type)
    require.Equal(t, int64(150000), txn.Amount)
    require.Equal(t, "completed", txn.Status)
    
    // Verify event marked completed
    event := getEventByReference(t, app.DB, "DEP-TEST-123")
    require.Equal(t, "completed", event.Status)
    require.NotNil(t, event.ProcessedAt)
    
    // Send duplicate webhook
    resp = sendWebhook(t, app, webhookPayload)
    require.Equal(t, 200, resp.StatusCode)
    
    // Wait
    time.Sleep(2 * time.Second)
    
    // Verify balance NOT changed (idempotency)
    finalWallet := getWallet(t, app.DB, wallet.ID)
    require.Equal(t, initialBalance+150000, finalWallet.MainBalance)
}
```

### Property-Based Tests (Optional)

**Not applicable** for this feature. Property-based testing (PBT) works best for:
- Pure functions with clear mathematical properties
- Parsers/serializers (round-trip properties)
- Algorithms with invariants

Queue system involves:
- External systems (RabbitMQ, PostgreSQL)
- Time-dependent behavior (reconciliation timing)
- Side effects (wallet updates, notifications)
- Infrastructure configuration

**Testing strategy instead**:
- Unit tests for pure logic (retry calculation, error classification)
- Integration tests for system behavior
- End-to-end tests for critical flows
- Manual testing for DLQ handling and monitoring

---

## Configuration

### Environment Variables

```bash
# .env
# RabbitMQ Configuration
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
QUEUE_MAX_RETRIES=3
QUEUE_RETRY_DELAY_BASE=5s
QUEUE_CONSUMER_CONCURRENCY=10

# Queue-specific overrides
DEPOSIT_QUEUE_CONCURRENCY=10
WITHDRAWAL_QUEUE_CONCURRENCY=5
NOTIFICATION_QUEUE_CONCURRENCY=20
RECEIPT_QUEUE_CONCURRENCY=10

# Reconciliation
RECONCILIATION_INTERVAL=5m
RECONCILIATION_PENDING_THRESHOLD=10m
RECONCILIATION_STUCK_THRESHOLD=30m

# Circuit breaker
CIRCUIT_BREAKER_FAILURE_THRESHOLD=10
CIRCUIT_BREAKER_RESET_TIMEOUT=60s
```

### Configuration Loading

```go
// internal/config/queue_config.go
package config

import (
    "time"
    "github.com/spf13/viper"
)

type QueueConfig struct {
    RabbitMQURL               string
    MaxRetries                int
    RetryDelayBase            time.Duration
    ConsumerConcurrency       int
    
    // Per-queue overrides
    DepositConcurrency        int
    WithdrawalConcurrency     int
    NotificationConcurrency   int
    ReceiptConcurrency        int
    
    // Reconciliation
    ReconciliationInterval    time.Duration
    PendingThreshold          time.Duration
    StuckThreshold            time.Duration
    
    // Circuit breaker
    CircuitBreakerFailures    int
    CircuitBreakerResetTimeout time.Duration
}

func LoadQueueConfig() *QueueConfig {
    viper.SetDefault("RABBITMQ_URL", "")
    viper.SetDefault("QUEUE_MAX_RETRIES", 3)
    viper.SetDefault("QUEUE_RETRY_DELAY_BASE", "5s")
    viper.SetDefault("QUEUE_CONSUMER_CONCURRENCY", 10)
    
    viper.SetDefault("DEPOSIT_QUEUE_CONCURRENCY", 10)
    viper.SetDefault("WITHDRAWAL_QUEUE_CONCURRENCY", 5)
    viper.SetDefault("NOTIFICATION_QUEUE_CONCURRENCY", 20)
    viper.SetDefault("RECEIPT_QUEUE_CONCURRENCY", 10)
    
    viper.SetDefault("RECONCILIATION_INTERVAL", "5m")
    viper.SetDefault("RECONCILIATION_PENDING_THRESHOLD", "10m")
    viper.SetDefault("RECONCILIATION_STUCK_THRESHOLD", "30m")
    
    viper.SetDefault("CIRCUIT_BREAKER_FAILURE_THRESHOLD", 10)
    viper.SetDefault("CIRCUIT_BREAKER_RESET_TIMEOUT", "60s")
    
    return &QueueConfig{
        RabbitMQURL:               viper.GetString("RABBITMQ_URL"),
        MaxRetries:                viper.GetInt("QUEUE_MAX_RETRIES"),
        RetryDelayBase:            viper.GetDuration("QUEUE_RETRY_DELAY_BASE"),
        ConsumerConcurrency:       viper.GetInt("QUEUE_CONSUMER_CONCURRENCY"),
        
        DepositConcurrency:        viper.GetInt("DEPOSIT_QUEUE_CONCURRENCY"),
        WithdrawalConcurrency:     viper.GetInt("WITHDRAWAL_QUEUE_CONCURRENCY"),
        NotificationConcurrency:   viper.GetInt("NOTIFICATION_QUEUE_CONCURRENCY"),
        ReceiptConcurrency:        viper.GetInt("RECEIPT_QUEUE_CONCURRENCY"),
        
        ReconciliationInterval:    viper.GetDuration("RECONCILIATION_INTERVAL"),
        PendingThreshold:          viper.GetDuration("RECONCILIATION_PENDING_THRESHOLD"),
        StuckThreshold:            viper.GetDuration("RECONCILIATION_STUCK_THRESHOLD"),
        
        CircuitBreakerFailures:    viper.GetInt("CIRCUIT_BREAKER_FAILURE_THRESHOLD"),
        CircuitBreakerResetTimeout: viper.GetDuration("CIRCUIT_BREAKER_RESET_TIMEOUT"),
    }
}

func (c *QueueConfig) Validate() error {
    if c.MaxRetries < 0 || c.MaxRetries > 10 {
        return fmt.Errorf("QUEUE_MAX_RETRIES must be 0-10")
    }
    
    if c.ConsumerConcurrency < 1 || c.ConsumerConcurrency > 100 {
        return fmt.Errorf("QUEUE_CONSUMER_CONCURRENCY must be 1-100")
    }
    
    if c.ReconciliationInterval < time.Minute {
        return fmt.Errorf("RECONCILIATION_INTERVAL must be >= 1m")
    }
    
    return nil
}
```

---

## Observability

### Metrics Collection

```go
// internal/metrics/queue_metrics.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Queue depth (gauge)
    QueueDepth = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "propvest_queue_depth",
            Help: "Number of messages in queue",
        },
        []string{"queue"},
    )
    
    // Messages processed (counter)
    MessagesProcessed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "propvest_messages_processed_total",
            Help: "Total messages processed",
        },
        []string{"queue", "status"}, // status: success, failure, retry
    )
    
    // Processing time (histogram)
    ProcessingDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "propvest_message_processing_duration_seconds",
            Help:    "Message processing duration",
            Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
        },
        []string{"queue"},
    )
    
    // DLQ depth (gauge)
    DLQDepth = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "propvest_dlq_depth",
            Help: "Number of messages in DLQ",
        },
        []string{"queue"},
    )
    
    // Circuit breaker state (gauge)
    CircuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "propvest_circuit_breaker_state",
            Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
        },
        []string{"component"},
    )
    
    // Reconciliation metrics
    ReconciliationRuns = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "propvest_reconciliation_runs_total",
            Help: "Total reconciliation runs",
        },
    )
    
    ReconciliationEventsRepublished = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "propvest_reconciliation_events_republished_total",
            Help: "Total events republished by reconciliation",
        },
    )
)
```

### Metrics Collection in Consumer

```go
// cmd/worker/metrics.go
func (c *Consumer) ConsumeWithMetrics(queueName string, handler func(context.Context, []byte) error) {
    c.queueClient.Consume(queueName, config, func(ctx context.Context, body []byte) error {
        start := time.Now()
        
        err := handler(ctx, body)
        
        duration := time.Since(start).Seconds()
        metrics.ProcessingDuration.WithLabelValues(queueName).Observe(duration)
        
        if err != nil {
            if IsTransient(err) {
                metrics.MessagesProcessed.WithLabelValues(queueName, "retry").Inc()
            } else {
                metrics.MessagesProcessed.WithLabelValues(queueName, "failure").Inc()
            }
            return err
        }
        
        metrics.MessagesProcessed.WithLabelValues(queueName, "success").Inc()
        return nil
    })
}
```

### Prometheus Queries

```promql
# Queue depth by queue
propvest_queue_depth{queue=~".+"}

# Processing rate (messages/second)
rate(propvest_messages_processed_total[1m])

# Error rate
rate(propvest_messages_processed_total{status="failure"}[1m]) 
/ 
rate(propvest_messages_processed_total[1m])

# P95 processing latency
histogram_quantile(0.95, rate(propvest_message_processing_duration_seconds_bucket[5m]))

# DLQ alert
propvest_dlq_depth{queue="propvest.deposit.confirmed"} > 5
```

### Structured Logging

```go
// internal/logger/structured.go
package logger

import (
    "context"
    "log/slog"
    "os"
)

func InitQueueLogger() *slog.Logger {
    return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
}

func LogMessageProcessed(ctx context.Context, queueName, messageID string, duration time.Duration, err error) {
    logger := FromContext(ctx)
    
    if err != nil {
        logger.Error("message processing failed",
            "queue", queueName,
            "message_id", messageID,
            "duration_ms", duration.Milliseconds(),
            "error", err.Error())
    } else {
        logger.Info("message processed",
            "queue", queueName,
            "message_id", messageID,
            "duration_ms", duration.Milliseconds())
    }
}
```

### Log Format

```json
{
  "time": "2026-08-05T12:34:56.789Z",
  "level": "INFO",
  "msg": "message processed",
  "queue": "propvest.deposit.confirmed",
  "message_id": "1722862496789-12345",
  "event_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "payment_reference": "DEP-TEST-123",
  "duration_ms": 1234,
  "request_id": "req-xyz789"
}
```

---

## Correctness Properties

**Assessment**: Property-based testing is **NOT applicable** for this feature.

**Reasoning**:

This feature is **infrastructure-heavy** with the following characteristics:

1. **External System Dependencies**:
   - RabbitMQ message broker
   - PostgreSQL database
   - Payment provider APIs (Paystack/Flutterwave)

2. **Time-Dependent Behavior**:
   - Reconciliation runs every 5 minutes
   - Retry delays with exponential backoff
   - Circuit breaker timeouts

3. **Side Effects**:
   - Wallet balance updates
   - Transaction ledger writes
   - Notifications sent
   - DLQ routing

4. **Configuration-Driven**:
   - Queue topology (exchanges, queues, bindings)
   - Retry policies
   - Concurrency settings

**Appropriate Testing Strategies**:

1. **Unit Tests**: Pure logic functions
   - Retry delay calculation
   - Error classification (transient vs permanent)
   - Circuit breaker state transitions

2. **Integration Tests**: System behavior with real dependencies
   - End-to-end deposit/withdrawal flows
   - Idempotency verification
   - Retry and DLQ behavior
   - Reconciliation logic

3. **Contract Tests**: External API interactions
   - RabbitMQ message formats
   - PostgreSQL schema constraints
   - Payment provider API responses

4. **Manual Tests**: Operational scenarios
   - DLQ monitoring and requeuing
   - RabbitMQ failover
   - Database connection recovery

**Conclusion**: No Correctness Properties section included. Testing strategy focuses on integration and end-to-end tests.

---

## Summary

This design document specifies a production-grade asynchronous message queue system for the PropVest wallet/payment backend using RabbitMQ. The architecture transforms synchronous webhook processing into a reliable, scalable, fault-tolerant system with the following guarantees:

**Reliability**:
- Exactly-once processing via idempotency keys in event store
- Retry with exponential backoff for transient failures
- Dead-letter queues for manual investigation of permanent failures
- Reconciliation worker catches missed or stuck events

**Performance**:
- Webhook handlers respond in <50ms (signature + DB write only)
- Horizontal scaling of consumers (multiple worker instances)
- Concurrent message processing within each consumer (10 goroutines)
- Priority queues (withdrawals > deposits > notifications)

**Resilience**:
- Graceful degradation when RabbitMQ is unavailable (events stored, reconciled later)
- Circuit breaker prevents cascading failures
- Automatic reconnection with exponential backoff
- Consumer crash recovery via status tracking

**Observability**:
- Real-time metrics (queue depth, processing rate, error rate)
- Structured logging with distributed tracing (request_id)
- Alerts for DLQ buildup, queue backlog, consumer lag
- Audit trail in event store for debugging

**Extensibility**:
- Topic-based routing for future event types
- Pluggable consumers (add new queue without changing webhook handler)
- Metadata field in event store for event-specific data
- Exchange fanout for multi-destination events

The implementation follows Go best practices with dependency injection, error wrapping, context propagation, and structured logging. The system is designed for operational excellence with fail-fast startup validation, graceful shutdown, and comprehensive monitoring.
