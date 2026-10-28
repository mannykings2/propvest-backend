package errors

import "fmt"

// WithdrawalError creates contextual withdrawal errors with amount formatting.
// This provides better user experience by including the actual limits in error messages.
//
// Every helper below WRAPS its sentinel error (fmt.Errorf with %w) instead of
// returning a bare fmt.Errorf. A bare error carries no identity: the handler's
// status/code registry can't match it, so a below-minimum withdrawal answered
// 500 code=internal_error instead of 422 (P0-4). Wrapping keeps the formatted
// message while errors.Is() finds the sentinel.
//
// Example:
//   return NewMinimumWithdrawalError(50000)  // "Minimum withdrawal is ₦500.00"

// NewMinimumWithdrawalError creates an error with the minimum withdrawal amount.
func NewMinimumWithdrawalError(minKobo int64) error {
	naira := float64(minKobo) / 100
	return fmt.Errorf("%w: amount is below minimum withdrawal of ₦%.2f", ErrMinimumWithdrawal, naira)
}

// NewMaximumWithdrawalError creates an error with the maximum withdrawal amount.
func NewMaximumWithdrawalError(maxKobo int64) error {
	naira := float64(maxKobo) / 100
	return fmt.Errorf("%w: amount exceeds maximum withdrawal of ₦%.2f", ErrMaximumWithdrawal, naira)
}

// NewInsufficientFundsError creates an error showing the requested vs available amount.
func NewInsufficientFundsError(requestedKobo, availableKobo int64) error {
	requestedNaira := float64(requestedKobo) / 100
	availableNaira := float64(availableKobo) / 100
	return fmt.Errorf("%w: requested ₦%.2f but only ₦%.2f available",
		ErrInsufficientFunds, requestedNaira, availableNaira)
}

// NewAccountNameMismatchError creates an error showing the mismatch.
func NewAccountNameMismatchError(provided, bankRecords string) error {
	return fmt.Errorf("%w: you entered '%s' but bank records show '%s'",
		ErrAccountNameMismatch, provided, bankRecords)
}
