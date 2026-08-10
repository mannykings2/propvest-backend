package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// VerificationTokenRepository persists single-use email-verification and
// password-reset tokens (only their hashes). See models.VerificationToken.
type VerificationTokenRepository interface {
	Create(ctx context.Context, t *models.VerificationToken) error
	FindByHash(ctx context.Context, tokenHash string) (*models.VerificationToken, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
	// InvalidateActive deletes any still-unused tokens for a user+purpose so that
	// issuing a new one invalidates previous ones (docs 4.2 §9/§10).
	InvalidateActive(ctx context.Context, userID uuid.UUID, purpose string) error
	DeleteExpired(ctx context.Context) error
}

type verificationTokenRepository struct {
	*BaseRepository
}

// NewVerificationTokenRepository constructs the repository.
func NewVerificationTokenRepository(db *gorm.DB) VerificationTokenRepository {
	return &verificationTokenRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *verificationTokenRepository) Create(ctx context.Context, t *models.VerificationToken) error {
	return r.WithContext(ctx).Create(t).Error
}

func (r *verificationTokenRepository) FindByHash(ctx context.Context, tokenHash string) (*models.VerificationToken, error) {
	var t models.VerificationToken
	if err := r.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *verificationTokenRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	return r.WithContext(ctx).
		Model(&models.VerificationToken{}).
		Where("id = ?", id).
		Update("used_at", now).Error
}

func (r *verificationTokenRepository) InvalidateActive(ctx context.Context, userID uuid.UUID, purpose string) error {
	// Hard-delete unused tokens of this purpose (they are ephemeral, no audit value).
	return r.WithContext(ctx).
		Where("user_id = ? AND purpose = ? AND used_at IS NULL", userID, purpose).
		Delete(&models.VerificationToken{}).Error
}

func (r *verificationTokenRepository) DeleteExpired(ctx context.Context) error {
	return r.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&models.VerificationToken{}).Error
}
