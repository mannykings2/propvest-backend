package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN USER HANDLER (Milestone 7 - Phase 4)
// ═══════════════════════════════════════════════════════════════════════════

// AdminUserHandler handles admin user management endpoints
type AdminUserHandler struct {
	adminUserService services.AdminUserService
}

// NewAdminUserHandler creates a new admin user handler
func NewAdminUserHandler(adminUserService services.AdminUserService) *AdminUserHandler {
	return &AdminUserHandler{
		adminUserService: adminUserService,
	}
}

// ListUsers returns paginated list of users
// GET /api/v1/admin/users
func (h *AdminUserHandler) ListUsers(c *gin.Context) {
	// Extract actor from JWT
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Build filter from query params
	filter := repositories.UserFilter{
		Search: c.Query("search"),
	}

	if role := c.Query("role"); role != "" {
		filter.Role = &role
	}
	if kycStatus := c.Query("kyc_status"); kycStatus != "" {
		filter.KYCStatus = &kycStatus
	}
	if suspended := c.Query("suspended"); suspended == "true" {
		t := true
		filter.IsSuspended = &t
	} else if suspended == "false" {
		f := false
		filter.IsSuspended = &f
	}

	// Call service
	result, err := h.adminUserService.ListUsers(c.Request.Context(), actorUUID, filter, page, pageSize)
	if err != nil {
		logger.Error("failed to list users", "error", err, "actor_id", actorUUID)
		response.Error(c, http.StatusInternalServerError, "failed to list users", nil)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// GetUser returns detailed user information
// GET /api/v1/admin/users/:id
func (h *AdminUserHandler) GetUser(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	result, err := h.adminUserService.GetUserDetail(c.Request.Context(), actorUUID, targetUserID)
	if err != nil {
		logger.Error("failed to get user detail", "error", err, "target_user_id", targetUserID)
		response.Error(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// UpdateUser updates user profile fields
// PATCH /api/v1/admin/users/:id
func (h *AdminUserHandler) UpdateUser(c *gin.Context) {
	response.Success(c, http.StatusOK, gin.H{
		"message": "Not implemented yet - Phase 4",
	}, nil)
}

// SuspendUser suspends a user account
// PATCH /api/v1/admin/users/:id/suspend
func (h *AdminUserHandler) SuspendUser(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	var req dto.SuspendUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request", nil)
		return
	}

	err = h.adminUserService.SuspendUser(auditContext(c), actorUUID, targetUserID, req.Reason)
	if err != nil {
		logger.Error("failed to suspend user", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, gin.H{"message": "user suspended successfully"})
}

// ActivateUser activates a suspended user account
// PATCH /api/v1/admin/users/:id/activate
func (h *AdminUserHandler) ActivateUser(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	err = h.adminUserService.ActivateUser(auditContext(c), actorUUID, targetUserID)
	if err != nil {
		logger.Error("failed to activate user", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, gin.H{"message": "user activated successfully"})
}

// ForcePasswordReset initiates password reset for user
// POST /api/v1/admin/users/:id/reset-password
func (h *AdminUserHandler) ForcePasswordReset(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	var req dto.ForcePasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request", nil)
		return
	}

	err = h.adminUserService.ForcePasswordReset(auditContext(c), actorUUID, targetUserID, req.Reason)
	if err != nil {
		logger.Error("failed to force password reset", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, gin.H{"message": "password reset initiated"})
}

// LogoutAllSessions revokes all sessions for user
// POST /api/v1/admin/users/:id/logout-all
func (h *AdminUserHandler) LogoutAllSessions(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	err = h.adminUserService.LogoutAllSessions(auditContext(c), actorUUID, targetUserID)
	if err != nil {
		logger.Error("failed to logout all sessions", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, gin.H{"message": "all sessions revoked"})
}

// ChangeUserRole changes user's role (super_admin only)
// PATCH /api/v1/admin/users/:id/role
func (h *AdminUserHandler) ChangeUserRole(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	var req dto.ChangeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request", nil)
		return
	}

	err = h.adminUserService.ChangeRole(auditContext(c), actorUUID, targetUserID, req.NewRole, req.Reason)
	if err != nil {
		logger.Error("failed to change role", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, gin.H{"message": "role changed successfully"})
}

// DecideKYC approves or rejects KYC
// PATCH /api/v1/admin/users/:id/kyc
func (h *AdminUserHandler) DecideKYC(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	var req dto.DecideKYCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request", nil)
		return
	}

	err = h.adminUserService.DecideKYC(auditContext(c), actorUUID, targetUserID, req.Decision, req.Reason)
	if err != nil {
		// Phase 6.2/6.3: policy refusals (missing reason, missing or failed
		// screening) are 422 — the request was well-formed but cannot be
		// honoured as submitted. Everything else stays a 400.
		if errors.Is(err, apperrors.ErrKYCReasonRequired) ||
			errors.Is(err, apperrors.ErrKYCScreeningRequired) ||
			errors.Is(err, apperrors.ErrKYCScreeningHit) {
			response.Error(c, http.StatusUnprocessableEntity, err.Error(), nil)
			return
		}
		logger.Error("failed to decide KYC", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, gin.H{"message": "KYC decision recorded"})
}

// GetKYCDetail handles GET /api/v1/admin/kyc/submissions/:id
//
// The id is the USER id (one submission per user). This is the only endpoint
// that returns full BVN/NIN: admin-only via RequireAdmin, and the service
// writes a `user.kyc_viewed` audit row for every read (Phase 6.2/6.5).
func (h *AdminUserHandler) GetKYCDetail(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	detail, err := h.adminUserService.GetKYCDetail(c.Request.Context(), actorUUID, targetUserID)
	if err != nil {
		if err.Error() == "user not found" {
			response.Error(c, http.StatusNotFound, "user not found", nil)
			return
		}
		logger.Error("failed to load KYC detail", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to load KYC detail", nil)
		return
	}

	response.Success(c, http.StatusOK, detail)
}

// RecordScreening handles POST /api/v1/admin/kyc/submissions/:id/screening
//
// Stores the admin's manual sanctions/PEP attestation (Phase 6.3). The result
// must be "clear" or "hit"; approval is refused until one exists and a "hit"
// blocks approval outright.
func (h *AdminUserHandler) RecordScreening(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	var req dto.RecordScreeningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	if req.Result != "clear" && req.Result != "hit" {
		response.Error(c, http.StatusUnprocessableEntity, "result must be 'clear' or 'hit'", nil)
		return
	}

	if err := h.adminUserService.RecordScreening(c.Request.Context(), actorUUID, targetUserID,
		req.Source, req.Result, req.Notes); err != nil {
		logger.Error("failed to record screening", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.SuccessWithMessage(c, http.StatusOK, "screening recorded", gin.H{
		"user_id": targetUserID,
		"result":  req.Result,
	})
}

// ListKYCSubmissions handles GET /api/v1/admin/kyc/submissions
//
// Returns the KYC review queue (defaults to pending) with a user summary per
// row. PII-safe: only BVN/NIN last-4 leave the server. Phase 2, Workstream A4.
func (h *AdminUserHandler) ListKYCSubmissions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")
	if status == "" {
		status = "pending"
	}

	queue, err := h.adminUserService.ListKYCSubmissions(c.Request.Context(), status, page, pageSize)
	if err != nil {
		logger.Error("failed to list KYC submissions", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list KYC submissions", nil)
		return
	}

	response.Success(c, http.StatusOK, queue)
}

// DeleteUser permanently deletes user (super_admin only)
// DELETE /api/v1/admin/users/:id
func (h *AdminUserHandler) DeleteUser(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}

	err = h.adminUserService.DeleteUser(auditContext(c), actorUUID, targetUserID)
	if err != nil {
		logger.Error("failed to delete user", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, gin.H{"message": "user deleted successfully"})
}
