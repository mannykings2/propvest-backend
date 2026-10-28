package dto

import "time"

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN DASHBOARD DTOs (Milestone 7 - Phase 3)
// ═══════════════════════════════════════════════════════════════════════════

// DashboardResponse represents the admin dashboard with platform-wide metrics.
//
// BOUNDED QUERIES:
// All metric queries are bounded to prevent performance issues:
//   - User metrics: COUNT queries with WHERE conditions (no table scan)
//   - Financial metrics: SUM queries on indexed columns
//   - Date range filtering: Uses indexed created_at columns
//   - No unbounded queries or aggregations
//
// CACHING:
// Dashboard data can be cached for 5-10 minutes since metrics don't need
// real-time accuracy for operational monitoring.
type DashboardResponse struct {
	Users      UserMetrics      `json:"users"`
	Properties PropertyMetrics  `json:"properties"`
	Investments InvestmentMetrics `json:"investments"`
	Financial  FinancialMetrics `json:"financial"`
	Timestamp  time.Time        `json:"timestamp"`
}

// UserMetrics contains user-related platform statistics.
type UserMetrics struct {
	Total             int64 `json:"total"`              // Total non-deleted users
	ActiveThisMonth   int64 `json:"active_this_month"`  // Users with activity this month
	NewThisMonth      int64 `json:"new_this_month"`     // Users created this month
	NewLastMonth      int64 `json:"new_last_month"`     // Users created last month
	KYCPending        int64 `json:"kyc_pending"`        // Users with KYC status = pending
	KYCVerified       int64 `json:"kyc_verified"`       // Users with KYC status = verified
	SuspendedCount    int64 `json:"suspended_count"`    // Users currently suspended
}

// PropertyMetrics contains property-related statistics.
type PropertyMetrics struct {
	Total          int64 `json:"total"`            // Total properties (all statuses)
	Active         int64 `json:"active"`           // Status = active (accepting investments)
	Funded         int64 `json:"funded"`           // Status = funded (fully funded)
	Draft          int64 `json:"draft"`            // Status = draft (not published)
	Completed      int64 `json:"completed"`        // Status = completed (investment period ended)
	TotalValue     int64 `json:"total_value"`      // Sum of all property values (in kobo)
}

// InvestmentMetrics contains investment-related statistics.
type InvestmentMetrics struct {
	TotalCount         int64 `json:"total_count"`          // Total investment records
	ActiveCount        int64 `json:"active_count"`         // Status = active
	TotalValue         int64 `json:"total_value"`          // Sum of investment amounts (kobo)
	TotalInvestors     int64 `json:"total_investors"`      // Distinct user count with investments
	AverageInvestment  int64 `json:"average_investment"`   // Average investment amount (kobo)
}

// FinancialMetrics contains financial statistics.
type FinancialMetrics struct {
	TotalDeposits         int64 `json:"total_deposits"`          // Sum of completed deposits (kobo)
	TotalWithdrawals      int64 `json:"total_withdrawals"`       // Sum of completed withdrawals (kobo)
	PendingWithdrawals    int64 `json:"pending_withdrawals"`     // Sum of pending withdrawals (kobo)
	TotalPlatformBalance  int64 `json:"total_platform_balance"`  // Sum of all wallet main_balance + earnings_balance (kobo)
	DepositCount          int64 `json:"deposit_count"`           // Count of completed deposits
	WithdrawalCount       int64 `json:"withdrawal_count"`        // Count of completed withdrawals
}

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN USER MANAGEMENT DTOs (Milestone 7 - Phase 4)
// ═══════════════════════════════════════════════════════════════════════════

// AdminUserListResponse represents paginated user list for admin.
type AdminUserListResponse struct {
	Users      []AdminUserSummary `json:"users"`
	TotalCount int64              `json:"total_count"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int                `json:"total_pages"`
}

// AdminUserSummary is a condensed user view for list endpoints.
type AdminUserSummary struct {
	ID                string    `json:"id"`
	UserCode          string    `json:"user_code"`
	FullName          string    `json:"full_name"`
	Email             string    `json:"email"`
	Phone             *string   `json:"phone,omitempty"`
	Role              string    `json:"role"`
	KYCStatus         string    `json:"kyc_status"`
	IsActive          bool      `json:"is_active"`
	IsSuspended       bool      `json:"is_suspended"`
	InvestmentCount   int64     `json:"investment_count"`
	TotalInvested     int64     `json:"total_invested"`      // In kobo
	WalletBalance     int64     `json:"wallet_balance"`      // Main + Earnings (kobo)
	CreatedAt         time.Time `json:"created_at"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
}

// AdminUserDetailResponse represents full user details for admin.
type AdminUserDetailResponse struct {
	ID                string     `json:"id"`
	UserCode          string     `json:"user_code"`
	FullName          string     `json:"full_name"`
	Email             string     `json:"email"`
	Phone             *string    `json:"phone,omitempty"`
	Role              string     `json:"role"`
	KYCStatus         string     `json:"kyc_status"`
	KYCSubmittedAt    *time.Time `json:"kyc_submitted_at,omitempty"`
	KYCVerifiedAt     *time.Time `json:"kyc_verified_at,omitempty"`
	IsActive          bool       `json:"is_active"`
	IsSuspended       bool       `json:"is_suspended"`
	SuspendedAt       *time.Time `json:"suspended_at,omitempty"`
	SuspendedBy       *string    `json:"suspended_by,omitempty"`       // Admin user ID
	SuspendedReason   *string    `json:"suspended_reason,omitempty"`
	InvestmentCount   int64      `json:"investment_count"`
	TotalInvested     int64      `json:"total_invested"`      // In kobo
	WalletMainBalance int64      `json:"wallet_main_balance"` // In kobo
	WalletEarnings    int64      `json:"wallet_earnings"`     // In kobo
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
}

// UpdateUserRequest represents admin's request to update user details.
type UpdateUserRequest struct {
	FullName  *string `json:"full_name,omitempty" binding:"omitempty,min=2,max=100"`
	Phone     *string `json:"phone,omitempty" binding:"omitempty,e164"`
	KYCStatus *string `json:"kyc_status,omitempty" binding:"omitempty,oneof=pending verified rejected"`
	IsActive  *bool   `json:"is_active,omitempty"`
}

// SuspendUserRequest represents admin's request to suspend a user.
type SuspendUserRequest struct {
	Reason string `json:"reason" binding:"required,min=10,max=500"`
}

// ChangeRoleRequest represents super-admin's request to change user role.
type ChangeRoleRequest struct {
	NewRole string `json:"new_role" binding:"required,oneof=investor developer staff admin super_admin"`
	Reason  string `json:"reason" binding:"required,min=10,max=500"`
}

// DecideKYCRequest represents admin's KYC approval/rejection decision.
type DecideKYCRequest struct {
	Decision string  `json:"decision" binding:"required,oneof=verified rejected"`
	Reason   *string `json:"reason,omitempty" binding:"omitempty,min=10,max=500"` // Required if rejected
}

// ForcePasswordResetRequest represents admin's request to force password reset.
type ForcePasswordResetRequest struct {
	Reason string `json:"reason" binding:"required,min=10,max=500"`
}
