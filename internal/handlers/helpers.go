package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/errors"
)

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

	// Step 2: Type assertion to string (middleware stores it as string)
	// The auth middleware calls claims.UserID.String() before storing
	userIDStr, ok := userIDValue.(string)
	if !ok {
		return uuid.Nil, errors.ErrUnauthorized
	}

	// Step 3: Parse string to UUID
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, errors.ErrUnauthorized
	}

	return userID, nil
}
