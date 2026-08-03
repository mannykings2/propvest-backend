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
//
// What IS here is everything a legitimate client needs to render
// the authenticated user's profile screen.
type UserResponse struct {
	ID            uuid.UUID `json:"id"`
	UserCode      string    `json:"user_code"`
	FullName      string    `json:"full_name"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	AvatarURL     *string   `json:"avatar_url,omitempty"`
	EmailVerified bool      `json:"email_verified"`
	KYCStatus     string    `json:"kyc_status"`
	Role          string    `json:"role"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpdateProfileRequest is used for PATCH /api/v1/users/me
// Allows updating the user's full name
type UpdateProfileRequest struct {
	FullName string `json:"full_name" binding:"required,min=2,max=100"`
}

// ChangePasswordRequest is used for PATCH /api/v1/users/password
// Requires current password for security (prevents account takeover if session stolen)
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=12,max=72"`
}

// RequestPhoneChangeRequest is used for POST /api/v1/users/phone/request
// User requests to change phone number - we send OTP to new number
type RequestPhoneChangeRequest struct {
	NewPhone string `json:"new_phone" binding:"required"`
}

// VerifyPhoneChangeRequest is used for POST /api/v1/users/phone/verify
// User submits OTP code to complete phone change
type VerifyPhoneChangeRequest struct {
	NewPhone string `json:"new_phone" binding:"required"`
	OTPCode  string `json:"otp_code" binding:"required,len=6"`
}

// PhoneChangeResponse is returned after requesting phone change
type PhoneChangeResponse struct {
	Message        string `json:"message"`
	ExpiresIn      int    `json:"expires_in"`       // Seconds until OTP expires
	CanResendAfter int    `json:"can_resend_after"` // Seconds to wait before requesting new OTP
}