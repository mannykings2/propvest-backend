package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// NotificationRepository is the data-access contract for notifications.
// (docs 5.5 - Notification System)
type NotificationRepository interface {
	// Create inserts a notification into the database.
	// The notification is the durable, in-app copy shown in the notification center.
	// Note: tx parameter is optional (pass nil for non-transactional create)
	Create(ctx context.Context, notif *models.Notification) error

	// FindByID retrieves a single notification by ID.
	// Returns gorm.ErrRecordNotFound if not found.
	FindByID(ctx context.Context, id uuid.UUID) (*models.Notification, error)

	// FindByIDAndUserID retrieves a notification ensuring it belongs to the user.
	// Critical for ownership enforcement (WHERE id = ? AND user_id = ?).
	FindByIDAndUserID(ctx context.Context, id, userID uuid.UUID) (*models.Notification, error)

	// ListByUser fetches a user's notifications with pagination and optional unread filter.
	// Orders by created_at DESC (newest first).
	// Returns notifications slice, total count, and error.
	ListByUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, int64, error)

	// CountUnread counts unread notifications for a user.
	// Used for the unread count badge in the notification center.
	CountUnread(ctx context.Context, userID uuid.UUID) (int64, error)

	// MarkRead marks a single notification as read.
	// Sets read=true and read_at=NOW() WHERE id = ? AND user_id = ?.
	// Enforces ownership to prevent users from marking others' notifications.
	MarkRead(ctx context.Context, id, userID uuid.UUID) error

	// MarkAllRead marks all unread notifications for a user as read.
	// Batch update: SET read=true, read_at=NOW() WHERE user_id = ? AND read = false.
	MarkAllRead(ctx context.Context, userID uuid.UUID) error

	// Delete removes a notification by ID.
	// Enforces ownership: WHERE id = ? AND user_id = ?.
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

type notificationRepository struct {
	*BaseRepository
}

// NewNotificationRepository constructs the repository.
func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *notificationRepository) Create(ctx context.Context, notif *models.Notification) error {
	return r.WithContext(ctx).Create(notif).Error
}

func (r *notificationRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Notification, error) {
	var notif models.Notification
	if err := r.WithContext(ctx).Where("id = ?", id).First(&notif).Error; err != nil {
		return nil, err
	}
	return &notif, nil
}

func (r *notificationRepository) FindByIDAndUserID(ctx context.Context, id, userID uuid.UUID) (*models.Notification, error) {
	var notif models.Notification
	if err := r.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&notif).Error; err != nil {
		return nil, err
	}
	return &notif, nil
}

func (r *notificationRepository) ListByUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, int64, error) {
	q := r.WithContext(ctx).Model(&models.Notification{}).Where("user_id = ?", userID)

	// Apply unread filter if requested
	if unreadOnly {
		q = q.Where("read = ?", false)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var notifs []models.Notification
	if err := q.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&notifs).Error; err != nil {
		return nil, 0, err
	}

	return notifs, total, nil
}

func (r *notificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", userID, false).
		Count(&count).Error
	return count, err
}

func (r *notificationRepository) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	// Enforces ownership: WHERE id = ? AND user_id = ?
	result := r.WithContext(ctx).
		Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{
			"read":    true,
			"read_at": gorm.Expr("NOW()"),
		})

	if result.Error != nil {
		return result.Error
	}

	// If no rows affected, notification doesn't exist or doesn't belong to user
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

func (r *notificationRepository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return r.WithContext(ctx).
		Model(&models.Notification{}).
		Where("user_id = ? AND read = ?", userID, false).
		Updates(map[string]interface{}{
			"read":    true,
			"read_at": gorm.Expr("NOW()"),
		}).Error
}

func (r *notificationRepository) Delete(ctx context.Context, id, userID uuid.UUID) error {
	// Enforces ownership: WHERE id = ? AND user_id = ?
	result := r.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.Notification{})

	if result.Error != nil {
		return result.Error
	}

	// If no rows affected, notification doesn't exist or doesn't belong to user
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}
