package queue_test

import (
	"errors"
	"fmt"

	"github.com/mannykings2/propvest-backend/internal/queue"
)

// Example_errorClassification demonstrates how consumers use error classification
// to determine retry vs DLQ routing.
func Example_errorClassification() {
	// Simulate processing a message
	err := processMessage()

	if err != nil {
		if queue.IsTransient(err) {
			fmt.Println("Transient error: will retry with exponential backoff")
			// NACK message with requeue=true
			// Apply backoff delay: 5s, 15s, 45s
		} else if queue.IsPermanent(err) {
			fmt.Println("Permanent error: routing to DLQ immediately")
			// ACK message (remove from queue)
			// Publish to DLQ with error metadata
		} else {
			fmt.Println("Unknown error: treating as transient for safety")
			// Conservative approach: retry unknown errors
		}
	}

	// Output:
	// Transient error: will retry with exponential backoff
}

func processMessage() error {
	// Simulate a database connection failure (transient)
	return queue.ErrDatabaseConnection
}

// Example_transientError demonstrates handling of transient errors.
func Example_transientError() {
	// Database connection lost during wallet credit
	err := creditWallet()

	if queue.IsTransient(err) {
		fmt.Println("Transient failure - will retry")
	}

	// Output:
	// Transient failure - will retry
}

func creditWallet() error {
	return queue.WrapTransient(queue.ErrDatabaseConnection, "wallet credit failed")
}

// Example_permanentError demonstrates handling of permanent errors.
func Example_permanentError() {
	// User not found - no point retrying
	err := lookupUser()

	if queue.IsPermanent(err) {
		fmt.Println("Permanent failure - routing to DLQ")
	}

	// Output:
	// Permanent failure - routing to DLQ
}

func lookupUser() error {
	return queue.WrapPermanent(queue.ErrUserNotFound, "deposit processing failed")
}

// Example_errorWrapping demonstrates how error wrapping preserves classification.
func Example_errorWrapping() {
	// Error wrapped multiple times
	original := queue.ErrProviderTimeout
	wrapped := fmt.Errorf("payment API call: %w", original)
	wrapped2 := queue.WrapTransient(wrapped, "deposit webhook processing")

	// Classification preserved through wrapping
	if queue.IsTransient(wrapped2) && errors.Is(wrapped2, original) {
		fmt.Println("Classification preserved through wrapping chain")
	}

	// Output:
	// Classification preserved through wrapping chain
}
