package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutboxEventBuilder provides a fluent interface for creating outbox events.
//
// This builder simplifies event creation and ensures all required fields are set.
// It also handles JSON serialization of event payloads automatically.
//
// Usage:
//
//	event := NewOutboxEventBuilder(EventTypeWithdrawalProcess).
//	    WithAggregate("withdrawal", withdrawalID).
//	    WithPayload(withdrawalMsg).
//	    Build()
type OutboxEventBuilder struct {
	event *OutboxEvent
	err   error
}

// NewOutboxEventBuilder creates a new builder for the given event type.
func NewOutboxEventBuilder(eventType string) *OutboxEventBuilder {
	now := time.Now()
	return &OutboxEventBuilder{
		event: &OutboxEvent{
			EventType:   eventType,
			Status:      OutboxStatusPending,
			Attempts:    0,
			AvailableAt: now,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
}

// WithAggregate sets the aggregate type and ID for the event.
//
// The aggregate represents the business entity this event relates to.
// Examples:
//   - aggregateType: "withdrawal", aggregateID: withdrawal transaction UUID
//   - aggregateType: "deposit", aggregateID: deposit transaction UUID
//   - aggregateType: "user", aggregateID: user UUID
func (b *OutboxEventBuilder) WithAggregate(aggregateType string, aggregateID uuid.UUID) *OutboxEventBuilder {
	b.event.AggregateType = aggregateType
	b.event.AggregateID = aggregateID
	return b
}

// WithPayload sets the event payload by marshaling the given data to JSON.
//
// The payload can be any struct that represents the message to be sent.
// Common payloads:
//   - WithdrawalMessage
//   - DepositMessage
//   - EmailMessage
//   - SMSMessage
func (b *OutboxEventBuilder) WithPayload(payload interface{}) *OutboxEventBuilder {
	if b.err != nil {
		return b // Already have an error, skip
	}

	data, err := json.Marshal(payload)
	if err != nil {
		b.err = fmt.Errorf("failed to marshal payload: %w", err)
		return b
	}

	b.event.Payload = data
	return b
}

// WithRawPayload sets the event payload directly (already JSON).
//
// Use this if you already have JSON bytes and want to avoid double-marshaling.
func (b *OutboxEventBuilder) WithRawPayload(payload []byte) *OutboxEventBuilder {
	b.event.Payload = payload
	return b
}

// WithDelay schedules the event to be available after the given duration.
//
// Useful for:
//   - Exponential backoff on retries
//   - Delayed notifications
//   - Rate limiting
func (b *OutboxEventBuilder) WithDelay(delay time.Duration) *OutboxEventBuilder {
	b.event.AvailableAt = time.Now().Add(delay)
	return b
}

// Build finalizes the event and returns it.
//
// Returns error if:
//   - Payload serialization failed
//   - Required fields are missing
func (b *OutboxEventBuilder) Build() (*OutboxEvent, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Validate required fields
	if b.event.EventType == "" {
		return nil, fmt.Errorf("event type is required")
	}
	if b.event.AggregateType == "" {
		return nil, fmt.Errorf("aggregate type is required")
	}
	if b.event.AggregateID == uuid.Nil {
		return nil, fmt.Errorf("aggregate ID is required")
	}
	if len(b.event.Payload) == 0 {
		return nil, fmt.Errorf("payload is required")
	}

	return b.event, nil
}

// MustBuild is like Build but panics on error.
//
// Use only when you're certain the event is valid (e.g., in tests).
func (b *OutboxEventBuilder) MustBuild() *OutboxEvent {
	event, err := b.Build()
	if err != nil {
		panic(err)
	}
	return event
}

// ═══════════════════════════════════════════════════════════════════════════
// CONVENIENCE FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

// NewWithdrawalEvent creates an outbox event for a withdrawal.
//
// Usage:
//
//	event, err := NewWithdrawalEvent(withdrawalID, WithdrawalMessage{
//	    WithdrawalID: withdrawalID,
//	    UserID:       userID,
//	    Amount:       amount,
//	    // ... other fields ...
//	})
func NewWithdrawalEvent(withdrawalID uuid.UUID, payload interface{}) (*OutboxEvent, error) {
	return NewOutboxEventBuilder(EventTypeWithdrawalProcess).
		WithAggregate("withdrawal", withdrawalID).
		WithPayload(payload).
		Build()
}

// NewDepositReceiptEvent creates an outbox event for a deposit receipt notification.
//
// This is sent AFTER a deposit is confirmed, not during deposit initiation.
// The payment has already been received and the wallet credited.
func NewDepositReceiptEvent(depositID uuid.UUID, payload interface{}) (*OutboxEvent, error) {
	return NewOutboxEventBuilder(EventTypeDepositReceipt).
		WithAggregate("deposit", depositID).
		WithPayload(payload).
		Build()
}

// NewEmailEvent creates an outbox event for sending an email.
//
// This allows email sending to be transactional and retryable.
func NewEmailEvent(aggregateType string, aggregateID uuid.UUID, payload interface{}) (*OutboxEvent, error) {
	return NewOutboxEventBuilder(EventTypeEmailDispatch).
		WithAggregate(aggregateType, aggregateID).
		WithPayload(payload).
		Build()
}

// NewSMSEvent creates an outbox event for sending an SMS.
//
// This allows SMS sending to be transactional and retryable.
func NewSMSEvent(aggregateType string, aggregateID uuid.UUID, payload interface{}) (*OutboxEvent, error) {
	return NewOutboxEventBuilder(EventTypeSMSDispatch).
		WithAggregate(aggregateType, aggregateID).
		WithPayload(payload).
		Build()
}

// NewRealtimeEvent creates an outbox event for a realtime push notification.
//
// This allows realtime notifications to be transactional and retryable.
func NewRealtimeEvent(aggregateType string, aggregateID uuid.UUID, payload interface{}) (*OutboxEvent, error) {
	return NewOutboxEventBuilder(EventTypeRealtimePush).
		WithAggregate(aggregateType, aggregateID).
		WithPayload(payload).
		Build()
}

// ═══════════════════════════════════════════════════════════════════════════
// PAYLOAD EXTRACTION
// ═══════════════════════════════════════════════════════════════════════════

// UnmarshalPayload deserializes the event payload into the given target struct.
//
// Usage:
//
//	var msg WithdrawalMessage
//	if err := event.UnmarshalPayload(&msg); err != nil {
//	    return err
//	}
func (e *OutboxEvent) UnmarshalPayload(target interface{}) error {
	if len(e.Payload) == 0 {
		return fmt.Errorf("payload is empty")
	}

	if err := json.Unmarshal(e.Payload, target); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// EXAMPLES
// ═══════════════════════════════════════════════════════════════════════════

// Example: Creating a withdrawal event in a service
//
//	func (s *WalletService) InitiateWithdrawal(ctx context.Context, userID uuid.UUID, amount int64) error {
//	    return s.db.Transaction(func(tx *gorm.DB) error {
//	        // 1. Lock funds
//	        if err := s.walletRepo.LockFunds(ctx, userID, amount, tx); err != nil {
//	            return err
//	        }
//
//	        // 2. Create pending transaction
//	        transaction := &WalletTransaction{
//	            UserID: userID,
//	            Type:   "withdrawal",
//	            Status: "pending",
//	            Amount: amount,
//	        }
//	        if err := tx.Create(transaction).Error; err != nil {
//	            return err
//	        }
//
//	        // 3. Create outbox event (atomic with business state)
//	        event, err := NewWithdrawalEvent(transaction.ID, WithdrawalMessage{
//	            WithdrawalID: transaction.ID,
//	            UserID:       userID,
//	            Amount:       amount,
//	            BankAccount:  accountDetails,
//	        })
//	        if err != nil {
//	            return err
//	        }
//
//	        // 4. Save event in same transaction
//	        if err := s.outboxRepo.CreateEvent(ctx, event, tx); err != nil {
//	            return err
//	        }
//
//	        return nil // Commit: both business state and event are persisted atomically
//	    })
//	}

// Example: Processing an event in the dispatcher
//
//	func (d *Dispatcher) processEvent(ctx context.Context, event *OutboxEvent) error {
//	    // 1. Unmarshal payload
//	    var msg WithdrawalMessage
//	    if err := event.UnmarshalPayload(&msg); err != nil {
//	        // Permanent error - mark failed
//	        return d.outboxRepo.MarkFailed(ctx, event.ID, err.Error())
//	    }
//
//	    // 2. Publish to RabbitMQ
//	    if err := d.queue.Publish(ctx, QueueWithdrawal, &msg); err != nil {
//	        // Transient error - retry later
//	        nextAttempt := calculateBackoff(event.Attempts)
//	        return d.outboxRepo.IncrementAttempts(ctx, event.ID, nextAttempt, err.Error())
//	    }
//
//	    // 3. Success - mark published
//	    return d.outboxRepo.MarkPublished(ctx, event.ID)
//	}
