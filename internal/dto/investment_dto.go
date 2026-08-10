package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateInvestmentRequest is the investor's purchase payload. Slots is the number
// of property slots to buy; the amount is derived server-side (slots * price) so
// the client can't dictate the price.
type CreateInvestmentRequest struct {
	PropertyID uuid.UUID `json:"property_id" binding:"required"`
	Slots      int       `json:"slots" binding:"required,gt=0"`
}

// InvestmentResponse is the public representation of an investment.
type InvestmentResponse struct {
	ID         uuid.UUID         `json:"id"`
	PropertyID uuid.UUID         `json:"property_id"`
	Property   *PropertyResponse `json:"property,omitempty"`
	Slots      int               `json:"slots"`
	AmountKobo int64             `json:"amount_kobo"`
	Status     string            `json:"status"`
	Reference  string            `json:"reference"`
	CreatedAt  time.Time         `json:"created_at"`
}

// PortfolioSummaryResponse aggregates a user's holdings for the dashboard.
type PortfolioSummaryResponse struct {
	TotalInvested   int64 `json:"total_invested"`   // kobo
	ActiveCount     int64 `json:"active_count"`     // number of active investments
	WalletBalance   int64 `json:"wallet_balance"`   // main balance (kobo)
	EarningsBalance int64 `json:"earnings_balance"` // earnings balance (kobo)
}
