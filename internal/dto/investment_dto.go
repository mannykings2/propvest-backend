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
	
	// IdempotencyKey is optional. If provided, prevents duplicate investments on retries.
	// Should be a unique string per investment attempt (e.g., UUID).
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// InvestmentResponse is the public representation of an investment.
type InvestmentResponse struct {
	ID            uuid.UUID         `json:"id"`
	PropertyID    uuid.UUID         `json:"property_id"`
	Property      *PropertyResponse `json:"property,omitempty"`
	Slots         int               `json:"slots"`
	AmountKobo    int64             `json:"amount_kobo"`
	UnitPriceKobo int64             `json:"unit_price_kobo"` // Price snapshot
	Currency      string            `json:"currency"`
	Status        string            `json:"status"`
	Reference     string            `json:"reference"`
	CancelledAt   *time.Time        `json:"cancelled_at,omitempty"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
	RefundedAt    *time.Time        `json:"refunded_at,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// PortfolioSummaryResponse aggregates a user's holdings for the dashboard.
type PortfolioSummaryResponse struct {
	TotalInvested   int64 `json:"total_invested"`   // kobo
	ActiveCount     int64 `json:"active_count"`     // number of active investments
	WalletBalance   int64 `json:"wallet_balance"`   // main balance (kobo)
	EarningsBalance int64 `json:"earnings_balance"` // earnings balance (kobo)
}

// InvestmentListQuery holds query parameters for listing investments with filters.
type InvestmentListQuery struct {
	// Pagination
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
	
	// Filters
	Status     string    `form:"status" binding:"omitempty,oneof=active completed cancelled refunded"`
	PropertyID uuid.UUID `form:"property_id" binding:"omitempty"`
	
	// Sorting
	SortBy    string `form:"sort_by" binding:"omitempty,oneof=created_at amount_kobo"`
	SortOrder string `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// SetDefaults sets default values for pagination if not provided.
func (q *InvestmentListQuery) SetDefaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	if q.SortBy == "" {
		q.SortBy = "created_at"
	}
	if q.SortOrder == "" {
		q.SortOrder = "desc"
	}
}

// GetLimitOffset converts page-based pagination to limit/offset.
func (q *InvestmentListQuery) GetLimitOffset() (int, int) {
	limit := q.PageSize
	offset := (q.Page - 1) * q.PageSize
	return limit, offset
}

// InvestmentListResponse wraps a list of investments with pagination metadata.
type InvestmentListResponse struct {
	Investments []InvestmentResponse `json:"investments"`
	Pagination  PaginationMetadata   `json:"pagination"`
}

// NewInvestmentPaginationMetadata creates pagination metadata from query and total count.
// Uses the existing PaginationMetadata struct from property_dto.go
func NewInvestmentPaginationMetadata(page, pageSize int, totalItems int64) PaginationMetadata {
	totalPages := int(totalItems) / pageSize
	if int(totalItems)%pageSize > 0 {
		totalPages++
	}
	
	return PaginationMetadata{
		CurrentPage:  page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
		TotalRecords: totalItems,
		HasNext:      page < totalPages,
		HasPrevious:  page > 1,
	}
}

// CancelInvestmentRequest holds the payload for cancelling an investment.
type CancelInvestmentRequest struct {
	Reason string `json:"reason" binding:"required,min=10,max=500"`
}

// InvestmentMetricsResponse holds platform-wide investment statistics for admin dashboard.
type InvestmentMetricsResponse struct {
	TotalInvestments      int64   `json:"total_investments"`
	TotalAmountKobo       int64   `json:"total_amount_kobo"`
	TotalAmountNaira      float64 `json:"total_amount_naira"` // Derived for convenience
	ActiveInvestments     int64   `json:"active_investments"`
	CompletedInvestments  int64   `json:"completed_investments"`
	CancelledInvestments  int64   `json:"cancelled_investments"`
	RefundedInvestments   int64   `json:"refunded_investments"`
	UniqueInvestors       int64   `json:"unique_investors"`
	AverageInvestmentKobo int64   `json:"average_investment_kobo"`
	AverageInvestmentNaira float64 `json:"average_investment_naira"` // Derived for convenience
}
