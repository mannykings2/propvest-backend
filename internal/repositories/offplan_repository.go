package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// OffplanRepository is the data-access contract for off-plan projects and
// subscriptions.
type OffplanRepository interface {
	// ListProjects returns active, non-deleted projects (public read).
	ListProjects(ctx context.Context) ([]models.OffplanProject, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.OffplanProject, error)

	// CreateSubscription inserts a subscription row, participating in the
	// caller's transaction when tx is non-nil (so the wallet debit is atomic).
	CreateSubscription(ctx context.Context, s *models.OffplanSubscription, tx *gorm.DB) error

	// ListSubscriptions returns a user's subscriptions (newest first).
	ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]models.OffplanSubscription, error)
}

type offplanRepository struct {
	*BaseRepository
}

// NewOffplanRepository constructs the repository.
func NewOffplanRepository(db *gorm.DB) OffplanRepository {
	return &offplanRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *offplanRepository) ListProjects(ctx context.Context) ([]models.OffplanProject, error) {
	var projects []models.OffplanProject
	if err := r.WithContext(ctx).
		Where("status = ?", "active").
		Order("created_at ASC").
		Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *offplanRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.OffplanProject, error) {
	var p models.OffplanProject
	if err := r.WithContext(ctx).Where("id = ?", id).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *offplanRepository) CreateSubscription(ctx context.Context, s *models.OffplanSubscription, tx *gorm.DB) error {
	db := r.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}
	return db.Create(s).Error
}

func (r *offplanRepository) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]models.OffplanSubscription, error) {
	var subs []models.OffplanSubscription
	if err := r.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&subs).Error; err != nil {
		return nil, err
	}
	return subs, nil
}
