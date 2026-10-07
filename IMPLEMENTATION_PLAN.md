# PropVest Backend — Outbox + Idempotency Implementation Plan

**Date:** 2026-10-06  
**Based On:** Complete Phase 1 Audit (7 audit reports)  
**Status:** 📋 **AWAITING APPROVAL**

---

## Table of Contents

1. [Plan Overview](#plan-overview)
2. [Implementation Phases](#implementation-phases)
3. [Detailed Steps](#detailed-steps)
4. [Risk Mitigation](#risk-mitigation)
5. [Testing Strategy](#testing-strategy)
6. [Rollback Plan](#rollback-plan)
7. [Success Criteria](#success-criteria)

---

## Plan Overview

### Problem Statement

PropVest has a **critical reliability gap**: RabbitMQ messages are published OUTSIDE database transactions, creating a failure window where messages can be lost, causing:
- Withdrawals that never process (money locked forever)
- System failure when RabbitMQ is unavailable
- No recovery from connection loss
- Duplicate processing possible

### Solution

Implement **transactional outbox pattern** + **RabbitMQ reliability improvements** + **comprehensive idempotency**.

### Scope

**In Scope:**
- ✅ Transactional outbox for withdrawals
- ✅ Outbox dispatcher with retry logic
- ✅ RabbitMQ reconnection and consumer recovery
- ✅ Atomic state transitions
- ✅ API-level idempotency
- ✅ Retry limits and DLQ
- ✅ Comprehensive testing

**Out of Scope (Future Phases):**
- ⏭️ Outbox for emails/SMS
- ⏭️ Monitoring dashboards
- ⏭️ Publisher confirms (may not be needed)

### Estimated Timeline

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Database Layer | 1-2 days | None |
| Outbox Implementation | 2-3 days | Database complete |
| RabbitMQ Improvements | 2-3 days | Can parallel |
| Idempotency Layer | 1-2 days | Outbox in progress |
| Retry & DLQ | 1 day | RabbitMQ improvements |
| Testing & Verification | 2-3 days | All above complete |
| **Total** | **9-14 days** | |

---

## Implementation Phases

### Phase A: Database Foundation (Days 1-2)

**Goal:** Create outbox infrastructure in database

**Deliverables:**
- Migration 000014: outbox_events table
- Migration 000015: idempotency improvements
- OutboxEvent model
- Database indexes and constraints

**Success Criteria:**
- Migrations run successfully
- Outbox table created with proper indexes
- Rollback tested

---

### Phase B: Outbox Core (Days 3-5)

**Goal:** Implement outbox repository and basic dispatcher

**Deliverables:**
- OutboxRepository interface and implementation
- Event claiming logic (FOR UPDATE SKIP LOCKED)
- Basic outbox dispatcher
- Retry logic with exponential backoff
- Stale event recovery

**Success Criteria:**
- Events can be created, claimed, and marked published
- Dispatcher polls and publishes events
- Concurrent dispatchers don't double-process
- Unit tests passing

---

### Phase C: Business Integration (Days 4-6)

**Goal:** Integrate outbox into withdrawal flow

**Deliverables:**
- Modified InitiateWithdrawal to create outbox events
- Atomic withdrawal status transitions
- Updated worker to use atomic claims
- Preserved existing business logic

**Success Criteria:**
- Withdrawals create outbox events atomically
- No RabbitMQ publish outside transaction
- Existing withdrawal flow still works
- Integration tests passing

---

### Phase D: RabbitMQ Reliability (Days 5-8)

**Goal:** Make RabbitMQ client production-ready

**Deliverables:**
- Connection retry on startup
- Runtime reconnection with backoff
- Consumer recovery
- Connection monitoring
- Graceful degradation improvements

**Success Criteria:**
- System starts with RabbitMQ down
- Automatic reconnection after connection loss
- Consumers restart after reconnection
- Connection loss detected immediately

---

### Phase E: Idempotency & DLQ (Days 7-9)

**Goal:** Complete idempotency and error handling

**Deliverables:**
- API idempotency middleware
- Withdrawal idempotency key enforcement
- Retry limits (max 3 attempts)
- DLQ infrastructure
- Poison message handling

**Success Criteria:**
- Duplicate API requests handled correctly
- Duplicate messages don't cause duplicate processing
- Poison messages routed to DLQ after 3 attempts
- DLQ monitoring in place

---

### Phase F: Testing & Verification (Days 10-14)

**Goal:** Comprehensive testing of all scenarios

**Deliverables:**
- Unit tests for all new components
- Integration tests for withdrawal flow
- Failure scenario tests
- Performance tests
- Documentation updates

**Success Criteria:**
- All test scenarios pass
- System handles all failure modes correctly
- Performance acceptable
- Documentation complete

---

## Detailed Steps

### A1: Create Outbox Events Table

**Migration:** `000014_create_outbox_events.up.sql`

```sql
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMP WITH TIME ZONE,
    claimed_by VARCHAR(255),
    published_at TIMESTAMP WITH TIME ZONE,
    last_error TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    CONSTRAINT chk_outbox_status CHECK (status IN ('pending', 'claimed', 'published', 'failed')),
    CONSTRAINT chk_outbox_attempts CHECK (attempts >= 0)
);

-- Index for claiming events (most important query)
CREATE INDEX idx_outbox_events_claim
    ON outbox_events(status, available_at)
    WHERE status IN ('pending', 'claimed');

-- Index for querying by aggregate
CREATE INDEX idx_outbox_events_aggregate
    ON outbox_events(aggregate_type, aggregate_id);

-- Index for monitoring
CREATE INDEX idx_outbox_events_created_at
    ON outbox_events(created_at DESC);

-- Updated_at trigger
CREATE TRIGGER update_outbox_events_updated_at
    BEFORE UPDATE ON outbox_events
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE outbox_events IS 'Transactional outbox for reliable async message delivery';
COMMENT ON COLUMN outbox_events.available_at IS 'When event becomes eligible for processing (for delayed retry)';
COMMENT ON COLUMN outbox_events.claimed_by IS 'Dispatcher instance ID that claimed this event';
```

**Down Migration:**
```sql
DROP TABLE IF EXISTS outbox_events CASCADE;
```

---

### A2: Add Idempotency Constraints

**Migration:** `000015_add_idempotency_constraints.up.sql`

```sql
-- Enforce one pending withdrawal per user (prevents race condition)
CREATE UNIQUE INDEX idx_one_pending_withdrawal_per_user
    ON wallet_transactions(user_id)
    WHERE type = 'withdrawal' AND status = 'pending';

COMMENT ON INDEX idx_one_pending_withdrawal_per_user IS 'Ensures users cannot have multiple pending withdrawals simultaneously';

-- Add processing status to wallet transactions
-- (migration to update existing records if needed)
```

---

### A3: Create OutboxEvent Model

**File:** `internal/models/outbox_event.go`

```go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/datatypes"
)

type OutboxEvent struct {
    ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    EventType     string         `gorm:"not null;index" json:"event_type"`
    AggregateType string         `gorm:"not null" json:"aggregate_type"`
    AggregateID   uuid.UUID      `gorm:"type:uuid;not null;index" json:"aggregate_id"`
    Payload       datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
    Status        string         `gorm:"not null;default:'pending'" json:"status"`
    Attempts      int            `gorm:"default:0" json:"attempts"`
    AvailableAt   time.Time      `gorm:"not null;default:now()" json:"available_at"`
    ClaimedAt     *time.Time     `json:"claimed_at,omitempty"`
    ClaimedBy     *string        `json:"claimed_by,omitempty"`
    PublishedAt   *time.Time     `json:"published_at,omitempty"`
    LastError     *string        `gorm:"type:text" json:"last_error,omitempty"`
    CreatedAt     time.Time      `json:"created_at"`
    UpdatedAt     time.Time      `json:"updated_at"`
}

// Status constants
const (
    OutboxStatusPending   = "pending"
    OutboxStatusClaimed   = "claimed"
    OutboxStatusPublished = "published"
    OutboxStatusFailed    = "failed"
)

// Event type constants
const (
    EventTypeWithdrawalProcess = "withdrawal.process"
    EventTypeDepositReceipt    = "deposit.receipt"
    EventTypeEmailDispatch     = "email.dispatch"
    EventTypeSMSDispatch       = "sms.dispatch"
)
```

---

### B1: Create Outbox Repository

**File:** `internal/repositories/outbox_repository.go`

```go
package repositories

import (
    "context"
    "fmt"
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
    "github.com/mannykings2/propvest-backend/internal/models"
)

type OutboxRepository interface {
    // CreateEvent creates a new outbox event within a transaction
    CreateEvent(ctx context.Context, event *models.OutboxEvent, tx *gorm.DB) error
    
    // ClaimEvents atomically claims up to batchSize pending events
    // Uses FOR UPDATE SKIP LOCKED for safe concurrent claiming
    ClaimEvents(ctx context.Context, batchSize int, instanceID string) ([]*models.OutboxEvent, error)
    
    // MarkPublished marks an event as successfully published
    MarkPublished(ctx context.Context, eventID uuid.UUID) error
    
    // MarkFailed marks an event as failed with error details
    // Automatically schedules retry with exponential backoff
    MarkFailed(ctx context.Context, eventID uuid.UUID, errorMsg string) error
    
    // RecoverStaleEvents finds events claimed but not completed
    // Used to recover from dispatcher crashes
    RecoverStaleEvents(ctx context.Context, staleTimeout time.Duration) error
    
    // GetPendingCount returns count of pending events (for monitoring)
    GetPendingCount(ctx context.Context) (int64, error)
}

type outboxRepository struct {
    db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) OutboxRepository {
    return &outboxRepository{db: db}
}

func (r *outboxRepository) CreateEvent(ctx context.Context, event *models.OutboxEvent, tx *gorm.DB) error {
    db := r.getDB(tx)
    return db.WithContext(ctx).Create(event).Error
}

func (r *outboxRepository) ClaimEvents(ctx context.Context, batchSize int, instanceID string) ([]*models.OutboxEvent, error) {
    var events []*models.OutboxEvent
    
    // Raw SQL for FOR UPDATE SKIP LOCKED (GORM doesn't support SKIP LOCKED directly)
    err := r.db.WithContext(ctx).Raw(`
        UPDATE outbox_events
        SET status = ?,
            claimed_at = NOW(),
            claimed_by = ?,
            updated_at = NOW()
        WHERE id IN (
            SELECT id
            FROM outbox_events
            WHERE status = ?
              AND available_at <= NOW()
            ORDER BY created_at
            LIMIT ?
            FOR UPDATE SKIP LOCKED
        )
        RETURNING *
    `, models.OutboxStatusClaimed, instanceID, models.OutboxStatusPending, batchSize).
        Scan(&events).Error
    
    return events, err
}

func (r *outboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
    return r.db.WithContext(ctx).Model(&models.OutboxEvent{}).
        Where("id = ?", eventID).
        Updates(map[string]interface{}{
            "status":       models.OutboxStatusPublished,
            "published_at": time.Now(),
        }).Error
}

func (r *outboxRepository) MarkFailed(ctx context.Context, eventID uuid.UUID, errorMsg string) error {
    var event models.OutboxEvent
    if err := r.db.WithContext(ctx).Where("id = ?", eventID).First(&event).Error; err != nil {
        return err
    }
    
    event.Attempts++
    event.LastError = &errorMsg
    
    // Exponential backoff: 5s, 15s, 45s
    delays := []int{5, 15, 45}
    var delay int
    if event.Attempts <= len(delays) {
        delay = delays[event.Attempts-1]
    } else {
        delay = delays[len(delays)-1]
    }
    
    // Max 3 attempts
    if event.Attempts >= 3 {
        event.Status = models.OutboxStatusFailed
        event.AvailableAt = time.Now() // No more retries
    } else {
        event.Status = models.OutboxStatusPending
        event.AvailableAt = time.Now().Add(time.Duration(delay) * time.Second)
    }
    
    event.ClaimedAt = nil
    event.ClaimedBy = nil
    
    return r.db.WithContext(ctx).Save(&event).Error
}

func (r *outboxRepository) RecoverStaleEvents(ctx context.Context, staleTimeout time.Duration) error {
    cutoff := time.Now().Add(-staleTimeout)
    
    return r.db.WithContext(ctx).Model(&models.OutboxEvent{}).
        Where("status = ? AND claimed_at < ?", models.OutboxStatusClaimed, cutoff).
        Updates(map[string]interface{}{
            "status":     models.OutboxStatusPending,
            "claimed_at": nil,
            "claimed_by": nil,
        }).Error
}

func (r *outboxRepository) GetPendingCount(ctx context.Context) (int64, error) {
    var count int64
    err := r.db.WithContext(ctx).Model(&models.OutboxEvent{}).
        Where("status = ?", models.OutboxStatusPending).
        Count(&count).Error
    return count, err
}

func (r *outboxRepository) getDB(tx *gorm.DB) *gorm.DB {
    if tx != nil {
        return tx
    }
    return r.db
}
```

---

### B2: Create Outbox Dispatcher

**File:** `internal/dispatcher/outbox_dispatcher.go`

```go
package dispatcher

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/mannykings2/propvest-backend/internal/logger"
    "github.com/mannykings2/propvest-backend/internal/models"
    "github.com/mannykings2/propvest-backend/internal/queue"
    "github.com/mannykings2/propvest-backend/internal/repositories"
)

type OutboxDispatcher struct {
    outboxRepo repositories.OutboxRepository
    mqClient   *queue.Client
    instanceID string
    batchSize  int
    pollInterval time.Duration
    staleTimeout time.Duration
    stopChan   chan struct{}
    doneChan   chan struct{}
}

func NewOutboxDispatcher(
    outboxRepo repositories.OutboxRepository,
    mqClient *queue.Client,
    instanceID string,
) *OutboxDispatcher {
    return &OutboxDispatcher{
        outboxRepo:   outboxRepo,
        mqClient:     mqClient,
        instanceID:   instanceID,
        batchSize:    10,
        pollInterval: 1 * time.Second,
        staleTimeout: 5 * time.Minute,
        stopChan:     make(chan struct{}),
        doneChan:     make(chan struct{}),
    }
}

func (d *OutboxDispatcher) Start(ctx context.Context) {
    log := logger.FromContext(ctx)
    log.Info("📤 outbox dispatcher starting", "instance_id", d.instanceID)
    
    go d.pollLoop(ctx)
    go d.recoveryLoop(ctx)
    
    log.Info("✓ outbox dispatcher started")
}

func (d *OutboxDispatcher) Stop() {
    close(d.stopChan)
    <-d.doneChan
}

func (d *OutboxDispatcher) pollLoop(ctx context.Context) {
    defer close(d.doneChan)
    
    ticker := time.NewTicker(d.pollInterval)
    defer ticker.Stop()
    
    log := logger.FromContext(ctx)
    
    for {
        select {
        case <-d.stopChan:
            log.Info("📤 outbox dispatcher stopping")
            return
        case <-ticker.C:
            d.processBatch(ctx)
        }
    }
}

func (d *OutboxDispatcher) processBatch(ctx context.Context) {
    log := logger.FromContext(ctx)
    
    // Claim events
    events, err := d.outboxRepo.ClaimEvents(ctx, d.batchSize, d.instanceID)
    if err != nil {
        log.Error("failed to claim events", "error", err)
        return
    }
    
    if len(events) == 0 {
        return // No events to process
    }
    
    log.Info("processing outbox batch", "count", len(events))
    
    for _, event := range events {
        if err := d.processEvent(ctx, event); err != nil {
            log.Error("failed to process event",
                "event_id", event.ID,
                "event_type", event.EventType,
                "error", err)
            
            // Mark failed (will retry with backoff)
            _ = d.outboxRepo.MarkFailed(ctx, event.ID, err.Error())
        } else {
            // Mark published
            _ = d.outboxRepo.MarkPublished(ctx, event.ID)
        }
    }
}

func (d *OutboxDispatcher) processEvent(ctx context.Context, event *models.OutboxEvent) error {
    log := logger.FromContext(ctx)
    
    // Map event type to queue name
    queueName := d.getQueueName(event.EventType)
    if queueName == "" {
        return fmt.Errorf("unknown event type: %s", event.EventType)
    }
    
    // Unmarshal payload
    var payload interface{}
    if err := json.Unmarshal(event.Payload, &payload); err != nil {
        return fmt.Errorf("failed to unmarshal payload: %w", err)
    }
    
    // Publish to RabbitMQ
    if err := d.mqClient.Publish(ctx, queueName, payload); err != nil {
        return fmt.Errorf("failed to publish: %w", err)
    }
    
    log.Info("✓ event published",
        "event_id", event.ID,
        "event_type", event.EventType,
        "queue", queueName)
    
    return nil
}

func (d *OutboxDispatcher) getQueueName(eventType string) string {
    mapping := map[string]string{
        models.EventTypeWithdrawalProcess: queue.QueueWithdrawalProcess,
        models.EventTypeDepositReceipt:    queue.QueueDepositReceipt,
        models.EventTypeEmailDispatch:     queue.QueueEmailDispatch,
        models.EventTypeSMSDispatch:       queue.QueueSMSDispatch,
    }
    return mapping[eventType]
}

func (d *OutboxDispatcher) recoveryLoop(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    log := logger.FromContext(ctx)
    
    for {
        select {
        case <-d.stopChan:
            return
        case <-ticker.C:
            if err := d.outboxRepo.RecoverStaleEvents(ctx, d.staleTimeout); err != nil {
                log.Error("failed to recover stale events", "error", err)
            }
        }
    }
}
```

---

### C1: Modify InitiateWithdrawal

**Changes to:** `internal/services/wallet_service.go`

```go
func (s *walletService) InitiateWithdrawal(ctx context.Context, userID uuid.UUID, req dto.WithdrawRequest) (*dto.TransactionResponse, error) {
    // ... existing validation ...
    
    // Generate unique reference
    reference := "WD-" + strings.ToUpper(uuid.NewString()[:12])
    
    // ============================================
    // CHANGE: Create transaction + outbox event atomically
    // ============================================
    var ledger *models.WalletTransaction
    err = s.db.Transaction(func(tx *gorm.DB) error {
        // Lock wallet and funds
        wallet, werr := s.walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
        if werr != nil {
            return werr
        }
        
        if lerr := s.walletRepo.LockFunds(ctx, userID, req.Amount, tx); lerr != nil {
            return lerr
        }
        
        // Create pending transaction
        meta, _ := json.Marshal(map[string]any{...})
        ledger = &models.WalletTransaction{
            WalletID:      wallet.ID,
            UserID:        userID,
            Type:          "withdrawal",
            Amount:        req.Amount,
            BalanceBefore: wallet.MainBalance,
            BalanceAfter:  wallet.MainBalance,
            Reference:     reference,
            Description:   fmt.Sprintf("Withdrawal to %s", resolution.BankName),
            Status:        "pending",
            Metadata:      datatypes.JSON(meta),
        }
        if err := tx.Create(ledger).Error; err != nil {
            return err
        }
        
        // ============================================
        // NEW: Create outbox event
        // ============================================
        withdrawalMsg := queue.WithdrawalMessage{
            TransactionID: ledger.ID.String(),
            UserID:        userID.String(),
            AmountKobo:    req.Amount,
            Reference:     reference,
        }
        payload, _ := json.Marshal(withdrawalMsg)
        
        outboxEvent := &models.OutboxEvent{
            EventType:     models.EventTypeWithdrawalProcess,
            AggregateType: "wallet_transaction",
            AggregateID:   ledger.ID,
            Payload:       datatypes.JSON(payload),
            Status:        models.OutboxStatusPending,
        }
        
        return s.outboxRepo.CreateEvent(ctx, outboxEvent, tx)
    })
    
    if err != nil {
        // Handle errors...
        return nil, apperrors.ErrInternalServer
    }
    
    // ============================================
    // REMOVED: Direct RabbitMQ publish
    // ============================================
    // Old code deleted:
    // s.mq.Publish(ctx, queue.QueueWithdrawalProcess, message)
    
    logger.FromContext(ctx).Info("withdrawal initiated",
        "reference", reference,
        "amount", req.Amount,
        "transaction_id", ledger.ID)
    
    // Notify user (unchanged)
    if s.notifier != nil {
        s.notifier.Notify(...)
    }
    
    return txnToResponse(ledger), nil
}
```

---

### C2: Add Atomic Status Transitions

**Changes to:** Worker in `cmd/worker/main.go`

```go
func startWithdrawalProcessor(...) {
    mq.Consume(queue.QueueWithdrawalProcess, func(ctx context.Context, body []byte) error {
        var msg queue.WithdrawalMessage
        json.Unmarshal(body, &msg)
        
        transactionID, _ := uuid.Parse(msg.TransactionID)
        
        // ============================================
        // NEW: Atomic status transition
        // ============================================
        result := db.Model(&models.WalletTransaction{}).
            Where("id = ? AND status = ?", transactionID, "pending").
            Update("status", "processing")
        
        if result.RowsAffected == 0 {
            // Already claimed by another worker or already processed
            log.Info("transaction already claimed or processed", "transaction_id", transactionID)
            return nil // Idempotent
        }
        
        // Fetch transaction
        var txn models.WalletTransaction
        db.Where("id = ?", transactionID).First(&txn)
        
        // Extract bank details and initiate transfer
        // ... existing logic ...
        
        result, err := provider.InitiateTransfer(ctx, transferReq)
        if err != nil {
            // Mark transaction as failed and release funds
            walletService.FinalizeWithdrawal(ctx, transactionID, false, "", err.Error())
            return nil // Don't requeue
        }
        
        // ... rest of existing logic ...
    })
}
```

---

### D1-D5: RabbitMQ Improvements

*(Implementation details similar to previous sections, focusing on:)*
- Connection retry with exponential backoff
- Runtime reconnection
- Consumer registry and recovery
- Connection health monitoring
- Graceful degradation

---

### E1: API Idempotency Middleware

**File:** `internal/middleware/idempotency.go`

```go
package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type IdempotencyMiddleware struct {
    db *gorm.DB
}

func NewIdempotencyMiddleware(db *gorm.DB) *IdempotencyMiddleware {
    return &IdempotencyMiddleware{db: db}
}

func (m *IdempotencyMiddleware) Handle() gin.HandlerFunc {
    return func(c *gin.Context) {
        idempotencyKey := c.GetHeader("Idempotency-Key")
        if idempotencyKey == "" {
            c.Next()
            return
        }
        
        // Check if key already exists
        var result IdempotencyRecord
        err := m.db.Where("idempotency_key = ?", idempotencyKey).First(&result).Error
        
        if err == nil {
            // Key exists, return cached response
            c.Data(result.StatusCode, result.ContentType, result.ResponseBody)
            c.Abort()
            return
        }
        
        // New key, continue and cache response
        // (implementation using response writer wrapper)
        c.Next()
    }
}

type IdempotencyRecord struct {
    ID             uint      `gorm:"primaryKey"`
    IdempotencyKey string    `gorm:"uniqueIndex;not null"`
    StatusCode     int       `gorm:"not null"`
    ContentType    string    `gorm:"not null"`
    ResponseBody   []byte    `gorm:"type:bytea"`
    CreatedAt      time.Time
    ExpiresAt      time.Time `gorm:"index"`
}
```

---

## Risk Mitigation

### Database Migration Risks

**Risk:** Migration fails halfway  
**Mitigation:**
- Test migrations in development first
- Test rollback before deploying
- Use transaction-safe migrations
- Have rollback script ready

---

### Data Loss Risks

**Risk:** Outbox events lost during transition  
**Mitigation:**
- Keep both systems running initially
- Monitor outbox queue depth
- Reconciliation continues as safety net
- Gradual rollout

---

### Performance Risks

**Risk:** Outbox polling adds latency  
**Mitigation:**
- 1-second poll interval (fast)
- Batch processing (10 events at once)
- Database indexes optimized for claim query
- Monitor outbox depth and latency

---

### Compatibility Risks

**Risk:** Breaking existing functionality  
**Mitigation:**
- Preserve existing business logic
- Extensive testing before deployment
- Feature flags for gradual rollout
- Easy rollback plan

---

## Testing Strategy

### Unit Tests

- Outbox repository operations
- Event claiming logic
- Atomic status transitions
- Retry logic with backoff
- Stale event recovery

### Integration Tests

- End-to-end withdrawal with outbox
- Dispatcher polling and publishing
- RabbitMQ reconnection
- Consumer recovery
- Idempotency at all layers

### Failure Scenario Tests

1. RabbitMQ down at startup
2. RabbitMQ crashes during operation
3. API crashes after DB commit
4. Duplicate message delivery
5. Poison message handling
6. Concurrent withdrawal requests
7. Worker crash after Paystack success

---

## Rollback Plan

### Database Rollback

```bash
# Run down migrations
migrate -path internal/database/migrations -database $DATABASE_URL down 2
```

### Code Rollback

```bash
git revert <commit-range>
go build ./cmd/api
go build ./cmd/worker
# Redeploy
```

### Verification

- Check outbox table dropped
- Verify old code running
- Verify withdrawals still work
- Monitor for issues

---

## Success Criteria

### Functional

- ✅ Withdrawals work with RabbitMQ down
- ✅ System recovers from RabbitMQ crashes
- ✅ No duplicate transfers
- ✅ Poison messages routed to DLQ
- ✅ All tests passing

### Performance

- ✅ Withdrawal latency < 100ms (database only)
- ✅ Event processing latency < 5s (outbox to queue)
- ✅ Throughput: 100+ withdrawals/minute

### Reliability

- ✅ Zero message loss
- ✅ Zero duplicate transfers
- ✅ Automatic recovery from all failure modes
- ✅ Reconciliation rate < 1%

---

## Approval Checklist

Before proceeding, confirm:

- [ ] Audit reports reviewed and understood
- [ ] Implementation plan reviewed and approved
- [ ] Timeline acceptable
- [ ] Testing approach adequate
- [ ] Rollback plan understood
- [ ] Resource allocation confirmed

---

**Status:** 📋 **AWAITING APPROVAL**

Once approved, this plan will be merged with `OUTBOX_IDEMPOTENCY_IMPLEMENTATION_TASKS.md` for execution tracking.
