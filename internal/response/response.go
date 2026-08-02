package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
)

// StandardResponse is the consistent response format for all API endpoints.
// Every response follows this structure for predictability.
//
// Success response:
//   {
//     "success": true,
//     "message": "Operation completed successfully",
//     "data": { ... },
//     "request_id": "abc-123"
//   }
//
// Error response:
//   {
//     "success": false,
//     "error": "Insufficient wallet balance",
//     "code": "insufficient_funds",
//     "request_id": "abc-123"
//   }
type StandardResponse struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Code      string      `json:"code,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
}

// PaginatedResponse extends StandardResponse with pagination metadata.
// Used for any endpoint that returns a list of items.
//
// Example:
//   {
//     "success": true,
//     "data": [...],
//     "pagination": {
//       "page": 1,
//       "limit": 20,
//       "total": 156,
//       "pages": 8
//     }
//   }
type PaginatedResponse struct {
	Success    bool               `json:"success"`
	Data       interface{}        `json:"data"`
	Pagination PaginationMetadata `json:"pagination"`
	RequestID  string             `json:"request_id,omitempty"`
}

// PaginationMetadata contains pagination information.
type PaginationMetadata struct {
	Page  int   `json:"page"`  // Current page number (1-indexed)
	Limit int   `json:"limit"` // Items per page
	Total int64 `json:"total"` // Total number of items
	Pages int   `json:"pages"` // Total number of pages
}

// Success returns a successful response with optional data.
// Flexible signature - can be called multiple ways:
//   response.Success(c, data)                    // 200 OK with data
//   response.Success(c, http.StatusOK, data)     // Explicit status with data
func Success(c *gin.Context, args ...interface{}) {
	requestID := c.GetString("request_id")

	// Default values
	statusCode := http.StatusOK
	var data interface{}

	// Parse flexible arguments
	switch len(args) {
	case 1:
		// Success(c, data)
		data = args[0]
	case 2:
		// Success(c, statusCode, data)
		if code, ok := args[0].(int); ok {
			statusCode = code
			data = args[1]
		} else {
			// Fallback: treat both as data (shouldn't happen)
			data = args[0]
		}
	}

	c.JSON(statusCode, StandardResponse{
		Success:   true,
		Data:      data,
		RequestID: requestID,
	})
}

// SuccessWithMessage returns a successful response with a custom message.
// Flexible signature - can be called multiple ways:
//   response.SuccessWithMessage(c, message, data)                    // 200 OK with message + data
//   response.SuccessWithMessage(c, statusCode, message, data)        // Explicit status with message + data
//   response.SuccessWithMessage(c, statusCode, message)              // Explicit status with message only
func SuccessWithMessage(c *gin.Context, args ...interface{}) {
	requestID := c.GetString("request_id")

	// Default values
	statusCode := http.StatusOK
	message := ""
	var data interface{}

	// Parse flexible arguments
	switch len(args) {
	case 2:
		// SuccessWithMessage(c, message, data) OR (c, statusCode, message)
		// Try to detect if first arg is status code (int) or message (string)
		if code, ok := args[0].(int); ok {
			// SuccessWithMessage(c, statusCode, message)
			statusCode = code
			if msg, ok := args[1].(string); ok {
				message = msg
			}
		} else if msg, ok := args[0].(string); ok {
			// SuccessWithMessage(c, message, data)
			message = msg
			data = args[1]
		}
	case 3:
		// SuccessWithMessage(c, statusCode, message, data)
		if code, ok := args[0].(int); ok {
			statusCode = code
			if msg, ok := args[1].(string); ok {
				message = msg
				data = args[2]
			}
		}
	}

	c.JSON(statusCode, StandardResponse{
		Success:   true,
		Message:   message,
		Data:      data,
		RequestID: requestID,
	})
}

// Created returns a 201 Created response for resource creation.
//
// Usage:
//   response.Created(c, user)
func Created(c *gin.Context, data interface{}) {
	requestID := c.GetString("request_id")

	c.JSON(http.StatusCreated, StandardResponse{
		Success:   true,
		Message:   "Resource created successfully",
		Data:      data,
		RequestID: requestID,
	})
}

// NoContent returns a 204 No Content response.
// Used when an operation succeeds but has no response body.
//
// Usage:
//   response.NoContent(c)
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Error returns an error response.
// Flexible signature - can be called multiple ways:
//   response.Error(c, err)                               // Auto-map status from error type
//   response.Error(c, statusCode, message)               // Explicit status + message
//   response.Error(c, statusCode, message, code)         // Explicit status + message + code
func Error(c *gin.Context, args ...interface{}) {
	requestID := c.GetString("request_id")

	// Default values
	statusCode := http.StatusInternalServerError
	message := "An error occurred"
	code := "internal_error"

	// Parse flexible arguments
	switch len(args) {
	case 1:
		// Error(c, err) - Auto-map from error type
		if err, ok := args[0].(error); ok {
			statusCode = apperrors.HTTPStatusFromError(err)
			message = apperrors.ClientMessage(err)
			code = apperrors.ErrorCode(err)
		}
	case 2:
		// Error(c, statusCode, message)
		if sc, ok := args[0].(int); ok {
			statusCode = sc
			if msg, ok := args[1].(string); ok {
				message = msg
			}
		}
	case 3:
		// Error(c, statusCode, message, code)
		if sc, ok := args[0].(int); ok {
			statusCode = sc
			if msg, ok := args[1].(string); ok {
				message = msg
				if c, ok := args[2].(string); ok {
					code = c
				}
			}
		}
	}

	c.JSON(statusCode, StandardResponse{
		Success:   false,
		Error:     message,
		Code:      code,
		RequestID: requestID,
	})
}

// ErrorWithStatus returns an error response with explicit status code.
// Use this when you need to override the automatic status mapping.
//
// Usage:
//   response.ErrorWithStatus(c, http.StatusBadGateway, apperrors.ErrExternal)
func ErrorWithStatus(c *gin.Context, statusCode int, err error) {
	requestID := c.GetString("request_id")
	message := apperrors.ClientMessage(err)
	code := apperrors.ErrorCode(err)

	c.JSON(statusCode, StandardResponse{
		Success:   false,
		Error:     message,
		Code:      code,
		RequestID: requestID,
	})
}

// ValidationError returns a 400 response with field-level validation errors.
//
// This parses validator.ValidationErrors and returns them in a client-friendly format:
//   {
//     "success": false,
//     "error": "Validation failed",
//     "code": "validation_error",
//     "fields": {
//       "email": "must be a valid email address",
//       "password": "must be at least 8 characters"
//     }
//   }
//
// Usage:
//   if err := c.ShouldBindJSON(&req); err != nil {
//       response.ValidationError(c, err)
//       return
//   }
func ValidationError(c *gin.Context, err error) {
	requestID := c.GetString("request_id")

	// Check if it's a validator.ValidationErrors
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		// Not a validation error, treat as bad request
		c.JSON(http.StatusBadRequest, StandardResponse{
			Success:   false,
			Error:     "Invalid request format",
			Code:      "bad_request",
			RequestID: requestID,
		})
		return
	}

	// Convert validation errors to field map
	fields := make(map[string]string)
	for _, fieldErr := range validationErrors {
		fields[fieldErr.Field()] = formatValidationError(fieldErr)
	}

	c.JSON(http.StatusBadRequest, gin.H{
		"success":    false,
		"error":      "Validation failed",
		"code":       "validation_error",
		"fields":     fields,
		"request_id": requestID,
	})
}

// Paginated returns a paginated response with metadata.
//
// Usage:
//   response.Paginated(c, users, 1, 20, 156)
func Paginated(c *gin.Context, data interface{}, page, limit int, total int64) {
	requestID := c.GetString("request_id")

	// Calculate total pages
	pages := int(total) / limit
	if int(total)%limit != 0 {
		pages++
	}

	c.JSON(http.StatusOK, PaginatedResponse{
		Success: true,
		Data:    data,
		Pagination: PaginationMetadata{
			Page:  page,
			Limit: limit,
			Total: total,
			Pages: pages,
		},
		RequestID: requestID,
	})
}

// formatValidationError converts a validator.FieldError to a human-readable message.
func formatValidationError(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Must be a valid email address"
	case "min":
		return "Must be at least " + err.Param() + " characters"
	case "max":
		return "Must be at most " + err.Param() + " characters"
	case "gte":
		return "Must be greater than or equal to " + err.Param()
	case "lte":
		return "Must be less than or equal to " + err.Param()
	case "uuid":
		return "Must be a valid UUID"
	case "oneof":
		return "Must be one of: " + err.Param()
	default:
		return "Invalid value"
	}
}

// BindJSON is a helper that binds JSON and returns validation errors.
// This combines c.ShouldBindJSON() with automatic error response.
//
// Usage:
//   var req dto.RegisterRequest
//   if !response.BindJSON(c, &req) {
//       return
//   }
//   // req is now validated and ready to use
//
// Returns true if binding succeeded, false if it failed (and sent error response).
func BindJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		ValidationError(c, err)
		return false
	}
	return true
}
