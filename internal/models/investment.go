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

	// Status lifecycle: created -> active -> completed (or cancelled/refunded).
	Status string `gorm:"default:'active';not null;index" json:"status"`

	// Reference ties this investment to its wallet_transactions ledger row.
	Reference string `gorm:"uniqueIndex;not null" json:"reference"`

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
