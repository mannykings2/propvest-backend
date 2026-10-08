package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// InvestmentService implements Milestone 5: Investment Module.
//
// Core workflow (CreateInvestment):
//   1. Check idempotency (if key provided)
//   2. Lock property (SELECT FOR UPDATE) - validate status and capacity
//   3. Lock wallet (SELECT FOR UPDATE) - check sufficient balance
//   4. Calculate amount (slots * property.UnitPrice)
//   5. Debit wallet (create wallet transaction)
//   6. Update property funding (RaisedAmount, UnitsSold, InvestorCount)
//   7. Create investment record
//   8. Create outbox event for async notifications
//   9. Commit transaction atomically
//
// FINANCIAL SAFETY (docs 2.3, 5.3):
//   - All operations in single database transaction
//   - Lock order: Property → Wallet (prevents deadlocks)
//   - Amount calculated server-side (never trust client)
//   - Partial purchases rejected (all-or-nothing)
//   - Idempotency supported (via unique key constraint)
//
// BUSINESS RULES:
//   - Property must be "active" and not fully funded
//   - User must have sufficient MainBalance
//   - Investment amount = slots * property.UnitPrice
//   - Minimum investment enforced (property.MinimumInvestment)
//   - InvestorCount incremented only for new investors
type InvestmentService interface {
	// CreateInvestment orchestrates the investment purchase workflow.
	// Returns the created investment or an error if any validation fails.
	CreateInvestment(ctx context.Context, userID uuid.UUID, req dto.CreateInvestmentRequest) (*dto.InvestmentResponse, error)
	
	// GetInvestment retrieves a single investment by ID.
	// Only the owner or admin can access.
	GetInvestment(ctx context.Context, userID uuid.UUID, investmentID uuid.UUID, isAdmin bool) (*dto.InvestmentResponse, error)
	
	// ListUserInvestments retrieves all investments for a user with pagination.
	ListUserInvestments(ctx context.Context, userID uuid.UUID, limit, offset int) ([]dto.InvestmentResponse, int64, error)
	
	// GetPortfolioSummary returns aggregate portfolio metrics for a user.
	GetPortfolioSummary(ctx context.Context, userID uuid.UUID) (*dto.PortfolioSummaryResponse, error)
	
	// Admin methods
	
	// ListAllInvestments retrieves all investments with optional status filter (admin only).
	ListAllInvestments(ctx context.Context, status string, limit, offset int) ([]dto.InvestmentResponse, int64, error)
	
	// ListPropertyInvestments retrieves all investors in a specific property (admin only).
	ListPropertyInvestments(ctx context.Context, propertyID uuid.UUID, limit, offset int) ([]dto.InvestmentResponse, int64, error)
	
	// GetInvestmentMetrics returns platform-wide investment statistics (admin only).
	GetInvestmentMetrics(ctx context.Context) (*dto.InvestmentMetricsResponse, error)
}

type investmentService struct {
	investmentRepo repositories.InvestmentRepository
	propertyRepo   repositories.PropertyRepository
	walletRepo     repositories.WalletRepository
	outboxRepo     repositories.OutboxRepository
	db             *gorm.DB
}

// NewInvestmentService constructs the investment service with all dependencies.
func NewInvestmentService(
	investmentRepo repositories.InvestmentRepository,
	propertyRepo repositories.PropertyRepository,
	walletRepo repositories.WalletRepository,
	outboxRepo repositories.OutboxRepository,
	db *gorm.DB,
) InvestmentService {
	return &investmentService{
		investmentRepo: investmentRepo,
		propertyRepo:   propertyRepo,
		walletRepo:     walletRepo,
		outboxRepo:     outboxRepo,
		db:             db,
	}
}

// CreateInvestment implements the core investment purchase workflow.
//
// Transaction flow:
//   BEGIN TRANSACTION
//     1. Check idempotency
//     2. Lock & validate property
//     3. Lock & validate wallet
//     4. Debit wallet
//     5. Update property funding
//     6. Create investment
//     7. Create outbox event
//   COMMIT (or ROLLBACK on any error)
func (s *investmentService) CreateInvestment(ctx context.Context, userID uuid.UUID, req dto.CreateInvestmentRequest) (*dto.InvestmentResponse, error) {
	logger.Info("Investment creation started",
		"user_id", userID,
		"property_id", req.PropertyID,
		"slots", req.Slots,
		"idempotency_key", req.IdempotencyKey)

	var createdInvestment *models.Investment

	// Execute entire workflow in a single transaction
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// ═══════════════════════════════════════════════════════════════════
		// STEP 1: IDEMPOTENCY CHECK
		// ═══════════════════════════════════════════════════════════════════
		if req.IdempotencyKey != "" {
			existing, err := s.investmentRepo.FindByUserAndIdempotencyKey(ctx, userID, req.IdempotencyKey, tx)
			if err == nil {
				// Investment with this key already exists - return it (idempotent)
				logger.Info("Idempotent request detected - returning existing investment",
					"investment_id", existing.ID,
					"idempotency_key", req.IdempotencyKey)
				createdInvestment = existing
				return nil // Early return - skip rest of workflow
			}
			if err != gorm.ErrRecordNotFound {
				logger.Error("Failed to check idempotency", "error", err)
				return apperrors.ErrInternalServer
			}
			// Not found - proceed with creation
		}

		// ═══════════════════════════════════════════════════════════════════
		// STEP 2: LOCK & VALIDATE PROPERTY
		// ═══════════════════════════════════════════════════════════════════
		// Lock order: Property FIRST (to prevent deadlocks)
		property, err := s.propertyRepo.FindByIDForUpdate(ctx, req.PropertyID, tx)
		if err != nil {
			if repositories.IsErrRecordNotFound(err) {
				logger.Warn("Property not found", "property_id", req.PropertyID)
				return apperrors.NewAppError("Property not found", "not_found")
			}
			logger.Error("Failed to lock property", "error", err)
			return apperrors.ErrInternalServer
		}

		// Validate property can accept investments
		if !property.CanAcceptInvestments() {
			logger.Warn("Property cannot accept investments",
				"property_id", req.PropertyID,
				"status", property.Status,
				"is_fully_funded", property.IsFullyFunded())
			return apperrors.NewAppError("Property is not available for investment", "resource_unavailable")
		}

		// Calculate investment amount (server-side - NEVER trust client)
		amountKobo := int64(req.Slots) * property.UnitPrice

		// Validate minimum investment
		if amountKobo < property.MinimumInvestment {
			logger.Warn("Investment below minimum",
				"amount_kobo", amountKobo,
				"minimum_investment", property.MinimumInvestment)
			return apperrors.NewAppError(
				fmt.Sprintf("Minimum investment is ₦%.2f", float64(property.MinimumInvestment)/100.0),
				"validation_error")
		}

		// Check if enough slots available
		availableSlots := int(property.TotalUnits - property.UnitsSold)
		if req.Slots > availableSlots {
			logger.Warn("Insufficient slots available",
				"requested_slots", req.Slots,
				"available_slots", availableSlots)
			return apperrors.NewAppError(
				fmt.Sprintf("Only %d slots available", availableSlots),
				"resource_unavailable")
		}

		// ═══════════════════════════════════════════════════════════════════
		// STEP 3: LOCK & VALIDATE WALLET
		// ═══════════════════════════════════════════════════════════════════
		wallet, err := s.walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
		if err != nil {
			if repositories.IsErrRecordNotFound(err) {
				logger.Warn("Wallet not found", "user_id", userID)
				return apperrors.ErrWalletNotFound
			}
			logger.Error("Failed to lock wallet", "error", err)
			return apperrors.ErrInternalServer
		}

		// Check sufficient balance (MainBalance only - not earnings)
		if !wallet.CanInvest(amountKobo) {
			availableBalance := wallet.AvailableBalance()
			logger.Warn("Insufficient wallet balance",
				"required_kobo", amountKobo,
				"available_kobo", availableBalance)
			return apperrors.NewAppError(
				fmt.Sprintf("Insufficient balance. Required: ₦%.2f, Available: ₦%.2f",
					float64(amountKobo)/100.0,
					float64(availableBalance)/100.0),
				"insufficient_funds")
		}

		// ═══════════════════════════════════════════════════════════════════
		// STEP 4: DEBIT WALLET
		// ═══════════════════════════════════════════════════════════════════
		// Generate unique reference for this investment
		investmentReference := fmt.Sprintf("INV-%s", uuid.New().String())

		// Create wallet transaction (debit)
		walletTx := &models.WalletTransaction{
			WalletID:      wallet.ID,
			UserID:        userID,
			Type:          "investment",
			Amount:        amountKobo,
			BalanceBefore: wallet.MainBalance,
			BalanceAfter:  wallet.MainBalance - amountKobo,
			Reference:     investmentReference,
			Description:   fmt.Sprintf("Investment in %s (%d slots)", property.Title, req.Slots),
			Status:        "completed",
		}

		err = s.walletRepo.CreateTransaction(ctx, walletTx)
		if err != nil {
			logger.Error("Failed to create wallet transaction", "error", err)
			return apperrors.ErrInternalServer
		}

		// Update wallet balance
		wallet.MainBalance -= amountKobo
		err = s.walletRepo.Update(ctx, wallet)
		if err != nil {
			logger.Error("Failed to update wallet balance", "error", err)
			return apperrors.ErrInternalServer
		}

		logger.Info("Wallet debited successfully",
			"amount_kobo", amountKobo,
			"new_balance", wallet.MainBalance)

		// ═══════════════════════════════════════════════════════════════════
		// STEP 5: UPDATE PROPERTY FUNDING
		// ═══════════════════════════════════════════════════════════════════
		// Check if this is a new investor (for InvestorCount)
		hasInvested, err := s.investmentRepo.HasActiveInvestmentByUserAndProperty(ctx, tx, userID, req.PropertyID)
		if err != nil {
			logger.Error("Failed to check investor existence", "error", err)
			return apperrors.ErrInternalServer
		}

		investorDelta := 0
		if !hasInvested {
			investorDelta = 1 // This is a new investor
		}

		// Increment property funding
		err = s.propertyRepo.IncrementFunding(ctx, req.PropertyID, amountKobo, int64(req.Slots), investorDelta, tx)
		if err != nil {
			logger.Error("Failed to update property funding", "error", err)
			return apperrors.ErrInternalServer
		}

		logger.Info("Property funding updated",
			"amount_kobo", amountKobo,
			"slots", req.Slots,
			"investor_delta", investorDelta)

		// ═══════════════════════════════════════════════════════════════════
		// STEP 6: CREATE INVESTMENT RECORD
		// ═══════════════════════════════════════════════════════════════════
		investment := &models.Investment{
			UserID:         userID,
			PropertyID:     req.PropertyID,
			Slots:          req.Slots,
			AmountKobo:     amountKobo,
			UnitPriceKobo:  property.UnitPrice, // Snapshot price
			Currency:       "NGN",
			Status:         models.InvestmentStatusActive,
			Reference:      investmentReference,
		}

		// Set idempotency key if provided
		if req.IdempotencyKey != "" {
			investment.IdempotencyKey = &req.IdempotencyKey
		}

		err = s.investmentRepo.Create(ctx, investment, tx)
		if err != nil {
			logger.Error("Failed to create investment record", "error", err)
			return apperrors.ErrInternalServer
		}

		logger.Info("Investment record created", "investment_id", investment.ID)

		// ═══════════════════════════════════════════════════════════════════
		// STEP 7: CREATE OUTBOX EVENT
		// ═══════════════════════════════════════════════════════════════════
		// Create event payload
		eventPayload := map[string]interface{}{
			"investment_id": investment.ID,
			"user_id":       userID,
			"property_id":   req.PropertyID,
			"property_title": property.Title,
			"slots":         req.Slots,
			"amount_kobo":   amountKobo,
			"unit_price":    property.UnitPrice,
			"reference":     investmentReference,
			"created_at":    time.Now(),
		}

		payloadJSON, err := json.Marshal(eventPayload)
		if err != nil {
			logger.Error("Failed to marshal event payload", "error", err)
			return apperrors.ErrInternalServer
		}

		// Create outbox event
		outboxEvent := &models.OutboxEvent{
			EventType:     models.EventTypeInvestmentCreated,
			AggregateType: "investment",
			AggregateID:   investment.ID,
			Payload:       datatypes.JSON(payloadJSON),
			Status:        models.OutboxStatusPending,
			AvailableAt:   time.Now(),
		}

		err = s.outboxRepo.CreateEvent(ctx, outboxEvent, tx)
		if err != nil {
			logger.Error("Failed to create outbox event", "error", err)
			return apperrors.ErrInternalServer
		}

		logger.Info("Outbox event created",
			"event_id", outboxEvent.ID,
			"event_type", models.EventTypeInvestmentCreated)

		// Store the created investment for response
		createdInvestment = investment

		return nil // Success - commit transaction
	})

	if err != nil {
		return nil, err
	}

	logger.Info("Investment creation completed successfully",
		"investment_id", createdInvestment.ID,
		"amount_kobo", createdInvestment.AmountKobo)

	// Return response
	response := dto.InvestmentToResponse(*createdInvestment)
	return &response, nil
}

// GetInvestment retrieves a single investment by ID.
func (s *investmentService) GetInvestment(ctx context.Context, userID uuid.UUID, investmentID uuid.UUID, isAdmin bool) (*dto.InvestmentResponse, error) {
	investment, err := s.investmentRepo.FindByID(ctx, investmentID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, apperrors.NewAppError("Investment not found", "not_found")
		}
		logger.Error("Failed to find investment", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	// Authorization: only owner or admin can view
	if !isAdmin && investment.UserID != userID {
		logger.Warn("Unauthorized investment access attempt",
			"user_id", userID,
			"investment_user_id", investment.UserID)
		return nil, apperrors.NewAppError("You don't have permission to view this investment", "forbidden")
	}

	response := dto.InvestmentToResponse(*investment)
	return &response, nil
}

// ListUserInvestments retrieves all investments for a user with pagination.
func (s *investmentService) ListUserInvestments(ctx context.Context, userID uuid.UUID, limit, offset int) ([]dto.InvestmentResponse, int64, error) {
	investments, total, err := s.investmentRepo.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		logger.Error("Failed to list user investments", "error", err)
		return nil, 0, apperrors.ErrInternalServer
	}

	responses := dto.InvestmentsToResponse(investments)
	return responses, total, nil
}

// GetPortfolioSummary returns aggregate portfolio metrics for a user.
func (s *investmentService) GetPortfolioSummary(ctx context.Context, userID uuid.UUID) (*dto.PortfolioSummaryResponse, error) {
	// Get investment summary
	totalInvested, activeCount, err := s.investmentRepo.PortfolioSummary(ctx, userID)
	if err != nil {
		logger.Error("Failed to get portfolio summary", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	// Get wallet balances
	wallet, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, apperrors.ErrWalletNotFound
		}
		logger.Error("Failed to get wallet", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	return &dto.PortfolioSummaryResponse{
		TotalInvested:   totalInvested,
		ActiveCount:     activeCount,
		WalletBalance:   wallet.MainBalance,
		EarningsBalance: wallet.EarningsBalance,
	}, nil
}



// ═══════════════════════════════════════════════════════════════════════════
// ADMIN METHODS
// ═══════════════════════════════════════════════════════════════════════════

// ListAllInvestments retrieves all investments with optional status filter.
// Admin only - used in admin investments list view.
func (s *investmentService) ListAllInvestments(ctx context.Context, status string, limit, offset int) ([]dto.InvestmentResponse, int64, error) {
	investments, total, err := s.investmentRepo.ListAll(ctx, status, limit, offset)
	if err != nil {
		logger.Error("Failed to list all investments", "error", err)
		return nil, 0, apperrors.ErrInternalServer
	}

	responses := dto.InvestmentsToResponse(investments)
	return responses, total, nil
}

// ListPropertyInvestments retrieves all investors in a specific property.
// Admin only - used in property details view to see all investors.
func (s *investmentService) ListPropertyInvestments(ctx context.Context, propertyID uuid.UUID, limit, offset int) ([]dto.InvestmentResponse, int64, error) {
	// Verify property exists
	_, err := s.propertyRepo.FindByID(ctx, propertyID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, 0, apperrors.NewAppError("Property not found", "not_found")
		}
		logger.Error("Failed to find property", "error", err)
		return nil, 0, apperrors.ErrInternalServer
	}

	investments, total, err := s.investmentRepo.ListByProperty(ctx, propertyID, limit, offset)
	if err != nil {
		logger.Error("Failed to list property investments", "error", err)
		return nil, 0, apperrors.ErrInternalServer
	}

	responses := dto.InvestmentsToResponse(investments)
	return responses, total, nil
}

// GetInvestmentMetrics returns platform-wide investment statistics.
// Admin only - used in admin dashboard for metrics display.
func (s *investmentService) GetInvestmentMetrics(ctx context.Context) (*dto.InvestmentMetricsResponse, error) {
	metrics, err := s.investmentRepo.Metrics(ctx)
	if err != nil {
		logger.Error("Failed to get investment metrics", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	response := dto.InvestmentMetricsToResponse(metrics)
	return &response, nil
}
