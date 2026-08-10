package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// AuditLog is the append-only security/compliance trail (docs 1.2 §15, 4.2 §17,
// 6.2 §19). It answers "who did what, when, from where". Rows are written by the
// audit recorder and are NEVER updated or deleted.
//
// This is deliberately separate from application logs: audit records are
// business/security evidence, not debugging output.
type AuditLog struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// ActorID is the user who performed the action (nil for anonymous/system).
	ActorID *uuid.UUID `gorm:"type:uuid;index" json:"actor_id,omitempty"`
	Action  string     `gorm:"not null;index" json:"action"`

	TargetType string     `json:"target_type"`
	TargetID   *uuid.UUID `gorm:"type:uuid" json:"target_id,omitempty"`

	IPAddress string         `json:"ip_address"`
	UserAgent string         `json:"user_agent"`
	RequestID string         `json:"request_id"`
	Metadata  datatypes.JSON `gorm:"type:jsonb" json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}
