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
	Password string `json:"password"  binding:"required,min=8,max=72"   example:"SecurePass123!"`
	// max=72 on password is intentional — bcrypt silently truncates input
	// beyond 72 bytes. Setting the max here makes that behaviour explicit
	// and prevents user confusion ("my 100-char password works but only
	// the first 72 chars are checked").
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
	Password string `json:"password" binding:"required,min=8,max=72" example:"SecurePass123!"`
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