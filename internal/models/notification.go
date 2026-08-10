package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Notification is an in-app message shown in the user's notification centre
// (docs 1.2 §11, 5.5, 2.2 §16). Email/SMS delivery is handled asynchronously by
// the worker; this row is the durable, in-app copy and the source of the
// unread-count badge.
type Notification struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	// Type is a machine label the frontend can switch an icon on, e.g.
	// "deposit_success", "investment_created", "property_approved".
	Type  string `gorm:"not null" json:"type"`
	Title string `gorm:"not null" json:"title"`
	Body  string `gorm:"not null" json:"body"`

	// Read is the unread/read flag; ReadAt records when it flipped.
	Read   bool       `gorm:"default:false;not null;index" json:"read"`
	ReadAt *time.Time `json:"read_at,omitempty"`

	// Metadata carries structured context (ids, amounts) for deep-linking.
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Common notification type labels.
const (
	NotificationDepositSuccess    = "deposit_success"
	NotificationWithdrawalUpdate  = "withdrawal_update"
	NotificationInvestmentCreated = "investment_created"
	NotificationPropertyApproved  = "property_approved"
	NotificationWelcome           = "welcome"
)
