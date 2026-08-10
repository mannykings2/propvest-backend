package dto

import (
	"encoding/json"
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
