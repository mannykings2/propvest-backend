package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PaymentRepository persists provider-side payment lifecycle records.
type PaymentRepository interface {
	Create(ctx context.Context, p *models.Payment) error
	FindByReference(ctx context.Context, reference string) (*models.Payment, error)
	// MarkSuccess/MarkFailed update a payment's terminal status and store the raw
	// provider payload for reconciliation.
	MarkSuccess(ctx context.Context, reference string, providerRef string, raw datatypes.JSON) error
	MarkFailed(ctx context.Context, reference string, raw datatypes.JSON) error
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Payment, int64, error)
}

type paymentRepository struct {
	*BaseRepository
}

// NewPaymentRepository constructs the repository.
func NewPaymentRepository(db *gorm.DB) PaymentRepository {
	return &paymentRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *paymentRepository) Create(ctx context.Context, p *models.Payment) error {
	return r.WithContext(ctx).Create(p).Error
}

func (r *paymentRepository) FindByReference(ctx context.Context, reference string) (*models.Payment, error) {
	var p models.Payment
	if err := r.WithContext(ctx).Where("reference = ?", reference).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *paymentRepository) MarkSuccess(ctx context.Context, reference string, providerRef string, raw datatypes.JSON) error {
	updates := map[string]any{
		"status":             models.PaymentStatusSuccess,
		"provider_reference": providerRef,
	}
	if raw != nil {
		updates["raw_payload"] = raw
	}
	return r.WithContext(ctx).Model(&models.Payment{}).Where("reference = ?", reference).Updates(updates).Error
}

func (r *paymentRepository) MarkFailed(ctx context.Context, reference string, raw datatypes.JSON) error {
	updates := map[string]any{"status": models.PaymentStatusFailed}
	if raw != nil {
		updates["raw_payload"] = raw
	}
	return r.WithContext(ctx).Model(&models.Payment{}).Where("reference = ?", reference).Updates(updates).Error
}

func (r *paymentRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.Payment, int64, error) {
	q := r.WithContext(ctx).Model(&models.Payment{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var payments []models.Payment
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&payments).Error; err != nil {
		return nil, 0, err
	}
	return payments, total, nil
}
