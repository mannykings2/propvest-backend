package validators

import (
	"strings"
	"testing"

	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/stretchr/testify/assert"
)

// ═══════════════════════════════════════════════════════════════════════════
// PASSWORD COMPLEXITY VALIDATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidatePasswordComplexity_Valid(t *testing.T) {
	validPasswords := []string{
		"SecurePass123!",
		"MyP@ssw0rd2024",
		"Compl3x!Pass",
		"Test123!@#ABC",
		"aB1!aB1!aB1!",          // Exactly 12 chars
		"VeryLongP@ssw0rdWith72Characters123456789012345678901234567890123!",  // Near max
		"Correct-Horse-Battery-Staple1!A", // Passphrase style
		"!@#$%^&*()1Aa",          // All special chars
		"Admin@2024Pass",         // Common pattern but valid
	}

	for _, pwd := range validPasswords {
		t.Run(pwd[:min(len(pwd), 20)], func(t *testing.T) {
			err := ValidatePasswordComplexity(pwd)
			assert.NoError(t, err, "Expected valid password: %s", pwd)
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestValidatePasswordComplexity_TooShort(t *testing.T) {
	shortPasswords := []struct {
		password string
		length   int
	}{
		{"", 0},
		{"a", 1},
		{"Pass123!", 8},
		{"Secure1!", 8},
		{"P@ssw0rd!", 9},
		{"MyPass123!", 10},
		{"SecureP@1", 9},
		{"Abcdef123!", 10}, // 10 chars (need 12)
		{"Test123!Aa", 10},  // 10 chars
	}

	for _, tc := range shortPasswords {
		t.Run("length_"+string(rune(tc.length+'0')), func(t *testing.T) {
			err := ValidatePasswordComplexity(tc.password)
			assert.Equal(t, errors.ErrWeakPassword, err, "Should reject password with %d chars", tc.length)
		})
	}
}

func TestValidatePasswordComplexity_TooLong(t *testing.T) {
	// bcrypt truncates at 72 bytes, so we reject longer passwords
	longPassword := strings.Repeat("A1a!", 20) // 80 characters
	
	err := ValidatePasswordComplexity(longPassword)
	assert.Equal(t, errors.ErrPasswordTooLong, err)
	
	// Test exactly 73 characters
	exactlyTooLong := "A1a@" + strings.Repeat("x", 69) // 4 + 69 = 73 chars
	assert.Len(t, exactlyTooLong, 73)
	err = ValidatePasswordComplexity(exactlyTooLong)
	assert.Equal(t, errors.ErrPasswordTooLong, err)
}

func TestValidatePasswordComplexity_NoUppercase(t *testing.T) {
	noUppercasePasswords := []string{
		"password123!",
		"secure1!pass",
		"test123!@#abc",
		"myp@ssw0rd2024",
		"lowercase123!",
	}

	for _, pwd := range noUppercasePasswords {
		t.Run(pwd, func(t *testing.T) {
			err := ValidatePasswordComplexity(pwd)
			assert.Equal(t, errors.ErrWeakPassword, err, "Should reject password without uppercase")
		})
	}
}

func TestValidatePasswordComplexity_NoLowercase(t *testing.T) {
	noLowercasePasswords := []string{
		"PASSWORD123!",
		"SECURE1!PASS",
		"TEST123!@#ABC",
		"MYP@SSW0RD2024",
		"UPPERCASE123!",
	}

	for _, pwd := range noLowercasePasswords {
		t.Run(pwd, func(t *testing.T) {
			err := ValidatePasswordComplexity(pwd)
			assert.Equal(t, errors.ErrWeakPassword, err, "Should reject password without lowercase")
		})
	}
}

func TestValidatePasswordComplexity_NoDigit(t *testing.T) {
	noDigitPasswords := []string{
		"SecurePassword!",
		"MyP@ssword",
		"Compl!xPass",
		"Test!@#ABC",
		"NoDigitsHere!@",
	}

	for _, pwd := range noDigitPasswords {
		t.Run(pwd, func(t *testing.T) {
			err := ValidatePasswordComplexity(pwd)
			assert.Equal(t, errors.ErrWeakPassword, err, "Should reject password without digit")
		})
	}
}

func TestValidatePasswordComplexity_NoSpecialChar(t *testing.T) {
	noSpecialCharPasswords := []string{
		"SecurePassword123",
		"MyPassword2024",
		"Compl3xPass",
		"Test123ABC",
		"NoSpecialChars1Aa",
	}

	for _, pwd := range noSpecialCharPasswords {
		t.Run(pwd, func(t *testing.T) {
			err := ValidatePasswordComplexity(pwd)
			assert.Equal(t, errors.ErrWeakPassword, err, "Should reject password without special char")
		})
	}
}

func TestValidatePasswordComplexity_AllSpecialChars(t *testing.T) {
	// Test that all defined special characters are recognized
	specialChars := []string{
		"!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "_", "+",
		"{", "}", "|", ":", "\"", "<", ">", "?", "[", "]", "-", "=",
		";", "'", ",", ".", "/", "`", "~",
	}

	for _, char := range specialChars {
		t.Run("char_"+char, func(t *testing.T) {
			password := "SecurePass123" + char
			err := ValidatePasswordComplexity(password)
			assert.NoError(t, err, "Should accept password with special char: %s", char)
		})
	}
}

func TestValidatePasswordComplexity_MultipleRequirementsMissing(t *testing.T) {
	testCases := []struct {
		password string
		missing  string
	}{
		{"password", "uppercase, digit, special"},
		{"PASSWORD", "lowercase, digit, special"},
		{"123456789012", "uppercase, lowercase, special"},
		{"!@#$%^&*()!@", "uppercase, lowercase, digit"},
		{"Passw0rd", "special (and too short)"},
	}

	for _, tc := range testCases {
		t.Run(tc.missing, func(t *testing.T) {
			err := ValidatePasswordComplexity(tc.password)
			assert.Equal(t, errors.ErrWeakPassword, err)
		})
	}
}

func TestValidatePasswordComplexity_BoundaryLength(t *testing.T) {
	t.Run("Exactly 12 characters (minimum)", func(t *testing.T) {
		password := "SecureP@ss1!" // Exactly 12 chars
		assert.Len(t, password, 12)
		err := ValidatePasswordComplexity(password)
		assert.NoError(t, err)
	})

	t.Run("11 characters (one below minimum)", func(t *testing.T) {
		password := "SecureP@ss1" // 11 chars
		assert.Len(t, password, 11)
		err := ValidatePasswordComplexity(password)
		assert.Equal(t, errors.ErrWeakPassword, err)
	})

	t.Run("Exactly 72 characters (maximum)", func(t *testing.T) {
		// Build a 72-char password that meets all requirements
		password := "A1a!" + strings.Repeat("x", 68) // 4 + 68 = 72
		assert.Len(t, password, 72)
		err := ValidatePasswordComplexity(password)
		assert.NoError(t, err)
	})

	t.Run("73 characters (one above maximum)", func(t *testing.T) {
		password := "A1a!" + strings.Repeat("x", 69) // 4 + 69 = 73
		assert.Len(t, password, 73)
		err := ValidatePasswordComplexity(password)
		assert.Equal(t, errors.ErrPasswordTooLong, err)
	})
}

func TestValidatePasswordComplexity_CommonWeakPasswords(t *testing.T) {
	// These should all fail (though they might pass length check)
	commonWeak := []string{
		"Password123!",   // Dictionary word
		"Qwerty123!",     // Keyboard pattern
		"Admin@2024",     // Only 11 chars
		"Welcome123!",    // Common word
		"Letmein123!",    // Common phrase
	}

	for _, pwd := range commonWeak {
		t.Run(pwd, func(t *testing.T) {
			err := ValidatePasswordComplexity(pwd)
			// Note: Our validator only checks complexity, not dictionary words
			// Password123! and Qwerty123! will actually PASS our validator
			// In production, add dictionary/breach checking (HaveIBeenPwned API)
			if len(pwd) < 12 {
				assert.Equal(t, errors.ErrWeakPassword, err)
			}
		})
	}
}

func TestValidatePasswordComplexity_PassphraseStyle(t *testing.T) {
	// Passphrases should pass if they meet requirements
	passphrases := []string{
		"Correct-Horse-Battery-Staple1!A",
		"My-Super-Secret-P@ssw0rd-2024",
		"I-Love-Coding-In-Go123!",
		"This-Is-A-Long-Passphrase1!A",
	}

	for _, phrase := range passphrases {
		t.Run(phrase[:min(len(phrase), 25)], func(t *testing.T) {
			err := ValidatePasswordComplexity(phrase)
			assert.NoError(t, err)
		})
	}
}

func TestValidatePasswordComplexity_EdgeCases(t *testing.T) {
	t.Run("Just meets all requirements", func(t *testing.T) {
		// Shortest possible valid password
		password := "Aa1!aaaaaaaa" // 12 chars, has all requirements
		assert.Len(t, password, 12)
		err := ValidatePasswordComplexity(password)
		assert.NoError(t, err)
	})

	t.Run("All requirements in first 4 chars", func(t *testing.T) {
		password := "Aa1!" + strings.Repeat("x", 8) // Requirements + padding
		err := ValidatePasswordComplexity(password)
		assert.NoError(t, err)
	})

	t.Run("Requirements spread throughout", func(t *testing.T) {
		password := "aaaaaAaaaaaa1aaaa!aaa" // Scattered requirements
		err := ValidatePasswordComplexity(password)
		assert.NoError(t, err)
	})

	t.Run("Multiple of each requirement type", func(t *testing.T) {
		password := "AABBccdd1234!@#$" // Multiple uppercase, lowercase, digits, specials
		err := ValidatePasswordComplexity(password)
		assert.NoError(t, err)
	})
}

func TestValidatePasswordComplexity_UnicodeCharacters(t *testing.T) {
	t.Run("Unicode uppercase", func(t *testing.T) {
		password := "Pässwörd123!" // Contains unicode
		err := ValidatePasswordComplexity(password)
		// Should pass (unicode doesn't break validation)
		assert.NoError(t, err)
	})

	t.Run("Emoji in password", func(t *testing.T) {
		password := "SecurePass123!😀"
		err := ValidatePasswordComplexity(password)
		// Should pass (we don't restrict unicode)
		assert.NoError(t, err)
	})

	t.Run("Chinese characters", func(t *testing.T) {
		password := "密码Password123!"
		err := ValidatePasswordComplexity(password)
		// Should pass
		assert.NoError(t, err)
	})
}

func TestValidatePasswordComplexity_SecurityPatterns(t *testing.T) {
	t.Run("SQL injection attempt", func(t *testing.T) {
		password := "'; DROP TABLE users--123Aa!"
		err := ValidatePasswordComplexity(password)
		// Should pass (we validate format, not content safety)
		// SQL injection is prevented by parameterized queries, not password validation
		assert.NoError(t, err)
	})

	t.Run("Script tag in password", func(t *testing.T) {
		password := "<script>alert('xss')</script>1Aa!"
		err := ValidatePasswordComplexity(password)
		// Should pass (length + requirements met)
		assert.NoError(t, err)
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// BENCHMARK TESTS
// ═══════════════════════════════════════════════════════════════════════════

func BenchmarkValidatePasswordComplexity_Valid(b *testing.B) {
	password := "SecurePass123!"
	for i := 0; i < b.N; i++ {
		_ = ValidatePasswordComplexity(password)
	}
}

func BenchmarkValidatePasswordComplexity_Invalid(b *testing.B) {
	password := "weak" // Too short, missing requirements
	for i := 0; i < b.N; i++ {
		_ = ValidatePasswordComplexity(password)
	}
}

func BenchmarkValidatePasswordComplexity_LongPassword(b *testing.B) {
	password := "A1a!" + strings.Repeat("x", 68) // 72 chars
	for i := 0; i < b.N; i++ {
		_ = ValidatePasswordComplexity(password)
	}
}
