package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
	"gorm.io/gorm"
)

// AdminRiskHandler serves the Risk layer's admin surface (Phase 8): the alert
// triage queue, rule on/off switches with editable thresholds, the manual
// screening history + re-screen reminder, and the STR/CTR filing screens.
//
// Every write below flows through RiskService, which writes an audit row in
// the same transaction — the API never touches risk tables directly.
type AdminRiskHandler struct {
	risk services.RiskService
}

func NewAdminRiskHandler(risk services.RiskService) *AdminRiskHandler {
	return &AdminRiskHandler{risk: risk}
}

// actor extracts the acting admin for audited writes.
func (h *AdminRiskHandler) actor(c *gin.Context) (services.Actor, bool) {
	id, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return services.Actor{}, false
	}
	return services.Actor{ID: id, Role: c.GetString("role")}, true
}

// writeErr maps a service error onto a status code for triage/filing writes.
func writeRiskErr(c *gin.Context, err error, what string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.Error(c, http.StatusNotFound, what+" not found", nil)
	case errors.Is(err, repositories.ErrAlertExists):
		response.Error(c, http.StatusConflict, "an alert for this pattern is already open", nil)
	default:
		// Business-rule refusals (invalid transition, missing notes, missing
		// goAML reference) are 422; anything else is a real failure.
		response.Error(c, http.StatusUnprocessableEntity, err.Error(), nil)
	}
}

// ── Alerts (8.4) ───────────────────────────────────────────────────────────

// ListAlerts handles GET /api/v1/admin/risk/alerts
//
// Filters: status, severity, rule, user_id, page, limit. The total in the
// paginated envelope powers the dashboard's open-count badge.
func (h *AdminRiskHandler) ListAlerts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	q := services.AlertQuery{
		Status:   c.Query("status"),
		Severity: c.Query("severity"),
		RuleCode: c.Query("rule"),
		Page:     page,
		Limit:    limit,
	}
	if raw := c.Query("user_id"); raw != "" {
		uid, err := uuid.Parse(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid user_id", nil)
			return
		}
		q.UserID = &uid
	}

	alerts, total, err := h.risk.ListAlerts(c.Request.Context(), q)
	if err != nil {
		logger.Error("risk alert list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list alerts", nil)
		return
	}
	response.Paginated(c, alerts, page, limit, total)
}

// GetAlert handles GET /api/v1/admin/risk/alerts/:id — full detail for the
// triage drawer (user, rule, details payload, assignee, timestamps).
func (h *AdminRiskHandler) GetAlert(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid alert ID", nil)
		return
	}
	alert, err := h.risk.GetAlert(c.Request.Context(), id)
	if err != nil {
		writeRiskErr(c, err, "alert")
		return
	}
	response.Success(c, http.StatusOK, alert)
}

// TriageAlert handles PATCH /api/v1/admin/risk/alerts/:id
//
// Assign, move open→in_review→escalated|cleared (resolution notes mandatory
// for cleared), edit notes. Escalation drafts the STR and starts the 24h
// clock server-side.
func (h *AdminRiskHandler) TriageAlert(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid alert ID", nil)
		return
	}
	var req services.AlertTriageInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	alert, err := h.risk.TriageAlert(auditContext(c), actor, id, req)
	if err != nil {
		writeRiskErr(c, err, "alert")
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "alert updated", alert)
}

// FlagUser handles POST /api/v1/admin/risk/users/:id/flag
//
// Body: { "reason": "...", "flag": true|false } (flag defaults true). A
// flagged user's withdrawals are blocked by the risk_flagged_user rule;
// flag=false is the manual clear.
func (h *AdminRiskHandler) FlagUser(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}
	var req struct {
		Reason string `json:"reason"`
		Flag   *bool  `json:"flag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}
	flag := true
	if req.Flag != nil {
		flag = *req.Flag
	}

	if err := h.risk.FlagUser(auditContext(c), actor, userID, flag, req.Reason); err != nil {
		writeRiskErr(c, err, "user")
		return
	}
	msg := "user flagged for risk review"
	if !flag {
		msg = "risk flag cleared"
	}
	response.SuccessWithMessage(c, http.StatusOK, msg, gin.H{"user_id": userID, "flagged": flag})
}

// ── Rules (8.4) ────────────────────────────────────────────────────────────

// ListRules handles GET /api/v1/admin/risk/rules — every rule with its mode,
// enabled flag and editable params (thresholds), for the rules table.
func (h *AdminRiskHandler) ListRules(c *gin.Context) {
	rules, err := h.risk.ListRules(c.Request.Context())
	if err != nil {
		logger.Error("risk rule list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list rules", nil)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"rules": rules})
}

// UpdateRule handles PATCH /api/v1/admin/risk/rules/:code
//
// Body: { "enabled": bool } and/or { "params": {...} }. Disabling a rule
// stops its alerts on the next scan immediately; re-enabling resumes them.
// The edit and its audit row commit together.
func (h *AdminRiskHandler) UpdateRule(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	code := c.Param("code")
	var req services.RuleUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	rule, err := h.risk.UpdateRule(auditContext(c), actor, code, req)
	if err != nil {
		writeRiskErr(c, err, "rule")
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "rule updated", rule)
}

// ── Screening (8.6) ────────────────────────────────────────────────────────

// ListScreening handles GET /api/v1/admin/risk/screening?result=clear|hit
// — the full manual-check history (Phase 6.3 attestations + re-screens).
func (h *AdminRiskHandler) ListScreening(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	rows, total, err := h.risk.ListScreening(c.Request.Context(), c.Query("result"), page, limit)
	if err != nil {
		logger.Error("screening list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list screenings", nil)
		return
	}
	response.Paginated(c, rows, page, limit, total)
}

// RescreenDue handles GET /api/v1/admin/risk/rescreen
//
// Approved users with no screening in the last 30 days (or ever) — the
// reminder banner's list. Computed on demand; there is no cron for it.
func (h *AdminRiskHandler) RescreenDue(c *gin.Context) {
	rows, err := h.risk.RescreenDue(c.Request.Context())
	if err != nil {
		logger.Error("rescreen reminder failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to load re-screen reminder", nil)
		return
	}
	response.Success(c, http.StatusOK, gin.H{"due": rows, "count": len(rows)})
}

// RunScan handles POST /api/v1/admin/risk/scan/run
//
// Executes the monitoring pass on demand — the worker runs the same code
// every 5 minutes; this lets an operator prove a rule toggle end-to-end
// (disable → run → no alert → enable → run → alert) without waiting.
func (h *AdminRiskHandler) RunScan(c *gin.Context) {
	result, err := h.risk.RunScan(c.Request.Context())
	if err != nil {
		logger.Error("manual risk scan failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "scan failed to run", nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "scan completed", result)
}

// ── STR (8.7) ──────────────────────────────────────────────────────────────

// ListSTR handles GET /api/v1/admin/risk/str?status=draft|filed
// Rows carry deadline_at — the UI renders the 24h countdown / red overdue.
func (h *AdminRiskHandler) ListSTR(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	reports, total, err := h.risk.ListSTR(c.Request.Context(), c.Query("status"), page, limit)
	if err != nil {
		logger.Error("STR list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list STRs", nil)
		return
	}
	response.Paginated(c, reports, page, limit, total)
}

// UpdateSTR handles PATCH /api/v1/admin/risk/str/:id
//
// Edit the narrative, or mark filed — filing requires the goAML portal
// reference and stamps filed_at. Filing and edits are audited.
func (h *AdminRiskHandler) UpdateSTR(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid STR ID", nil)
		return
	}
	var req services.STRUpdateInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	report, err := h.risk.UpdateSTR(auditContext(c), actor, id, req)
	if err != nil {
		writeRiskErr(c, err, "STR report")
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "STR report updated", report)
}

// ── CTR (8.8) ──────────────────────────────────────────────────────────────

// ListCTR handles GET /api/v1/admin/risk/ctr?status=open|filed
// Rows carry window_date + deadline_at (the 7-day clock).
func (h *AdminRiskHandler) ListCTR(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	reports, total, err := h.risk.ListCTR(c.Request.Context(), c.Query("status"), page, limit)
	if err != nil {
		logger.Error("CTR list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list CTRs", nil)
		return
	}
	response.Paginated(c, reports, page, limit, total)
}

// FileCTR handles PATCH /api/v1/admin/risk/ctr/:id/file
// Body: { "notes": "..." } — filing notes are mandatory (audited as reason).
func (h *AdminRiskHandler) FileCTR(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid CTR ID", nil)
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	if err := h.risk.FileCTR(auditContext(c), actor, id, req.Notes); err != nil {
		writeRiskErr(c, err, "CTR report")
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "CTR report marked filed", gin.H{"id": id})
}
