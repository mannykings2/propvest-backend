package models

import (
    "time"
    "github.com/google/uuid"
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


type WalletTransaction struct {
    ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    WalletID      uuid.UUID `gorm:"type:uuid;not null;index"`
    Type          string    `gorm:"not null"` // deposit|withdrawal|investment|rental_income|transfer|fee
    Amount        int64     `gorm:"not null"`
    BalanceBefore int64     `gorm:"not null"`
    BalanceAfter  int64     `gorm:"not null"`
    Reference     string    `gorm:"uniqueIndex;not null"`
    Description   string
    Status        string    `gorm:"default:'completed'"` // pending|completed|failed|reversed
    CreatedAt     time.Time
    // NOTE: Never UPDATE or DELETE rows in this table — it is an append-only ledger
}