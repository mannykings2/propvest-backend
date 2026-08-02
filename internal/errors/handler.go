package errors

import (
	"errors"
	"net/http"

	"gorm.io/gorm"
)

// HTTPStatusFromError maps application errors to HTTP status codes.
// This centralizes the error-to-status mapping so handlers don't need to know it.
//
// Why centralize this?
//   If you later decide ErrUserNotFound should return 404 instead of 400,
//   you change it in one place instead of hunting through every handler.
//
// Usage in handlers:
//   statusCode := errors.HTTPStatusFromError(err)
//   c.JSON(statusCode, response.Error(err))
func HTTPStatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}

	// Check for GORM errors first
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound
	}

	// Check for AppError
	var appErr *AppError
	if errors.As(err, &appErr) {
		// Internal errors always return 500
		if appErr.Internal {
			return http.StatusInternalServerError
		}
	}

	// Check for specific application errors
	switch {
	// ───────────────────────────────────────────────────────────────────
	// 400 BAD REQUEST
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrValidation),
		errors.Is(err, ErrInvalidEmail),
		errors.Is(err, ErrInvalidPhone),
		errors.Is(err, ErrInvalidUUID),
		errors.Is(err, ErrMissingField),
		errors.Is(err, ErrInvalidJSON),
		errors.Is(err, ErrInvalidAmount),
		errors.Is(err, ErrInvalidUserID),
		errors.Is(err, ErrInvalidPropertyID):
		return http.StatusBadRequest

	// ───────────────────────────────────────────────────────────────────
	// 401 UNAUTHORIZED
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrInvalidCredentials),
		errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrTokenExpired),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrPasswordWrong):
		return http.StatusUnauthorized

	// ───────────────────────────────────────────────────────────────────
	// 403 FORBIDDEN
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrForbidden),
		errors.Is(err, ErrInsufficientRole),
		errors.Is(err, ErrAdminOnly),
		errors.Is(err, ErrDeveloperOnly),
		errors.Is(err, ErrEmailNotVerified),
		errors.Is(err, ErrAccountSuspended),
		errors.Is(err, ErrUnauthorizedProperty),
		errors.Is(err, ErrKYCRequired):
		return http.StatusForbidden

	// ───────────────────────────────────────────────────────────────────
	// 404 NOT FOUND
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrUserNotFound),
		errors.Is(err, ErrWalletNotFound),
		errors.Is(err, ErrPropertyNotFound),
		errors.Is(err, ErrInvestmentNotFound):
		return http.StatusNotFound

	// ───────────────────────────────────────────────────────────────────
	// 409 CONFLICT
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrEmailTaken),
		errors.Is(err, ErrEmailAlreadyExists), // Alias
		errors.Is(err, ErrPhoneTaken),
		errors.Is(err, ErrUserExists),
		errors.Is(err, ErrDuplicateReference),
		errors.Is(err, ErrAlreadyInvested),
		errors.Is(err, ErrSamePassword):
		return http.StatusConflict

	// ───────────────────────────────────────────────────────────────────
	// 422 UNPROCESSABLE ENTITY (Business Rule Violations)
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrInsufficientFunds),
		errors.Is(err, ErrNegativeBalance),
		errors.Is(err, ErrMinimumDeposit),
		errors.Is(err, ErrMaximumDeposit),
		errors.Is(err, ErrMinimumWithdrawal),
		errors.Is(err, ErrMaximumWithdrawal),
		errors.Is(err, ErrPropertyClosed),
		errors.Is(err, ErrPropertyNotApproved),
		errors.Is(err, ErrPropertyFullyFunded),
		errors.Is(err, ErrInvestmentTooSmall),
		errors.Is(err, ErrInvestmentTooLarge),
		errors.Is(err, ErrInvestmentExceeds),
		errors.Is(err, ErrPasswordWeak),
		errors.Is(err, ErrPasswordTooLong),
		errors.Is(err, ErrInvalidPhoneFormat):
		return http.StatusUnprocessableEntity

	// ───────────────────────────────────────────────────────────────────
	// 429 TOO MANY REQUESTS
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrRateLimitExceeded),
		errors.Is(err, ErrTooManyRequests):
		return http.StatusTooManyRequests

	// ───────────────────────────────────────────────────────────────────
	// 500 INTERNAL SERVER ERROR
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrDatabase),
		errors.Is(err, ErrCache),
		errors.Is(err, ErrStorage),
		errors.Is(err, ErrExternal),
		errors.Is(err, ErrInternal):
		return http.StatusInternalServerError

	// ───────────────────────────────────────────────────────────────────
	// DEFAULT: 500 INTERNAL SERVER ERROR
	// ───────────────────────────────────────────────────────────────────
	default:
		return http.StatusInternalServerError
	}
}

// ClientMessage returns a safe error message for the client.
// Internal errors are sanitized to prevent information leakage.
//
// Why sanitize errors?
//   Database errors, stack traces, and internal details should never
//   reach the client. They could reveal:
//   - Database schema
//   - File paths
//   - Internal architecture
//   - Security vulnerabilities
//
// Usage:
//   message := errors.ClientMessage(err)
func ClientMessage(err error) string {
	if err == nil {
		return ""
	}

	// Check for AppError
	var appErr *AppError
	if errors.As(err, &appErr) {
		// Internal errors get a generic message
		if appErr.Internal {
			return "An internal error occurred. Please try again later."
		}
		// Business errors get their actual message
		if appErr.Message != "" {
			return appErr.Message
		}
	}

	// For known application errors, return the error message
	// These are safe because they're predefined in errors.go
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "Resource not found"
	case errors.Is(err, ErrInvalidCredentials),
		errors.Is(err, ErrEmailTaken),
		errors.Is(err, ErrEmailAlreadyExists),
		errors.Is(err, ErrPhoneTaken),
		errors.Is(err, ErrUserNotFound),
		errors.Is(err, ErrInsufficientFunds),
		errors.Is(err, ErrPropertyClosed),
		errors.Is(err, ErrKYCRequired),
		errors.Is(err, ErrWeakPassword),
		errors.Is(err, ErrPasswordTooLong),
		errors.Is(err, ErrInvalidPhoneFormat),
		errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrTokenExpired),
		errors.Is(err, ErrAccountSuspended),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrForbidden):
		return err.Error()
	default:
		// Unknown errors get a generic message
		return "An error occurred. Please try again later."
	}
}

// ErrorCode returns a machine-readable error code.
// Clients can use this for programmatic error handling.
//
// Example client-side:
//   if (error.code === "insufficient_funds") {
//       showFundWalletPrompt();
//   }
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}

	// Check for AppError with explicit code
	var appErr *AppError
	if errors.As(err, &appErr) {
		if appErr.Code != "" {
			return appErr.Code
		}
	}

	// Map known errors to codes
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return "invalid_credentials"
	case errors.Is(err, ErrEmailTaken):
		return "email_taken"
	case errors.Is(err, ErrUserNotFound):
		return "user_not_found"
	case errors.Is(err, ErrInsufficientFunds):
		return "insufficient_funds"
	case errors.Is(err, ErrPropertyClosed):
		return "property_closed"
	case errors.Is(err, ErrKYCRequired):
		return "kyc_required"
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "not_found"
	default:
		return "internal_error"
	}
}
