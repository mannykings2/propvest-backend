package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Investment records a user's purchase of slots/shares in a property (docs 1.2
// §9, 5.3, 2.2 §14). Creating one is the platform's core financial workflow and
// must be atomic with the wallet debit and the property funding update
// (see InvestmentService).
type Investment struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	PropertyID uuid.UUID `gorm:"type:uuid;not null;index" json:"property_id"`

	// Slots purchased and the total amount paid (kobo). AmountKobo == Slots *
	// property.SlotPrice at purchase time (we snapshot it so later price changes
	// don't rewrite history).
	Slots      int   `gorm:"not null" json:"slots"`
	AmountKobo int64 `gorm:"not null" json:"amount_kobo"`

	// UnitPriceKobo is the price per slot at the moment of purchase (immutable).
	// This snapshot protects against property price changes. NEVER recalculate
	// from AmountKobo/Slots after creation.
	UnitPriceKobo int64 `gorm:"not null" json:"unit_price_kobo"`

	// Currency tracks the investment currency (NGN, USD, etc.).
	// Defaults to NGN for all investments.
	Currency string `gorm:"default:'NGN';not null;index" json:"currency"`

	// Status lifecycle: created -> active -> completed (or cancelled/refunded).
	Status string `gorm:"default:'active';not null;index" json:"status"`

	// Reference ties this investment to its wallet_transactions ledger row.
	Reference string `gorm:"uniqueIndex;not null" json:"reference"`

	// IdempotencyKey prevents duplicate investments when clients retry requests.
	// Unique per user (enforced by partial unique index in migration 000021).
	IdempotencyKey *string `gorm:"index" json:"idempotency_key,omitempty"`

	// Lifecycle timestamps for tracking status changes
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	RefundedAt  *time.Time `json:"refunded_at,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Association for eager-loading the property in portfolio views.
	Property Property `gorm:"foreignKey:PropertyID" json:"property,omitempty"`
}

// Investment status constants.
const (
	InvestmentStatusActive    = "active"
	InvestmentStatusCompleted = "completed"
	InvestmentStatusCancelled = "cancelled"
	InvestmentStatusRefunded  = "refunded"
)

// IsActive checks if the investment is in active state.
func (i *Investment) IsActive() bool {
	return i.Status == InvestmentStatusActive
}

// IsCancelled checks if the investment has been cancelled.
func (i *Investment) IsCancelled() bool {
	return i.Status == InvestmentStatusCancelled
}

// IsRefunded checks if the investment has been refunded.
func (i *Investment) IsRefunded() bool {
	return i.Status == InvestmentStatusRefunded
}

// IsCompleted checks if the investment has reached completed state.
func (i *Investment) IsCompleted() bool {
	return i.Status == InvestmentStatusCompleted
}

// CanBeCancelled checks if the investment can be cancelled.
// Only active investments can be cancelled.
func (i *Investment) CanBeCancelled() bool {
	return i.Status == InvestmentStatusActive
}
