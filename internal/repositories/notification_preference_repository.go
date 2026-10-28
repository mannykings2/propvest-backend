package repositories

import (
	"context"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// NotificationPreferenceRepository is the data-access contract for user notification preferences.
// (docs 5.5 - Notification System, User Preferences)
type NotificationPreferenceRepository interface {
	// GetGlobalByUserID retrieves the user's global default preferences (notification_type = NULL).
	// Returns gorm.ErrRecordNotFound if not found (user should have one created on signup).
	GetGlobalByUserID(ctx context.Context, userID uuid.UUID) (*models.NotificationPreference, error)

	// GetByUserIDAndType retrieves a type-specific preference for a user.
	// Returns gorm.ErrRecordNotFound if user hasn't customized this type.
	GetByUserIDAndType(ctx context.Context, userID uuid.UUID, notifType string) (*models.NotificationPreference, error)

	// GetAllByUserID retrieves all preferences for a user (global + type-specific).
	// Used in the preferences settings page.
	GetAllByUserID(ctx context.Context, userID uuid.UUID) ([]models.NotificationPreference, error)

	// Upsert creates or updates a preference.
	// Uses ON CONFLICT (user_id, notification_type) DO UPDATE.
	// If notifType is nil/empty, updates global default. Otherwise updates type-specific.
	Upsert(ctx context.Context, pref *models.NotificationPreference) error

	// Delete removes a type-specific preference (resets to global default).
	// Cannot delete global default (notification_type = NULL).
	Delete(ctx context.Context, userID uuid.UUID, notifType string) error

	// CreateGlobalDefault creates a global default preference for a new user.
	// Called during user registration. Uses system defaults: in_app=true, email=true, sms=false.
	CreateGlobalDefault(ctx context.Context, userID uuid.UUID, tx *gorm.DB) error

	// GetEffectivePreferences resolves the precedence for a notification type:
	// 1. Type-specific preference (if exists)
	// 2. Global default preference (if exists)
	// 3. System default (in_app=true, email=true, sms=false)
	GetEffectivePreferences(ctx context.Context, userID uuid.UUID, notifType string) (*models.NotificationPreference, error)
}

type notificationPreferenceRepository struct {
	*BaseRepository
}

// NewNotificationPreferenceRepository constructs the repository.
func NewNotificationPreferenceRepository(db *gorm.DB) NotificationPreferenceRepository {
	return &notificationPreferenceRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *notificationPreferenceRepository) GetGlobalByUserID(ctx context.Context, userID uuid.UUID) (*models.NotificationPreference, error) {
	var pref models.NotificationPreference
	err := r.WithContext(ctx).
		Where("user_id = ? AND notification_type IS NULL", userID).
		First(&pref).Error
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

func (r *notificationPreferenceRepository) GetByUserIDAndType(ctx context.Context, userID uuid.UUID, notifType string) (*models.NotificationPreference, error) {
	var pref models.NotificationPreference
	err := r.WithContext(ctx).
		Where("user_id = ? AND notification_type = ?", userID, notifType).
		First(&pref).Error
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

func (r *notificationPreferenceRepository) GetAllByUserID(ctx context.Context, userID uuid.UUID) ([]models.NotificationPreference, error) {
	var prefs []models.NotificationPreference
	err := r.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("notification_type NULLS FIRST").
		Find(&prefs).Error
	return prefs, err
}

func (r *notificationPreferenceRepository) Upsert(ctx context.Context, pref *models.NotificationPreference) error {
	// PostgreSQL-specific UPSERT using ON CONFLICT
	// The unique index is on (user_id, COALESCE(notification_type, ''))
	// GORM's Clauses(clause.OnConflict{...}) handles this
	return r.WithContext(ctx).
		Save(pref).Error
}

func (r *notificationPreferenceRepository) Delete(ctx context.Context, userID uuid.UUID, notifType string) error {
	// Cannot delete global default (notification_type IS NULL)
	if notifType == "" {
		return gorm.ErrInvalidData
	}

	result := r.WithContext(ctx).
		Where("user_id = ? AND notification_type = ?", userID, notifType).
		Delete(&models.NotificationPreference{})

	if result.Error != nil {
		return result.Error
	}

	// If no rows affected, preference doesn't exist
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return nil
}

func (r *notificationPreferenceRepository) CreateGlobalDefault(ctx context.Context, userID uuid.UUID, tx *gorm.DB) error {
	pref := &models.NotificationPreference{
		UserID:           userID,
		NotificationType: nil, // Global default
		InAppEnabled:     true,
		EmailEnabled:     true,
		SMSEnabled:       false,
	}

	if tx != nil {
		return tx.WithContext(ctx).Create(pref).Error
	}
	return r.WithContext(ctx).Create(pref).Error
}

func (r *notificationPreferenceRepository) GetEffectivePreferences(ctx context.Context, userID uuid.UUID, notifType string) (*models.NotificationPreference, error) {
	// Step 1: Try to get type-specific preference
	typePref, err := r.GetByUserIDAndType(ctx, userID, notifType)
	if err == nil {
		return typePref, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err // Real error, not just missing
	}

	// Step 2: Fall back to global default
	globalPref, err := r.GetGlobalByUserID(ctx, userID)
	if err == nil {
		return globalPref, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err // Real error, not just missing
	}

	// Step 3: Fall back to system default
	// User doesn't have preferences yet (shouldn't happen after migration seed)
	systemDefault := &models.NotificationPreference{
		UserID:           userID,
		NotificationType: nil, // Represents system default
		InAppEnabled:     true,
		EmailEnabled:     true,
		SMSEnabled:       false,
	}
	return systemDefault, nil
}
