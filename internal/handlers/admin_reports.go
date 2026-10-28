package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// AdminReportsHandler handles audit log search and export endpoints.
type AdminReportsHandler struct {
	reportsService services.AdminReportsService
}

// NewAdminReportsHandler creates a new admin reports handler.
func NewAdminReportsHandler(reportsService services.AdminReportsService) *AdminReportsHandler {
	return &AdminReportsHandler{
		reportsService: reportsService,
	}
}

// ListAuditLogs handles GET /api/v1/admin/audit-logs
//
// QUERY PARAMETERS:
//   - page: page number (default 1)
//   - page_size: results per page (default 20, max 100)
//   - actor_id: filter by actor UUID
//   - actor_type: filter by actor type (admin, user, system)
//   - action: filter by exact action
//   - action_like: filter by action pattern (e.g., "user." matches "user.suspended")
//   - target_type: filter by target type
//   - target_id: filter by target UUID
//   - date_from: filter by created_at >= (RFC3339 format)
//   - date_to: filter by created_at < (RFC3339 format)
//   - request_id: filter by request ID
//
// RESPONSE: 200 OK with paginated audit logs
func (h *AdminReportsHandler) ListAuditLogs(c *gin.Context) {
	// Get actor from context (set by auth middleware)
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Build filter
	filter := services.AuditLogSearchFilter{}

	if actorIDStr := c.Query("actor_id"); actorIDStr != "" {
		actorUUID, err := uuid.Parse(actorIDStr)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid actor_id format")
			return
		}
		filter.ActorID = &actorUUID
	}

	if actorType := c.Query("actor_type"); actorType != "" {
		filter.ActorType = &actorType
	}

	if action := c.Query("action"); action != "" {
		filter.Action = &action
	}

	if actionLike := c.Query("action_like"); actionLike != "" {
		filter.ActionLike = &actionLike
	}

	if targetType := c.Query("target_type"); targetType != "" {
		filter.TargetType = &targetType
	}

	if targetIDStr := c.Query("target_id"); targetIDStr != "" {
		targetUUID, err := uuid.Parse(targetIDStr)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid target_id format")
			return
		}
		filter.TargetID = &targetUUID
	}

	if dateFromStr := c.Query("date_from"); dateFromStr != "" {
		dateFrom, err := time.Parse(time.RFC3339, dateFromStr)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid date_from format (use RFC3339)")
			return
		}
		filter.DateFrom = &dateFrom
	}

	if dateToStr := c.Query("date_to"); dateToStr != "" {
		dateTo, err := time.Parse(time.RFC3339, dateToStr)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid date_to format (use RFC3339)")
			return
		}
		filter.DateTo = &dateTo
	}

	if requestID := c.Query("request_id"); requestID != "" {
		filter.RequestID = &requestID
	}

	// Call service
	result, err := h.reportsService.ListAuditLogs(c.Request.Context(), actorUUID, filter, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "failed to list audit logs: "+err.Error())
		return
	}

	response.Success(c, http.StatusOK, "audit logs retrieved", result)
}

// GetAuditLogDetail handles GET /api/v1/admin/audit-logs/:id
//
// PARAMETERS:
//   - id: audit log UUID
//
// RESPONSE: 200 OK with full audit log details
func (h *AdminReportsHandler) GetAuditLogDetail(c *gin.Context) {
	// Get actor from context
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Parse audit log ID
	logIDStr := c.Param("id")
	logID, err := uuid.Parse(logIDStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid audit log ID format")
		return
	}

	// Call service
	log, err := h.reportsService.GetAuditLogDetail(c.Request.Context(), actorUUID, logID)
	if err != nil {
		if err.Error() == "audit log not found" {
			response.Error(c, http.StatusNotFound, "audit log not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "failed to get audit log: "+err.Error())
		return
	}

	response.Success(c, http.StatusOK, "audit log retrieved", log)
}

// ExportAuditLogsCSV handles GET /api/v1/admin/audit-logs/export
//
// QUERY PARAMETERS (REQUIRED):
//   - date_from: start date (RFC3339 format)
//   - date_to: end date (RFC3339 format)
//
// QUERY PARAMETERS (OPTIONAL):
//   - actor_id: filter by actor UUID
//   - actor_type: filter by actor type
//   - action: filter by exact action
//   - target_type: filter by target type
//
// SECURITY:
//   - Formula injection prevented
//   - Sensitive fields redacted
//   - Max 10,000 rows
//   - Max 1 year date range
//   - Export action audited
//
// RESPONSE: 200 OK with CSV content-type
func (h *AdminReportsHandler) ExportAuditLogsCSV(c *gin.Context) {
	// Get actor from context
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Parse required date range
	dateFromStr := c.Query("date_from")
	dateToStr := c.Query("date_to")

	if dateFromStr == "" || dateToStr == "" {
		response.Error(c, http.StatusBadRequest, "date_from and date_to are required")
		return
	}

	dateFrom, err := time.Parse(time.RFC3339, dateFromStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid date_from format (use RFC3339)")
		return
	}

	dateTo, err := time.Parse(time.RFC3339, dateToStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid date_to format (use RFC3339)")
		return
	}

	// Build export filter
	filter := services.AuditLogExportFilter{
		DateFrom: dateFrom,
		DateTo:   dateTo,
	}

	if actorIDStr := c.Query("actor_id"); actorIDStr != "" {
		actorUUID, err := uuid.Parse(actorIDStr)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid actor_id format")
			return
		}
		filter.ActorID = &actorUUID
	}

	if actorType := c.Query("actor_type"); actorType != "" {
		filter.ActorType = &actorType
	}

	if action := c.Query("action"); action != "" {
		filter.Action = &action
	}

	if targetType := c.Query("target_type"); targetType != "" {
		filter.TargetType = &targetType
	}

	// Call service
	csv, err := h.reportsService.ExportAuditLogsCSV(c.Request.Context(), actorUUID, filter)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	// Return CSV with appropriate headers
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=audit_logs_export.csv")
	c.String(http.StatusOK, csv)
}

// GenerateFinancialReport handles POST /api/v1/admin/reports/generate
//
// REQUEST BODY:
//   {
//     "report_type": "user_activity | financial_summary | investment_summary",
//     "date_from": "2024-01-01T00:00:00Z",
//     "date_to": "2024-12-31T23:59:59Z"
//   }
//
// RESPONSE: 200 OK with report data
func (h *AdminReportsHandler) GenerateFinancialReport(c *gin.Context) {
	// Get actor from context
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Parse request body
	var req struct {
		ReportType string `json:"report_type" binding:"required"`
		DateFrom   string `json:"date_from" binding:"required"`
		DateTo     string `json:"date_to" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Parse dates
	dateFrom, err := time.Parse(time.RFC3339, req.DateFrom)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid date_from format (use RFC3339)")
		return
	}

	dateTo, err := time.Parse(time.RFC3339, req.DateTo)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid date_to format (use RFC3339)")
		return
	}

	// Validate report type
	validTypes := map[string]bool{
		"user_activity":      true,
		"financial_summary":  true,
		"investment_summary": true,
	}
	if !validTypes[req.ReportType] {
		response.Error(c, http.StatusBadRequest, "invalid report_type. Valid types: user_activity, financial_summary, investment_summary")
		return
	}

	// Call service
	report, err := h.reportsService.GenerateFinancialReport(
		c.Request.Context(),
		actorUUID,
		req.ReportType,
		dateFrom,
		dateTo,
	)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(c, http.StatusOK, "report generated", report)
}
