package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN FINANCIAL HANDLER (Milestone 7 - Phase 7)
// ═══════════════════════════════════════════════════════════════════════════

// AdminFinancialHandler handles admin financial monitoring endpoints
type AdminFinancialHandler struct {
	financialService services.AdminFinancialService
}

// NewAdminFinancialHandler creates a new admin financial handler
func NewAdminFinancialHandler(financialService services.AdminFinancialService) *AdminFinancialHandler {
	return &AdminFinancialHandler{
		financialService: financialService,
	}
}

// ListTransactions returns paginated list of transactions
// GET /api/v1/admin/transactions
//
// Query Parameters:
//   - page: Page number (default: 1)
//   - page_size: Items per page (default: 20, max: 100)
//   - user_id: Filter by user ID (UUID)
//   - type: Filter by type (deposit, withdrawal, investment, etc.)
//   - status: Filter by status (pending, completed, failed)
//   - reference: Search by reference
//   - min_amount: Minimum amount in kobo
//   - max_amount: Maximum amount in kobo
//   - date_from: Start date (RFC3339)
//   - date_to: End date (RFC3339)
func (h *AdminFinancialHandler) ListTransactions(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Build filter
	filter := services.TransactionFilter{}

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if userID, err := uuid.Parse(userIDStr); err == nil {
			filter.UserID = &userID
		}
	}

	if txType := c.Query("type"); txType != "" {
		filter.Type = &txType
	}

	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	if reference := c.Query("reference"); reference != "" {
		filter.Reference = &reference
	}

	if minAmountStr := c.Query("min_amount"); minAmountStr != "" {
		if minAmount, err := strconv.ParseInt(minAmountStr, 10, 64); err == nil {
			filter.MinAmount = &minAmount
		}
	}

	if maxAmountStr := c.Query("max_amount"); maxAmountStr != "" {
		if maxAmount, err := strconv.ParseInt(maxAmountStr, 10, 64); err == nil {
			filter.MaxAmount = &maxAmount
		}
	}

	if dateFromStr := c.Query("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse(time.RFC3339, dateFromStr); err == nil {
			filter.DateFrom = &dateFrom
		}
	}

	if dateToStr := c.Query("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse(time.RFC3339, dateToStr); err == nil {
			filter.DateTo = &dateTo
		}
	}

	// Call service
	result, err := h.financialService.ListTransactions(c.Request.Context(), actorUUID, filter, page, pageSize)
	if err != nil {
		logger.Error("failed to list transactions", "error", err, "actor_id", actorUUID)
		response.Error(c, http.StatusInternalServerError, "failed to list transactions", nil)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// GetTransactionDetail returns detailed transaction information
// GET /api/v1/admin/transactions/:id
func (h *AdminFinancialHandler) GetTransactionDetail(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	transactionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid transaction ID", nil)
		return
	}

	result, err := h.financialService.GetTransactionDetail(c.Request.Context(), actorUUID, transactionID)
	if err != nil {
		logger.Error("failed to get transaction detail", "error", err, "transaction_id", transactionID)
		response.Error(c, http.StatusNotFound, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// ListPayments returns paginated list of payments
// GET /api/v1/admin/payments
//
// Query Parameters:
//   - page: Page number (default: 1)
//   - page_size: Items per page (default: 20, max: 100)
//   - user_id: Filter by user ID (UUID)
//   - status: Filter by status (pending, completed, failed)
//   - provider: Filter by provider (paystack, flutterwave, etc.)
//   - reference: Search by reference
//   - date_from: Start date (RFC3339)
//   - date_to: End date (RFC3339)
func (h *AdminFinancialHandler) ListPayments(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Build filter
	filter := services.PaymentFilter{}

	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if userID, err := uuid.Parse(userIDStr); err == nil {
			filter.UserID = &userID
		}
	}

	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	if provider := c.Query("provider"); provider != "" {
		filter.Provider = &provider
	}

	if reference := c.Query("reference"); reference != "" {
		filter.Reference = &reference
	}

	if dateFromStr := c.Query("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse(time.RFC3339, dateFromStr); err == nil {
			filter.DateFrom = &dateFrom
		}
	}

	if dateToStr := c.Query("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse(time.RFC3339, dateToStr); err == nil {
			filter.DateTo = &dateTo
		}
	}

	// Call service
	result, err := h.financialService.ListPayments(c.Request.Context(), actorUUID, filter, page, pageSize)
	if err != nil {
		logger.Error("failed to list payments", "error", err, "actor_id", actorUUID)
		response.Error(c, http.StatusInternalServerError, "failed to list payments", nil)
		return
	}

	response.Success(c, http.StatusOK, result)
}

// GetPaymentDetail returns detailed payment information
// GET /api/v1/admin/payments/:id
func (h *AdminFinancialHandler) GetPaymentDetail(c *gin.Context) {
	actorUUID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid payment ID", nil)
		return
	}

	result, err := h.financialService.GetPaymentDetail(c.Request.Context(), actorUUID, paymentID)
	if err != nil {
		logger.Error("failed to get payment detail", "error", err, "payment_id", paymentID)
		response.Error(c, http.StatusNotFound, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, result)
}
