package models

import (
	"time"

	"github.com/google/uuid"
)

// Support ticket lifecycle states. Stored as strings with a CHECK constraint
// in the migration, so the allowed set is pinned in both places.
const (
	SupportStatusOpen       = "open"
	SupportStatusInProgress = "in_progress"
	SupportStatusResolved   = "resolved"
	SupportStatusClosed     = "closed"
)

// Support reply author roles (also CHECK-constrained in the migration).
const (
	SupportAuthorUser  = "user"
	SupportAuthorAgent = "agent"
)

// ValidSupportStatuses is the ordered lifecycle used for validation.
func ValidSupportStatuses() []string {
	return []string{
		SupportStatusOpen,
		SupportStatusInProgress,
		SupportStatusResolved,
		SupportStatusClosed,
	}
}

// SupportTicket is one support request from a user (Phase 4.2).
//
// The ticket lives with its owner: every user-facing query is scoped by
// user_id so one user can never read another's conversation. Agents
// (staff/admin) address tickets by ticket ID through the admin service path.
type SupportTicket struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// UserID is the ticket owner. Enforced on every user-side read/reply.
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	// TicketNumber is the human-friendly reference (TKT-000123) allocated from
	// the support_ticket_seq Postgres sequence inside the creating transaction.
	TicketNumber string `gorm:"column:ticket_number;size:20;not null;uniqueIndex" json:"ticket_number"`

	Subject  string `gorm:"size:255;not null" json:"subject"`
	Category string `gorm:"size:100;not null" json:"category"`

	// Status: open|in_progress|resolved|closed (CHECK in migration).
	Status string `gorm:"size:20;not null;default:open" json:"status"`
	// Priority: low|medium|high (CHECK in migration).
	Priority string `gorm:"size:20;not null;default:medium" json:"priority"`

	// LastReplyAt tracks the newest reply (either side) for queue sorting.
	LastReplyAt *time.Time `json:"last_reply_at,omitempty"`
	// ClosedAt is set when Status becomes closed.
	ClosedAt *time.Time `json:"closed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// User is the ticket owner, loaded for the agent queue so agents know who
	// wrote in. json:"-" — the handler picks the fields it exposes (summary).
	User *User `gorm:"foreignKey:UserID" json:"-"`

	// Replies is the ordered thread; loaded explicitly (eager load) for
	// detail views, never for lists.
	Replies []SupportReply `gorm:"foreignKey:TicketID" json:"replies,omitempty"`

	// ReplyCount is filled by a correlated subquery in the list queries
	// (`->` marks it read-only so INSERT/UPDATE never touch it — the
	// column does not exist in the table).
	ReplyCount int `gorm:"->;column:reply_count" json:"-"`
}

// TableName sets the table name for GORM.
func (SupportTicket) TableName() string {
	return "support_tickets"
}

// SupportReply is one message in a ticket's conversation thread.
type SupportReply struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TicketID uuid.UUID `gorm:"type:uuid;not null;index" json:"ticket_id"`

	// AuthorID is the user (owner) or agent who wrote the message.
	AuthorID uuid.UUID `gorm:"type:uuid;not null" json:"author_id"`
	// AuthorRole: "user" | "agent" — drives which side the UI aligns the
	// bubble to without a join back to users.
	AuthorRole string `gorm:"size:20;not null" json:"author_role"`

	Message   string    `gorm:"not null" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName sets the table name for GORM.
func (SupportReply) TableName() string {
	return "support_replies"
}
