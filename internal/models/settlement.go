package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Settlement batch lifecycle: expected (we booked the receivable) → received
// (the provider's payout report arrived) → matched (every item tied to a
// payment) → disputed (mismatch raised an alert).
const (
	SettlementStatusExpected = "expected"
	SettlementStatusReceived = "received"
	SettlementStatusMatched  = "matched"
	SettlementStatusDisputed = "disputed"
)

// SettlementBatch is one provider payout (Paystack settles on a schedule).
type SettlementBatch struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Provider    string     `gorm:"size:40;not null;default:'paystack'" json:"provider"`
	ExternalRef string     `gorm:"size:160;uniqueIndex;not null" json:"external_ref"`
	BatchDate   *time.Time `json:"batch_date,omitempty"`

	GrossKobo int64 `gorm:"not null;default:0" json:"gross_kobo"`
	FeesKobo  int64 `gorm:"not null;default:0" json:"fees_kobo"`
	NetKobo   int64 `gorm:"not null;default:0" json:"net_kobo"`

	Status string  `gorm:"size:20;not null;default:'expected'" json:"status"`
	Alert  *string `json:"alert,omitempty"`

	Items []SettlementItem `gorm:"foreignKey:BatchID" json:"items,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SettlementBatch) TableName() string { return "settlement_batches" }

// SettlementItem is a single provider transaction line inside a payout batch.
// Unmatched items are the ones that belong in Suspense (9000) until tied to a
// payment we booked.
type SettlementItem struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	BatchID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"batch_id"`
	ExternalRef string     `gorm:"size:160;not null;index" json:"external_ref"`
	AmountKobo  int64      `gorm:"not null" json:"amount_kobo"`
	Matched     bool       `gorm:"not null;default:false" json:"matched"`
	PaymentID   *uuid.UUID `gorm:"type:uuid" json:"payment_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (SettlementItem) TableName() string { return "settlement_items" }

// ReconciliationRun is one trust check (Phase 7.6). A run FAILS when the trial
// balance does not balance, a user sub-ledger does not mirror its wallet
// column, or Suspense holds money. status=fail raises an alert.
type ReconciliationRun struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Kind   string    `gorm:"size:40;not null;default:'nightly'" json:"kind"`
	RunAt  time.Time `gorm:"not null;default:now()" json:"run_at"`
	Status string    `gorm:"size:20;not null" json:"status"` // pass | fail

	TotalDebitsKobo   int64 `gorm:"not null;default:0" json:"total_debits_kobo"`
	TotalCreditsKobo  int64 `gorm:"not null;default:0" json:"total_credits_kobo"`
	UserLiabilityKobo int64 `gorm:"not null;default:0" json:"user_liability_kobo"`
	WalletBalanceKobo int64 `gorm:"not null;default:0" json:"wallet_balance_kobo"`
	CustodyAssetsKobo int64 `gorm:"not null;default:0" json:"custody_assets_kobo"`
	SuspenseKobo      int64 `gorm:"not null;default:0" json:"suspense_kobo"`
	DiscrepancyKobo   int64 `gorm:"not null;default:0" json:"discrepancy_kobo"`

	Details datatypes.JSON `gorm:"type:jsonb" json:"details,omitempty"`
	Alerted bool           `gorm:"not null;default:false" json:"alerted"`
}

func (ReconciliationRun) TableName() string { return "reconciliation_runs" }
