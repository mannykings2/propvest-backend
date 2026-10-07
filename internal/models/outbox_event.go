package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// OutboxEvent represents a transactional outbox event for reliable async messaging.
//
// THE OUTBOX PATTERN:
// The outbox pattern ensures that business state changes and corresponding async
// events are committed atomically in a single database transaction. This eliminates
// the reliability gap where messages could be lost if RabbitMQ is unavailable or
// the API crashes after committing database changes.
//
// USAGE:
// When a business operation needs to trigger async processing:
//
//	err := db.Transaction(func(tx *gorm.DB) error {
//	    // 1. Perform business operation
//	    withdrawal := &WalletTransaction{...}
//	    tx.Create(withdrawal)
//
//	    // 2. Create outbox event in SAME transaction
//	    event := &OutboxEvent{
//	        EventType:     EventTypeWithdrawalProcess,
//	        AggregateType: "wallet_transaction",
//	        AggregateID:   withdrawal.ID,
//	        Payload:       marshalledMessage,
//	    }
//	    tx.Create(event)
//
//	    return nil  // Both commit atomically or both rollback
//	})
//
// A separate dispatcher process polls the outbox table and publishes events
// to RabbitMQ with retry logic.
//
// GUARANTEES:
// - At-least-once delivery (events may be delivered multiple times)
// - Zero message loss (if committed to DB, will eventually be published)
// - Survives RabbitMQ downtime (events queued in DB until available)
// - Survives API crashes (events persisted before response sent)
type OutboxEvent struct {
	// ID is the unique identifier for this outbox event
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// EventType identifies what kind of event this is (e.g., "withdrawal.process")
	// This determines which message queue the event will be published to
	EventType string `gorm:"not null;index" json:"event_type"`

	// AggregateType identifies the business entity type (e.g., "wallet_transaction")
	// Used for querying/debugging events related to specific entity types
	AggregateType string `gorm:"not null" json:"aggregate_type"`

	// AggregateID is the ID of the specific business entity this event relates to
	AggregateID uuid.UUID `gorm:"type:uuid;not null;index" json:"aggregate_id"`

	// Payload is the JSON message body that will be published to the message queue
	// Contains all data needed by the consumer to process the event
	Payload datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`

	// Status tracks the processing state of this event
	// pending:   Event created, waiting to be claimed by dispatcher
	// claimed:   Dispatcher has claimed event and is publishing it
	// published: Event successfully published to message queue
	// failed:    Event failed max retry attempts, moved to failed status
	Status string `gorm:"not null;default:'pending';index" json:"status"`

	// Attempts tracks how many times we've tried to publish this event
	// Used for retry logic and determining when to give up
	Attempts int `gorm:"default:0" json:"attempts"`

	// AvailableAt indicates when this event becomes eligible for processing
	// Used for delayed retry with exponential backoff:
	// - Attempt 1 fails: available_at = now + 5 seconds
	// - Attempt 2 fails: available_at = now + 15 seconds
	// - Attempt 3 fails: available_at = now + 45 seconds
	AvailableAt time.Time `gorm:"not null;default:now();index" json:"available_at"`

	// ClaimedAt records when this event was claimed by a dispatcher instance
	// Used for detecting stale claims (dispatcher crashed before completing)
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`

	// ClaimedBy identifies which dispatcher instance claimed this event
	// Useful for debugging and detecting stuck events
	ClaimedBy *string `json:"claimed_by,omitempty"`

	// PublishedAt records when the event was successfully published
	// Used for metrics and auditing
	PublishedAt *time.Time `json:"published_at,omitempty"`

	// LastError stores the most recent error message from a failed publish attempt
	// Useful for debugging and monitoring
	LastError *string `gorm:"type:text" json:"last_error,omitempty"`

	// Standard audit timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Status constants for OutboxEvent.Status field
const (
	// OutboxStatusPending means event is waiting to be claimed and published
	OutboxStatusPending = "pending"

	// OutboxStatusClaimed means dispatcher has claimed this event and is processing it
	OutboxStatusClaimed = "claimed"

	// OutboxStatusPublished means event was successfully published to message queue
	OutboxStatusPublished = "published"

	// OutboxStatusFailed means event failed max retry attempts (typically 3)
	// Failed events should be investigated and may need manual intervention
	OutboxStatusFailed = "failed"
)

// Event type constants for OutboxEvent.EventType field
// These map to specific message queues and consumer handlers
const (
	// EventTypeWithdrawalProcess triggers async withdrawal processing by worker
	// Queue: propvest.withdrawal.process
	// Payload: WithdrawalMessage
	EventTypeWithdrawalProcess = "withdrawal.process"

	// EventTypeDepositReceipt triggers deposit receipt email generation
	// Queue: propvest.deposit.receipt
	// Payload: DepositReceiptMessage
	EventTypeDepositReceipt = "deposit.receipt"

	// EventTypeEmailDispatch triggers email sending
	// Queue: propvest.email.dispatch
	// Payload: EmailMessage
	EventTypeEmailDispatch = "email.dispatch"

	// EventTypeSMSDispatch triggers SMS sending
	// Queue: propvest.sms.dispatch
	// Payload: SMSMessage
	EventTypeSMSDispatch = "sms.dispatch"

	// EventTypeRealtimePush triggers realtime notification to connected clients
	// Queue: propvest.realtime.push
	// Payload: RealtimeMessage
	EventTypeRealtimePush = "realtime.push"

	// ═══════════════════════════════════════════════════════════════════════════
	// PROPERTY MODULE EVENTS (Milestone 4)
	// ═══════════════════════════════════════════════════════════════════════════

	// EventTypePropertyPublished triggers notifications when a property is published
	// Queue: propvest.property.published
	// Payload: PropertyPublishedPayload
	// Triggered: When admin publishes a draft property (draft → active)
	// Consumers:
	//   - Email service (notify admin/stakeholders)
	//   - Analytics service (track property launches)
	//   - Search indexer (add to search index)
	EventTypePropertyPublished = "property.published"

	// EventTypePropertyFunded triggers notifications when a property reaches funding goal
	// Queue: propvest.property.funded
	// Payload: PropertyFundedPayload
	// Triggered: When property reaches 100% funding (active → funded)
	// Consumers:
	//   - Email service (notify all investors)
	//   - Notification service (push notifications)
	//   - Analytics service (track funding completion)
	EventTypePropertyFunded = "property.funded"

	// EventTypePropertyCompleted triggers notifications when property investment period ends
	// Queue: propvest.property.completed
	// Payload: PropertyCompletedPayload
	// Triggered: When property maturity date is reached (funded → completed)
	// Consumers:
	//   - Returns calculator (calculate investor returns)
	//   - Email service (notify investors of completion)
	//   - Payout processor (initiate return payments)
	EventTypePropertyCompleted = "property.completed"
)

// IsPending returns true if the event is waiting to be processed
func (e *OutboxEvent) IsPending() bool {
	return e.Status == OutboxStatusPending
}

// IsClaimed returns true if the event has been claimed by a dispatcher
func (e *OutboxEvent) IsClaimed() bool {
	return e.Status == OutboxStatusClaimed
}

// IsPublished returns true if the event was successfully published
func (e *OutboxEvent) IsPublished() bool {
	return e.Status == OutboxStatusPublished
}

// IsFailed returns true if the event failed max retry attempts
func (e *OutboxEvent) IsFailed() bool {
	return e.Status == OutboxStatusFailed
}

// CanRetry returns true if the event hasn't exceeded max retry attempts
func (e *OutboxEvent) CanRetry(maxAttempts int) bool {
	return e.Attempts < maxAttempts
}

// IsStale returns true if the event was claimed but not completed within the timeout
// Used by the dispatcher to detect and recover events from crashed instances
func (e *OutboxEvent) IsStale(staleTimeout time.Duration) bool {
	return e.IsClaimed() &&
		e.ClaimedAt != nil &&
		time.Since(*e.ClaimedAt) > staleTimeout
}
