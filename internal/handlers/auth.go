package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// AuthHandler handles all authentication-related HTTP requests
//
// Responsibilities:
//   - Parse and validate HTTP request bodies
//   - Call appropriate service methods
//   - Transform service responses into HTTP responses
//   - Handle errors and return appropriate status codes
//
// This is the HTTP layer - it knows about Gin, status codes, and JSON,
// but knows nothing about business logic (that's in the service layer)
type AuthHandler struct {
	authService services.AuthService
}

// NewAuthHandler creates a new authentication handler
// Called once at startup and injected with AuthService
func NewAuthHandler(authService services.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Register handles POST /api/v1/auth/register
//
// Request body:
//   {
//     "full_name": "Chukwuemeka Obi",
//     "email": "chukwuemeka@example.com",
//     "password": "SecurePass123!",
//     "phone": "+2348012345678"
//   }
//
// Success response (201 Created):
//   {
//     "success": true,
//     "message": "Registration successful",
//     "data": {
//       "user": {...},
//       "access_token": "eyJhbGc...",
//       "refresh_token": "eyJhbGc...",
//       "token_type": "Bearer"
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Validation failed (missing fields, invalid format)
//   - 409 Conflict: Email already exists
//   - 422 Unprocessable Entity: Weak password, invalid phone format
//   - 500 Internal Server Error: Database error, unexpected failure
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/auth/register \
//     -H "Content-Type: application/json" \
//     -d '{"full_name":"John Doe","email":"john@example.com","password":"SecurePass123!","phone":"+2348012345678"}'
func (h *AuthHandler) Register(c *gin.Context) {
	// Step 1: Parse and validate request body
	// ShouldBindJSON does two things:
	//   1. Unmarshals JSON into the struct
	//   2. Validates using the `binding` tags (required, email, min, max)
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Validation failed (missing required field, invalid email format, etc.)
		// Gin's validator provides detailed error messages
		response.ValidationError(c, err)
		return
	}

	// Step 2: Call service layer
	// Context (c.Request.Context()) carries request-scoped values:
	//   - Request ID (from middleware)
	//   - Cancellation signal (if client disconnects)
	//   - Deadline (if timeout is configured)
	result, err := h.authService.Register(c.Request.Context(), req)
	if err != nil {
		// Service returned an error - map it to HTTP status code
		h.handleError(c, err)
		return
	}

	// Step 3: Return success response
	// 201 Created is the standard status for successful resource creation
	response.Success(c, http.StatusCreated, result)
}

// Login handles POST /api/v1/auth/login
//
// Request body:
//   {
//     "email": "chukwuemeka@example.com",
//     "password": "SecurePass123!"
//   }
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Login successful",
//     "data": {
//       "user": {...},
//       "access_token": "eyJhbGc...",
//       "refresh_token": "eyJhbGc...",
//       "token_type": "Bearer"
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Validation failed
//   - 401 Unauthorized: Invalid credentials
//   - 403 Forbidden: Account suspended
//   - 500 Internal Server Error: Unexpected failure
//
// Security notes:
//   - Error message is generic ("invalid email or password")
//   - Doesn't reveal whether email exists or password is wrong
//   - Rate limiting should be applied at middleware level
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/auth/login \
//     -H "Content-Type: application/json" \
//     -d '{"email":"john@example.com","password":"SecurePass123!"}'
func (h *AuthHandler) Login(c *gin.Context) {
	// Parse and validate request
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Call service
	result, err := h.authService.Login(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Return success
	response.SuccessWithMessage(c, http.StatusOK, "Login successful", result)
}
//
// Request body:
//   {
//     "refresh_token": "eyJhbGc..."
//   }
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Token refreshed successfully",
//     "data": {
//       "access_token": "eyJhbGc...",
//       "refresh_token": "eyJhbGc...",  // NEW token (rotation)
//       "token_type": "Bearer"
//     }
//   }
//
// Error responses:
//   - 400 Bad Request: Missing refresh token
//   - 401 Unauthorized: Invalid or expired token, token revoked
//   - 500 Internal Server Error: Unexpected failure
//
// Token rotation:
//   - Old refresh token is revoked immediately
//   - New refresh token must be used for next refresh
//   - This makes token theft detectable
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/auth/refresh \
//     -H "Content-Type: application/json" \
//     -d '{"refresh_token":"eyJhbGc..."}'
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// Parse request
	var req dto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Call service
	result, err := h.authService.RefreshAccessToken(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Return success
	response.Success(c, result)
}

// Logout handles POST /api/v1/auth/logout
//
// Request body:
//   {
//     "refresh_token": "eyJhbGc..."
//   }
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Logout successful",
//     "data": null
//   }
//
// Error responses:
//   - 400 Bad Request: Missing refresh token
//   - 500 Internal Server Error: Unexpected failure
//
// Notes:
//   - This revokes only the provided refresh token (single device logout)
//   - Access token remains valid until expiration (can't be revoked - it's stateless)
//   - Client should discard both tokens locally
//   - Logout is idempotent (safe to call multiple times)
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/auth/logout \
//     -H "Content-Type: application/json" \
//     -d '{"refresh_token":"eyJhbGc..."}'
func (h *AuthHandler) Logout(c *gin.Context) {
	// Parse request
	var req dto.RefreshRequest // Reuse RefreshRequest (same structure)
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Call service
	err := h.authService.Logout(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Return success
	response.SuccessWithMessage(c, http.StatusOK, "Logout successful")
}

// LogoutAll handles POST /api/v1/auth/logout-all
//
// Request headers:
//   Authorization: Bearer <access_token>
//
// Success response (200 OK):
//   {
//     "success": true,
//     "message": "Logged out from all devices",
//     "data": null
//   }
//
// Error responses:
//   - 401 Unauthorized: Missing or invalid access token
//   - 500 Internal Server Error: Unexpected failure
//
// Notes:
//   - Requires authentication (user must be logged in)
//   - Revokes ALL user's refresh tokens (all devices)
//   - Called when user clicks "Logout everywhere" button
//   - Also called automatically on password change (by service layer)
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/auth/logout-all \
//     -H "Authorization: Bearer eyJhbGc..."
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	// Get user ID from context (set by authentication middleware)
	// Middleware extracts user_id from JWT and stores it in context
	userID, exists := c.Get("user_id")
	if !exists {
		// This should never happen if middleware is working correctly
		// But we check defensively
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Type assertion: convert interface{} to string
	// Middleware stores user_id as string (UUID.String())
	userIDTyped, ok := userID.(string)
	if !ok {
		response.Error(c, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Parse string to UUID
	// uuid.Parse returns error if string is not valid UUID format
	parsedUserID, err := uuid.Parse(userIDTyped)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "internal_error")
		return
	}

	// Call service to revoke all tokens
	err = h.authService.LogoutAllDevices(c.Request.Context(), parsedUserID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Return success
	response.SuccessWithMessage(c, http.StatusOK, "Logged out from all devices")
}

// handleError maps service-layer errors to HTTP responses
//
// This uses the response.Error() helper which automatically:
//   1. Maps error to HTTP status code (via errors.HTTPStatusFromError)
//   2. Sanitizes error message for client (via errors.ClientMessage)
//   3. Generates error code (via errors.ErrorCode)
//
// The beauty of this approach:
//   - Handlers don't need to know status codes
//   - Error-to-status mapping is centralized
//   - Same error always gets same status code
//   - Internal errors are automatically sanitized
//
// Example flow:
//   Service returns: errors.ErrInvalidCredentials
//   HTTPStatusFromError(): 401
//   ClientMessage(): "Invalid email or password"
//   ErrorCode(): "invalid_credentials"
//   Response: {"success": false, "error": "Invalid email or password", "code": "invalid_credentials"}
func (h *AuthHandler) handleError(c *gin.Context, err error) {
	// The response.Error() function handles everything:
	//   - Determines appropriate HTTP status code
	//   - Formats error message for client
	//   - Includes error code for programmatic handling
	//   - Adds request_id for tracing
	response.Error(c, err)
}
