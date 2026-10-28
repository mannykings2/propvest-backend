package dto

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// NotificationResponse is the public representation of an in-app notification.
type NotificationResponse struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Read      bool            `json:"read"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// NotificationListResponse wraps a page of notifications with the unread count
// so the frontend can render the badge without a second request.
type NotificationListResponse struct {
	Notifications []NotificationResponse `json:"notifications"`
	UnreadCount   int64                  `json:"unread_count"`
}

// NotificationPreferenceResponse is the public representation of user's notification preferences.
// Each field represents a channel that can be enabled/disabled.
// (docs 5.5 - Notification System, User Preferences)
type NotificationPreferenceResponse struct {
	UserID           uuid.UUID `json:"user_id"`
	NotificationType *string   `json:"notification_type,omitempty"` // NULL = global default
	InAppEnabled     bool      `json:"in_app_enabled"`
	EmailEnabled     bool      `json:"email_enabled"`
	SMSEnabled       bool      `json:"sms_enabled"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// NotificationPreferencesListResponse wraps all preferences for a user.
type NotificationPreferencesListResponse struct {
	Preferences []NotificationPreferenceResponse `json:"preferences"`
}

// UpdateNotificationPreferenceRequest is the request payload for updating preferences.
// Uses pointers to distinguish "not provided" (nil) from "false" (explicit disable).
//
// Example payloads:
// 1. Update global email preference: {"email_enabled": false}
// 2. Update specific type: {"notification_type": "deposit_success", "sms_enabled": true}
// 3. Update multiple channels: {"in_app_enabled": true, "email_enabled": true, "sms_enabled": false}
type UpdateNotificationPreferenceRequest struct {
	NotificationType *string `json:"notification_type,omitempty"` // NULL = global default
	InAppEnabled     *bool   `json:"in_app_enabled,omitempty"`
	EmailEnabled     *bool   `json:"email_enabled,omitempty"`
	SMSEnabled       *bool   `json:"sms_enabled,omitempty"`
}

// Validate validates the update request.
func (r *UpdateNotificationPreferenceRequest) Validate() error {
	// At least one field must be provided
	if r.InAppEnabled == nil && r.EmailEnabled == nil && r.SMSEnabled == nil {
		return errors.New("at least one preference field must be provided")
	}
	
	// Notification type must not be empty string if provided
	if r.NotificationType != nil && *r.NotificationType == "" {
		return errors.New("notification_type must not be empty string")
	}
	
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN BULK MESSAGING DTOs
// ═══════════════════════════════════════════════════════════════════════════

// SendBulkMessageRequest is the admin request to send bulk messages to users.
// Supports targeting single user, segment of users, or all users.
// (Admin-only feature for broadcasting announcements, promotions, alerts)
type SendBulkMessageRequest struct {
	// Targeting options
	TargetType string   `json:"target_type" binding:"required,oneof=single segment all"` // single, segment, or all
	UserIDs    []string `json:"user_ids,omitempty"`                                      // For target_type=single (UUIDs)
	Filters    *UserFilters `json:"filters,omitempty"`                                   // For target_type=segment
	
	// Message content
	MessageType string `json:"message_type" binding:"required,oneof=email sms both"` // email, sms, or both
	Subject     string `json:"subject,omitempty"`                                     // For email (required if message_type=email or both)
	EmailBody   string `json:"email_body,omitempty"`                                  // HTML email body
	SMSMessage  string `json:"sms_message,omitempty"`                                 // Plain text SMS (required if message_type=sms or both)
	
	// Optional in-app notification
	CreateInApp bool   `json:"create_in_app"`           // Also create in-app notification
	NotifTitle  string `json:"notification_title,omitempty"` // Title for in-app notification
	NotifBody   string `json:"notification_body,omitempty"`  // Body for in-app notification
	NotifType   string `json:"notification_type,omitempty"`  // Type for in-app notification
	
	// Safety confirmation
	ConfirmAllUsers bool `json:"confirm_all_users"` // Required to be true when target_type=all
}

// UserFilters defines criteria for segment targeting.
// Filters are AND-ed together (all conditions must match).
type UserFilters struct {
	Role        string `json:"role,omitempty"`          // Filter by role (e.g., "user", "admin")
	KYCStatus   string `json:"kyc_status,omitempty"`    // Filter by KYC status
	MinBalance  *int64 `json:"min_balance,omitempty"`   // Minimum wallet balance (kobo)
	HasInvested *bool  `json:"has_invested,omitempty"`  // Has made at least one investment
}

// BulkMessageResponse is the response after queuing bulk messages.
type BulkMessageResponse struct {
	MessageID     string    `json:"message_id"`              // Unique message ID for tracking
	TargetType    string    `json:"target_type"`             // single, segment, or all
	MessageType   string    `json:"message_type"`            // email, sms, or both
	TargetedUsers int       `json:"targeted_users"`          // Number of users targeted
	QueuedEmails  int       `json:"queued_emails,omitempty"` // Emails queued
	QueuedSMS     int       `json:"queued_sms,omitempty"`    // SMS queued
	InAppCreated  int       `json:"in_app_created,omitempty"` // In-app notifications created
	CreatedAt     time.Time `json:"created_at"`
}

// Validate validates the bulk message request.
func (r *SendBulkMessageRequest) Validate() error {
	// Validate targeting
	switch r.TargetType {
	case "single":
		if len(r.UserIDs) == 0 {
			return errors.New("user_ids required for target_type=single")
		}
		// Validate UUIDs
		for _, id := range r.UserIDs {
			if _, err := uuid.Parse(id); err != nil {
				return errors.New("invalid user_id format: must be valid UUIDs")
			}
		}
	case "segment":
		if r.Filters == nil {
			return errors.New("filters required for target_type=segment")
		}
	case "all":
		if !r.ConfirmAllUsers {
			return errors.New("confirm_all_users must be true for target_type=all (safety check)")
		}
	}
	
	// Validate message content
	switch r.MessageType {
	case "email":
		if r.Subject == "" {
			return errors.New("subject required for message_type=email")
		}
		if r.EmailBody == "" {
			return errors.New("email_body required for message_type=email")
		}
	case "sms":
		if r.SMSMessage == "" {
			return errors.New("sms_message required for message_type=sms")
		}
		if len(r.SMSMessage) > 160 {
			return errors.New("sms_message must be 160 characters or less")
		}
	case "both":
		if r.Subject == "" || r.EmailBody == "" {
			return errors.New("subject and email_body required for message_type=both")
		}
		if r.SMSMessage == "" {
			return errors.New("sms_message required for message_type=both")
		}
		if len(r.SMSMessage) > 160 {
			return errors.New("sms_message must be 160 characters or less")
		}
	}
	
	// Validate in-app notification fields if enabled
	if r.CreateInApp {
		if r.NotifTitle == "" {
			return errors.New("notification_title required when create_in_app=true")
		}
		if r.NotifBody == "" {
			return errors.New("notification_body required when create_in_app=true")
		}
		if r.NotifType == "" {
			r.NotifType = "system_update" // Default type
		}
	}
	
	return nil
}
