package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Wallet represents the financial account belonging to a single User.
// Every user gets exactly one wallet, created atomically with their
// account during registration (enforced by the transaction in Step 7).
type Wallet struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// UserID is the foreign key linking this wallet to its owner.
	// `uniqueIndex` enforces the one-to-one relationship at the DB level.
	// Without this constraint, a bug in our code could create two wallets
	// for the same user — and both balances would be wrong.
	UserID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null" json:"user_id"`

	// MainBalance is the primary spendable balance.
	// Stored in kobo (smallest Naira unit).
	// int64 can hold up to 9,223,372,036,854,775,807 kobo =
	// roughly ₦92 trillion. More than enough.
	// NEVER use float64 for money. See the design notes above.
	MainBalance int64 `gorm:"default:0;not null" json:"main_balance"`

	// EarningsBalance holds rental income and investment returns.
	// Kept separate from MainBalance so the frontend can display them
	// distinctly (as your Dashboard mockup does).
	EarningsBalance int64 `gorm:"default:0;not null" json:"earnings_balance"`

	// VirtualAcctNo and VirtualBank are populated later when we integrate
	// with a payment provider (Paystack/Flutterwave) to generate a
	// dedicated virtual bank account for each user's top-ups.
	// They are nullable because they don't exist at registration time.
	VirtualAcctNo *string `json:"virtual_acct_no,omitempty"`
	VirtualBank   *string `json:"virtual_bank,omitempty"`

	// Currency tracks the wallet currency (NGN, USD, etc.)
	// Defaults to NGN (Nigerian Naira) for all users
	// Allows future multi-currency support without schema changes
	Currency string `gorm:"default:'NGN';not null" json:"currency"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}


// WalletTransaction is the append-only financial ledger. Every balance change
// writes exactly one row here (docs 2.3 Transaction Model). Rows are NEVER
// updated or deleted once completed; corrections are new compensating rows.
//
// DECISION D5: the original table only had wallet_id/type/amount/reference. We
// extended it (migration 000007) with user_id, external_reference,
// idempotency_key, and metadata so it can satisfy the transaction-model doc and
// support idempotent deposits/withdrawals — WITHOUT renaming the table (which
// would break existing code/data).
type WalletTransaction struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WalletID uuid.UUID `gorm:"type:uuid;not null;index" json:"wallet_id"`
	// UserID denormalizes the owner so we can query "all of a user's
	// transactions" without joining through wallets.
	UserID uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	// Type: deposit|withdrawal|investment|refund|reversal|fee|rental_income|transfer
	Type          string `gorm:"not null" json:"type"`
	Amount        int64  `gorm:"not null" json:"amount"` // kobo, always positive
	BalanceBefore int64  `gorm:"not null" json:"balance_before"`
	BalanceAfter  int64  `gorm:"not null" json:"balance_after"`
	// Reference is our unique internal reference and doubles as the idempotency
	// anchor for ledger writes.
	Reference string `gorm:"uniqueIndex;not null" json:"reference"`
	// ExternalReference is the payment provider's reference (Paystack), when any.
	ExternalReference *string `gorm:"index" json:"external_reference,omitempty"`
	// IdempotencyKey is the client-supplied key that prevents duplicate creation
	// on retries. Unique when present.
	IdempotencyKey *string `gorm:"uniqueIndex" json:"idempotency_key,omitempty"`
	Description    string  `json:"description"`
	// Status: pending|processing|completed|failed|reversed
	Status string `gorm:"default:'completed'" json:"status"`
	// Metadata is free-form JSON for provider payloads / extra context.
	Metadata  datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}