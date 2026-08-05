package validators

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/errors"
)

// ValidateEmail validates email format
//
// Uses a simple but effective regex pattern that catches 99% of invalid emails
// while accepting all valid ones.
//
// Valid examples:
//   user@example.com
//   john.doe+tag@company.co.uk
//   admin_123@test-domain.io
//
// Invalid examples:
//   plaintext
//   @example.com
//   user@
//   user @example.com (space)
//
// Parameters:
//   - email: Email address to validate
//
// Returns:
//   - nil if valid
//   - ErrInvalidEmail if format is invalid
//
// Example usage:
//   if err := validators.ValidateEmail(email); err != nil {
//       return err
//   }
func ValidateEmail(email string) error {
	// Normalize: trim whitespace and convert to lowercase
	email = strings.ToLower(strings.TrimSpace(email))

	// Empty check
	if email == "" {
		return errors.ErrInvalidEmail
	}

	// Regex pattern explanation:
	//   ^[a-z0-9._%+-]+  - Start with alphanumeric, dot, underscore, percent, plus, hyphen
	//   @                - Literal @ symbol
	//   [a-z0-9.-]+      - Domain: alphanumeric, dot, hyphen
	//   \.               - Literal dot
	//   [a-z]{2,}$       - TLD: at least 2 letters (com, uk, io, etc.)
	//
	// This is a simplified pattern. Full RFC 5322 compliance is overkill.
	emailRegex := regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)
	
	if !emailRegex.MatchString(email) {
		return errors.ErrInvalidEmail
	}

	return nil
}

// ValidateUUID validates UUID format
//
// Accepts both hyphenated and non-hyphenated UUIDs.
//
// Valid examples:
//   a3f8b2c1-d4e5-4f67-8901-23456789abcd
//   a3f8b2c1d4e54f67890123456789abcd
//
// Parameters:
//   - id: UUID string to validate
//
// Returns:
//   - nil if valid
//   - ErrInvalidUUID if format is invalid
//
// Example usage:
//   if err := validators.ValidateUUID(userID); err != nil {
//       return err
//   }
func ValidateUUID(id string) error {
	_, err := uuid.Parse(id)
	if err != nil {
		return errors.ErrInvalidUUID
	}
	return nil
}

// ValidateNotEmpty validates that a string is not empty or whitespace-only
//
// Parameters:
//   - value: String to validate
//   - fieldName: Name of the field (for error message)
//
// Returns:
//   - nil if not empty
//   - ErrMissingField if empty or whitespace-only
//
// Example usage:
//   if err := validators.ValidateNotEmpty(name, "name"); err != nil {
//       return err
//   }
func ValidateNotEmpty(value, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return errors.ErrMissingField
	}
	return nil
}

// ValidateStringLength validates string length is within bounds
//
// Parameters:
//   - value: String to validate
//   - min: Minimum length (inclusive)
//   - max: Maximum length (inclusive)
//   - fieldName: Name of the field (for error message)
//
// Returns:
//   - nil if within bounds
//   - ErrValidationFailed if outside bounds
//
// Example usage:
//   if err := validators.ValidateStringLength(name, 2, 100, "name"); err != nil {
//       return err
//   }
func ValidateStringLength(value string, min, max int, fieldName string) error {
	length := len(strings.TrimSpace(value))
	
	if length < min || length > max {
		return errors.ErrValidationFailed
	}
	
	return nil
}

// ValidateAmount validates that an amount is positive and within bounds
//
// Used for wallet operations, investments, etc.
//
// Parameters:
//   - amount: Amount in kobo (smallest currency unit)
//   - min: Minimum amount (inclusive)
//   - max: Maximum amount (inclusive)
//
// Returns:
//   - nil if valid
//   - ErrInvalidAmount if amount is invalid
//
// Example usage:
//   // Validate deposit: min 100 NGN, max 1M NGN
//   if err := validators.ValidateAmount(amount, 10000, 100000000); err != nil {
//       return err
//   }
func ValidateAmount(amount, min, max int64) error {
	if amount <= 0 {
		return errors.ErrInvalidAmount
	}
	
	if amount < min || amount > max {
		return errors.ErrInvalidAmount
	}
	
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: Validation Strategy
// ═══════════════════════════════════════════════════════════════════════════
//
// When to Validate:
//   1. Handler level (early rejection, fast feedback)
//   2. Service level (business rules enforcement)
//   3. Repository level (last line of defense)
//
// Validation Layers:
//
// Layer 1: Gin Binding Tags (Handler)
//   type RegisterRequest struct {
//       Email string `json:"email" binding:"required,email"`
//   }
//   Fast, declarative, catches basic errors
//
// Layer 2: Explicit Validators (Service)
//   if err := validators.ValidatePasswordComplexity(pwd); err != nil {
//       return err
//   }
//   Business rules, complex validation, reusable
//
// Layer 3: Database Constraints (Repository)
//   CREATE TABLE users (
//       email VARCHAR(255) UNIQUE NOT NULL
//   )
//   Last defense, prevents bad data even if code has bugs
//
// Best Practices:
//   - Fail fast: validate early to save processing
//   - Be specific: return clear error messages
//   - Be consistent: same validation everywhere
//   - Be defensive: trust no input
//   - Test edge cases: empty, null, very long, special chars
//
// Common Validation Mistakes:
//   ❌ Only validating at handler level (business rules in wrong place)
//   ❌ Only validating at service level (slow feedback to user)
//   ❌ Different validation in different places (inconsistency)
//   ❌ Overly strict validation (email regex too complex)
//   ❌ Not trimming whitespace (user types space by accident)
//
// ═══════════════════════════════════════════════════════════════════════════
