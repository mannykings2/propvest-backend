package dto

import (
	"time"

	"github.com/google/uuid"
)

// UserResponse is the safe, public representation of a user.
// This is what goes into every API response — never models.User directly.
//
// Compare this to models.User and notice what is deliberately absent:
//   - No PasswordHash (security: must never leave the server)
//   - No DeletedAt (internal implementation detail)
//   - No KYCScore (internal admin field)
//   - No IsActive (internal admin field)
//
// What IS here is everything a legitimate client needs to render
// the authenticated user's profile screen.
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	UserCode  string    `json:"user_code"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	KYCStatus string    `json:"kyc_status"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// UpdateProfileRequest is used for PATCH /api/v1/users/me (Phase 2+).
// We define it now as a placeholder so the dto package is complete.
// All fields are optional — the client sends only what they want to change.
// Using pointers means nil = "not provided, don't update this field".
type UpdateProfileRequest struct {
	FullName *string `json:"full_name" binding:"omitempty,min=2,max=100"`
	Phone    *string `json:"phone"     binding:"omitempty,min=10,max=15"`
}