package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// AdminPrivacyHandler serves the admin Privacy surface (Phase 9.5/9.6):
// view a user's consent evidence, run the suppression list, watch data-export
// jobs, approve/cancel erasure requests inside their grace period, and bump
// policy versions. Every write is audited server-side by the service.
//
// Privacy by design: admins can see that a data export exists and re-issue its
// link — but the link is delivered TO THE USER, and no admin endpoint ever
// returns a bundle or a download token.
type AdminPrivacyHandler struct {
	privacy services.PrivacyService
}

func NewAdminPrivacyHandler(privacy services.PrivacyService) *AdminPrivacyHandler {
	return &AdminPrivacyHandler{privacy: privacy}
}

// GetConsents handles GET /api/v1/admin/privacy/consents/:userId
func (h *AdminPrivacyHandler) GetConsents(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user id", "invalid_id")
		return
	}
	view, err := h.privacy.AdminConsentView(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	response.Success(c, http.StatusOK, view)
}

// ListSuppressions handles GET /api/v1/admin/privacy/suppression?page=&limit=
func (h *AdminPrivacyHandler) ListSuppressions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, total, err := h.privacy.AdminListSuppressions(c.Request.Context(), page, limit)
	if err != nil {
		logger.Error("suppression list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list suppressions", nil)
		return
	}
	response.Paginated(c, rows, page, limit, total)
}

// AddSuppression handles POST /api/v1/admin/privacy/suppression
func (h *AdminPrivacyHandler) AddSuppression(c *gin.Context) {
	actor, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	var req dto.AddSuppressionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}
	row, err := h.privacy.AdminAddSuppression(c.Request.Context(), actor, req.Channel, req.Address, req.Reason)
	if err != nil {
		logger.Error("add suppression failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to add suppression", nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusCreated, "address suppressed", row)
}

// RemoveSuppression handles DELETE /api/v1/admin/privacy/suppression/:id
func (h *AdminPrivacyHandler) RemoveSuppression(c *gin.Context) {
	actor, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid suppression id", "invalid_id")
		return
	}
	if err := h.privacy.AdminRemoveSuppression(c.Request.Context(), actor, id); err != nil {
		response.Error(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "suppression removed", nil)
}

// ListExports handles GET /api/v1/admin/privacy/exports?page=&limit=
func (h *AdminPrivacyHandler) ListExports(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	rows, total, err := h.privacy.AdminListExports(c.Request.Context(), page, limit)
	if err != nil {
		logger.Error("export list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list exports", nil)
		return
	}
	response.Paginated(c, rows, page, limit, total)
}

// ReissueExport handles POST /api/v1/admin/privacy/exports/:id/reissue
// → rotates the link and delivers it TO THE USER; the admin sees only a
// confirmation (no bundle, no token).
func (h *AdminPrivacyHandler) ReissueExport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid export id", "invalid_id")
		return
	}
	if err := h.privacy.AdminReissueExport(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusConflict, err.Error(), nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK,
		"a fresh link was sent to the user (admins never receive export links)", nil)
}

// ListDeletions handles GET /api/v1/admin/privacy/deletions?page=&limit=
func (h *AdminPrivacyHandler) ListDeletions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	rows, total, err := h.privacy.AdminListDeletions(c.Request.Context(), page, limit)
	if err != nil {
		logger.Error("deletion list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list deletion requests", nil)
		return
	}
	response.Paginated(c, rows, page, limit, total)
}

// DecideDeletion handles PATCH /api/v1/admin/privacy/deletions/:id
// → action=approve accelerates execution (worker sweep), cancel stops it.
func (h *AdminPrivacyHandler) DecideDeletion(c *gin.Context) {
	actor, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid deletion id", "invalid_id")
		return
	}
	var req dto.DecideDeletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}
	if err := h.privacy.AdminDecideDeletion(c.Request.Context(), actor, id, req.Action == "approve", req.Reason); err != nil {
		response.Error(c, http.StatusConflict, err.Error(), nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "deletion request "+req.Action+"d", nil)
}

// ListPolicies handles GET /api/v1/admin/privacy/policies → version history.
func (h *AdminPrivacyHandler) ListPolicies(c *gin.Context) {
	rows, err := h.privacy.ListPolicyVersions(c.Request.Context())
	if err != nil {
		logger.Error("policy list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list policy versions", nil)
		return
	}
	response.Success(c, http.StatusOK, rows)
}

// BumpPolicy handles POST /api/v1/admin/privacy/policies/version
// → publishes a new version; every user on the old one must re-consent.
func (h *AdminPrivacyHandler) BumpPolicy(c *gin.Context) {
	actor, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	var req dto.BumpPolicyVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}
	row, err := h.privacy.BumpPolicyVersion(c.Request.Context(), actor, req.Policy, req.Version)
	if err != nil {
		logger.Error("policy bump failed", "error", err)
		response.Error(c, http.StatusConflict, err.Error(), nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusCreated,
		"published "+req.Policy+" v"+row.Version+" — users on the old version must re-consent", row)
}