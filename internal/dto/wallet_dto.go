package dto

import (
	"time"

	"github.com/google/uuid"
)

// WalletResponse is the public representation of a user's wallet.
// main_balance and earnings_balance are returned in KOBO.
// The frontend is responsible for dividing by 100 to display Naira.
//
// Why return raw kobo instead of formatted strings?
// The API should return data. The frontend should handle presentation.
// If you return "₦1,500.00" as a string, the frontend can't do math on it.
// If you return 150000 as an integer, the frontend can both display it
// and calculate things like "total portfolio value".
type WalletResponse struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	MainBalance     int64     `json:"main_balance"`
	EarningsBalance int64     `json:"earnings_balance"`
	VirtualAcctNo   *string   `json:"virtual_acct_no,omitempty"`
	VirtualBank     *string   `json:"virtual_bank,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// WalletSummaryResponse is a lighter version used in dashboard widgets
// where you only need balances, not the full wallet object.
type WalletSummaryResponse struct {
	MainBalance     int64 `json:"main_balance"`
	EarningsBalance int64 `json:"earnings_balance"`
}