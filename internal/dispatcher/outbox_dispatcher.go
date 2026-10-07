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

// OutboxDispatcher polls the outbox_events table and publishes pending events to RabbitMQ.
//
// This is the CRITICAL component that ensures outbox events are delivered to the message
// broker. It runs as a background goroutine and continuously processes pending events.
//
// Key Features:
//   - Polls every 1 second for pending events
//   - Claims events using FOR UPDATE SKIP LOCKED (safe for multiple instances)
//   - Publishes to RabbitMQ
//   - Retries failed events with exponential backoff
//   - Recovers stale events from crashed dispatcher instances
//   - Graceful shutdown support
//
// Multiple Instances:
//   You can run multiple dispatcher instances for high availability and throughput.
//   The FOR UPDATE SKIP LOCKED ensures they don't process the same events.
//
// Usage:
//
//	dispatcher := NewOutboxDispatcher(outboxRepo, mqClient, "dispatcher-1")
//	dispatcher.Start(ctx)
//	defer dispatcher.Stop()
type OutboxDispatcher struct {
	outboxRepo   repositories.OutboxRepository
	mqClient     *queue.Client
	instanceID   string
	batchSize    int
	pollInterval time.Duration
	staleTimeout time.Duration
	maxRetries   int
	stopChan     chan struct{}
	doneChan     chan struct{}
}

// Config holds dispatcher configuration.
type Config struct {
	// BatchSize is the maximum number of events to claim per poll (default: 10)
	BatchSize int

	// PollInterval is how often to check for pending events (default: 1 second)
	PollInterval time.Duration

	// StaleTimeout is how long to wait before recovering claimed events (default: 5 minutes)
	StaleTimeout time.Duration

	// MaxRetries is the maximum number of retry attempts before marking failed (default: 10)
	MaxRetries int
}

// DefaultConfig returns sensible defaults for the dispatcher.
func DefaultConfig() Config {
	return Config{
		BatchSize:    10,
		PollInterval: 1 * time.Second,
		StaleTimeout: 5 * time.Minute,
		MaxRetries:   10,
	}
}

// NewOutboxDispatcher creates a new dispatcher instance.
//
// Parameters:
//   - outboxRepo: Repository for outbox operations
//   - mqClient: RabbitMQ client for publishing
//   - instanceID: Unique identifier for this dispatcher (e.g., "dispatcher-1", hostname, etc.)
//
// The instanceID is used for debugging (to see which dispatcher claimed an event).
func NewOutboxDispatcher(
	outboxRepo repositories.OutboxRepository,
	mqClient *queue.Client,
	instanceID string,
) *OutboxDispatcher {
	cfg := DefaultConfig()
	return NewOutboxDispatcherWithConfig(outboxRepo, mqClient, instanceID, cfg)
}

// NewOutboxDispatcherWithConfig creates a dispatcher with custom configuration.
func NewOutboxDispatcherWithConfig(
	outboxRepo repositories.OutboxRepository,
	mqClient *queue.Client,
	instanceID string,
	cfg Config,
) *OutboxDispatcher {
	return &OutboxDispatcher{
		outboxRepo:   outboxRepo,
		mqClient:     mqClient,
		instanceID:   instanceID,
		batchSize:    cfg.BatchSize,
		pollInterval: cfg.PollInterval,
		staleTimeout: cfg.StaleTimeout,
		maxRetries:   cfg.MaxRetries,
		stopChan:     make(chan struct{}),
		doneChan:     make(chan struct{}),
	}
}

// Start begins the dispatcher background loops.
//
// This starts two goroutines:
//   1. pollLoop: Polls for pending events and publishes them
//   2. recoveryLoop: Recovers stale events from crashed dispatchers
//
// Call Stop() to gracefully shut down.
func (d *OutboxDispatcher) Start(ctx context.Context) {
	log := logger.FromContext(ctx)
	log.Info("📤 outbox dispatcher starting",
		"instance_id", d.instanceID,
		"batch_size", d.batchSize,
		"poll_interval", d.pollInterval)

	go d.pollLoop(ctx)
	go d.recoveryLoop(ctx)

	log.Info("✓ outbox dispatcher started")
}

// Stop gracefully shuts down the dispatcher.
//
// This signals both loops to stop and waits for them to finish.
// Call this during application shutdown.
func (d *OutboxDispatcher) Stop() {
	close(d.stopChan)
	<-d.doneChan
}

// pollLoop continuously polls for pending events and processes them.
func (d *OutboxDispatcher) pollLoop(ctx context.Context) {
	defer close(d.doneChan)

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	log := logger.FromContext(ctx)

	for {
		select {
		case <-d.stopChan:
			log.Info("📤 outbox dispatcher stopping", "instance_id", d.instanceID)
			return
		case <-ticker.C:
			d.processBatch(ctx)
		}
	}
}

// processBatch claims and processes a batch of pending events.
func (d *OutboxDispatcher) processBatch(ctx context.Context) {
	log := logger.FromContext(ctx)

	// Claim events (atomic operation with FOR UPDATE SKIP LOCKED)
	events, err := d.outboxRepo.ClaimEvents(ctx, d.batchSize, d.instanceID)
	if err != nil {
		log.Error("failed to claim events", "error", err, "instance_id", d.instanceID)
		return
	}

	if len(events) == 0 {
		return // No events to process (normal condition)
	}

	log.Info("processing outbox batch",
		"count", len(events),
		"instance_id", d.instanceID)

	// Process each event
	for _, event := range events {
		if err := d.processEvent(ctx, event); err != nil {
			log.Error("failed to process event",
				"event_id", event.ID,
				"event_type", event.EventType,
				"aggregate_id", event.AggregateID,
				"attempts", event.Attempts,
				"error", err)

			// Decide: retry or fail permanently
			d.handleFailure(ctx, event, err)
		} else {
			// Success - mark published
			if err := d.outboxRepo.MarkPublished(ctx, event.ID); err != nil {
				log.Error("failed to mark event published",
					"event_id", event.ID,
					"error", err)
			}
		}
	}
}

// processEvent publishes a single event to RabbitMQ.
func (d *OutboxDispatcher) processEvent(ctx context.Context, event *models.OutboxEvent) error {
	log := logger.FromContext(ctx)

	// Map event type to queue name
	queueName := d.getQueueName(event.EventType)
	if queueName == "" {
		return fmt.Errorf("unknown event type: %s", event.EventType)
	}

	// Unmarshal payload to generic map for RabbitMQ
	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Publish to RabbitMQ
	if err := d.mqClient.Publish(ctx, queueName, payload); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", queueName, err)
	}

	log.Info("✓ event published",
		"event_id", event.ID,
		"event_type", event.EventType,
		"queue", queueName,
		"aggregate_id", event.AggregateID,
		"attempts", event.Attempts)

	return nil
}

// handleFailure decides whether to retry or permanently fail an event.
func (d *OutboxDispatcher) handleFailure(ctx context.Context, event *models.OutboxEvent, publishErr error) {
	log := logger.FromContext(ctx)

	// Check if max retries exceeded
	if event.Attempts+1 >= d.maxRetries {
		log.Warn("event max retries exceeded, marking failed",
			"event_id", event.ID,
			"event_type", event.EventType,
			"attempts", event.Attempts+1,
			"max_retries", d.maxRetries)

		if err := d.outboxRepo.MarkFailed(ctx, event.ID, publishErr.Error()); err != nil {
			log.Error("failed to mark event as failed", "event_id", event.ID, "error", err)
		}
		return
	}

	// Retry with exponential backoff
	backoff := d.calculateBackoff(event.Attempts)
	nextAvailableAt := time.Now().Add(backoff)

	log.Info("scheduling event retry",
		"event_id", event.ID,
		"attempts", event.Attempts+1,
		"backoff", backoff,
		"next_attempt", nextAvailableAt)

	if err := d.outboxRepo.IncrementAttempts(ctx, event.ID, nextAvailableAt, publishErr.Error()); err != nil {
		log.Error("failed to increment attempts", "event_id", event.ID, "error", err)
	}
}

// calculateBackoff computes exponential backoff duration.
//
// Retry schedule:
//   - Attempt 1: 30 seconds
//   - Attempt 2: 60 seconds
//   - Attempt 3: 120 seconds
//   - Attempt 4: 240 seconds (4 minutes)
//   - Attempt 5: 480 seconds (8 minutes)
//   - ... up to max 1 hour
func (d *OutboxDispatcher) calculateBackoff(attempts int) time.Duration {
	base := 30 * time.Second
	backoff := base * (1 << uint(attempts)) // Exponential: 30s, 60s, 120s, 240s, ...

	// Cap at 1 hour
	maxBackoff := 1 * time.Hour
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	return backoff
}

// getQueueName maps event types to RabbitMQ queue names.
func (d *OutboxDispatcher) getQueueName(eventType string) string {
	mapping := map[string]string{
		models.EventTypeWithdrawalProcess: queue.QueueWithdrawalProcess,
		models.EventTypeDepositReceipt:    queue.QueueDepositReceipt,
		models.EventTypeEmailDispatch:     queue.QueueEmailDispatch,
		models.EventTypeSMSDispatch:       queue.QueueSMSDispatch,
		models.EventTypeRealtimePush:      queue.QueueRealtimePush,
	}
	return mapping[eventType]
}

// recoveryLoop periodically recovers stale events from crashed dispatchers.
//
// A "stale" event is one that has been claimed but not marked published/failed
// within the stale timeout (default: 5 minutes). This indicates the dispatcher
// that claimed it crashed or is stuck.
//
// Recovery runs every 1 minute.
func (d *OutboxDispatcher) recoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log := logger.FromContext(ctx)

	for {
		select {
		case <-d.stopChan:
			log.Info("recovery loop stopping", "instance_id", d.instanceID)
			return
		case <-ticker.C:
			count, err := d.outboxRepo.RecoverStaleEvents(ctx, d.staleTimeout)
			if err != nil {
				log.Error("failed to recover stale events",
					"error", err,
					"instance_id", d.instanceID)
			} else if count > 0 {
				log.Warn("recovered stale events",
					"count", count,
					"instance_id", d.instanceID,
					"stale_timeout", d.staleTimeout)
			}
		}
	}
}

// GetPendingCount returns the number of events waiting to be processed.
//
// This is useful for health checks and monitoring.
func (d *OutboxDispatcher) GetPendingCount(ctx context.Context) (int64, error) {
	return d.outboxRepo.GetPendingCount(ctx)
}

// ═══════════════════════════════════════════════════════════════════════════
// HEALTH CHECK SUPPORT
// ═══════════════════════════════════════════════════════════════════════════

// HealthStatus represents the dispatcher health status.
type HealthStatus struct {
	Running      bool   `json:"running"`
	InstanceID   string `json:"instance_id"`
	PendingCount int64  `json:"pending_count"`
	FailedCount  int64  `json:"failed_count,omitempty"` // Future: add GetFailedCount to repo
}

// Health returns the current health status of the dispatcher.
func (d *OutboxDispatcher) Health(ctx context.Context) HealthStatus {
	pendingCount, _ := d.GetPendingCount(ctx)

	return HealthStatus{
		Running:      true, // If this is called, dispatcher is running
		InstanceID:   d.instanceID,
		PendingCount: pendingCount,
	}
}
