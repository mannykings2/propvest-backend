package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// NotificationRepository is the data-access contract for in-app notifications.
type NotificationRepository interface {
	Create(ctx context.Context, n *models.Notification) error
	ListByUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, int64, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int64, error)
	MarkRead(ctx context.Context, id, userID uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

type notificationRepository struct {
	*BaseRepository
}

// NewNotificationRepository constructs the repository.
func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *notificationRepository) Create(ctx context.Context, n *models.Notification) error {
	return r.WithContext(ctx).Create(n).Error
}

func (r *notificationRepository) ListByUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, int64, error) {
	q := r.WithContext(ctx).Model(&models.Notification{}).Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("read = ?", false)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var notifs []models.Notification
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&notifs).Error; err != nil {
		return nil, 0, err
	}
	return notifs, total, nil
}

func (r *notificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	var c int64
	err := r.WithContext(ctx).Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", userID, false).Count(&c).Error
	return c, err
}

// MarkRead flips a single notification to read, scoped to its owner so a user
// can only mark their OWN notifications (prevents IDOR).
func (r *notificationRepository) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	now := time.Now()
	return r.WithContext(ctx).
		Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"read": true, "read_at": now}).Error
}

func (r *notificationRepository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	return r.WithContext(ctx).
		Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", userID, false).
		Updates(map[string]any{"read": true, "read_at": now}).Error
}

func (r *notificationRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return r.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.Notification{}).Error
}
