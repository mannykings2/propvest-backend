package errors

import (
	"errors"
	"fmt"
)

// Application-level errors that represent business rule violations.
// These are sentinel errors — they can be compared directly with errors.Is().
//
// Why sentinel errors?
//   Instead of comparing error strings ("user not found" == "user not found"),
//   we compare error values (errors.Is(err, ErrUserNotFound)).
//   This is type-safe and survives error wrapping.
//
// Usage in services:
//   if user == nil {
//       return ErrUserNotFound
//   }
//
// Usage in handlers:
//   if errors.Is(err, errors.ErrUserNotFound) {
//       return c.JSON(404, ...)
//   }

var (
	// ───────────────────────────────────────────────────────────────────
	// AUTHENTICATION ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrEmailAlreadyExists   = errors.New("email already registered")
	ErrEmailTaken           = errors.New("email already registered") // Alias for backward compatibility
	ErrPhoneTaken           = errors.New("phone number already registered")
	ErrInvalidToken         = errors.New("invalid or expired token")
	ErrTokenExpired         = errors.New("token has expired")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrEmailNotVerified     = errors.New("email not verified")
	ErrAccountSuspended     = errors.New("account has been suspended")
	ErrWeakPassword         = errors.New("password does not meet complexity requirements: minimum 12 characters, at least 1 uppercase, 1 lowercase, 1 digit, and 1 special character")
	ErrPasswordTooLong      = errors.New("password exceeds maximum length of 72 characters")
	ErrInvalidPhoneFormat   = errors.New("phone number must be in E.164 format: +234XXXXXXXXXX")

	// ───────────────────────────────────────────────────────────────────
	// USER ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrUserNotFound   = errors.New("user not found")
	ErrUserExists     = errors.New("user already exists")
	ErrInvalidUserID  = errors.New("invalid user ID")
	ErrPasswordWeak   = errors.New("password does not meet security requirements")
	ErrPasswordWrong  = errors.New("current password is incorrect")
	ErrSamePassword   = errors.New("new password cannot be the same as current password")
	ErrPhoneAlreadyExists = errors.New("phone number already registered")
	ErrInvalidImageFormat = errors.New("invalid image format: only jpg, jpeg, png, gif, webp allowed")
	ErrImageTooLarge      = errors.New("image file too large: maximum 5MB for avatars, 10MB for properties")
	ErrImageUploadFailed  = errors.New("failed to upload image")

	// ───────────────────────────────────────────────────────────────────
	// OTP ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrInvalidOTP         = errors.New("invalid or expired OTP code")
	ErrOTPExpired         = errors.New("OTP code has expired")
	ErrOTPAlreadyUsed     = errors.New("OTP code has already been used")
	ErrTooManyOTPAttempts = errors.New("too many failed attempts: please request a new OTP")
	ErrOTPAlreadySent     = errors.New("OTP already sent: please wait before requesting another")
	ErrTooManyOTPRequests = errors.New("too many OTP requests: please try again later")
	ErrOTPNotFound        = errors.New("no active OTP found for this phone number")

	// ───────────────────────────────────────────────────────────────────
	// WALLET ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrInsufficientFunds   = errors.New("insufficient wallet balance")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrNegativeBalance     = errors.New("balance cannot be negative")
	ErrDuplicateReference  = errors.New("transaction with this reference already exists")
	ErrMinimumDeposit      = errors.New("amount is below minimum deposit")
	ErrMaximumDeposit      = errors.New("amount exceeds maximum deposit")
	ErrMinimumWithdrawal   = errors.New("amount is below minimum withdrawal")
	ErrMaximumWithdrawal   = errors.New("amount exceeds maximum withdrawal")

	// ───────────────────────────────────────────────────────────────────
	// PROPERTY ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrPropertyNotFound    = errors.New("property not found")
	ErrPropertyClosed      = errors.New("property is no longer accepting investments")
	ErrPropertyNotApproved = errors.New("property has not been approved")
	ErrPropertyFullyFunded = errors.New("property is fully funded")
	ErrInvalidPropertyID   = errors.New("invalid property ID")
	ErrUnauthorizedProperty = errors.New("you do not have permission to modify this property")

	// ───────────────────────────────────────────────────────────────────
	// INVESTMENT ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrInvestmentNotFound   = errors.New("investment not found")
	ErrInvestmentTooSmall   = errors.New("investment amount is below minimum")
	ErrInvestmentTooLarge   = errors.New("investment amount exceeds maximum")
	ErrInvestmentExceeds    = errors.New("investment would exceed property funding goal")
	ErrAlreadyInvested      = errors.New("you have already invested in this property")
	ErrKYCRequired          = errors.New("KYC verification required to invest")

	// ───────────────────────────────────────────────────────────────────
	// VALIDATION ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrValidation       = errors.New("validation failed")
	ErrValidationFailed = errors.New("validation failed") // Alias for consistency
	ErrInvalidEmail     = errors.New("invalid email format")
	ErrInvalidPhone     = errors.New("invalid phone number format")
	ErrInvalidUUID      = errors.New("invalid UUID format")
	ErrMissingField     = errors.New("required field is missing")
	ErrInvalidJSON      = errors.New("invalid JSON payload")

	// ───────────────────────────────────────────────────────────────────
	// PERMISSION ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrForbidden         = errors.New("forbidden")
	ErrInsufficientRole  = errors.New("insufficient role permissions")
	ErrAdminOnly         = errors.New("this action requires administrator privileges")
	ErrDeveloperOnly     = errors.New("this action requires developer role")

	// ───────────────────────────────────────────────────────────────────
	// RATE LIMITING ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrTooManyRequests   = errors.New("too many requests")

	// ───────────────────────────────────────────────────────────────────
	// INFRASTRUCTURE ERRORS
	// ───────────────────────────────────────────────────────────────────
	ErrDatabase       = errors.New("database error")
	ErrCache          = errors.New("cache error")
	ErrStorage        = errors.New("storage error")
	ErrExternal       = errors.New("external service error")
	ErrInternal       = errors.New("internal server error")
	ErrInternalServer = errors.New("internal server error") // Alias for consistency
	ErrNotImplemented = errors.New("feature not yet implemented")
)

// AppError wraps an error with additional context.
// This allows errors to carry more information as they bubble up.
//
// Example:
//   if err := db.Create(&user).Error; err != nil {
//       return WrapError(err, "failed to create user", "user_creation")
//   }
type AppError struct {
	Err      error  // The underlying error
	Message  string // Human-readable context
	Code     string // Machine-readable error code
	Internal bool   // Should this be logged but not exposed to client?
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Err.Error()
}

// Unwrap allows errors.Is() and errors.As() to work with wrapped errors.
func (e *AppError) Unwrap() error {
	return e.Err
}

// WrapError wraps an error with additional context.
func WrapError(err error, message string, code string) *AppError {
	if err == nil {
		return nil
	}
	return &AppError{
		Err:     err,
		Message: message,
		Code:    code,
	}
}

// NewAppError creates a new application error.
func NewAppError(message string, code string) *AppError {
	return &AppError{
		Err:     errors.New(message),
		Message: message,
		Code:    code,
	}
}

// NewInternalError creates an error that should not be exposed to clients.
// The error is logged for debugging but clients see a generic message.
func NewInternalError(err error, message string) *AppError {
	return &AppError{
		Err:      err,
		Message:  message,
		Code:     "internal_error",
		Internal: true,
	}
}
