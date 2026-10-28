package models

import (
	"time"

	"github.com/google/uuid"
)

// NotificationPreference stores user's delivery channel preferences per notification type.
// (docs 5.5 - Notification System, User Preferences)
//
// Each user can have multiple preference rows:
// - One global default (notification_type = NULL) for unspecified types
// - Per-type overrides (notification_type = "deposit_success", etc.)
//
// Lookup precedence: type-specific preference > global default > system default
// System defaults: in_app=true, email=true, sms=false
type NotificationPreference struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	// NotificationType is the type this preference applies to (e.g., "deposit_success").
	// NULL means this is the user's global default for unspecified types.
	NotificationType *string `gorm:"type:varchar(100)" json:"notification_type,omitempty"`

	// Channel preferences
	InAppEnabled bool `gorm:"not null;default:true" json:"in_app_enabled"`
	EmailEnabled bool `gorm:"not null;default:true" json:"email_enabled"`
	SMSEnabled   bool `gorm:"not null;default:false" json:"sms_enabled"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name for GORM
func (NotificationPreference) TableName() string {
	return "notification_preferences"
}

// IsGlobalDefault returns true if this preference is the user's global default
func (np *NotificationPreference) IsGlobalDefault() bool {
	return np.NotificationType == nil
}

// IsTypeSpecific returns true if this preference is for a specific notification type
func (np *NotificationPreference) IsTypeSpecific() bool {
	return np.NotificationType != nil
}

// HasAnyChannelEnabled returns true if at least one delivery channel is enabled
func (np *NotificationPreference) HasAnyChannelEnabled() bool {
	return np.InAppEnabled || np.EmailEnabled || np.SMSEnabled
}

// GetNotificationType safely returns the notification type or empty string if nil
func (np *NotificationPreference) GetNotificationType() string {
	if np.NotificationType == nil {
		return ""
	}
	return *np.NotificationType
}
