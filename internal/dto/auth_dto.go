package dto

// ─────────────────────────────────────────────
// REGISTER
// ─────────────────────────────────────────────

// RegisterRequest is the shape of the JSON body the client sends
// to POST /api/v1/auth/register.
//
// Notice what is NOT here:
//   - No ID (the database generates it)
//   - No Role (users cannot self-assign roles)
//   - No KYCStatus (starts as "pending" by default)
//   - No IsActive (starts as true by default)
//   - No PasswordHash (we hash the Password field in the service)
//
// The binding tags are our first line of defence.
// They reject malformed requests before any business logic runs.
type RegisterRequest struct {
	FullName string `json:"full_name" binding:"required,min=2,max=100"  example:"Chukwuemeka Obi"`
	Email    string `json:"email"     binding:"required,email"           example:"chukwuemeka@example.com"`
	Password string `json:"password"  binding:"required,min=12,max=72"   example:"SecurePass123!"`
	// min=12,max=72 mirrors the real password policy enforced by
	// validators.ValidatePasswordComplexity in the service layer. Keeping
	// the binding tag in sync with the policy means the client gets a fast,
	// accurate 400 at bind time instead of a confusing service-layer error
	// (this was FIX-01: the tag previously said min=8 while the policy is 12).
	// max=72 is a bcrypt limit — bcrypt silently truncates beyond 72 bytes.
	Phone string `json:"phone" binding:"required,min=10,max=15" example:"+2348012345678"`
	// Phone is required at registration and must be in E.164 format (+2348012345678).
	// We validate Nigerian phone numbers specifically in the service layer.
}

// RegisterResponse is what the client receives after successful registration.
// We return tokens immediately so the user is logged in right after signing up —
// this is the modern UX pattern (no "please log in" after registering).
type RegisterResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"` // always "Bearer"
}

// ─────────────────────────────────────────────
// LOGIN
// ─────────────────────────────────────────────

// LoginRequest is the shape of the JSON body for POST /api/v1/auth/login.
// Intentionally minimal — email and password only.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"        example:"chukwuemeka@example.com"`
	Password string `json:"password" binding:"required"              example:"SecurePass123!"`
	// Login intentionally only requires that the password is present. We do
	// NOT re-apply the min/max complexity rules here: the password policy can
	// change over time, and enforcing length on login could lock out a user
	// whose (valid) stored password predates a policy change. bcrypt.Verify
	// is the real gate.
}

// LoginResponse is what the client receives after successful login.
// Identical shape to RegisterResponse — the frontend handles both the same way.
type LoginResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	TokenType    string       `json:"token_type"`
}

// ─────────────────────────────────────────────
// REFRESH TOKEN
// ─────────────────────────────────────────────

// RefreshRequest is the body for POST /api/v1/auth/refresh.
// The client sends only the refresh token — the access token is expired
// (that's why they're calling this endpoint).
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required" example:"eyJhbGci..."`
}

// RefreshResponse returns BOTH a new access token AND a new refresh token.
// This implements refresh token rotation for security:
//   - Every /refresh call invalidates the old refresh token
//   - Issues a new refresh token
//   - If someone steals a token, they can only use it once
//   - If a stolen token is used, the legitimate user's next refresh fails,
//     alerting them to the breach
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"` // NEW refresh token (rotation)
	TokenType    string `json:"token_type"`
}

// ─────────────────────────────────────────────
// TOKENS (internal helper, used by services)
// ─────────────────────────────────────────────

// TokenPair holds both tokens together.
// Services return this type; handlers embed it into the response DTOs above.
// Keeping it separate means we don't duplicate token-generation logic.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// ─────────────────────────────────────────────
// EMAIL VERIFICATION  (Milestone 1 completion)
// ─────────────────────────────────────────────

// VerifyEmailRequest carries the token from the verification email link.
type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

// ResendVerificationRequest asks for a fresh verification email.
type ResendVerificationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ─────────────────────────────────────────────
// PASSWORD RESET  (Milestone 1 completion)
// ─────────────────────────────────────────────

// ForgotPasswordRequest starts the reset flow. We always respond success even if
// the email is unknown (don't leak which emails are registered).
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest completes the reset using the emailed token.
type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=12,max=72"`
}

// MessageResponse is a simple {message} payload for flows that only confirm.
type MessageResponse struct {
	Message string `json:"message"`
}