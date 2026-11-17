package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// LoginHistoryRepository is the data access for sign-in evidence.
type LoginHistoryRepository interface {
	// Record writes one login row AND stamps users.last_login_at in the same
	// transaction — either both land or neither does, so the admin list's
	// fast column never disagrees with the trail behind it.
	Record(ctx context.Context, entry *models.LoginHistory) error

	// ListByUser returns the user's most recent sign-ins, newest first.
	// Used by GET /api/v1/me/login-history ("Recent sign-ins" on Profile).
	ListByUser(ctx context.Context, userID uuid.UUID, limit int) ([]*models.LoginHistory, error)
}

type loginHistoryRepository struct {
	*BaseRepository
}

// NewLoginHistoryRepository wires the repository (called from main.go).
func NewLoginHistoryRepository(db *gorm.DB) LoginHistoryRepository {
	return &loginHistoryRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *loginHistoryRepository) Record(ctx context.Context, entry *models.LoginHistory) error {
	return r.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).
			Where("id = ?", entry.UserID).
			UpdateColumn("last_login_at", entry.CreatedAt).Error
	})
}

func (r *loginHistoryRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit int) ([]*models.LoginHistory, error) {
	if limit < 1 || limit > 50 {
		limit = 10
	}
	var rows []*models.LoginHistory
	err := r.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
