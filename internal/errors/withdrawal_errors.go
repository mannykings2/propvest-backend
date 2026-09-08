package errors

import "fmt"

// WithdrawalError creates contextual withdrawal errors with amount formatting.
// This provides better user experience by including the actual limits in error messages.
//
// Example:
//   return NewMinimumWithdrawalError(50000)  // "Minimum withdrawal is ₦500.00"
//   return NewMaximumWithdrawalError(10000000) // "Maximum withdrawal is ₦100,000.00"

// NewMinimumWithdrawalError creates an error with the minimum withdrawal amount.
func NewMinimumWithdrawalError(minKobo int64) error {
	naira := float64(minKobo) / 100
	return fmt.Errorf("amount is below minimum withdrawal of ₦%.2f", naira)
}

// NewMaximumWithdrawalError creates an error with the maximum withdrawal amount.
func NewMaximumWithdrawalError(maxKobo int64) error {
	naira := float64(maxKobo) / 100
	return fmt.Errorf("amount exceeds maximum withdrawal of ₦%.2f", naira)
}

// NewInsufficientFundsError creates an error showing the requested vs available amount.
func NewInsufficientFundsError(requestedKobo, availableKobo int64) error {
	requestedNaira := float64(requestedKobo) / 100
	availableNaira := float64(availableKobo) / 100
	return fmt.Errorf("insufficient balance: requested ₦%.2f but only ₦%.2f available", 
		requestedNaira, availableNaira)
}

// NewAccountNameMismatchError creates an error showing the mismatch.
func NewAccountNameMismatchError(provided, bankRecords string) error {
	return fmt.Errorf("account name mismatch: you entered '%s' but bank records show '%s'", 
		provided, bankRecords)
}
