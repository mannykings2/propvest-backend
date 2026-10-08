package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// InvestmentHandler handles all investment-related HTTP requests.
//
// Responsibilities:
//   - Parse and validate HTTP request bodies and query parameters
//   - Extract user ID from JWT context (set by auth middleware)
//   - Call appropriate service methods
//   - Transform service responses into HTTP responses
//   - Handle errors and return appropriate status codes
//
// This is the HTTP layer - it knows about Gin, status codes, and JSON,
// but knows nothing about business logic (that's in the service layer).
type InvestmentHandler struct {
	investmentService services.InvestmentService
}

// NewInvestmentHandler creates a new investment handler.
// Called once at startup and injected with InvestmentService.
func NewInvestmentHandler(investmentService services.InvestmentService) *InvestmentHandler {
	return &InvestmentHandler{
		investmentService: investmentService,
	}
}

// CreateInvestment handles POST /api/v1/investments
//
// Creates a new investment (user purchases property slots).
// Protected route - requires authentication.
//
// Request body:
//   {
//     "property_id": "uuid",
//     "slots": 10,
//     "idempotency_key": "optional-unique-key"
//   }
//
// Success response (201 Created):
//   {
//     "success": true,
//     "message": "Investment created successfully",
//     "data": {
//       "id": "uuid",
//       "property_id": "uuid",
//       "slots": 10,
//       "amount_kobo": 1000000,
//       "unit_price_kobo": 100000,
//       "currency": "NGN",
//       "status": "active",
//       "reference": "INV-...",
//       "created_at": "2024-01-01T00:00:00Z",
//       ...
//     }
//   }
//
// Error responses:
//   - 400: Validation failed (missing fields, invalid slots)
//   - 404: Property not found
//   - 409: Idempotency conflict (same key, different request)
//   - 422: Property not available, insufficient balance, below minimum
//   - 500: Internal server error
func (h *InvestmentHandler) CreateInvestment(c *gin.Context) {
	// Extract user ID from JWT context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse and validate request body
	var req dto.CreateInvestmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Call service layer
	investment, err := h.investmentService.CreateInvestment(c.Request.Context(), userID.(uuid.UUID), req)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response
	response.Success(c, http.StatusCreated, "Investment created successfully", investment)
}

// GetInvestment handles GET /api/v1/investments/:id
//
// Retrieves a single investment by ID.
// Protected route - requires authentication.
// Authorization: Only the owner or admin can view.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Investment retrieved successfully",
//     "data": { ... }
//   }
//
// Error responses:
//   - 403: Forbidden (not the owner)
//   - 404: Investment not found
//   - 500: Internal server error
func (h *InvestmentHandler) GetInvestment(c *gin.Context) {
	// Extract user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Check if user is admin
	role, _ := c.Get("role")
	isAdmin := role == "admin"

	// Parse investment ID from URL parameter
	investmentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid investment ID format")
		return
	}

	// Call service layer
	investment, err := h.investmentService.GetInvestment(c.Request.Context(), userID.(uuid.UUID), investmentID, isAdmin)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response
	response.Success(c, http.StatusOK, "Investment retrieved successfully", investment)
}

// ListMyInvestments handles GET /api/v1/investments
//
// Retrieves all investments for the authenticated user.
// Protected route - requires authentication.
//
// Query parameters:
//   - page: Page number (default: 1)
//   - page_size: Items per page (default: 20, max: 100)
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Investments retrieved successfully",
//     "data": {
//       "investments": [...],
//       "pagination": {
//         "current_page": 1,
//         "page_size": 20,
//         "total_pages": 5,
//         "total_records": 98,
//         "has_next": true,
//         "has_previous": false
//       }
//     }
//   }
func (h *InvestmentHandler) ListMyInvestments(c *gin.Context) {
	// Extract user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Validate and normalize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Calculate limit and offset
	limit := pageSize
	offset := (page - 1) * pageSize

	// Call service layer
	investments, total, err := h.investmentService.ListUserInvestments(c.Request.Context(), userID.(uuid.UUID), limit, offset)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Build pagination metadata
	pagination := dto.NewInvestmentPaginationMetadata(page, pageSize, total)

	// Build response
	responseData := dto.InvestmentListResponse{
		Investments: investments,
		Pagination:  pagination,
	}

	// Success response
	response.Success(c, http.StatusOK, "Investments retrieved successfully", responseData)
}

// GetPortfolioSummary handles GET /api/v1/portfolio/summary
//
// Returns aggregate portfolio metrics for the authenticated user.
// Protected route - requires authentication.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Portfolio summary retrieved successfully",
//     "data": {
//       "total_invested": 5000000,
//       "active_count": 3,
//       "wallet_balance": 2000000,
//       "earnings_balance": 150000
//     }
//   }
func (h *InvestmentHandler) GetPortfolioSummary(c *gin.Context) {
	// Extract user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Call service layer
	summary, err := h.investmentService.GetPortfolioSummary(c.Request.Context(), userID.(uuid.UUID))
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response
	response.Success(c, http.StatusOK, "Portfolio summary retrieved successfully", summary)
}

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// ListAllInvestments handles GET /api/v1/admin/investments
//
// Retrieves all investments with optional filters.
// Protected route - requires admin role.
//
// Query parameters:
//   - page: Page number (default: 1)
//   - page_size: Items per page (default: 20, max: 100)
//   - status: Filter by status (optional: active, completed, cancelled, refunded)
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Investments retrieved successfully",
//     "data": {
//       "investments": [...],
//       "pagination": {...}
//     }
//   }
func (h *InvestmentHandler) ListAllInvestments(c *gin.Context) {
	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status") // Optional filter

	// Validate and normalize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Calculate limit and offset
	limit := pageSize
	offset := (page - 1) * pageSize

	// Call service layer
	investments, total, err := h.investmentService.ListAllInvestments(c.Request.Context(), status, limit, offset)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Build pagination metadata
	pagination := dto.NewInvestmentPaginationMetadata(page, pageSize, total)

	// Build response
	responseData := dto.InvestmentListResponse{
		Investments: investments,
		Pagination:  pagination,
	}

	// Success response
	response.Success(c, http.StatusOK, "Investments retrieved successfully", responseData)
}

// ListPropertyInvestments handles GET /api/v1/admin/properties/:id/investments
//
// Retrieves all investors in a specific property.
// Protected route - requires admin role.
//
// Query parameters:
//   - page: Page number (default: 1)
//   - page_size: Items per page (default: 20, max: 100)
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Property investments retrieved successfully",
//     "data": {
//       "investments": [...],
//       "pagination": {...}
//     }
//   }
func (h *InvestmentHandler) ListPropertyInvestments(c *gin.Context) {
	// Parse property ID from URL parameter
	propertyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid property ID format")
		return
	}

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Validate and normalize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Calculate limit and offset
	limit := pageSize
	offset := (page - 1) * pageSize

	// Call service layer
	investments, total, err := h.investmentService.ListPropertyInvestments(c.Request.Context(), propertyID, limit, offset)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Build pagination metadata
	pagination := dto.NewInvestmentPaginationMetadata(page, pageSize, total)

	// Build response
	responseData := dto.InvestmentListResponse{
		Investments: investments,
		Pagination:  pagination,
	}

	// Success response
	response.Success(c, http.StatusOK, "Property investments retrieved successfully", responseData)
}

// GetInvestmentMetrics handles GET /api/v1/admin/investments/metrics
//
// Returns platform-wide investment statistics.
// Protected route - requires admin role.
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Investment metrics retrieved successfully",
//     "data": {
//       "total_investments": 1250,
//       "total_amount_kobo": 125000000000,
//       "total_amount_naira": 1250000000.00,
//       "active_investments": 1000,
//       "completed_investments": 200,
//       "cancelled_investments": 30,
//       "refunded_investments": 20,
//       "unique_investors": 450,
//       "average_investment_kobo": 100000000,
//       "average_investment_naira": 1000000.00
//     }
//   }
func (h *InvestmentHandler) GetInvestmentMetrics(c *gin.Context) {
	// Call service layer
	metrics, err := h.investmentService.GetInvestmentMetrics(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}

	// Success response
	response.Success(c, http.StatusOK, "Investment metrics retrieved successfully", metrics)
}

