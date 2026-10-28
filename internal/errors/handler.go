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
//
//	If you later decide ErrUserNotFound should return 404 instead of 400,
//	you change it in one place instead of hunting through every handler.
//
// Usage in handlers:
//
//	statusCode := errors.HTTPStatusFromError(err)
//	c.JSON(statusCode, response.Error(err))
//
// businessStatusByCode maps the machine-readable codes carried by AppError to
// the HTTP status the API must answer with.
//
// Why a registry instead of a switch in the handler?
//
//	Services report business rejections as NewAppError(msg, code). Without
//	this table the handler can't match those against any sentinel, so every
//	one of them fell through to the default and surfaced as a 500. The table
//	is the single place that decides a code's status, and handler_test.go
//	pins every entry.
//
// Contract:
//   - 402  payment required  (the request is financially impossible)
//   - 403  caller may not do this
//   - 404  the named resource does not exist
//   - 409  state conflict (unavailable / already done)
//   - 422  the request is well-formed but violates a business rule
//   - 503  dependency is switched off or unreachable
//   - anything not listed here stays on the sentinel/default path.
var businessStatusByCode = map[string]int{
	"insufficient_funds":   http.StatusPaymentRequired,     // 402
	"forbidden":            http.StatusForbidden,           // 403
	"not_found":            http.StatusNotFound,            // 404
	"resource_unavailable": http.StatusConflict,            // 409
	"validation_error":     http.StatusUnprocessableEntity, // 422
	"business_logic_error": http.StatusUnprocessableEntity, // 422
	"invalid_target":       http.StatusUnprocessableEntity, // 422
	"invalid_status":       http.StatusUnprocessableEntity, // 422
	"invalid_plan":         http.StatusUnprocessableEntity, // 422
	"invalid_operation":    http.StatusUnprocessableEntity, // 422
	"invalid_finish_type":  http.StatusUnprocessableEntity, // 422
	"no_targets":           http.StatusUnprocessableEntity, // 422
	"no_electors":          http.StatusUnprocessableEntity, // 422
	"queue_disabled":       http.StatusServiceUnavailable,  // 503
}

// StatusCodeForCode returns the HTTP status for an AppError code and whether
// the code is known. Unknown codes return ok=false so callers keep their
// existing default instead of guessing.
func StatusCodeForCode(code string) (int, bool) {
	status, ok := businessStatusByCode[code]
	return status, ok
}

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
		// Business errors carry their own code; resolve it through the registry
		// so a code never degrades into a blanket 500.
	}

	// One source of truth for code -> status: resolve the machine-readable code
	// first (AppError.Code when present, otherwise the sentinel table in
	// ErrorCode) and look it up. This keeps "insufficient_funds" at 402 whether
	// the caller returned ErrInsufficientFunds or NewAppError(msg,
	// "insufficient_funds").
	if status, ok := businessStatusByCode[ErrorCode(err)]; ok {
		return status
	}

	// Check for specific application errors
	switch {
	// ───────────────────────────────────────────────────────────────────
	// 400 BAD REQUEST
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrInvalidEmail),
		errors.Is(err, ErrInvalidPhone),
		errors.Is(err, ErrInvalidUUID),
		errors.Is(err, ErrMissingField),
		errors.Is(err, ErrInvalidJSON),
		errors.Is(err, ErrInvalidAmount),
		errors.Is(err, ErrInvalidUserID),
		errors.Is(err, ErrInvalidPropertyID),
		errors.Is(err, ErrInvalidOTP),
		errors.Is(err, ErrOTPExpired),
		errors.Is(err, ErrOTPAlreadyUsed),
		errors.Is(err, ErrOTPNotFound),
		errors.Is(err, ErrOTPAlreadySent),
		errors.Is(err, ErrTokenInvalid),
		errors.Is(err, ErrTokenUsed),
		errors.Is(err, ErrNotInvestor):
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
		errors.Is(err, ErrKYCRequired),
		errors.Is(err, ErrSaleVoteNotEligible):
		return http.StatusForbidden

	// ───────────────────────────────────────────────────────────────────
	// 404 NOT FOUND
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrUserNotFound),
		errors.Is(err, ErrWalletNotFound),
		errors.Is(err, ErrPropertyNotFound),
		errors.Is(err, ErrInvestmentNotFound),
		errors.Is(err, ErrBankAccountNotFound),
		errors.Is(err, ErrTicketNotFound):
		return http.StatusNotFound

	// ───────────────────────────────────────────────────────────────────
	// 413 PAYLOAD TOO LARGE / 415 UNSUPPORTED MEDIA TYPE
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrImageTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrInvalidImageFormat):
		return http.StatusUnsupportedMediaType

	// ───────────────────────────────────────────────────────────────────
	// 409 CONFLICT
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrEmailTaken),
		errors.Is(err, ErrEmailAlreadyExists), // Alias
		errors.Is(err, ErrPhoneTaken),
		errors.Is(err, ErrUserExists),
		errors.Is(err, ErrDuplicateReference),
		errors.Is(err, ErrAlreadyInvested),
		errors.Is(err, ErrSamePassword),
		errors.Is(err, ErrSaleVoteAlreadyOpen),
		errors.Is(err, ErrAlreadyVoted),
		errors.Is(err, ErrTicketClosed):
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
		errors.Is(err, ErrInvalidBankAccount),
		errors.Is(err, ErrAccountNameMismatch),
		errors.Is(err, ErrWithdrawalPending),
		errors.Is(err, ErrRiskBlocked),
		errors.Is(err, ErrPropertyClosed),
		errors.Is(err, ErrPropertyNotApproved),
		errors.Is(err, ErrPropertyFullyFunded),
		errors.Is(err, ErrInvestmentTooSmall),
		errors.Is(err, ErrInvestmentTooLarge),
		errors.Is(err, ErrInvestmentExceeds),
		errors.Is(err, ErrPasswordWeak),
		errors.Is(err, ErrPasswordTooLong),
		errors.Is(err, ErrInvalidPhoneFormat),
		errors.Is(err, ErrInsufficientEarnings),
		errors.Is(err, ErrSaleVoteNotOpen),
		errors.Is(err, ErrInvalidBankCode),
		errors.Is(err, ErrPaymentVerificationFailed):
		return http.StatusUnprocessableEntity

	// ───────────────────────────────────────────────────────────────────
	// 429 TOO MANY REQUESTS
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrRateLimitExceeded),
		errors.Is(err, ErrTooManyRequests),
		errors.Is(err, ErrTooManyOTPAttempts),
		errors.Is(err, ErrTooManyOTPRequests):
		return http.StatusTooManyRequests

	// ───────────────────────────────────────────────────────────────────
	// 500 INTERNAL SERVER ERROR
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrDatabase),
		errors.Is(err, ErrCache),
		errors.Is(err, ErrStorage),
		errors.Is(err, ErrExternal),
		errors.Is(err, ErrInternal),
		errors.Is(err, ErrInternalServer),
		errors.Is(err, ErrWithdrawalFailed),
		errors.Is(err, ErrTransferFailed):
		return http.StatusInternalServerError

	// ───────────────────────────────────────────────────────────────────
	// 501 NOT IMPLEMENTED / 502 BAD GATEWAY (dependency failed, not us)
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrNotImplemented):
		return http.StatusNotImplemented
	case errors.Is(err, ErrImageUploadFailed),
		errors.Is(err, ErrInsufficientProviderBalance):
		return http.StatusBadGateway

	// ───────────────────────────────────────────────────────────────────
	// 503 SERVICE UNAVAILABLE
	// ───────────────────────────────────────────────────────────────────
	case errors.Is(err, ErrPaymentProviderUnavailable):
		return http.StatusServiceUnavailable

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
//
//	Database errors, stack traces, and internal details should never
//	reach the client. They could reveal:
//	- Database schema
//	- File paths
//	- Internal architecture
//	- Security vulnerabilities
//
// Usage:
//
//	message := errors.ClientMessage(err)
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
		errors.Is(err, ErrMinimumWithdrawal),
		errors.Is(err, ErrMaximumWithdrawal),
		errors.Is(err, ErrInvalidBankAccount),
		errors.Is(err, ErrAccountNameMismatch),
		errors.Is(err, ErrWithdrawalPending),
		errors.Is(err, ErrRiskBlocked),
		errors.Is(err, ErrWithdrawalFailed),
		errors.Is(err, ErrPaymentProviderUnavailable),
		errors.Is(err, ErrTransferFailed),
		errors.Is(err, ErrPropertyClosed),
		errors.Is(err, ErrKYCRequired),
		errors.Is(err, ErrWeakPassword),
		errors.Is(err, ErrPasswordTooLong),
		errors.Is(err, ErrInvalidPhoneFormat),
		errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrTokenExpired),
		errors.Is(err, ErrAccountSuspended),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrForbidden),
		errors.Is(err, ErrInvalidOTP),
		errors.Is(err, ErrOTPExpired),
		errors.Is(err, ErrOTPAlreadyUsed),
		errors.Is(err, ErrTooManyOTPAttempts),
		errors.Is(err, ErrOTPAlreadySent),
		errors.Is(err, ErrTooManyOTPRequests),
		errors.Is(err, ErrOTPNotFound),
		errors.Is(err, ErrTokenInvalid),
		errors.Is(err, ErrTokenUsed),
		errors.Is(err, ErrInsufficientEarnings),
		errors.Is(err, ErrSaleVoteNotEligible),
		errors.Is(err, ErrSaleVoteAlreadyOpen),
		errors.Is(err, ErrAlreadyVoted),
		errors.Is(err, ErrSaleVoteNotOpen),
		errors.Is(err, ErrNotInvestor),
		errors.Is(err, ErrTicketNotFound),
		errors.Is(err, ErrTicketClosed):
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
//
//	if (error.code === "insufficient_funds") {
//	    showFundWalletPrompt();
//	}
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
	case errors.Is(err, ErrPhoneTaken): // ErrPhoneAlreadyExists is this same value
		return "phone_taken"
	case errors.Is(err, ErrUserNotFound):
		return "user_not_found"
	case errors.Is(err, ErrInsufficientFunds):
		return "insufficient_funds"
	case errors.Is(err, ErrMinimumWithdrawal):
		return "minimum_withdrawal"
	case errors.Is(err, ErrMaximumWithdrawal):
		return "maximum_withdrawal"
	case errors.Is(err, ErrInvalidBankAccount):
		return "invalid_bank_account"
	case errors.Is(err, ErrAccountNameMismatch):
		return "account_name_mismatch"
	case errors.Is(err, ErrWithdrawalPending):
		return "withdrawal_pending"
	case errors.Is(err, ErrRiskBlocked):
		return "risk_blocked"
	case errors.Is(err, ErrWithdrawalFailed):
		return "withdrawal_failed"
	case errors.Is(err, ErrInsufficientEarnings):
		return "insufficient_earnings"
	case errors.Is(err, ErrSaleVoteNotEligible):
		return "sale_vote_not_eligible"
	case errors.Is(err, ErrSaleVoteAlreadyOpen):
		return "sale_vote_already_open"
	case errors.Is(err, ErrAlreadyVoted):
		return "already_voted"
	case errors.Is(err, ErrSaleVoteNotOpen):
		return "sale_vote_not_open"
	case errors.Is(err, ErrNotInvestor):
		return "not_investor"
	case errors.Is(err, ErrTicketNotFound):
		return "ticket_not_found"
	case errors.Is(err, ErrTicketClosed):
		return "ticket_closed"
	case errors.Is(err, ErrBankAccountNotFound):
		return "bank_account_not_found"
	case errors.Is(err, ErrInvalidBankCode):
		return "invalid_bank_code"
	case errors.Is(err, ErrImageTooLarge):
		return "image_too_large"
	case errors.Is(err, ErrInvalidImageFormat):
		return "invalid_image_format"
	case errors.Is(err, ErrImageUploadFailed):
		return "image_upload_failed"
	case errors.Is(err, ErrPaymentVerificationFailed):
		return "payment_verification_failed"
	case errors.Is(err, ErrInsufficientProviderBalance):
		return "insufficient_provider_balance"
	case errors.Is(err, ErrNotImplemented):
		return "not_implemented"
	case errors.Is(err, ErrTokenInvalid):
		return "invalid_token"
	case errors.Is(err, ErrTokenUsed):
		return "token_already_used"
	case errors.Is(err, ErrInvalidToken):
		return "invalid_token"
	case errors.Is(err, ErrTokenExpired):
		return "token_expired"
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrInsufficientRole):
		return "insufficient_role"
	case errors.Is(err, ErrAdminOnly):
		return "admin_only"
	case errors.Is(err, ErrAccountSuspended):
		return "account_suspended"
	case errors.Is(err, ErrEmailNotVerified):
		return "email_not_verified"
	case errors.Is(err, ErrValidation),
		errors.Is(err, ErrValidationFailed):
		return "validation_error"
	case errors.Is(err, ErrRateLimitExceeded):
		return "rate_limit_exceeded"
	case errors.Is(err, ErrTooManyRequests):
		return "too_many_requests"
	case errors.Is(err, ErrWalletNotFound):
		return "wallet_not_found"
	case errors.Is(err, ErrPropertyNotFound):
		return "property_not_found"
	case errors.Is(err, ErrInvestmentNotFound):
		return "investment_not_found"
	case errors.Is(err, ErrPaymentProviderUnavailable):
		return "provider_unavailable"
	case errors.Is(err, ErrTransferFailed):
		return "transfer_failed"
	case errors.Is(err, ErrPropertyClosed):
		return "property_closed"
	case errors.Is(err, ErrKYCRequired):
		return "kyc_required"
	case errors.Is(err, ErrInvalidOTP):
		return "invalid_otp"
	case errors.Is(err, ErrOTPExpired):
		return "otp_expired"
	case errors.Is(err, ErrOTPAlreadyUsed):
		return "otp_already_used"
	case errors.Is(err, ErrTooManyOTPAttempts):
		return "too_many_otp_attempts"
	case errors.Is(err, ErrOTPAlreadySent):
		return "otp_already_sent"
	case errors.Is(err, ErrTooManyOTPRequests):
		return "too_many_otp_requests"
	case errors.Is(err, ErrOTPNotFound):
		return "otp_not_found"
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "not_found"
	default:
		return "internal_error"
	}
}
