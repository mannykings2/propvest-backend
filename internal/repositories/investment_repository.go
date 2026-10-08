package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InvestmentRepository is the data-access contract for investments.
type InvestmentRepository interface {
	// Create inserts an investment. Usually called inside a transaction via the
	// tx handle passed from the service (so it participates in the atomic wallet
	// debit + property funding update).
	Create(ctx context.Context, inv *models.Investment, tx *gorm.DB) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Investment, error)
	
	// FindByIDForUpdate retrieves an investment with row-level lock for update.
	// MUST be called within a transaction. Used in cancellation workflow.
	FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB) (*models.Investment, error)
	
	// FindByUserAndIdempotencyKey checks if an investment with the given idempotency
	// key already exists for the user. Used to prevent duplicate investments.
	FindByUserAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, tx *gorm.DB) (*models.Investment, error)
	
	// HasActiveInvestmentByUserAndProperty checks if a user has already invested
	// in a property. Used to determine if user should be counted as a new investor.
	HasActiveInvestmentByUserAndProperty(ctx context.Context, tx *gorm.DB, userID, propertyID uuid.UUID) (bool, error)
	
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Investment, int64, error)
	
	// ListByProperty retrieves all investments for a property with pagination.
	// Used in admin property details view.
	ListByProperty(ctx context.Context, propertyID uuid.UUID, limit, offset int) ([]models.Investment, int64, error)
	
	// ListAll retrieves all investments with optional filters.
	// Used in admin investments list view.
	ListAll(ctx context.Context, status string, limit, offset int) ([]models.Investment, int64, error)
	
	// PortfolioSummary aggregates a user's holdings: total invested and count.
	PortfolioSummary(ctx context.Context, userID uuid.UUID) (totalInvested int64, count int64, err error)
	
	// Metrics returns platform-wide investment metrics for admin dashboard.
	Metrics(ctx context.Context) (*InvestmentMetrics, error)
	
	CountAll(ctx context.Context) (int64, error)
	SumAll(ctx context.Context) (int64, error)
}

// InvestmentMetrics holds aggregate statistics for the admin dashboard.
type InvestmentMetrics struct {
	TotalInvestments      int64 `json:"total_investments"`
	TotalAmountKobo       int64 `json:"total_amount_kobo"`
	ActiveInvestments     int64 `json:"active_investments"`
	CompletedInvestments  int64 `json:"completed_investments"`
	CancelledInvestments  int64 `json:"cancelled_investments"`
	RefundedInvestments   int64 `json:"refunded_investments"`
	UniqueInvestors       int64 `json:"unique_investors"`
	AverageInvestmentKobo int64 `json:"average_investment_kobo"`
}

type investmentRepository struct {
	*BaseRepository
}

// NewInvestmentRepository constructs the repository.
func NewInvestmentRepository(db *gorm.DB) InvestmentRepository {
	return &investmentRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *investmentRepository) Create(ctx context.Context, inv *models.Investment, tx *gorm.DB) error {
	if tx != nil {
		return tx.WithContext(ctx).Create(inv).Error
	}
	return r.WithContext(ctx).Create(inv).Error
}

func (r *investmentRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Investment, error) {
	var inv models.Investment
	if err := r.WithContext(ctx).Preload("Property").Where("id = ?", id).First(&inv).Error; err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *investmentRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Investment, int64, error) {
	q := r.WithContext(ctx).Model(&models.Investment{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var invs []models.Investment
	if err := q.Preload("Property").Order("created_at DESC").Limit(limit).Offset(offset).Find(&invs).Error; err != nil {
		return nil, 0, err
	}
	return invs, total, nil
}

func (r *investmentRepository) PortfolioSummary(ctx context.Context, userID uuid.UUID) (int64, int64, error) {
	var result struct {
		Total int64
		Count int64
	}
	err := r.WithContext(ctx).
		Model(&models.Investment{}).
		Select("COALESCE(SUM(amount_kobo),0) AS total, COUNT(*) AS count").
		Where("user_id = ? AND status = ?", userID, models.InvestmentStatusActive).
		Scan(&result).Error
	return result.Total, result.Count, err
}

func (r *investmentRepository) CountAll(ctx context.Context) (int64, error) {
	var c int64
	err := r.WithContext(ctx).Model(&models.Investment{}).Count(&c).Error
	return c, err
}

func (r *investmentRepository) SumAll(ctx context.Context) (int64, error) {
	var sum int64
	err := r.WithContext(ctx).Model(&models.Investment{}).
		Select("COALESCE(SUM(amount_kobo),0)").Scan(&sum).Error
	return sum, err
}

// FindByIDForUpdate retrieves an investment with row-level lock.
// MUST be called within a transaction. Used in cancellation workflow to prevent
// concurrent modifications.
func (r *investmentRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID, tx *gorm.DB) (*models.Investment, error) {
	if tx == nil {
		return nil, gorm.ErrInvalidTransaction
	}

	var inv models.Investment
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Property").
		Where("id = ?", id).
		First(&inv).Error

	if err != nil {
		return nil, err
	}

	return &inv, nil
}

// FindByUserAndIdempotencyKey checks if an investment with the given idempotency
// key already exists for the user. Returns the existing investment if found,
// or gorm.ErrRecordNotFound if not found.
func (r *investmentRepository) FindByUserAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string, tx *gorm.DB) (*models.Investment, error) {
	var inv models.Investment
	db := r.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}

	err := db.
		Preload("Property").
		Where("user_id = ? AND idempotency_key = ?", userID, key).
		First(&inv).Error

	if err != nil {
		return nil, err
	}

	return &inv, nil
}

// HasActiveInvestmentByUserAndProperty checks if a user has already invested
// in a property. Used to determine if user should be counted as a new investor
// when incrementing property.InvestorCount.
//
// Returns true if user has at least one active investment in the property.
func (r *investmentRepository) HasActiveInvestmentByUserAndProperty(ctx context.Context, tx *gorm.DB, userID, propertyID uuid.UUID) (bool, error) {
	db := r.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}

	var count int64
	err := db.
		Model(&models.Investment{}).
		Where("user_id = ? AND property_id = ? AND status = ?", userID, propertyID, models.InvestmentStatusActive).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// ListByProperty retrieves all investments for a specific property with pagination.
// Orders by created_at DESC (newest first). Preloads Property for response building.
func (r *investmentRepository) ListByProperty(ctx context.Context, propertyID uuid.UUID, limit, offset int) ([]models.Investment, int64, error) {
	q := r.WithContext(ctx).Model(&models.Investment{}).Where("property_id = ?", propertyID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var invs []models.Investment
	if err := q.Preload("Property").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&invs).Error; err != nil {
		return nil, 0, err
	}

	return invs, total, nil
}

// ListAll retrieves all investments with optional status filter and pagination.
// Used in admin investments list view. Orders by created_at DESC (newest first).
func (r *investmentRepository) ListAll(ctx context.Context, status string, limit, offset int) ([]models.Investment, int64, error) {
	q := r.WithContext(ctx).Model(&models.Investment{})

	// Apply status filter if provided
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var invs []models.Investment
	if err := q.Preload("Property").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&invs).Error; err != nil {
		return nil, 0, err
	}

	return invs, total, nil
}

// Metrics returns platform-wide investment statistics for the admin dashboard.
// Calculates totals, counts by status, unique investors, and average investment size.
func (r *investmentRepository) Metrics(ctx context.Context) (*InvestmentMetrics, error) {
	var metrics InvestmentMetrics

	// Get total count and sum
	err := r.WithContext(ctx).
		Model(&models.Investment{}).
		Select("COUNT(*) as total_investments, COALESCE(SUM(amount_kobo), 0) as total_amount_kobo").
		Scan(&metrics).Error
	if err != nil {
		return nil, err
	}

	// Count by status
	type StatusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []StatusCount
	err = r.WithContext(ctx).
		Model(&models.Investment{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&statusCounts).Error
	if err != nil {
		return nil, err
	}

	// Map status counts
	for _, sc := range statusCounts {
		switch sc.Status {
		case models.InvestmentStatusActive:
			metrics.ActiveInvestments = sc.Count
		case models.InvestmentStatusCompleted:
			metrics.CompletedInvestments = sc.Count
		case models.InvestmentStatusCancelled:
			metrics.CancelledInvestments = sc.Count
		case models.InvestmentStatusRefunded:
			metrics.RefundedInvestments = sc.Count
		}
	}

	// Count unique investors
	err = r.WithContext(ctx).
		Model(&models.Investment{}).
		Distinct("user_id").
		Count(&metrics.UniqueInvestors).Error
	if err != nil {
		return nil, err
	}

	// Calculate average investment (avoid division by zero)
	if metrics.TotalInvestments > 0 {
		metrics.AverageInvestmentKobo = metrics.TotalAmountKobo / metrics.TotalInvestments
	}

	return &metrics, nil
}
