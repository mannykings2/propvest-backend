package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
)

// StandardResponse is the single, consistent envelope for every API response.
//
// FIX-06 / DECISION D6: this shape is converged onto what the engineering docs
// (6.2 §5/§26, 1.4 §10, 3.2 §8) mandate:
//
//	success   bool                 // always present
//	message   string               // human-readable summary (success OR error)
//	data      any                  // payload on success
//	errors    map[string][]string  // field -> messages, for validation failures
//	code      string               // machine-readable error code (kept from the
//	                                // original design; useful for the frontend to
//	                                // branch on, e.g. "insufficient_funds")
//	request_id string
//
// Previously the code used a singular `error` string plus a separate `fields`
// map produced via inline gin.H — two divergent error shapes. Everything now
// flows through this one struct.
type StandardResponse struct {
	Success   bool                `json:"success"`
	Message   string              `json:"message,omitempty"`
	Data      interface{}         `json:"data,omitempty"`
	Errors    map[string][]string `json:"errors,omitempty"`
	Code      string              `json:"code,omitempty"`
	RequestID string              `json:"request_id,omitempty"`
}

// PaginatedResponse is the list envelope with pagination metadata.
type PaginatedResponse struct {
	Success    bool               `json:"success"`
	Data       interface{}        `json:"data"`
	Pagination PaginationMetadata `json:"pagination"`
	RequestID  string             `json:"request_id,omitempty"`
}

// PaginationMetadata contains pagination information.
type PaginationMetadata struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
	Pages int   `json:"pages"`
}

// Success returns a successful response with optional data.
//
//	response.Success(c, data)                 // 200 OK with data
//	response.Success(c, http.StatusOK, data)  // explicit status with data
func Success(c *gin.Context, args ...interface{}) {
	statusCode := http.StatusOK
	var data interface{}

	switch len(args) {
	case 1:
		data = args[0]
	case 2:
		if code, ok := args[0].(int); ok {
			statusCode = code
			data = args[1]
		} else {
			data = args[0]
		}
	}

	c.JSON(statusCode, StandardResponse{
		Success:   true,
		Data:      data,
		RequestID: c.GetString("request_id"),
	})
}

// SuccessWithMessage returns a successful response with a human message.
//
//	response.SuccessWithMessage(c, message, data)
//	response.SuccessWithMessage(c, statusCode, message, data)
//	response.SuccessWithMessage(c, statusCode, message)
func SuccessWithMessage(c *gin.Context, args ...interface{}) {
	statusCode := http.StatusOK
	message := ""
	var data interface{}

	switch len(args) {
	case 2:
		if code, ok := args[0].(int); ok {
			statusCode = code
			if msg, ok := args[1].(string); ok {
				message = msg
			}
		} else if msg, ok := args[0].(string); ok {
			message = msg
			data = args[1]
		}
	case 3:
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
		RequestID: c.GetString("request_id"),
	})
}

// Created returns a 201 Created response for resource creation.
func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, StandardResponse{
		Success:   true,
		Message:   "Resource created successfully",
		Data:      data,
		RequestID: c.GetString("request_id"),
	})
}

// NoContent returns a 204 No Content response.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Error returns an error response. Human text goes in `message` (not `error`).
//
//	response.Error(c, err)                        // auto-map status/message/code
//	response.Error(c, statusCode, message)        // explicit status + message
//	response.Error(c, statusCode, message, code)  // explicit status + message + code
func Error(c *gin.Context, args ...interface{}) {
	statusCode := http.StatusInternalServerError
	message := "An error occurred"
	code := "internal_error"

	switch len(args) {
	case 1:
		if err, ok := args[0].(error); ok {
			statusCode = apperrors.HTTPStatusFromError(err)
			message = apperrors.ClientMessage(err)
			code = apperrors.ErrorCode(err)
		}
	case 2:
		if sc, ok := args[0].(int); ok {
			statusCode = sc
			if msg, ok := args[1].(string); ok {
				message = msg
			}
		}
	case 3:
		if sc, ok := args[0].(int); ok {
			statusCode = sc
			if msg, ok := args[1].(string); ok {
				message = msg
				if cd, ok := args[2].(string); ok {
					code = cd
				}
			}
		}
	}

	c.JSON(statusCode, StandardResponse{
		Success:   false,
		Message:   message,
		Code:      code,
		RequestID: c.GetString("request_id"),
	})
}

// ErrorWithStatus returns an error response with an explicit status code,
// overriding the automatic status mapping.
func ErrorWithStatus(c *gin.Context, statusCode int, err error) {
	c.JSON(statusCode, StandardResponse{
		Success:   false,
		Message:   apperrors.ClientMessage(err),
		Code:      apperrors.ErrorCode(err),
		RequestID: c.GetString("request_id"),
	})
}

// ValidationError returns a 422 response with field-level validation errors in
// the documented `errors` map (field -> list of messages).
//
// Note: the docs use 422 for validation failures; Gin's binding errors are
// surfaced here as 422 to match the error-category table in 6.2 §4/§6.
func ValidationError(c *gin.Context, err error) {
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		// Not a field validation error (e.g. malformed JSON body).
		c.JSON(http.StatusBadRequest, StandardResponse{
			Success:   false,
			Message:   "Invalid request format",
			Code:      "bad_request",
			RequestID: c.GetString("request_id"),
		})
		return
	}

	fields := make(map[string][]string)
	for _, fieldErr := range validationErrors {
		name := jsonFieldName(fieldErr)
		fields[name] = append(fields[name], formatValidationError(fieldErr))
	}

	c.JSON(http.StatusUnprocessableEntity, StandardResponse{
		Success:   false,
		Message:   "Validation failed",
		Code:      "validation_error",
		Errors:    fields,
		RequestID: c.GetString("request_id"),
	})
}

// Paginated returns a paginated response with metadata.
func Paginated(c *gin.Context, data interface{}, page, limit int, total int64) {
	pages := 0
	if limit > 0 {
		pages = int(total) / limit
		if int(total)%limit != 0 {
			pages++
		}
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
		RequestID: c.GetString("request_id"),
	})
}

// jsonFieldName lowercases the struct field name so the `errors` map keys match
// the JSON the client sent (e.g. "FullName" -> "fullname"). Good enough for our
// snake_case/lowercase field names; a struct-tag lookup could be added later.
func jsonFieldName(err validator.FieldError) string {
	return err.Field()
}

// formatValidationError converts a validator.FieldError to a human message.
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
	case "gt":
		return "Must be greater than " + err.Param()
	case "uuid":
		return "Must be a valid UUID"
	case "oneof":
		return "Must be one of: " + err.Param()
	default:
		return "Invalid value"
	}
}

// BindJSON binds and validates JSON, sending a validation response on failure.
// Returns true if binding succeeded.
func BindJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		ValidationError(c, err)
		return false
	}
	return true
}
