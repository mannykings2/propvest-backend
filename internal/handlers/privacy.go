package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// PrivacyHandler serves the user-facing data-rights surface (Phase 9):
// consent read/withdraw, the NDPA access right (async data export) and the
// deferred erasure request, plus the unsubscribe link the marketing
// objection is exercised through.
type PrivacyHandler struct {
	consents services.ConsentService
	privacy  services.PrivacyService
}

func NewPrivacyHandler(consents services.ConsentService, privacy services.PrivacyService) *PrivacyHandler {
	return &PrivacyHandler{consents: consents, privacy: privacy}
}

// GetConsents handles GET /api/v1/me/consents
// → live policy versions, latest decision per purpose, re-consent flag.
func (h *PrivacyHandler) GetConsents(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	status, err := h.consents.Status(c.Request.Context(), userID)
	if err != nil {
		logger.Error("consent status failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to load consents", nil)
		return
	}
	response.Success(c, http.StatusOK, status)
}

// GiveConsents handles POST /api/v1/me/consents
// → records fresh decisions (withdrawal included) and returns the new status.
func (h *PrivacyHandler) GiveConsents(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	var req dto.GiveConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}
	if req.PrivacyPolicy == nil && req.Terms == nil && req.Marketing == nil {
		response.Error(c, http.StatusUnprocessableEntity,
			"at least one consent purpose must be provided", "validation_error")
		return
	}
	status, err := h.consents.Give(c.Request.Context(), userID, req)
	if err != nil {
		logger.Error("consent update failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to record consents", nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "consents recorded", status)
}

// GetMarketing handles GET /api/v1/me/marketing
// → the user's marketing state and THEIR unsubscribe link (NDPA objection).
func (h *PrivacyHandler) GetMarketing(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	status, err := h.consents.Status(c.Request.Context(), userID)
	if err != nil {
		logger.Error("marketing status failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to load marketing preferences", nil)
		return
	}
	optedIn := false
	for _, row := range status.Consents {
		if row.Purpose == models.ConsentPurposeMarketing && row.Granted {
			optedIn = true
		}
	}
	response.Success(c, http.StatusOK, gin.H{
		"marketing_opted_in": optedIn,
		"unsubscribe_path":   "/api/v1/unsubscribe?token=" + h.privacy.UnsubscribeToken(userID),
	})
}

// RequestExport handles POST /api/v1/me/exports → queues the NDPA access job.
// The one-time download token is returned here AND delivered by notification.
func (h *PrivacyHandler) RequestExport(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	req, err := h.privacy.RequestDataExport(c.Request.Context(), userID)
	if err != nil {
		logger.Error("data export request failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to queue data export", nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusAccepted, "data export queued", exportToResponse(req))
}

// ListExports handles GET /api/v1/me/exports → the user's own export jobs.
func (h *PrivacyHandler) ListExports(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	jobs, err := h.privacy.ListDataExports(c.Request.Context(), userID)
	if err != nil {
		logger.Error("data export list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list data exports", nil)
		return
	}
	out := make([]dto.DataExportJobResponse, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, exportJobToResponse(j))
	}
	response.Success(c, http.StatusOK, out)
}

// DownloadExport handles GET /api/v1/me/exports/download/:token (PUBLIC — the
// bearer token is the credential; it hashes to the job row and expires in 24h).
func (h *PrivacyHandler) DownloadExport(c *gin.Context) {
	bundle, err := h.privacy.DownloadDataExport(c.Request.Context(), c.Param("token"))
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", `attachment; filename="propvest-data-export.json"`)
	c.Data(http.StatusOK, "application/json", bundle)
}

// RequestDeletion handles POST /api/v1/me/account/delete → starts the 30-day
// grace. Money-moving activity is blocked from this moment (middleware).
func (h *PrivacyHandler) RequestDeletion(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	var req dto.AccountDeletionRequest
	_ = c.ShouldBindJSON(&req) // body optional (reason only)

	row, err := h.privacy.RequestAccountDeletion(c.Request.Context(), userID, req.Reason)
	if err != nil {
		logger.Error("account deletion request failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to request account deletion", nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusAccepted,
		"deletion requested — you can cancel any time in the next 30 days", deletionToResponse(*row))
}

// DeletionStatus handles GET /api/v1/me/account/delete → the grace clock.
func (h *PrivacyHandler) DeletionStatus(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	row, err := h.privacy.AccountDeletionStatus(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, "no deletion request", nil)
		return
	}
	response.Success(c, http.StatusOK, deletionToResponse(*row))
}

// CancelDeletion handles DELETE /api/v1/me/account/delete → stops the clock.
func (h *PrivacyHandler) CancelDeletion(c *gin.Context) {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "authorization required", "unauthorized")
		return
	}
	row, err := h.privacy.CancelAccountDeletion(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusNotFound, err.Error(), nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "deletion cancelled — your account is fully active", deletionToResponse(*row))
}

// Unsubscribe handles GET /api/v1/unsubscribe?token=... (PUBLIC — it is the
// link inside every bulk message). Suppresses the user's marketing addresses
// and records the withdrawal as consent evidence.
func (h *PrivacyHandler) Unsubscribe(c *gin.Context) {
	email, err := h.privacy.Unsubscribe(c.Request.Context(), c.Query("token"))
	if err != nil {
		response.Error(c, http.StatusUnauthorized, err.Error(), nil)
		return
	}
	// Mask the address in the response — the page may be screenshotted.
	response.SuccessWithMessage(c, http.StatusOK, "You have been unsubscribed from marketing messages", gin.H{
		"suppressed_email": maskEmail(email),
	})
}

// ── helpers ───────────────────────────────────────────────────────────────

func exportToResponse(req *services.DataExportRequest) dto.DataExportRequestResponse {
	out := dto.DataExportRequestResponse{DataExportJobResponse: exportJobToResponse(req.Job)}
	if req.DownloadToken != "" {
		out.DownloadToken = req.DownloadToken
		out.DownloadPath = req.DownloadPath
	}
	return out
}

func exportJobToResponse(j models.DataExportJob) dto.DataExportJobResponse {
	return dto.DataExportJobResponse{
		ID: j.ID, Status: j.Status, Error: j.Error,
		ExpiresAt: j.ExpiresAt, CreatedAt: j.CreatedAt, CompletedAt: j.CompletedAt,
	}
}

func deletionToResponse(row models.AccountDeletionRequest) dto.AccountDeletionResponse {
	days := 0
	if row.Status == models.DeletionStatusRequested {
		if left := int(time.Until(row.ExecuteAfter).Hours() / 24); left > 0 {
			days = left
		}
	}
	return dto.AccountDeletionResponse{
		ID: row.ID, Status: row.Status, Reason: row.Reason,
		RequestedAt: row.RequestedAt, ExecuteAfter: row.ExecuteAfter,
		DaysLeft: days, CancelledAt: row.CancelledAt, ExecutedAt: row.ExecutedAt,
	}
}

func maskEmail(email string) string {
	at := -1
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			at = i
			break
		}
	}
	if at <= 0 {
		return "***"
	}
	return string(email[0]) + "***" + email[at:]
}
