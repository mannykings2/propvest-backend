package models

import (
	"time"

	"github.com/google/uuid"
)

// LoginHistory is one successful sign-in: who, from where, with what client.
//
// Written by the auth service on every successful Login (best-effort — a
// failed insert is logged but never blocks a legitimate sign-in). It answers
// two questions the system previously could not:
//
//   - Admin: "when did this user last sign in?" (users.last_login_at is the
//     fast column; this table keeps the trail behind it)
//   - User: "have I signed in from somewhere I don't recognise?" — surfaced
//     on the Profile page as "Recent sign-ins"
//
// Rows are plain evidence, no PII beyond IP/UA, and follow the 90-day
// retention schedule in docs/compliance/DATA_RETENTION_SCHEDULE.md.
type LoginHistory struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `json:"user_id" gorm:"type:uuid;not null;index"`
	IPAddress *string   `json:"ip_address,omitempty" gorm:"column:ip_address"`
	UserAgent *string   `json:"user_agent,omitempty"`
	RequestID *string   `json:"request_id,omitempty" gorm:"column:request_id"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:now()"`
}

// TableName maps the model to the login_history table (GORM's pluralisation
// would guess "login_histories").
func (LoginHistory) TableName() string { return "login_history" }
