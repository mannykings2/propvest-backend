package dto

import (
	"time"

	"github.com/google/uuid"
)

// ── Requests ────────────────────────────────────────────────────────────────

// CreateTicketRequest is the payload for POST /support/tickets.
// The category must be one of TicketCategories (validated in the service so
// the allowlist lives in one place, not in a binding tag the frontend can
// silently outgrow).
type CreateTicketRequest struct {
	Subject  string `json:"subject" binding:"required,min=5,max=255"`
	Category string `json:"category" binding:"required,min=2,max=100"`
	Message  string `json:"message" binding:"required,min=10,max=5000"`

	// Priority is optional; empty means "medium" (the column default).
	Priority string `json:"priority" binding:"omitempty,oneof=low medium high"`
}

// ReplyTicketRequest is the payload for POST /support/tickets/:id/replies.
type ReplyTicketRequest struct {
	Message string `json:"message" binding:"required,min=2,max=5000"`
}

// UpdateTicketStatusRequest is the payload for
// PATCH /admin/support/tickets/:id/status.
type UpdateTicketStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=open in_progress resolved closed"`
}

// ── Responses ───────────────────────────────────────────────────────────────

// TicketUserSummary identifies the ticket owner for the agent queue.
type TicketUserSummary struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
	Email    string    `json:"email"`
}

// TicketReplyResponse is one message in the thread.
type TicketReplyResponse struct {
	ID         uuid.UUID `json:"id"`
	AuthorID   uuid.UUID `json:"author_id"`
	AuthorRole string    `json:"author_role"` // "user" | "agent"
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

// TicketResponse is the list-row shape (no thread — lists stay cheap).
type TicketResponse struct {
	ID           uuid.UUID  `json:"id"`
	TicketNumber string     `json:"ticket_number"`
	Subject      string     `json:"subject"`
	Category     string     `json:"category"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	ReplyCount   int        `json:"reply_count"`
	LastReplyAt  *time.Time `json:"last_reply_at,omitempty"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// User is populated on admin/agent responses only.
	User *TicketUserSummary `json:"user,omitempty"`
}

// TicketDetailResponse is the ticket plus its ordered conversation thread.
type TicketDetailResponse struct {
	TicketResponse
	Replies []TicketReplyResponse `json:"replies"`
}

// TicketListResponse wraps a page of tickets with pagination metadata.
type TicketListResponse struct {
	Tickets    []TicketResponse   `json:"tickets"`
	Pagination PaginationMetadata `json:"pagination"`
}
