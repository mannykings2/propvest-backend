package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// InvestmentRepository is the data-access contract for investments.
type InvestmentRepository interface {
	// Create inserts an investment. Usually called inside a transaction via the
	// tx handle passed from the service (so it participates in the atomic wallet
	// debit + property funding update).
	Create(ctx context.Context, inv *models.Investment, tx *gorm.DB) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Investment, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Investment, int64, error)
	// PortfolioSummary aggregates a user's holdings: total invested and count.
	PortfolioSummary(ctx context.Context, userID uuid.UUID) (totalInvested int64, count int64, err error)
	CountAll(ctx context.Context) (int64, error)
	SumAll(ctx context.Context) (int64, error)
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
