package validators

import (
	"regexp"

	"github.com/mannykings2/propvest-backend/internal/errors"
)

// ValidatePasswordComplexity enforces password strength rules
//
// Requirements (from security doc):
//   - Minimum 12 characters
//   - At least 1 uppercase letter (A-Z)
//   - At least 1 lowercase letter (a-z)
//   - At least 1 digit (0-9)
//   - At least 1 special character (!@#$%^&*()_+{}|:"<>?[]-=;',./`~)
//
// Why these rules?
//   - Length: Increases brute-force difficulty exponentially
//   - Complexity: Prevents dictionary attacks ("password123" is common)
//   - Special chars: Forces mixing character sets (harder to guess)
//
// Trade-off:
//   Strict rules can frustrate users. Balance security with usability.
//   Consider allowing passphrases: "correct horse battery staple" (44 bits entropy)
//   vs complex password: "P@ssw0rd!" (28 bits entropy)
//
// Parameters:
//   - pwd: The password string to validate
//
// Returns:
//   - nil if password meets all requirements
//   - ErrWeakPassword if any requirement fails
//   - ErrPasswordTooLong if password exceeds bcrypt limit
//
// Example usage:
//   if err := validators.ValidatePasswordComplexity(password); err != nil {
//       return err
//   }
func ValidatePasswordComplexity(pwd string) error {
	// Check minimum length
	// 12 characters provides reasonable security without being burdensome
	if len(pwd) < 12 {
		return errors.ErrWeakPassword
	}

	// Check maximum length
	// bcrypt (used for hashing) truncates at 72 bytes
	// Reject passwords longer than this to avoid confusion
	if len(pwd) > 72 {
		return errors.ErrPasswordTooLong
	}

	// Define regex patterns for each requirement
	// [A-Z]+ means "one or more uppercase letters"
	// The + quantifier means "at least one"
	hasUppercase := regexp.MustCompile(`[A-Z]+`).MatchString(pwd)
	hasLowercase := regexp.MustCompile(`[a-z]+`).MatchString(pwd)
	hasDigit := regexp.MustCompile(`[0-9]+`).MatchString(pwd)
	
	// Special characters: common symbols on keyboard
	// We escape special regex characters with backslash
	// The backtick (`) is added at the end outside the character class
	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+{}\|:"<>?\[\]\-=;',./` + "`~]+").MatchString(pwd)

	// All requirements must be met
	// If any is false, password is weak
	if !hasUppercase || !hasLowercase || !hasDigit || !hasSpecial {
		return errors.ErrWeakPassword
	}

	// Password meets all requirements
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: Password Security Best Practices
// ═══════════════════════════════════════════════════════════════════════════
//
// Password Complexity vs Length:
//   - 12 random characters: 62^12 ≈ 3.2 × 10^21 combinations
//   - 8 complex characters: ~95^8 ≈ 6.6 × 10^15 combinations
//   - Length matters more than complexity
//
// Why 12 characters minimum?
//   - NIST recommends minimum 8, but 12 is more secure
//   - 12 chars with complexity: very strong password
//   - Balances security with usability
//
// Common Weak Passwords to Avoid:
//   - "Password123!" (dictionary word + pattern)
//   - "Qwerty123!" (keyboard pattern)
//   - "Admin@2024" (predictable year)
//   - Reused passwords from breached sites
//
// Better Alternatives:
//   - Passphrases: "correct-horse-battery-staple"
//   - Random generation: "X9$mK2pL@4nQ"
//   - Password managers (user can't remember anyway)
//
// Security Layers:
//   1. Complexity validation (this function)
//   2. bcrypt hashing (password.Hash)
//   3. Rate limiting (prevent brute-force)
//   4. Multi-factor auth (future milestone)
//   5. Breach detection (HaveIBeenPwned API)
//
// ═══════════════════════════════════════════════════════════════════════════
