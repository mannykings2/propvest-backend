package dto

import (
	"time"

	"github.com/google/uuid"
)

// AdminDashboardResponse is the operational summary for administrators.
type AdminDashboardResponse struct {
	TotalUsers        int64 `json:"total_users"`
	TotalProperties   int64 `json:"total_properties"`
	PendingApprovals  int64 `json:"pending_approvals"`
	TotalInvestments  int64 `json:"total_investments"`
	TotalInvestedKobo int64 `json:"total_invested_kobo"`
	TotalWalletVolume int64 `json:"total_wallet_volume_kobo"`
}

// AdminUserResponse is the admin view of a user (a bit more than the public one).
type AdminUserResponse struct {
	ID            uuid.UUID `json:"id"`
	UserCode      string    `json:"user_code"`
	FullName      string    `json:"full_name"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	Role          string    `json:"role"`
	KYCStatus     string    `json:"kyc_status"`
	IsActive      bool      `json:"is_active"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpdateUserRequest lets an admin change a user's status/role.
// Action is one of: suspend|activate|promote|demote|verify_kyc.
type UpdateUserRequest struct {
	Action string `json:"action" binding:"required,oneof=suspend activate promote demote verify_kyc"`
	Role   string `json:"role"` // used with promote/demote
}
