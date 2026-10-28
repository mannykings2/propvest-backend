package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN DASHBOARD HANDLER (Milestone 7 - Phase 3)
// ═══════════════════════════════════════════════════════════════════════════
//
// Provides admin dashboard with platform-wide metrics and operational views.
//
// ENDPOINTS:
//   GET /api/v1/admin/dashboard - Platform-wide metrics
//
// AUTHORIZATION:
//   - Requires authentication (JWT)
//   - Requires admin or super_admin role
//
// PERFORMANCE:
//   - All queries are bounded and use indexed columns
//   - Response can be cached for 5-10 minutes
//   - No unbounded queries or table scans

// AdminDashboardHandler handles admin dashboard requests.
type AdminDashboardHandler struct {
	dashboardService services.DashboardService
}

// NewAdminDashboardHandler creates a new admin dashboard handler.
func NewAdminDashboardHandler(dashboardService services.DashboardService) *AdminDashboardHandler {
	return &AdminDashboardHandler{
		dashboardService: dashboardService,
	}
}

// GetDashboard returns platform-wide metrics for admin dashboard.
//
// GET /api/v1/admin/dashboard
//
// Authorization: admin or super_admin
//
// Response:
//   {
//     "users": {
//       "total": 1250,
//       "active_this_month": 320,
//       "new_this_month": 45,
//       "new_last_month": 38,
//       "kyc_pending": 12,
//       "kyc_verified": 890,
//       "suspended_count": 3
//     },
//     "properties": {
//       "total": 25,
//       "active": 8,
//       "funded": 12,
//       "draft": 3,
//       "completed": 2,
//       "total_value": 250000000000
//     },
//     "investments": {
//       "total_count": 3420,
//       "active_count": 2890,
//       "total_value": 125000000000,
//       "total_investors": 780,
//       "average_investment": 36549707
//     },
//     "financial": {
//       "total_deposits": 180000000000,
//       "total_withdrawals": 55000000000,
//       "pending_withdrawals": 2500000000,
//       "total_platform_balance": 125000000000,
//       "deposit_count": 4560,
//       "withdrawal_count": 890
//     },
//     "timestamp": "2026-10-06T14:23:45Z"
//   }
func (h *AdminDashboardHandler) GetDashboard(c *gin.Context) {
	// Extract actor from JWT
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		logger.Error("user_id not found in context after auth middleware", "error", err)
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	// Get dashboard metrics
	dashboard, err := h.dashboardService.GetDashboard(c.Request.Context(), actorUUID)
	if err != nil {
		logger.Error("failed to get dashboard metrics",
			"error", err,
			"actor_id", actorUUID.String())
		response.Error(c, http.StatusInternalServerError, "failed to load dashboard", nil)
		return
	}

	response.Success(c, http.StatusOK, dashboard)
}
