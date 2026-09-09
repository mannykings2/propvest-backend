package queue

import (
	"errors"
	"fmt"

	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
)

// ═══════════════════════════════════════════════════════════════════════════
// TRANSIENT ERRORS (Retry with exponential backoff)
// ═══════════════════════════════════════════════════════════════════════════
//
// Transient errors represent temporary failures that may succeed on retry.
// Examples: database connection lost, API timeout, rate limit hit.
//
// When a consumer encounters a transient error:
//   1. Increment retry count
//   2. NACK message with requeue=true
//   3. Apply exponential backoff delay (5s, 15s, 45s)
//   4. If retry_count >= max_retries (default 3), route to DLQ
//
// Requirements: 4.2, 4.3

var (
	// ErrDatabaseConnection indicates database connection failure
	// Common causes: network partition, connection pool exhausted, database restart
	// Retry strategy: exponential backoff up to 3 attempts
	ErrDatabaseConnection = errors.New("database connection unavailable")

	// ErrDatabaseTimeout indicates database query timeout
	// Common causes: slow query, database under load, lock contention
	// Retry strategy: exponential backoff up to 3 attempts
	ErrDatabaseTimeout = errors.New("database operation timed out")

	// ErrProviderTimeout indicates payment provider API timeout
	// Common causes: network latency, provider under load, slow API response
	// Retry strategy: exponential backoff up to 3 attempts
	ErrProviderTimeout = errors.New("payment provider request timed out")

	// ErrProviderRateLimit indicates payment provider rate limit exceeded
	// Common causes: too many requests in short time window
	// Retry strategy: exponential backoff with longer delays
	ErrProviderRateLimit = errors.New("payment provider rate limit exceeded")
)

// ═══════════════════════════════════════════════════════════════════════════
// PERMANENT ERRORS (Route to DLQ immediately without retry)
// ═══════════════════════════════════════════════════════════════════════════
//
// Permanent errors represent failures that will never succeed on retry.
// Examples: invalid data, business rule violation, resource not found.
//
// When a consumer encounters a permanent error:
//   1. ACK original message (remove from queue)
//   2. Publish to DLQ with error metadata
//   3. Update event_store status to "failed"
//   4. Do NOT increment retry count (no retry will help)
//
// Requirements: 4.2, 4.3, 11.4, 11.5

var (
	// ErrInvalidEvent indicates message failed schema validation
	// Common causes: malformed JSON, missing required fields, invalid field types
	// Resolution: fix producer to emit valid schema, or update schema definition
	ErrInvalidEvent = errors.New("event failed schema validation")

	// ErrInvalidAmount indicates amount validation failure
	// Common causes: negative amount, zero amount, amount exceeds limits
	// Resolution: fix producer logic or investigate data corruption
	ErrInvalidAmount = errors.New("invalid amount: must be positive and within limits")

	// ErrUserNotFound indicates user does not exist
	// Common causes: user deleted after event published, invalid user_id in payload
	// Resolution: investigate user deletion flow or data consistency issues
	ErrUserNotFound = errors.New("user not found")

	// ErrInsufficientBalance indicates wallet has insufficient funds
	// Common causes: concurrent withdrawals, balance check race condition
	// Resolution: implement optimistic locking or SELECT FOR UPDATE
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
)

// ═══════════════════════════════════════════════════════════════════════════
// ERROR CLASSIFICATION HELPERS
// ═══════════════════════════════════════════════════════════════════════════
//
// IsTransient and IsPermanent determine retry behavior for queue consumers.
//
// Usage in consumer:
//   err := processMessage(msg)
//   if err != nil {
//       if IsTransient(err) {
//           return err // Requeue with backoff
//       } else {
//           routeToDLQ(msg, err) // Permanent failure
//           return nil
//       }
//   }
//
// Requirements: 4.2, 4.3

// IsTransient returns true if the error should trigger a retry.
// Transient errors are temporary failures that may succeed on retry.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	// Check queue-specific transient errors
	if errors.Is(err, ErrDatabaseConnection) ||
		errors.Is(err, ErrDatabaseTimeout) ||
		errors.Is(err, ErrProviderTimeout) ||
		errors.Is(err, ErrProviderRateLimit) {
		return true
	}

	// Check application-level transient errors
	// These are infrastructure errors from internal/errors package
	if errors.Is(err, apperrors.ErrDatabase) ||
		errors.Is(err, apperrors.ErrCache) ||
		errors.Is(err, apperrors.ErrExternal) ||
		errors.Is(err, apperrors.ErrPaymentProviderUnavailable) {
		return true
	}

	return false
}

// IsPermanent returns true if the error should route to DLQ immediately.
// Permanent errors will never succeed on retry and require manual investigation.
func IsPermanent(err error) bool {
	if err == nil {
		return false
	}

	// Check queue-specific permanent errors
	if errors.Is(err, ErrInvalidEvent) ||
		errors.Is(err, ErrInvalidAmount) ||
		errors.Is(err, ErrUserNotFound) ||
		errors.Is(err, ErrInsufficientBalance) {
		return true
	}

	// Check application-level permanent errors
	// These are business rule violations from internal/errors package
	if errors.Is(err, apperrors.ErrUserNotFound) ||
		errors.Is(err, apperrors.ErrInsufficientFunds) ||
		errors.Is(err, apperrors.ErrInvalidAmount) ||
		errors.Is(err, apperrors.ErrWalletNotFound) ||
		errors.Is(err, apperrors.ErrValidation) ||
		errors.Is(err, apperrors.ErrInvalidJSON) ||
		errors.Is(err, apperrors.ErrInvalidUUID) {
		return true
	}

	return false
}

// ═══════════════════════════════════════════════════════════════════════════
// ERROR WRAPPING UTILITIES
// ═══════════════════════════════════════════════════════════════════════════

// WrapTransient wraps an error as transient with additional context.
// This preserves the transient classification while adding debugging info.
func WrapTransient(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// WrapPermanent wraps an error as permanent with additional context.
// This preserves the permanent classification while adding debugging info.
func WrapPermanent(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}
