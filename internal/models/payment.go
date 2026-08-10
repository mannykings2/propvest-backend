package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Payment is the provider-side record of a funding attempt (docs 1.2 §13,
// 2.2 §18). It is SEPARATE from wallet_transactions:
//
//   - payments  = "what did the gateway tell us about this attempt?" (one row
//     per initialize/verify/webhook lifecycle, stores raw provider payload).
//   - wallet_transactions = "how did the user's balance actually change?" (the
//     immutable internal ledger).
//
// We only credit the wallet (create a wallet_transaction) once a payment is
// verified. Keeping them separate means we can reconcile our ledger against the
// provider and never lose the raw evidence of what happened.
type Payment struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	Provider string `gorm:"not null" json:"provider"` // "paystack" | "mock"
	// Reference is our internal reference (also used on the wallet_transaction).
	Reference string `gorm:"uniqueIndex;not null" json:"reference"`
	// ProviderReference is the gateway's own id, filled after init/verify.
	ProviderReference *string `gorm:"index" json:"provider_reference,omitempty"`

	AmountKobo int64  `gorm:"not null" json:"amount_kobo"`
	Currency   string `gorm:"default:'NGN';not null" json:"currency"`
	// Status: pending|success|failed
	Status string `gorm:"default:'pending';not null;index" json:"status"`
	// Channel: "deposit" | "withdrawal"
	Channel string `gorm:"not null" json:"channel"`

	AuthorizationURL *string        `json:"authorization_url,omitempty"`
	RawPayload       datatypes.JSON `gorm:"type:jsonb" json:"-"` // raw webhook/verify body

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Payment status + channel constants.
const (
	PaymentStatusPending = "pending"
	PaymentStatusSuccess = "success"
	PaymentStatusFailed  = "failed"

	PaymentChannelDeposit    = "deposit"
	PaymentChannelWithdrawal = "withdrawal"
)
