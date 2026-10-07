package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OutboxRepository defines the interface for outbox event data access operations.
//
// This repository implements the transactional outbox pattern for reliable
// message delivery. Events are created in the same database transaction as
// business state changes, then asynchronously published to RabbitMQ by a
// separate dispatcher process.
//
// Key Guarantees:
//   - Events are NEVER lost due to RabbitMQ unavailability
//   - Events and business state changes commit atomically
//   - Multiple dispatcher instances can run safely (via FOR UPDATE SKIP LOCKED)
//   - Failed events are retried with exponential backoff
//   - Stale events (from crashed dispatchers) are recovered
//
// Concurrency Safety:
//   - ClaimEvents uses FOR UPDATE SKIP LOCKED to prevent double-processing
//   - Multiple dispatchers can claim different events simultaneously
//   - A claimed event is invisible to other dispatchers until timeout
//
// Usage Pattern:
//
//	// 1. Create event inside business transaction
//	tx := db.Begin()
//	// ... make business state changes ...
//	event := &models.OutboxEvent{
//	    EventType:     models.EventTypeWithdrawalProcess,
//	    AggregateType: "withdrawal",
//	    AggregateID:   withdrawalID,
//	    Payload:       payloadJSON,
//	}
//	repo.CreateEvent(ctx, event, tx)
//	tx.Commit() // Event and business state committed atomically
//
//	// 2. Dispatcher claims and publishes (separate process)
//	events, _ := repo.ClaimEvents(ctx, 10, "dispatcher-1")
//	for _, event := range events {
//	    publishToRabbitMQ(event)
//	    repo.MarkPublished(ctx, event.ID)
//	}
type OutboxRepository interface {
	// CreateEvent inserts a new outbox event within a transaction.
	//
	// This MUST be called within the same transaction as the business state change
	// it represents. This ensures atomicity: either both the business state and
	// the event are committed, or both are rolled back.
	//
	// Parameters:
	//   - ctx: Request context for cancellation/timeout
	//   - event: The outbox event to create
	//   - tx: GORM transaction handle (REQUIRED - must not be nil)
	//
	// Returns error if:
	//   - tx is nil (programming error)
	//   - Database insert fails
	//
	// Example:
	//   tx := r.db.Begin()
	//   defer tx.Rollback()
	//   // ... update wallet balance ...
	//   event := &models.OutboxEvent{...}
	//   repo.CreateEvent(ctx, event, tx)
	//   tx.Commit()
	CreateEvent(ctx context.Context, event *models.OutboxEvent, tx *gorm.DB) error

	// ClaimEvents atomically claims a batch of pending events for processing.
	//
	// This method uses PostgreSQL's FOR UPDATE SKIP LOCKED to safely claim events
	// in a multi-dispatcher environment. Each dispatcher instance claims different
	// events, preventing double-processing.
	//
	// Claiming Logic:
	//   1. Find events with status='pending' AND available_at <= NOW()
	//   2. Lock those rows with FOR UPDATE SKIP LOCKED
	//   3. Update status='claimed', claimed_at=NOW(), claimed_by=instanceID
	//   4. Return the claimed events
	//
	// The SKIP LOCKED ensures that if two dispatchers try to claim the same event,
	// one succeeds and the other skips it and moves to the next available event.
	//
	// Parameters:
	//   - ctx: Request context
	//   - batchSize: Maximum number of events to claim (typically 10-100)
	//   - instanceID: Unique identifier for this dispatcher instance (for debugging)
	//
	// Returns:
	//   - Slice of claimed events (may be empty if no events available)
	//   - Error if database query fails
	//
	// Stale Event Handling:
	//   Events claimed by crashed dispatchers will have claimed_at in the past.
	//   RecoverStaleEvents should be called periodically to reclaim them.
	ClaimEvents(ctx context.Context, batchSize int, instanceID string) ([]*models.OutboxEvent, error)

	// MarkPublished marks an event as successfully published to RabbitMQ.
	//
	// This should be called immediately after successfully publishing the event
	// to the message broker. Once marked published, the event will not be
	// retried or reclaimed.
	//
	// Parameters:
	//   - ctx: Request context
	//   - eventID: UUID of the event to mark published
	//
	// Returns error if:
	//   - Event not found (may have been deleted)
	//   - Database update fails
	MarkPublished(ctx context.Context, eventID uuid.UUID) error

	// MarkFailed marks an event as failed after max retries or permanent error.
	//
	// This should be called when:
	//   - Max retry attempts reached
	//   - A permanent error occurs (e.g., invalid payload, business rule violation)
	//
	// Failed events will NOT be retried automatically. Manual intervention is
	// required (admin dashboard, reconciliation job, etc.).
	//
	// Parameters:
	//   - ctx: Request context
	//   - eventID: UUID of the event to mark failed
	//   - errorMsg: Human-readable error message explaining why it failed
	//
	// Updates:
	//   - status='failed'
	//   - last_error=errorMsg
	//   - updated_at=NOW()
	MarkFailed(ctx context.Context, eventID uuid.UUID, errorMsg string) error

	// IncrementAttempts increments the retry counter and schedules next attempt.
	//
	// This should be called when publishing fails with a transient error
	// (e.g., RabbitMQ connection lost, temporary network issue).
	//
	// Implements exponential backoff:
	//   - Attempt 1: retry in 30s
	//   - Attempt 2: retry in 60s
	//   - Attempt 3: retry in 120s
	//   - etc.
	//
	// Parameters:
	//   - ctx: Request context
	//   - eventID: UUID of the event
	//   - nextAvailableAt: When the event should be retried (backoff calculated by caller)
	//   - errorMsg: Error message from this attempt
	//
	// Updates:
	//   - attempts += 1
	//   - available_at = nextAvailableAt
	//   - status = 'pending' (unclaim it so dispatcher can retry)
	//   - last_error = errorMsg
	//   - claimed_at = NULL
	//   - claimed_by = NULL
	IncrementAttempts(ctx context.Context, eventID uuid.UUID, nextAvailableAt time.Time, errorMsg string) error

	// RecoverStaleEvents reclaims events from crashed/stuck dispatchers.
	//
	// A "stale" event is one that has been claimed but not marked published/failed
	// within a reasonable timeout (e.g., 5 minutes). This indicates the dispatcher
	// crashed or is stuck.
	//
	// Recovery Logic:
	//   1. Find events with status='claimed' AND claimed_at < (NOW() - timeout)
	//   2. Reset status='pending', claimed_at=NULL, claimed_by=NULL
	//   3. Increment attempts counter
	//   4. Set available_at based on retry backoff
	//
	// This should be called periodically by the dispatcher (e.g., every 1 minute).
	//
	// Parameters:
	//   - ctx: Request context
	//   - timeout: How long to wait before considering a claim stale (e.g., 5min)
	//
	// Returns:
	//   - Number of events recovered
	//   - Error if database query fails
	RecoverStaleEvents(ctx context.Context, timeout time.Duration) (int64, error)

	// GetPendingCount returns the number of events waiting to be processed.
	//
	// Useful for:
	//   - Health checks (alert if count grows too large)
	//   - Metrics/monitoring
	//   - Deciding whether to scale dispatcher instances
	//
	// Counts events with status='pending' AND available_at <= NOW().
	GetPendingCount(ctx context.Context) (int64, error)

	// GetEventByID retrieves a single event by its ID.
	//
	// Useful for:
	//   - Debugging
	//   - Admin dashboards
	//   - Manual retry operations
	GetEventByID(ctx context.Context, eventID uuid.UUID) (*models.OutboxEvent, error)

	// GetFailedEvents retrieves events that have permanently failed.
	//
	// Useful for:
	//   - Admin dashboards showing failed messages
	//   - Manual retry workflows
	//   - Incident investigation
	//
	// Parameters:
	//   - limit: Maximum number of events to return
	//   - offset: Pagination offset
	GetFailedEvents(ctx context.Context, limit, offset int) ([]*models.OutboxEvent, error)
}

// outboxRepository implements OutboxRepository using GORM.
type outboxRepository struct {
	*BaseRepository
}

// NewOutboxRepository creates a new outbox repository instance.
func NewOutboxRepository(db *gorm.DB) OutboxRepository {
	return &outboxRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// CreateEvent inserts a new outbox event within a transaction.
func (r *outboxRepository) CreateEvent(ctx context.Context, event *models.OutboxEvent, tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("transaction is required for CreateEvent")
	}

	// Use the provided transaction to ensure atomicity with business state
	return tx.WithContext(ctx).Create(event).Error
}

// ClaimEvents atomically claims a batch of pending events.
//
// Implementation uses FOR UPDATE SKIP LOCKED for safe concurrent claiming.
func (r *outboxRepository) ClaimEvents(ctx context.Context, batchSize int, instanceID string) ([]*models.OutboxEvent, error) {
	var events []*models.OutboxEvent

	// Start a transaction for atomic claim operation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find pending events ready for processing
		// Uses partial index: idx_outbox_events_claim (status, available_at)
		// WHERE status IN ('pending', 'claimed')
		err := tx.Model(&models.OutboxEvent{}).
			Where("status = ?", models.OutboxStatusPending).
			Where("available_at <= ?", time.Now()).
			Order("created_at ASC"). // Process oldest events first (FIFO)
			Limit(batchSize).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}). // Critical: prevents double-processing
			Find(&events).Error

		if err != nil {
			return fmt.Errorf("failed to find pending events: %w", err)
		}

		// If no events found, return early (not an error)
		if len(events) == 0 {
			return nil
		}

		// Claim the events by updating their status
		eventIDs := make([]uuid.UUID, len(events))
		for i, event := range events {
			eventIDs[i] = event.ID
		}

		now := time.Now()
		err = tx.Model(&models.OutboxEvent{}).
			Where("id IN ?", eventIDs).
			Updates(map[string]interface{}{
				"status":     models.OutboxStatusClaimed,
				"claimed_at": now,
				"claimed_by": instanceID,
				"updated_at": now,
			}).Error

		if err != nil {
			return fmt.Errorf("failed to claim events: %w", err)
		}

		// Update in-memory events to reflect claimed status
		for _, event := range events {
			event.Status = models.OutboxStatusClaimed
			event.ClaimedAt = &now
			event.ClaimedBy = &instanceID
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return events, nil
}

// MarkPublished marks an event as successfully published.
func (r *outboxRepository) MarkPublished(ctx context.Context, eventID uuid.UUID) error {
	now := time.Now()

	result := r.WithContext(ctx).
		Model(&models.OutboxEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]interface{}{
			"status":       models.OutboxStatusPublished,
			"published_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to mark event published: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("event not found: %s", eventID)
	}

	return nil
}

// MarkFailed marks an event as permanently failed.
func (r *outboxRepository) MarkFailed(ctx context.Context, eventID uuid.UUID, errorMsg string) error {
	now := time.Now()

	result := r.WithContext(ctx).
		Model(&models.OutboxEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]interface{}{
			"status":     models.OutboxStatusFailed,
			"last_error": errorMsg,
			"updated_at": now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to mark event failed: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("event not found: %s", eventID)
	}

	return nil
}

// IncrementAttempts increments retry counter and schedules next attempt.
func (r *outboxRepository) IncrementAttempts(ctx context.Context, eventID uuid.UUID, nextAvailableAt time.Time, errorMsg string) error {
	now := time.Now()

	// Unclaim the event and schedule retry
	// This makes it available for the next polling cycle
	result := r.WithContext(ctx).
		Model(&models.OutboxEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]interface{}{
			"attempts":     gorm.Expr("attempts + 1"),
			"available_at": nextAvailableAt,
			"status":       models.OutboxStatusPending, // Unclaim it
			"claimed_at":   nil,
			"claimed_by":   nil,
			"last_error":   errorMsg,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to increment attempts: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("event not found: %s", eventID)
	}

	return nil
}

// RecoverStaleEvents reclaims events from crashed dispatchers.
func (r *outboxRepository) RecoverStaleEvents(ctx context.Context, timeout time.Duration) (int64, error) {
	staleThreshold := time.Now().Add(-timeout)

	// Find events claimed longer than timeout ago (dispatcher likely crashed)
	result := r.WithContext(ctx).
		Model(&models.OutboxEvent{}).
		Where("status = ?", models.OutboxStatusClaimed).
		Where("claimed_at < ?", staleThreshold).
		Updates(map[string]interface{}{
			"status":       models.OutboxStatusPending,
			"claimed_at":   nil,
			"claimed_by":   nil,
			"attempts":     gorm.Expr("attempts + 1"), // Count as retry
			"available_at": time.Now(),                // Make immediately available
			"updated_at":   time.Now(),
		})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to recover stale events: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// GetPendingCount returns count of events ready for processing.
func (r *outboxRepository) GetPendingCount(ctx context.Context) (int64, error) {
	var count int64

	err := r.WithContext(ctx).
		Model(&models.OutboxEvent{}).
		Where("status = ?", models.OutboxStatusPending).
		Where("available_at <= ?", time.Now()).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count pending events: %w", err)
	}

	return count, nil
}

// GetEventByID retrieves a single event by ID.
func (r *outboxRepository) GetEventByID(ctx context.Context, eventID uuid.UUID) (*models.OutboxEvent, error) {
	var event models.OutboxEvent

	err := r.WithContext(ctx).
		Where("id = ?", eventID).
		First(&event).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("event not found: %s", eventID)
		}
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	return &event, nil
}

// GetFailedEvents retrieves permanently failed events.
func (r *outboxRepository) GetFailedEvents(ctx context.Context, limit, offset int) ([]*models.OutboxEvent, error) {
	var events []*models.OutboxEvent

	err := r.WithContext(ctx).
		Where("status = ?", models.OutboxStatusFailed).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&events).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get failed events: %w", err)
	}

	return events, nil
}
