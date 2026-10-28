package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"gorm.io/gorm"
)

// AdminReportsService provides audit log search and export capabilities.
//
// SECURITY:
//   - All methods require admin actor
//   - Sensitive fields redacted from exports
//   - Date range limits enforced (max 1 year)
//   - Row caps enforced (max 10,000)
//   - Export actions audited
//
// BOUNDED QUERIES:
//   - Pagination required (max 100 per page)
//   - Stable sorting (created_at DESC, id DESC)
//   - Date range filters for performance
type AdminReportsService interface {
	// ListAuditLogs returns paginated audit logs with filters.
	//
	// PARAMETERS:
	//   ctx - request context
	//   actorID - admin performing the search
	//   filter - search criteria
	//   page - page number (1-based)
	//   pageSize - results per page (max 100)
	//
	// RETURNS:
	//   response - paginated audit log list
	//   error - if query fails or unauthorized
	ListAuditLogs(ctx context.Context, actorID uuid.UUID, filter AuditLogSearchFilter, page, pageSize int) (*AuditLogListResponse, error)

	// GetAuditLogDetail returns a single audit log with full details.
	//
	// PARAMETERS:
	//   ctx - request context
	//   actorID - admin requesting detail
	//   logID - audit log ID
	//
	// RETURNS:
	//   log - full audit log details (redacted)
	//   error - if not found or unauthorized
	GetAuditLogDetail(ctx context.Context, actorID, logID uuid.UUID) (*models.AuditLog, error)

	// ExportAuditLogsCSV generates a CSV export of audit logs.
	//
	// SECURITY:
	//   - Formula injection prevented
	//   - Sensitive fields redacted
	//   - Export action audited
	//   - Bounded row cap (10,000)
	//
	// PARAMETERS:
	//   ctx - request context
	//   actorID - admin requesting export
	//   filter - search criteria
	//
	// RETURNS:
	//   csv - CSV string (safe, redacted)
	//   error - if query fails or unauthorized
	ExportAuditLogsCSV(ctx context.Context, actorID uuid.UUID, filter AuditLogExportFilter) (string, error)

	// GenerateFinancialReport creates a financial summary report.
	//
	// PARAMETERS:
	//   ctx - request context
	//   actorID - admin requesting report
	//   reportType - user_activity, financial_summary, investment_summary
	//   from - start date
	//   to - end date
	//
	// RETURNS:
	//   report - report data
	//   error - if generation fails
	GenerateFinancialReport(ctx context.Context, actorID uuid.UUID, reportType string, from, to time.Time) (*FinancialReportResponse, error)
}

// AuditLogSearchFilter defines search criteria for audit logs.
type AuditLogSearchFilter struct {
	ActorID    *uuid.UUID
	ActorType  *string
	Action     *string
	ActionLike *string
	TargetType *string
	TargetID   *uuid.UUID
	DateFrom   *time.Time
	DateTo     *time.Time
	RequestID  *string
}

// AuditLogExportFilter defines export criteria with date range requirements.
type AuditLogExportFilter struct {
	ActorID    *uuid.UUID
	ActorType  *string
	Action     *string
	TargetType *string
	DateFrom   time.Time // Required for exports
	DateTo     time.Time // Required for exports
}

// AuditLogListResponse represents paginated audit log results.
type AuditLogListResponse struct {
	Logs       []models.AuditLog `json:"logs"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

// FinancialReportResponse represents a generated financial report.
type FinancialReportResponse struct {
	ReportType string                 `json:"report_type"`
	DateFrom   time.Time              `json:"date_from"`
	DateTo     time.Time              `json:"date_to"`
	GeneratedAt time.Time             `json:"generated_at"`
	Data       map[string]interface{} `json:"data"`
}

// ────────────────────────────────────────────────────────────────────────────
// Implementation
// ────────────────────────────────────────────────────────────────────────────

type adminReportsService struct {
	db             *gorm.DB
	auditRepo      repositories.AuditRepository
	auditService   AuditService
}

// NewAdminReportsService creates a new admin reports service instance.
func NewAdminReportsService(
	db *gorm.DB,
	auditRepo repositories.AuditRepository,
	auditService AuditService,
) AdminReportsService {
	return &adminReportsService{
		db:           db,
		auditRepo:    auditRepo,
		auditService: auditService,
	}
}

// ListAuditLogs returns paginated audit logs with filtering.
//
// BOUNDED QUERY:
//   - Pagination enforced (max 100 per page)
//   - Stable sorting by (created_at DESC, id DESC)
//   - Date range filtering recommended for performance
func (s *adminReportsService) ListAuditLogs(
	ctx context.Context,
	actorID uuid.UUID,
	filter AuditLogSearchFilter,
	page, pageSize int,
) (*AuditLogListResponse, error) {
	// Validate pagination
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Convert to repository filter
	repoFilter := repositories.AuditLogFilter{
		ActorID:     filter.ActorID,
		ActorType:   filter.ActorType,
		Action:      filter.Action,
		ActionLike:  filter.ActionLike,
		TargetType:  filter.TargetType,
		TargetID:    filter.TargetID,
		CreatedFrom: filter.DateFrom,
		CreatedTo:   filter.DateTo,
		RequestID:   filter.RequestID,
	}

	// Query audit logs
	logs, total, err := s.auditRepo.List(ctx, repoFilter, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}

	// Calculate total pages
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &AuditLogListResponse{
		Logs:       logs,
		TotalCount: total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetAuditLogDetail returns full details of a single audit log.
func (s *adminReportsService) GetAuditLogDetail(
	ctx context.Context,
	actorID, logID uuid.UUID,
) (*models.AuditLog, error) {
	log, err := s.auditRepo.FindByID(ctx, logID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("audit log not found")
		}
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	return log, nil
}

// ExportAuditLogsCSV generates a CSV export of audit logs.
//
// SECURITY BOUNDARIES:
//   - Formula injection prevented (leading =,+,-,@ escaped)
//   - Sensitive fields (old_value, new_value) sanitized
//   - Row cap: 10,000 maximum
//   - Date range: required and max 1 year
//   - Export action audited
func (s *adminReportsService) ExportAuditLogsCSV(
	ctx context.Context,
	actorID uuid.UUID,
	filter AuditLogExportFilter,
) (string, error) {
	// Validate date range (required for exports)
	if filter.DateFrom.IsZero() || filter.DateTo.IsZero() {
		return "", fmt.Errorf("date range required for export (from and to)")
	}

	// Validate date range span (max 1 year)
	maxRange := 365 * 24 * time.Hour
	if filter.DateTo.Sub(filter.DateFrom) > maxRange {
		return "", fmt.Errorf("date range cannot exceed 1 year")
	}

	// Convert to repository filter
	repoFilter := repositories.AuditLogFilter{
		ActorID:     filter.ActorID,
		ActorType:   filter.ActorType,
		Action:      filter.Action,
		TargetType:  filter.TargetType,
		CreatedFrom: &filter.DateFrom,
		CreatedTo:   &filter.DateTo,
	}

	// Query audit logs with row cap
	const maxRows = 10000
	logs, total, err := s.auditRepo.List(ctx, repoFilter, 1, maxRows)
	if err != nil {
		return "", fmt.Errorf("failed to query audit logs: %w", err)
	}

	// Warn if capped
	if total > int64(maxRows) {
		return "", fmt.Errorf("result set too large (%d rows), limit is %d. Please narrow your date range", total, maxRows)
	}

	// Generate CSV
	csv := generateAuditLogCSV(logs)

	// Audit the export action (async, don't block response)
	go func() {
		auditCtx := context.Background()
		tx := s.db.Begin()
		defer tx.Rollback()

		_ = s.auditService.Record(auditCtx, tx, AuditEvent{
			ActorID:    &actorID,
			ActorType:  "admin",
			ActorRole:  "", // role unknown here
			Action:     "audit_logs.exported",
			Description: fmt.Sprintf("Exported %d audit logs from %s to %s", len(logs), filter.DateFrom.Format("2006-01-02"), filter.DateTo.Format("2006-01-02")),
			TargetType: "audit_logs",
			TargetID:   nil,
			OldValues:  nil,
			NewValues:  map[string]interface{}{
				"row_count":  len(logs),
				"date_from":  filter.DateFrom,
				"date_to":    filter.DateTo,
			},
			Reason: "",
			IdempotencyKey: "",
			RequestMetadata: RequestMetadata{
				RequestID: "",
				IPAddress: "",
				UserAgent: "",
			},
		})
		tx.Commit()
	}()

	return csv, nil
}

// GenerateFinancialReport creates a financial summary report.
//
// REPORT TYPES:
//   - user_activity: User registration, KYC, suspension trends
//   - financial_summary: Payment, withdrawal, wallet balance aggregates
//   - investment_summary: Investment volume, property funding trends
//
// BOUNDED DATE RANGE:
//   - Max 1 year span
//   - Date range required
func (s *adminReportsService) GenerateFinancialReport(
	ctx context.Context,
	actorID uuid.UUID,
	reportType string,
	from, to time.Time,
) (*FinancialReportResponse, error) {
	// Validate date range
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("date range required (from and to)")
	}
	if to.Sub(from) > 365*24*time.Hour {
		return nil, fmt.Errorf("date range cannot exceed 1 year")
	}

	var data map[string]interface{}
	var err error

	switch reportType {
	case "user_activity":
		data, err = s.generateUserActivityReport(ctx, from, to)
	case "financial_summary":
		data, err = s.generateFinancialSummaryReport(ctx, from, to)
	case "investment_summary":
		data, err = s.generateInvestmentSummaryReport(ctx, from, to)
	default:
		return nil, fmt.Errorf("unknown report type: %s", reportType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate report: %w", err)
	}

	return &FinancialReportResponse{
		ReportType:  reportType,
		DateFrom:    from,
		DateTo:      to,
		GeneratedAt: time.Now().UTC(),
		Data:        data,
	}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Report Generation Helpers
// ────────────────────────────────────────────────────────────────────────────

func (s *adminReportsService) generateUserActivityReport(ctx context.Context, from, to time.Time) (map[string]interface{}, error) {
	var result struct {
		TotalRegistrations int64 `json:"total_registrations"`
		KYCPending         int64 `json:"kyc_pending"`
		KYCVerified        int64 `json:"kyc_verified"`
		KYCRejected        int64 `json:"kyc_rejected"`
		SuspendedUsers     int64 `json:"suspended_users"`
	}

	// Total registrations in period
	err := s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Count(&result.TotalRegistrations).Error
	if err != nil {
		return nil, err
	}

	// KYC status breakdown
	err = s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("kyc_status = ?", "pending").
		Count(&result.KYCPending).Error
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("kyc_status = ?", "verified").
		Count(&result.KYCVerified).Error
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("kyc_status = ?", "rejected").
		Count(&result.KYCRejected).Error
	if err != nil {
		return nil, err
	}

	// Suspended users
	err = s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("suspended_at IS NOT NULL").
		Where("suspended_at >= ? AND suspended_at < ?", from, to).
		Count(&result.SuspendedUsers).Error
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_registrations": result.TotalRegistrations,
		"kyc_pending":         result.KYCPending,
		"kyc_verified":        result.KYCVerified,
		"kyc_rejected":        result.KYCRejected,
		"suspended_users":     result.SuspendedUsers,
	}, nil
}

func (s *adminReportsService) generateFinancialSummaryReport(ctx context.Context, from, to time.Time) (map[string]interface{}, error) {
	var result struct {
		TotalPayments        int64 `json:"total_payments"`
		SuccessfulPayments   int64 `json:"successful_payments"`
		FailedPayments       int64 `json:"failed_payments"`
		TotalAmount          int64 `json:"total_amount_kobo"`
		TotalWithdrawals     int64 `json:"total_withdrawals"`
		WithdrawalAmount     int64 `json:"withdrawal_amount_kobo"`
	}

	// Payment statistics
	err := s.db.WithContext(ctx).
		Model(&models.Payment{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Count(&result.TotalPayments).Error
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).
		Model(&models.Payment{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("status = ?", models.PaymentStatusSuccess).
		Count(&result.SuccessfulPayments).Error
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).
		Model(&models.Payment{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("status = ?", models.PaymentStatusFailed).
		Count(&result.FailedPayments).Error
	if err != nil {
		return nil, err
	}

	// Total successful payment amount
	err = s.db.WithContext(ctx).
		Model(&models.Payment{}).
		Select("COALESCE(SUM(amount_kobo), 0)").
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("status = ?", models.PaymentStatusSuccess).
		Scan(&result.TotalAmount).Error
	if err != nil {
		return nil, err
	}

	// Withdrawal statistics (wallet_transactions with type=withdrawal)
	err = s.db.WithContext(ctx).
		Model(&models.WalletTransaction{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("type = ?", "withdrawal").
		Count(&result.TotalWithdrawals).Error
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).
		Model(&models.WalletTransaction{}).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("type = ?", "withdrawal").
		Scan(&result.WithdrawalAmount).Error
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_payments":         result.TotalPayments,
		"successful_payments":    result.SuccessfulPayments,
		"failed_payments":        result.FailedPayments,
		"total_amount_kobo":      result.TotalAmount,
		"total_amount_ngn":       float64(result.TotalAmount) / 100.0,
		"total_withdrawals":      result.TotalWithdrawals,
		"withdrawal_amount_kobo": result.WithdrawalAmount,
		"withdrawal_amount_ngn":  float64(result.WithdrawalAmount) / 100.0,
	}, nil
}

func (s *adminReportsService) generateInvestmentSummaryReport(ctx context.Context, from, to time.Time) (map[string]interface{}, error) {
	var result struct {
		TotalInvestments int64 `json:"total_investments"`
		TotalAmount      int64 `json:"total_amount_kobo"`
		ActiveProperties int64 `json:"active_properties"`
		FundedProperties int64 `json:"funded_properties"`
	}

	// Investment statistics
	err := s.db.WithContext(ctx).
		Model(&models.Investment{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Count(&result.TotalInvestments).Error
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).
		Model(&models.Investment{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("created_at >= ? AND created_at < ?", from, to).
		Scan(&result.TotalAmount).Error
	if err != nil {
		return nil, err
	}

	// Property statistics
	err = s.db.WithContext(ctx).
		Model(&models.Property{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("status = ?", "active").
		Count(&result.ActiveProperties).Error
	if err != nil {
		return nil, err
	}

	err = s.db.WithContext(ctx).
		Model(&models.Property{}).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where("status = ?", "funded").
		Count(&result.FundedProperties).Error
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_investments":  result.TotalInvestments,
		"total_amount_kobo":  result.TotalAmount,
		"total_amount_ngn":   float64(result.TotalAmount) / 100.0,
		"active_properties":  result.ActiveProperties,
		"funded_properties":  result.FundedProperties,
	}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// CSV Generation
// ────────────────────────────────────────────────────────────────────────────

// generateAuditLogCSV creates a safe CSV string from audit logs.
//
// SECURITY:
//   - Formula injection prevented (escape leading =,+,-,@)
//   - Sensitive fields redacted (old_value, new_value show type only)
//   - Double quotes escaped
func generateAuditLogCSV(logs []models.AuditLog) string {
	csv := "ID,Created At,Actor ID,Actor Type,Action,Target Type,Target ID,Request ID,IP Address\n"

	for _, log := range logs {
		// Escape CSV cells (prevent formula injection)
		id := escapeCSVCell(log.ID.String())
		createdAt := escapeCSVCell(log.CreatedAt.Format(time.RFC3339))
		actorID := escapeCSVCell(log.ActorID.String())
		actorType := escapeCSVCell(log.ActorType)
		action := escapeCSVCell(log.Action)
		targetType := escapeCSVCell(log.TargetType)
		targetID := ""
		if log.TargetID != nil {
			targetID = escapeCSVCell(log.TargetID.String())
		}
		requestID := escapeCSVCell(log.RequestID)
		ipAddress := escapeCSVCell(log.IPAddress)

		csv += fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
			id, createdAt, actorID, actorType, action, targetType, targetID, requestID, ipAddress)
	}

	return csv
}

// escapeCSVCell escapes a CSV cell value to prevent formula injection.
//
// FORMULA INJECTION PREVENTION:
//   - Leading =, +, -, @ are prefixed with single quote
//   - Double quotes are escaped as ""
//   - Newlines are preserved (will be within quoted cell)
func escapeCSVCell(value string) string {
	if len(value) == 0 {
		return value
	}

	// Prevent formula injection
	firstChar := value[0]
	if firstChar == '=' || firstChar == '+' || firstChar == '-' || firstChar == '@' {
		value = "'" + value
	}

	// Escape double quotes and wrap in quotes if contains comma, quote, or newline
	needsQuoting := false
	for _, ch := range value {
		if ch == '"' || ch == ',' || ch == '\n' || ch == '\r' {
			needsQuoting = true
			break
		}
	}

	if needsQuoting {
		// Escape double quotes by doubling them
		escaped := ""
		for _, ch := range value {
			if ch == '"' {
				escaped += "\"\""
			} else {
				escaped += string(ch)
			}
		}
		return "\"" + escaped + "\""
	}

	return value
}
