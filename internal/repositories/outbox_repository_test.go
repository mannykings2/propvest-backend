package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing.
// SQLite is used instead of PostgreSQL for faster test execution.
// Note: FOR UPDATE SKIP LOCKED is PostgreSQL-specific and cannot be fully
// tested with SQLite. Those tests are marked with comments.
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test database")

	// Run migrations
	err = db.AutoMigrate(&models.OutboxEvent{})
	require.NoError(t, err, "failed to migrate test database")

	return db
}

// createTestEvent is a helper to create a test outbox event.
func createTestEvent(eventType, aggregateType string, aggregateID uuid.UUID) *models.OutboxEvent {
	return &models.OutboxEvent{
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       []byte(`{"test": "data"}`),
		Status:        models.OutboxStatusPending,
		Attempts:      0,
		AvailableAt:   time.Now(),
	}
}

// TestNewOutboxRepository verifies repository initialization.
func TestNewOutboxRepository(t *testing.T) {
	db := setupTestDB(t)

	repo := NewOutboxRepository(db)

	assert.NotNil(t, repo)
}

// TestCreateEvent_Success verifies event creation within a transaction.
func TestCreateEvent_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	event := createTestEvent(
		models.EventTypeWithdrawalProcess,
		"withdrawal",
		uuid.New(),
	)

	// Create event within a transaction
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event, tx)
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, event.ID, "event ID should be set")
	assert.Equal(t, models.OutboxStatusPending, event.Status)
	assert.Equal(t, 0, event.Attempts)
}

// TestCreateEvent_RequiresTransaction verifies error when tx is nil.
func TestCreateEvent_RequiresTransaction(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	event := createTestEvent(
		models.EventTypeWithdrawalProcess,
		"withdrawal",
		uuid.New(),
	)

	// Should fail without transaction
	err := repo.CreateEvent(ctx, event, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction is required")
}

// TestCreateEvent_RollbackOnError verifies atomicity.
func TestCreateEvent_RollbackOnError(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	event := createTestEvent(
		models.EventTypeWithdrawalProcess,
		"withdrawal",
		uuid.New(),
	)

	// Transaction that rolls back
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := repo.CreateEvent(ctx, event, tx); err != nil {
			return err
		}
		// Simulate error causing rollback
		return assert.AnError
	})

	require.Error(t, err)

	// Event should not exist in database
	var count int64
	db.Model(&models.OutboxEvent{}).Count(&count)
	assert.Equal(t, int64(0), count, "event should not exist after rollback")
}

// TestClaimEvents_Success verifies basic event claiming.
func TestClaimEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create 3 pending events
	for i := 0; i < 3; i++ {
		event := createTestEvent(
			models.EventTypeWithdrawalProcess,
			"withdrawal",
			uuid.New(),
		)
		err := db.Transaction(func(tx *gorm.DB) error {
			return repo.CreateEvent(ctx, event, tx)
		})
		require.NoError(t, err)
	}

	// Claim 2 events
	events, err := repo.ClaimEvents(ctx, 2, "dispatcher-1")

	require.NoError(t, err)
	assert.Len(t, events, 2, "should claim 2 events")

	for _, event := range events {
		assert.Equal(t, models.OutboxStatusClaimed, event.Status)
		assert.NotNil(t, event.ClaimedAt)
		assert.NotNil(t, event.ClaimedBy)
		assert.Equal(t, "dispatcher-1", *event.ClaimedBy)
	}

	// Verify 1 event still pending
	var pendingCount int64
	db.Model(&models.OutboxEvent{}).
		Where("status = ?", models.OutboxStatusPending).
		Count(&pendingCount)
	assert.Equal(t, int64(1), pendingCount)
}

// TestClaimEvents_EmptyQueue verifies behavior when no events available.
func TestClaimEvents_EmptyQueue(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	events, err := repo.ClaimEvents(ctx, 10, "dispatcher-1")

	require.NoError(t, err)
	assert.Empty(t, events, "should return empty slice when no events available")
}

// TestClaimEvents_OnlyReadyEvents verifies available_at filtering.
func TestClaimEvents_OnlyReadyEvents(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create event available now
	event1 := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
	event1.AvailableAt = time.Now().Add(-1 * time.Minute)
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event1, tx)
	})
	require.NoError(t, err)

	// Create event available in future (backoff)
	event2 := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
	event2.AvailableAt = time.Now().Add(10 * time.Minute)
	err = db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event2, tx)
	})
	require.NoError(t, err)

	// Should only claim event1
	events, err := repo.ClaimEvents(ctx, 10, "dispatcher-1")

	require.NoError(t, err)
	assert.Len(t, events, 1, "should only claim events with available_at <= now")
	assert.Equal(t, event1.ID, events[0].ID)
}

// TestClaimEvents_FIFO verifies oldest events are claimed first.
func TestClaimEvents_FIFO(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	var eventIDs []uuid.UUID

	// Create 3 events with different timestamps
	for i := 0; i < 3; i++ {
		event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
		event.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
		err := db.Transaction(func(tx *gorm.DB) error {
			return repo.CreateEvent(ctx, event, tx)
		})
		require.NoError(t, err)
		eventIDs = append(eventIDs, event.ID)

		// Small delay to ensure different created_at times
		time.Sleep(10 * time.Millisecond)
	}

	// Claim all events
	events, err := repo.ClaimEvents(ctx, 10, "dispatcher-1")

	require.NoError(t, err)
	assert.Len(t, events, 3)

	// Should be in creation order (FIFO)
	assert.Equal(t, eventIDs[0], events[0].ID, "oldest event should be first")
	assert.Equal(t, eventIDs[1], events[1].ID)
	assert.Equal(t, eventIDs[2], events[2].ID)
}

// TestMarkPublished_Success verifies marking event as published.
func TestMarkPublished_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create and claim event
	event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event, tx)
	})
	require.NoError(t, err)

	events, err := repo.ClaimEvents(ctx, 1, "dispatcher-1")
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Mark published
	err = repo.MarkPublished(ctx, events[0].ID)

	require.NoError(t, err)

	// Verify status changed
	var updated models.OutboxEvent
	err = db.First(&updated, events[0].ID).Error
	require.NoError(t, err)
	assert.Equal(t, models.OutboxStatusPublished, updated.Status)
	assert.NotNil(t, updated.PublishedAt)
}

// TestMarkPublished_EventNotFound verifies error handling.
func TestMarkPublished_EventNotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	err := repo.MarkPublished(ctx, uuid.New())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event not found")
}

// TestMarkFailed_Success verifies marking event as failed.
func TestMarkFailed_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create and claim event
	event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event, tx)
	})
	require.NoError(t, err)

	events, err := repo.ClaimEvents(ctx, 1, "dispatcher-1")
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Mark failed
	errorMsg := "max retries exceeded"
	err = repo.MarkFailed(ctx, events[0].ID, errorMsg)

	require.NoError(t, err)

	// Verify status and error message
	var updated models.OutboxEvent
	err = db.First(&updated, events[0].ID).Error
	require.NoError(t, err)
	assert.Equal(t, models.OutboxStatusFailed, updated.Status)
	assert.NotNil(t, updated.LastError)
	assert.Equal(t, errorMsg, *updated.LastError)
}

// TestIncrementAttempts_Success verifies retry scheduling.
func TestIncrementAttempts_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create and claim event
	event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event, tx)
	})
	require.NoError(t, err)

	events, err := repo.ClaimEvents(ctx, 1, "dispatcher-1")
	require.NoError(t, err)
	require.Len(t, events, 1)
	originalAttempts := events[0].Attempts

	// Increment attempts (simulate retry)
	nextAttempt := time.Now().Add(30 * time.Second)
	errorMsg := "connection lost"
	err = repo.IncrementAttempts(ctx, events[0].ID, nextAttempt, errorMsg)

	require.NoError(t, err)

	// Verify updates
	var updated models.OutboxEvent
	err = db.First(&updated, events[0].ID).Error
	require.NoError(t, err)

	assert.Equal(t, models.OutboxStatusPending, updated.Status, "should be unclaimed")
	assert.Nil(t, updated.ClaimedAt, "claimed_at should be cleared")
	assert.Nil(t, updated.ClaimedBy, "claimed_by should be cleared")
	assert.Equal(t, originalAttempts+1, updated.Attempts, "attempts should increment")
	assert.NotNil(t, updated.LastError)
	assert.Equal(t, errorMsg, *updated.LastError)
	assert.True(t, updated.AvailableAt.After(time.Now().Add(25*time.Second)),
		"available_at should be scheduled in future")
}

// TestRecoverStaleEvents_Success verifies stale event recovery.
func TestRecoverStaleEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create and claim event
	event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event, tx)
	})
	require.NoError(t, err)

	events, err := repo.ClaimEvents(ctx, 1, "dispatcher-1")
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Manually set claimed_at to past (simulate crashed dispatcher)
	staleTime := time.Now().Add(-10 * time.Minute)
	err = db.Model(&models.OutboxEvent{}).
		Where("id = ?", events[0].ID).
		Update("claimed_at", staleTime).Error
	require.NoError(t, err)

	// Recover stale events
	count, err := repo.RecoverStaleEvents(ctx, 5*time.Minute)

	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "should recover 1 stale event")

	// Verify event is now pending
	var recovered models.OutboxEvent
	err = db.First(&recovered, events[0].ID).Error
	require.NoError(t, err)
	assert.Equal(t, models.OutboxStatusPending, recovered.Status)
	assert.Nil(t, recovered.ClaimedAt)
	assert.Nil(t, recovered.ClaimedBy)
}

// TestRecoverStaleEvents_OnlyStale verifies non-stale events are not recovered.
func TestRecoverStaleEvents_OnlyStale(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create 2 claimed events
	for i := 0; i < 2; i++ {
		event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
		err := db.Transaction(func(tx *gorm.DB) error {
			return repo.CreateEvent(ctx, event, tx)
		})
		require.NoError(t, err)
	}

	events, err := repo.ClaimEvents(ctx, 2, "dispatcher-1")
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Make one stale, keep one fresh
	staleTime := time.Now().Add(-10 * time.Minute)
	err = db.Model(&models.OutboxEvent{}).
		Where("id = ?", events[0].ID).
		Update("claimed_at", staleTime).Error
	require.NoError(t, err)

	// Recover with 5min timeout
	count, err := repo.RecoverStaleEvents(ctx, 5*time.Minute)

	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "should only recover stale event")

	// Verify fresh event still claimed
	var fresh models.OutboxEvent
	err = db.First(&fresh, events[1].ID).Error
	require.NoError(t, err)
	assert.Equal(t, models.OutboxStatusClaimed, fresh.Status)
}

// TestGetPendingCount_Success verifies pending event counting.
func TestGetPendingCount_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create 3 pending events
	for i := 0; i < 3; i++ {
		event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
		err := db.Transaction(func(tx *gorm.DB) error {
			return repo.CreateEvent(ctx, event, tx)
		})
		require.NoError(t, err)
	}

	// Claim 1 event
	_, err := repo.ClaimEvents(ctx, 1, "dispatcher-1")
	require.NoError(t, err)

	// Count should be 2 (3 created - 1 claimed)
	count, err := repo.GetPendingCount(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

// TestGetEventByID_Success verifies single event retrieval.
func TestGetEventByID_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create event
	event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.CreateEvent(ctx, event, tx)
	})
	require.NoError(t, err)

	// Retrieve by ID
	retrieved, err := repo.GetEventByID(ctx, event.ID)

	require.NoError(t, err)
	assert.Equal(t, event.ID, retrieved.ID)
	assert.Equal(t, event.EventType, retrieved.EventType)
	assert.Equal(t, event.AggregateID, retrieved.AggregateID)
}

// TestGetEventByID_NotFound verifies error handling.
func TestGetEventByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	_, err := repo.GetEventByID(ctx, uuid.New())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event not found")
}

// TestGetFailedEvents_Success verifies failed event retrieval.
func TestGetFailedEvents_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create and fail 2 events
	for i := 0; i < 2; i++ {
		event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
		err := db.Transaction(func(tx *gorm.DB) error {
			return repo.CreateEvent(ctx, event, tx)
		})
		require.NoError(t, err)

		err = repo.MarkFailed(ctx, event.ID, "test failure")
		require.NoError(t, err)
	}

	// Retrieve failed events
	failed, err := repo.GetFailedEvents(ctx, 10, 0)

	require.NoError(t, err)
	assert.Len(t, failed, 2)
	for _, event := range failed {
		assert.Equal(t, models.OutboxStatusFailed, event.Status)
	}
}

// TestGetFailedEvents_Pagination verifies pagination.
func TestGetFailedEvents_Pagination(t *testing.T) {
	db := setupTestDB(t)
	repo := NewOutboxRepository(db)
	ctx := context.Background()

	// Create and fail 5 events
	for i := 0; i < 5; i++ {
		event := createTestEvent(models.EventTypeWithdrawalProcess, "withdrawal", uuid.New())
		err := db.Transaction(func(tx *gorm.DB) error {
			return repo.CreateEvent(ctx, event, tx)
		})
		require.NoError(t, err)

		err = repo.MarkFailed(ctx, event.ID, "test failure")
		require.NoError(t, err)

		// Small delay for different created_at
		time.Sleep(5 * time.Millisecond)
	}

	// Get page 1 (limit 2)
	page1, err := repo.GetFailedEvents(ctx, 2, 0)
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	// Get page 2 (limit 2, offset 2)
	page2, err := repo.GetFailedEvents(ctx, 2, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	// Events should be different
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}
