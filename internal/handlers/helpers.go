package handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// auditContext returns the request context stamped with HTTP metadata
// (request id, client IP, user agent) that services attach to the audit rows
// they write. Handlers that trigger a state change pass this instead of the
// bare c.Request.Context() so audit trails answer "from where, with what
// request" as well as "who did what".
func auditContext(c *gin.Context) context.Context {
	return services.WithRequestMetadata(c.Request.Context(), services.RequestMetadata{
		RequestID: c.GetString("request_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
}

// getUserIDFromContext extracts the authenticated user's ID from the Gin context.
//
// The auth middleware sets "user_id" in the context after validating the JWT.
// This helper retrieves it and handles missing/invalid cases gracefully.
//
// Returns:
//   - uuid.UUID: The user's ID if present and valid
//   - error: ErrUnauthorized if missing or invalid type
//
// Used by: All authenticated handler methods (GetWallet, InitiateDeposit, GetUser, etc.)
func getUserIDFromContext(c *gin.Context) (uuid.UUID, error) {
	// Step 1: Get user_id from context
	userIDValue, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, errors.ErrUnauthorized
	}

	// Step 2: The auth middleware stores the ID as a string (claims.UserID.String()).
	// Accept a uuid.UUID too so any other caller that sets the typed value can't
	// panic this handler — asserting on one concrete type is what made every
	// /admin/* route fail before.
	switch v := userIDValue.(type) {
	case string:
		userID, err := uuid.Parse(v)
		if err != nil {
			return uuid.Nil, errors.ErrUnauthorized
		}
		return userID, nil
	case uuid.UUID:
		if v == uuid.Nil {
			return uuid.Nil, errors.ErrUnauthorized
		}
		return v, nil
	default:
		return uuid.Nil, errors.ErrUnauthorized
	}
}
