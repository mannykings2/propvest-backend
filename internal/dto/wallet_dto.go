package dto

import (
	"time"

	"github.com/google/uuid"
)

// WalletResponse is the public representation of a user's wallet.
// main_balance and earnings_balance are returned in KOBO (DECISION D4).
// The frontend divides by 100 to display Naira; keeping integers lets the
// client do exact math (no float rounding).
type WalletResponse struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	MainBalance     int64     `json:"main_balance"`
	EarningsBalance int64     `json:"earnings_balance"`
	Currency        string    `json:"currency"`
	VirtualAcctNo   *string   `json:"virtual_acct_no,omitempty"`
	VirtualBank     *string   `json:"virtual_bank,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// WalletSummaryResponse is a lighter version for dashboard widgets.
type WalletSummaryResponse struct {
	MainBalance     int64 `json:"main_balance"`
	EarningsBalance int64 `json:"earnings_balance"`
}

// DepositRequest starts a wallet top-up. Amount is in kobo. An optional
// Idempotency-Key header (not this body) guards against duplicate submits.
type DepositRequest struct {
	Amount int64 `json:"amount" binding:"required,gt=0" example:"5000000"` // kobo
}

// DepositResponse returns the provider's hosted-payment URL and our reference.
type DepositResponse struct {
	AuthorizationURL string `json:"authorization_url"`
	Reference        string `json:"reference"`
}

// WithdrawRequest requests a payout to a bank account. Amount is in kobo.
type WithdrawRequest struct {
	Amount        int64  `json:"amount" binding:"required,gt=0"`
	BankCode      string `json:"bank_code" binding:"required"`
	AccountNumber string `json:"account_number" binding:"required,min=10,max=10"`
	AccountName   string `json:"account_name" binding:"required"`
}

// TransactionResponse is the public representation of a ledger row.
type TransactionResponse struct {
	ID            uuid.UUID `json:"id"`
	Type          string    `json:"type"`
	Amount        int64     `json:"amount"`
	BalanceBefore int64     `json:"balance_before"`
	BalanceAfter  int64     `json:"balance_after"`
	Reference     string    `json:"reference"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}
