package errors

import (
	"errors"
	"net/http"
	"testing"

	"gorm.io/gorm"
)

// TestErrorContract pins the error → HTTP status/code contract that the API
// documents. Every entry is what a client is allowed to rely on, so a change
// here is a breaking API change.
func TestErrorContract(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		// ── AppError codes resolved through the registry ────────────────
		{"app insufficient_funds", NewAppError("Insufficient wallet balance to cover the down payment", "insufficient_funds"), http.StatusPaymentRequired, "insufficient_funds"},
		{"app resource_unavailable", NewAppError("Property is not available for investment", "resource_unavailable"), http.StatusConflict, "resource_unavailable"},
		{"app validation_error", NewAppError("Minimum investment is ₦100,000.00", "validation_error"), http.StatusUnprocessableEntity, "validation_error"},
		{"app business_logic_error", NewAppError("cannot delete property with existing investments", "business_logic_error"), http.StatusUnprocessableEntity, "business_logic_error"},
		{"app not_found", NewAppError("Property not found", "not_found"), http.StatusNotFound, "not_found"},
		{"app forbidden", NewAppError("you do not have permission", "forbidden"), http.StatusForbidden, "forbidden"},
		{"app queue_disabled", NewAppError("queue is disabled", "queue_disabled"), http.StatusServiceUnavailable, "queue_disabled"},
		{"app invalid_operation", NewAppError("nothing to do", "invalid_operation"), http.StatusUnprocessableEntity, "invalid_operation"},

		// ── Sentinels ────────────────────────────────────────────────────
		{"sentinel insufficient_funds", ErrInsufficientFunds, http.StatusPaymentRequired, "insufficient_funds"},
		{"sentinel invalid credentials", ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials"},
		{"sentinel invalid token", ErrInvalidToken, http.StatusUnauthorized, "invalid_token"},
		{"sentinel user not found", ErrUserNotFound, http.StatusNotFound, "user_not_found"},
		{"sentinel already voted", ErrAlreadyVoted, http.StatusConflict, "already_voted"},
		{"sentinel forbidden", ErrForbidden, http.StatusForbidden, "forbidden"},
		{"sentinel kyc required", ErrKYCRequired, http.StatusForbidden, "kyc_required"},
		{"sentinel rate limit", ErrRateLimitExceeded, http.StatusTooManyRequests, "rate_limit_exceeded"},
		{"sentinel validation", ErrValidation, http.StatusUnprocessableEntity, "validation_error"},
		{"sentinel gorm not found", gorm.ErrRecordNotFound, http.StatusNotFound, "not_found"},

		// ── Sentinels that used to fall through to 500 ────────────────────
		{"sentinel duplicate phone", ErrPhoneAlreadyExists, http.StatusConflict, "phone_taken"},
		{"sentinel image too large", ErrImageTooLarge, http.StatusRequestEntityTooLarge, "image_too_large"},
		{"sentinel image format", ErrInvalidImageFormat, http.StatusUnsupportedMediaType, "invalid_image_format"},
		{"sentinel image upload failed", ErrImageUploadFailed, http.StatusBadGateway, "image_upload_failed"},
		{"sentinel payment verification", ErrPaymentVerificationFailed, http.StatusUnprocessableEntity, "payment_verification_failed"},
		{"sentinel provider balance", ErrInsufficientProviderBalance, http.StatusBadGateway, "insufficient_provider_balance"},
		{"sentinel bank account missing", ErrBankAccountNotFound, http.StatusNotFound, "bank_account_not_found"},
		{"sentinel not implemented", ErrNotImplemented, http.StatusNotImplemented, "not_implemented"},

		// ── Support tickets (Phase 4.2) ──────────────────────────────
		{"sentinel ticket not found", ErrTicketNotFound, http.StatusNotFound, "ticket_not_found"},
		{"sentinel ticket closed", ErrTicketClosed, http.StatusConflict, "ticket_closed"},

		// ── Internal / unknown must stay 5xx and be generic ─────────────
		{"internal server error", ErrInternalServer, http.StatusInternalServerError, "internal_error"},
		{"wrapped internal", WrapError(errors.New("connection reset"), "failed to create user", "user_creation"), http.StatusInternalServerError, "user_creation"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HTTPStatusFromError(tc.err); got != tc.wantStatus {
				t.Errorf("HTTPStatusFromError() = %d, want %d", got, tc.wantStatus)
			}
			if got := ErrorCode(tc.err); got != tc.wantCode {
				t.Errorf("ErrorCode() = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

// TestNoBusinessErrorIs500 enforces the production-readiness rule: a business
// rejection (one that carries a registry code) must never be reported as an
// internal server error. 503 is allowed — it means a dependency is off.
func TestNoBusinessErrorIs500(t *testing.T) {
	for code, status := range businessStatusByCode {
		if status == http.StatusInternalServerError {
			t.Errorf("business code %q maps to 500; business rejections must not be internal errors", code)
		}
	}
}

// TestInternalAppErrorBeatsRegistry: the Internal flag always wins, even if the
// code happens to sit in the business registry.
func TestInternalAppErrorBeatsRegistry(t *testing.T) {
	err := &AppError{Err: errors.New("boom"), Message: "boom", Code: "insufficient_funds", Internal: true}
	if got := HTTPStatusFromError(err); got != http.StatusInternalServerError {
		t.Errorf("HTTPStatusFromError() = %d, want 500", got)
	}
	if msg := ClientMessage(err); msg == "boom" {
		t.Errorf("ClientMessage leaked the internal error: %q", msg)
	}
}

// TestNilErrorIsOK keeps the success path honest.
func TestNilErrorIsOK(t *testing.T) {
	if got := HTTPStatusFromError(nil); got != http.StatusOK {
		t.Errorf("HTTPStatusFromError(nil) = %d, want 200", got)
	}
}
