package queue

import (
	"errors"
	"fmt"
	"testing"

	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
)

// ═══════════════════════════════════════════════════════════════════════════
// TRANSIENT ERROR TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestIsTransient_QueueErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not transient",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrDatabaseConnection is transient",
			err:      ErrDatabaseConnection,
			expected: true,
		},
		{
			name:     "ErrDatabaseTimeout is transient",
			err:      ErrDatabaseTimeout,
			expected: true,
		},
		{
			name:     "ErrProviderTimeout is transient",
			err:      ErrProviderTimeout,
			expected: true,
		},
		{
			name:     "ErrProviderRateLimit is transient",
			err:      ErrProviderRateLimit,
			expected: true,
		},
		{
			name:     "wrapped ErrDatabaseConnection is transient",
			err:      fmt.Errorf("context: %w", ErrDatabaseConnection),
			expected: true,
		},
		{
			name:     "wrapped ErrProviderTimeout is transient",
			err:      WrapTransient(ErrProviderTimeout, "payment API call failed"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransient(tt.err)
			if result != tt.expected {
				t.Errorf("IsTransient(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsTransient_AppErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "apperrors.ErrDatabase is transient",
			err:      apperrors.ErrDatabase,
			expected: true,
		},
		{
			name:     "apperrors.ErrCache is transient",
			err:      apperrors.ErrCache,
			expected: true,
		},
		{
			name:     "apperrors.ErrExternal is transient",
			err:      apperrors.ErrExternal,
			expected: true,
		},
		{
			name:     "apperrors.ErrPaymentProviderUnavailable is transient",
			err:      apperrors.ErrPaymentProviderUnavailable,
			expected: true,
		},
		{
			name:     "wrapped apperrors.ErrDatabase is transient",
			err:      fmt.Errorf("wallet query failed: %w", apperrors.ErrDatabase),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTransient(tt.err)
			if result != tt.expected {
				t.Errorf("IsTransient(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// PERMANENT ERROR TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestIsPermanent_QueueErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not permanent",
			err:      nil,
			expected: false,
		},
		{
			name:     "ErrInvalidEvent is permanent",
			err:      ErrInvalidEvent,
			expected: true,
		},
		{
			name:     "ErrInvalidAmount is permanent",
			err:      ErrInvalidAmount,
			expected: true,
		},
		{
			name:     "ErrUserNotFound is permanent",
			err:      ErrUserNotFound,
			expected: true,
		},
		{
			name:     "ErrInsufficientBalance is permanent",
			err:      ErrInsufficientBalance,
			expected: true,
		},
		{
			name:     "wrapped ErrInvalidEvent is permanent",
			err:      fmt.Errorf("context: %w", ErrInvalidEvent),
			expected: true,
		},
		{
			name:     "wrapped ErrUserNotFound is permanent",
			err:      WrapPermanent(ErrUserNotFound, "deposit processing failed"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPermanent(tt.err)
			if result != tt.expected {
				t.Errorf("IsPermanent(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsPermanent_AppErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "apperrors.ErrUserNotFound is permanent",
			err:      apperrors.ErrUserNotFound,
			expected: true,
		},
		{
			name:     "apperrors.ErrInsufficientFunds is permanent",
			err:      apperrors.ErrInsufficientFunds,
			expected: true,
		},
		{
			name:     "apperrors.ErrInvalidAmount is permanent",
			err:      apperrors.ErrInvalidAmount,
			expected: true,
		},
		{
			name:     "apperrors.ErrWalletNotFound is permanent",
			err:      apperrors.ErrWalletNotFound,
			expected: true,
		},
		{
			name:     "apperrors.ErrValidation is permanent",
			err:      apperrors.ErrValidation,
			expected: true,
		},
		{
			name:     "apperrors.ErrInvalidJSON is permanent",
			err:      apperrors.ErrInvalidJSON,
			expected: true,
		},
		{
			name:     "apperrors.ErrInvalidUUID is permanent",
			err:      apperrors.ErrInvalidUUID,
			expected: true,
		},
		{
			name:     "wrapped apperrors.ErrUserNotFound is permanent",
			err:      fmt.Errorf("user lookup failed: %w", apperrors.ErrUserNotFound),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPermanent(tt.err)
			if result != tt.expected {
				t.Errorf("IsPermanent(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// MUTUAL EXCLUSIVITY TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestErrorClassification_MutuallyExclusive(t *testing.T) {
	// Transient errors should not be permanent
	transientErrors := []error{
		ErrDatabaseConnection,
		ErrDatabaseTimeout,
		ErrProviderTimeout,
		ErrProviderRateLimit,
	}

	for _, err := range transientErrors {
		if IsPermanent(err) {
			t.Errorf("transient error %v incorrectly classified as permanent", err)
		}
	}

	// Permanent errors should not be transient
	permanentErrors := []error{
		ErrInvalidEvent,
		ErrInvalidAmount,
		ErrUserNotFound,
		ErrInsufficientBalance,
	}

	for _, err := range permanentErrors {
		if IsTransient(err) {
			t.Errorf("permanent error %v incorrectly classified as transient", err)
		}
	}
}

func TestErrorClassification_UnknownErrors(t *testing.T) {
	// Unknown errors should be neither transient nor permanent
	unknownErr := errors.New("some unknown error")

	if IsTransient(unknownErr) {
		t.Error("unknown error incorrectly classified as transient")
	}

	if IsPermanent(unknownErr) {
		t.Error("unknown error incorrectly classified as permanent")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// ERROR WRAPPING TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestWrapTransient(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		context string
		wantNil bool
	}{
		{
			name:    "wrapping nil returns nil",
			err:     nil,
			context: "some context",
			wantNil: true,
		},
		{
			name:    "wrapping transient error preserves classification",
			err:     ErrDatabaseConnection,
			context: "wallet credit failed",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapTransient(tt.err, tt.context)

			if tt.wantNil {
				if wrapped != nil {
					t.Errorf("WrapTransient(nil) should return nil, got %v", wrapped)
				}
				return
			}

			if wrapped == nil {
				t.Error("WrapTransient should not return nil for non-nil error")
				return
			}

			// Verify original error is wrapped
			if !errors.Is(wrapped, tt.err) {
				t.Errorf("wrapped error does not contain original error: %v", wrapped)
			}

			// Verify transient classification is preserved
			if !IsTransient(wrapped) {
				t.Error("wrapped error lost transient classification")
			}
		})
	}
}

func TestWrapPermanent(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		context string
		wantNil bool
	}{
		{
			name:    "wrapping nil returns nil",
			err:     nil,
			context: "some context",
			wantNil: true,
		},
		{
			name:    "wrapping permanent error preserves classification",
			err:     ErrUserNotFound,
			context: "deposit processing failed",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := WrapPermanent(tt.err, tt.context)

			if tt.wantNil {
				if wrapped != nil {
					t.Errorf("WrapPermanent(nil) should return nil, got %v", wrapped)
				}
				return
			}

			if wrapped == nil {
				t.Error("WrapPermanent should not return nil for non-nil error")
				return
			}

			// Verify original error is wrapped
			if !errors.Is(wrapped, tt.err) {
				t.Errorf("wrapped error does not contain original error: %v", wrapped)
			}

			// Verify permanent classification is preserved
			if !IsPermanent(wrapped) {
				t.Error("wrapped error lost permanent classification")
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// EDGE CASE TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestErrorClassification_ChainedWrapping(t *testing.T) {
	// Test deeply nested error wrapping
	original := ErrDatabaseConnection
	wrapped1 := fmt.Errorf("layer 1: %w", original)
	wrapped2 := fmt.Errorf("layer 2: %w", wrapped1)
	wrapped3 := WrapTransient(wrapped2, "layer 3")

	if !IsTransient(wrapped3) {
		t.Error("deeply wrapped transient error lost classification")
	}

	if !errors.Is(wrapped3, original) {
		t.Error("deeply wrapped error lost connection to original")
	}
}

func TestErrorClassification_MultipleErrors(t *testing.T) {
	// Simulate a consumer decision flow
	testCases := []struct {
		err           error
		shouldRetry   bool
		shouldRouteDLQ bool
	}{
		{ErrDatabaseConnection, true, false},
		{ErrProviderTimeout, true, false},
		{ErrInvalidEvent, false, true},
		{ErrUserNotFound, false, true},
		{errors.New("unknown"), false, false},
	}

	for _, tc := range testCases {
		isTransient := IsTransient(tc.err)
		isPermanent := IsPermanent(tc.err)

		if isTransient != tc.shouldRetry {
			t.Errorf("error %v: IsTransient=%v, expected %v", tc.err, isTransient, tc.shouldRetry)
		}

		if isPermanent != tc.shouldRouteDLQ {
			t.Errorf("error %v: IsPermanent=%v, expected %v", tc.err, isPermanent, tc.shouldRouteDLQ)
		}
	}
}
