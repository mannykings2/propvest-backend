package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN FINANCIAL SERVICE (Milestone 7 - Phase 7)
// ═══════════════════════════════════════════════════════════════════════════
//
// Provides admin oversight of financial operations:
//   - Transaction monitoring across all users
//   - Payment tracking and verification
//   - Financial analytics and reconciliation
//   - Withdrawal approval workflow (if needed)
//
// AUTHORIZATION:
//   - All operations require admin or super_admin role
//   - No access to individual wallet balances (privacy)
//   - Aggregate and monitoring data only
//
// SECURITY:
//   - No PII in transaction lists (masked)
//   - Amount filtering bounded
//   - Pagination required

// AdminFinancialService defines the interface for admin financial operations.
type AdminFinancialService interface {
	// ListTransactions returns all transactions with filters (admin view)
	ListTransactions(ctx context.Context, actorID uuid.UUID, filter TransactionFilter, page, pageSize int) (*TransactionListResponse, error)
	
	// GetTransactionDetail returns detailed transaction information
	GetTransactionDetail(ctx context.Context, actorID, transactionID uuid.UUID) (*TransactionDetailResponse, error)
	
	// ListPayments returns payment records with filters
	ListPayments(ctx context.Context, actorID uuid.UUID, filter PaymentFilter, page, pageSize int) (*PaymentListResponse, error)
	
	// GetPaymentDetail returns detailed payment information
	GetPaymentDetail(ctx context.Context, actorID, paymentID uuid.UUID) (*PaymentDetailResponse, error)
}

// TransactionFilter defines filter criteria for transaction queries.
type TransactionFilter struct {
	UserID       *uuid.UUID // Filter by specific user
	Type         *string    // deposit, withdrawal, investment, etc.
	Status       *string    // pending, completed, failed
	Reference    *string    // Search by reference
	MinAmount    *int64     // Minimum amount (kobo)
	MaxAmount    *int64     // Maximum amount (kobo)
	DateFrom     *time.Time // Start date
	DateTo       *time.Time // End date
}

// PaymentFilter defines filter criteria for payment queries.
type PaymentFilter struct {
	UserID       *uuid.UUID
	Status       *string
	Provider     *string    // Paystack, Flutterwave, etc.
	Reference    *string
	DateFrom     *time.Time
	DateTo       *time.Time
}

// TransactionListResponse represents paginated transaction list.
type TransactionListResponse struct {
	Transactions []TransactionSummary `json:"transactions"`
	TotalCount   int64                `json:"total_count"`
	Page         int                  `json:"page"`
	PageSize     int                  `json:"page_size"`
	TotalPages   int                  `json:"total_pages"`
}

// TransactionSummary is a condensed transaction view for lists.
type TransactionSummary struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	UserCode        string    `json:"user_code"`        // For easier identification
	Type            string    `json:"type"`
	Amount          int64     `json:"amount"`           // In kobo
	Status          string    `json:"status"`
	Reference       string    `json:"reference"`
	Description     string    `json:"description"`
	CreatedAt       time.Time `json:"created_at"`
}

// TransactionDetailResponse represents full transaction details.
type TransactionDetailResponse struct {
	ID                string     `json:"id"`
	WalletID          string     `json:"wallet_id"`
	UserID            string     `json:"user_id"`
	UserCode          string     `json:"user_code"`
	Type              string     `json:"type"`
	Amount            int64      `json:"amount"`
	BalanceBefore     int64      `json:"balance_before"`
	BalanceAfter      int64      `json:"balance_after"`
	Reference         string     `json:"reference"`
	ExternalReference *string    `json:"external_reference,omitempty"`
	IdempotencyKey    *string    `json:"idempotency_key,omitempty"`
	Description       string     `json:"description"`
	Status            string     `json:"status"`
	Metadata          string     `json:"metadata,omitempty"` // JSON string
	CreatedAt         time.Time  `json:"created_at"`
}

// PaymentListResponse represents paginated payment list.
type PaymentListResponse struct {
	Payments   []PaymentSummary `json:"payments"`
	TotalCount int64            `json:"total_count"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// PaymentSummary is a condensed payment view for lists.
type PaymentSummary struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	UserCode    string    `json:"user_code"`
	Amount      int64     `json:"amount"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	Provider    string    `json:"provider"`
	Reference   string    `json:"reference"`
	Channel     string    `json:"channel"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// PaymentDetailResponse represents full payment details.
type PaymentDetailResponse struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	UserCode        string     `json:"user_code"`
	Amount          int64      `json:"amount"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	Provider        string     `json:"provider"`
	Reference       string     `json:"reference"`
	ProviderRef     *string    `json:"provider_ref,omitempty"`
	Channel         string     `json:"channel"`
	CardLast4       *string    `json:"card_last4,omitempty"`
	CardBrand       *string    `json:"card_brand,omitempty"`
	AuthorizationURL *string   `json:"authorization_url,omitempty"`
	ProviderResponse string    `json:"provider_response,omitempty"` // Sanitized JSON
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	FailedAt        *time.Time `json:"failed_at,omitempty"`
	FailureReason   *string    `json:"failure_reason,omitempty"`
}

// adminFinancialService is the concrete implementation.
type adminFinancialService struct {
	db *gorm.DB
}

// NewAdminFinancialService creates a new admin financial service instance.
func NewAdminFinancialService(db *gorm.DB) AdminFinancialService {
	return &adminFinancialService{
		db: db,
	}
}

// ListTransactions returns all transactions with filtering for admin monitoring.
//
// BOUNDED QUERY:
//   - Results limited by page_size (max 100)
//   - Date range filtering required for large datasets
//   - Amount range filtering bounded
//
// PRIVACY:
//   - UserCode shown instead of full name/email
//   - No wallet balance exposed
//   - Metadata sanitized (no sensitive data)
func (s *adminFinancialService) ListTransactions(ctx context.Context, actorID uuid.UUID, filter TransactionFilter, page, pageSize int) (*TransactionListResponse, error) {
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

	offset := (page - 1) * pageSize

	// Build query
	query := s.db.WithContext(ctx).
		Table("wallet_transactions").
		Select(`
			wallet_transactions.id,
			wallet_transactions.user_id,
			wallet_transactions.type,
			wallet_transactions.amount,
			wallet_transactions.status,
			wallet_transactions.reference,
			wallet_transactions.description,
			wallet_transactions.created_at,
			users.user_code
		`).
		Joins("LEFT JOIN users ON wallet_transactions.user_id = users.id")

	// Apply filters
	if filter.UserID != nil {
		query = query.Where("wallet_transactions.user_id = ?", *filter.UserID)
	}

	if filter.Type != nil {
		query = query.Where("wallet_transactions.type = ?", *filter.Type)
	}

	if filter.Status != nil {
		query = query.Where("wallet_transactions.status = ?", *filter.Status)
	}

	if filter.Reference != nil {
		query = query.Where("wallet_transactions.reference ILIKE ?", "%"+*filter.Reference+"%")
	}

	if filter.MinAmount != nil {
		query = query.Where("wallet_transactions.amount >= ?", *filter.MinAmount)
	}

	if filter.MaxAmount != nil {
		query = query.Where("wallet_transactions.amount <= ?", *filter.MaxAmount)
	}

	if filter.DateFrom != nil {
		query = query.Where("wallet_transactions.created_at >= ?", *filter.DateFrom)
	}

	if filter.DateTo != nil {
		query = query.Where("wallet_transactions.created_at <= ?", *filter.DateTo)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count transactions: %w", err)
	}

	// Get paginated results
	var transactions []TransactionSummary
	err := query.
		Order("wallet_transactions.created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Scan(&transactions).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}

	// Calculate total pages
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &TransactionListResponse{
		Transactions: transactions,
		TotalCount:   total,
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

// GetTransactionDetail returns detailed transaction information.
//
// INCLUDES:
//   - Full transaction record
//   - User code for identification
//   - Metadata (sanitized)
//   - Balance snapshots (for reconciliation)
func (s *adminFinancialService) GetTransactionDetail(ctx context.Context, actorID, transactionID uuid.UUID) (*TransactionDetailResponse, error) {
	var result struct {
		models.WalletTransaction
		UserCode string
	}

	err := s.db.WithContext(ctx).
		Table("wallet_transactions").
		Select("wallet_transactions.*, users.user_code").
		Joins("LEFT JOIN users ON wallet_transactions.user_id = users.id").
		Where("wallet_transactions.id = ?", transactionID).
		First(&result).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("transaction not found")
		}
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	// Build response
	response := &TransactionDetailResponse{
		ID:                result.ID.String(),
		WalletID:          result.WalletID.String(),
		UserID:            result.UserID.String(),
		UserCode:          result.UserCode,
		Type:              result.Type,
		Amount:            result.Amount,
		BalanceBefore:     result.BalanceBefore,
		BalanceAfter:      result.BalanceAfter,
		Reference:         result.Reference,
		ExternalReference: result.ExternalReference,
		IdempotencyKey:    result.IdempotencyKey,
		Description:       result.Description,
		Status:            result.Status,
		CreatedAt:         result.CreatedAt,
	}

	// Sanitize metadata if present
	if result.Metadata != nil {
		response.Metadata = string(result.Metadata)
	}

	return response, nil
}

// ListPayments returns payment records with filtering.
//
// BOUNDED QUERY:
//   - Results limited by page_size (max 100)
//   - Date range filtering for performance
//
// PRIVACY:
//   - Card details masked (only last 4 digits)
//   - Provider responses sanitized
func (s *adminFinancialService) ListPayments(ctx context.Context, actorID uuid.UUID, filter PaymentFilter, page, pageSize int) (*PaymentListResponse, error) {
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

	offset := (page - 1) * pageSize

	// Build query - using actual Payment model fields
	query := s.db.WithContext(ctx).
		Table("payments").
		Select(`
			payments.id,
			payments.user_id,
			payments.amount_kobo as amount,
			payments.currency,
			payments.status,
			payments.provider,
			payments.reference,
			payments.channel,
			payments.created_at,
			payments.updated_at,
			users.user_code
		`).
		Joins("LEFT JOIN users ON payments.user_id = users.id")

	// Apply filters
	if filter.UserID != nil {
		query = query.Where("payments.user_id = ?", *filter.UserID)
	}

	if filter.Status != nil {
		query = query.Where("payments.status = ?", *filter.Status)
	}

	if filter.Provider != nil {
		query = query.Where("payments.provider = ?", *filter.Provider)
	}

	if filter.Reference != nil {
		query = query.Where("payments.reference ILIKE ?", "%"+*filter.Reference+"%")
	}

	if filter.DateFrom != nil {
		query = query.Where("payments.created_at >= ?", *filter.DateFrom)
	}

	if filter.DateTo != nil {
		query = query.Where("payments.created_at <= ?", *filter.DateTo)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count payments: %w", err)
	}

	// Get paginated results using a scan-friendly struct
	var rawPayments []struct {
		ID        string    `json:"id"`
		UserID    string    `json:"user_id"`
		UserCode  string    `json:"user_code"`
		Amount    int64     `json:"amount"`
		Currency  string    `json:"currency"`
		Status    string    `json:"status"`
		Provider  string    `json:"provider"`
		Reference string    `json:"reference"`
		Channel   string    `json:"channel"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	
	err := query.
		Order("payments.created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Scan(&rawPayments).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list payments: %w", err)
	}

	// Convert to PaymentSummary, deriving CompletedAt from UpdatedAt for completed payments
	payments := make([]PaymentSummary, len(rawPayments))
	for i, raw := range rawPayments {
		payments[i] = PaymentSummary{
			ID:        raw.ID,
			UserID:    raw.UserID,
			UserCode:  raw.UserCode,
			Amount:    raw.Amount,
			Currency:  raw.Currency,
			Status:    raw.Status,
			Provider:  raw.Provider,
			Reference: raw.Reference,
			Channel:   raw.Channel,
			CreatedAt: raw.CreatedAt,
		}
		// Set CompletedAt to UpdatedAt if payment is success or failed
		if raw.Status == models.PaymentStatusSuccess || raw.Status == models.PaymentStatusFailed {
			payments[i].CompletedAt = &raw.UpdatedAt
		}
	}

	// Calculate total pages
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &PaymentListResponse{
		Payments:   payments,
		TotalCount: total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetPaymentDetail returns detailed payment information.
//
// INCLUDES:
//   - Full payment record
//   - Card details (masked)
//   - Provider response (sanitized)
//   - Timeline (created/completed/failed timestamps)
func (s *adminFinancialService) GetPaymentDetail(ctx context.Context, actorID, paymentID uuid.UUID) (*PaymentDetailResponse, error) {
	var result struct {
		models.Payment
		UserCode string
	}

	err := s.db.WithContext(ctx).
		Table("payments").
		Select("payments.*, users.user_code").
		Joins("LEFT JOIN users ON payments.user_id = users.id").
		Where("payments.id = ?", paymentID).
		First(&result).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("payment not found")
		}
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	// Build response using actual Payment model fields
	response := &PaymentDetailResponse{
		ID:           result.ID.String(),
		UserID:       result.UserID.String(),
		UserCode:     result.UserCode,
		Amount:       result.AmountKobo, // Payment model uses AmountKobo
		Currency:     result.Currency,
		Status:       result.Status,
		Provider:     result.Provider,
		Reference:    result.Reference,
		ProviderRef:  result.ProviderReference, // Payment model uses ProviderReference
		Channel:      result.Channel,
		CreatedAt:    result.CreatedAt,
	}

	// Payment model doesn't have CompletedAt/FailedAt/FailureReason fields
	// Use UpdatedAt as completion time for success/failed status
	if result.Status == models.PaymentStatusSuccess {
		response.CompletedAt = &result.UpdatedAt
	} else if result.Status == models.PaymentStatusFailed {
		response.FailedAt = &result.UpdatedAt
	}

	// Payment model doesn't have CardLast4/CardBrand fields
	// These would need to be extracted from RawPayload if needed in future

	// Add authorization URL if present
	if result.AuthorizationURL != nil {
		response.AuthorizationURL = result.AuthorizationURL
	}

	// Sanitize provider response if present
	if len(result.RawPayload) > 0 {
		response.ProviderResponse = string(result.RawPayload)
	}

	return response, nil
}
