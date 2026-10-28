package token

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateRandomToken_Success tests successful token generation
func TestGenerateRandomToken_Success(t *testing.T) {
	length := 32
	token, err := GenerateRandomToken(length)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Token length should be approximately 4/3 of input length (base64 encoding)
	// For 32 bytes, we expect ~43 characters
	assert.Greater(t, len(token), length, "Base64 encoded token should be longer than byte length")
}

// TestGenerateRandomToken_URLSafe tests that token is URL-safe
func TestGenerateRandomToken_URLSafe(t *testing.T) {
	token, err := GenerateRandomToken(32)
	require.NoError(t, err)

	// URL-safe base64 uses: A-Z, a-z, 0-9, -, _
	// No +, /, or = characters
	matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, token)
	assert.True(t, matched, "Token should be URL-safe (no +, /, or = characters)")
}

// TestGenerateRandomToken_DifferentLengths tests various token lengths
func TestGenerateRandomToken_DifferentLengths(t *testing.T) {
	testCases := []struct {
		name       string
		byteLength int
		minChars   int // Minimum expected character length
	}{
		{"16 bytes", 16, 20},
		{"32 bytes", 32, 40},
		{"64 bytes", 64, 80},
		{"128 bytes", 128, 160},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := GenerateRandomToken(tc.byteLength)

			assert.NoError(t, err)
			assert.NotEmpty(t, token)
			assert.GreaterOrEqual(t, len(token), tc.minChars)

			// Verify URL-safe characters
			matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, token)
			assert.True(t, matched)
		})
	}
}

// TestGenerateRandomToken_Uniqueness tests that tokens are unique
func TestGenerateRandomToken_Uniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	iterations := 100
	length := 32

	for i := 0; i < iterations; i++ {
		token, err := GenerateRandomToken(length)
		require.NoError(t, err)
		tokens[token] = true
	}

	// All tokens should be unique (32 bytes = 256 bits of entropy)
	assert.Equal(t, iterations, len(tokens), "All tokens should be unique")
}

// TestGenerateRandomToken_ZeroLength tests generating zero-length token
func TestGenerateRandomToken_ZeroLength(t *testing.T) {
	token, err := GenerateRandomToken(0)

	assert.NoError(t, err)
	assert.Empty(t, token)
}

// TestGenerateRandomToken_SingleByte tests generating 1-byte token
func TestGenerateRandomToken_SingleByte(t *testing.T) {
	token, err := GenerateRandomToken(1)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// 1 byte should encode to 1-2 characters
	assert.LessOrEqual(t, len(token), 2)
}

// TestGenerateRandomToken_LargeToken tests generating large token
func TestGenerateRandomToken_LargeToken(t *testing.T) {
	token, err := GenerateRandomToken(256)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Greater(t, len(token), 300)
}

// TestGenerateRandomToken_CharacterDistribution tests character variety
func TestGenerateRandomToken_CharacterDistribution(t *testing.T) {
	// Generate a long token to ensure character diversity
	token, err := GenerateRandomToken(1000)
	require.NoError(t, err)

	hasLowercase := regexp.MustCompile(`[a-z]`).MatchString(token)
	hasUppercase := regexp.MustCompile(`[A-Z]`).MatchString(token)
	hasDigits := regexp.MustCompile(`[0-9]`).MatchString(token)

	// With 1000 bytes, we should see all character types
	assert.True(t, hasLowercase, "Should contain lowercase letters")
	assert.True(t, hasUppercase, "Should contain uppercase letters")
	assert.True(t, hasDigits, "Should contain digits")
}

// TestGenerateRandomToken_NoForbiddenCharacters tests absence of URL-unsafe chars
func TestGenerateRandomToken_NoForbiddenCharacters(t *testing.T) {
	for i := 0; i < 50; i++ {
		token, err := GenerateRandomToken(64)
		require.NoError(t, err)

		// Should not contain standard base64 characters
		assert.NotContains(t, token, "+", "Should not contain +")
		assert.NotContains(t, token, "/", "Should not contain /")
		assert.NotContains(t, token, "=", "Should not contain = (padding)")
	}
}

// TestGenerateNumericCode_Success tests successful numeric code generation
func TestGenerateNumericCode_Success(t *testing.T) {
	length := 6
	code, err := GenerateNumericCode(length)

	assert.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.Len(t, code, length)

	// Verify all digits
	matched, _ := regexp.MatchString(`^\d+$`, code)
	assert.True(t, matched, "Code should be all digits")
}

// TestGenerateNumericCode_DifferentLengths tests various code lengths
func TestGenerateNumericCode_DifferentLengths(t *testing.T) {
	testLengths := []int{4, 6, 8, 10, 12}

	for _, length := range testLengths {
		t.Run(string(rune(length)), func(t *testing.T) {
			code, err := GenerateNumericCode(length)

			assert.NoError(t, err)
			assert.Len(t, code, length)

			// Verify numeric
			matched, _ := regexp.MatchString(`^\d+$`, code)
			assert.True(t, matched)
		})
	}
}

// TestGenerateNumericCode_Uniqueness tests that codes are mostly unique
func TestGenerateNumericCode_Uniqueness(t *testing.T) {
	codes := make(map[string]bool)
	iterations := 100
	length := 8 // 8 digits = 100 million combinations

	for i := 0; i < iterations; i++ {
		code, err := GenerateNumericCode(length)
		require.NoError(t, err)
		codes[code] = true
	}

	// With 8 digits and 100 iterations, we should have mostly unique codes
	assert.Greater(t, len(codes), 95, "Should generate mostly unique codes")
}

// TestGenerateNumericCode_AllDigitsPresent tests digit distribution
func TestGenerateNumericCode_AllDigitsPresent(t *testing.T) {
	// Generate many codes to see all digits 0-9
	allCodes := ""
	for i := 0; i < 50; i++ {
		code, err := GenerateNumericCode(20)
		require.NoError(t, err)
		allCodes += code
	}

	// Check that all digits 0-9 appear
	for digit := '0'; digit <= '9'; digit++ {
		assert.Contains(t, allCodes, string(digit), "Should contain digit %c", digit)
	}
}

// TestGenerateNumericCode_ZeroLength tests zero-length code
func TestGenerateNumericCode_ZeroLength(t *testing.T) {
	code, err := GenerateNumericCode(0)

	assert.NoError(t, err)
	assert.Empty(t, code)
	assert.Len(t, code, 0)
}

// TestGenerateNumericCode_SingleDigit tests single-digit code
func TestGenerateNumericCode_SingleDigit(t *testing.T) {
	code, err := GenerateNumericCode(1)

	assert.NoError(t, err)
	assert.Len(t, code, 1)

	// Should be a digit
	matched, _ := regexp.MatchString(`^\d$`, code)
	assert.True(t, matched)
}

// TestGenerateNumericCode_LeadingZeros tests codes with leading zeros
func TestGenerateNumericCode_LeadingZeros(t *testing.T) {
	foundLeadingZero := false

	// Generate many codes to find one with leading zero
	for i := 0; i < 100; i++ {
		code, err := GenerateNumericCode(6)
		require.NoError(t, err)

		if code[0] == '0' {
			foundLeadingZero = true
			// Verify it's still 6 digits
			assert.Len(t, code, 6)
			break
		}
	}

	// Should eventually generate a code with leading zero
	assert.True(t, foundLeadingZero, "Should generate codes with leading zeros")
}

// TestGenerateNumericCode_NoLetters tests that code has no letters
func TestGenerateNumericCode_NoLetters(t *testing.T) {
	for i := 0; i < 20; i++ {
		code, err := GenerateNumericCode(10)
		require.NoError(t, err)

		// Should not contain any letters
		hasLetters := regexp.MustCompile(`[a-zA-Z]`).MatchString(code)
		assert.False(t, hasLetters, "Code should not contain letters: %s", code)

		// Should not contain special characters
		hasSpecial := regexp.MustCompile(`[^0-9]`).MatchString(code)
		assert.False(t, hasSpecial, "Code should not contain special chars: %s", code)
	}
}

// TestMustGenerateRandomToken_Success tests MustGenerateRandomToken success case
func TestMustGenerateRandomToken_Success(t *testing.T) {
	// Should not panic
	assert.NotPanics(t, func() {
		token := MustGenerateRandomToken(32)
		assert.NotEmpty(t, token)
	})
}

// TestMustGenerateRandomToken_Uniqueness tests that Must variant generates unique tokens
func TestMustGenerateRandomToken_Uniqueness(t *testing.T) {
	tokens := make(map[string]bool)

	for i := 0; i < 10; i++ {
		token := MustGenerateRandomToken(32)
		tokens[token] = true
	}

	assert.Equal(t, 10, len(tokens), "All tokens should be unique")
}

// TestMustGenerateRandomToken_URLSafe tests that Must variant is URL-safe
func TestMustGenerateRandomToken_URLSafe(t *testing.T) {
	token := MustGenerateRandomToken(32)

	matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, token)
	assert.True(t, matched, "Token should be URL-safe")
}

// TestTokenUsageScenarios tests realistic usage scenarios
func TestTokenUsageScenarios(t *testing.T) {
	t.Run("Email verification token", func(t *testing.T) {
		// 32 bytes for email verification
		token, err := GenerateRandomToken(32)
		require.NoError(t, err)

		// Should be suitable for URL
		assert.NotContains(t, token, "=")
		assert.NotContains(t, token, "+")
		assert.NotContains(t, token, "/")

		// Should be long enough for security
		assert.Greater(t, len(token), 40)
	})

	t.Run("Password reset token", func(t *testing.T) {
		// 32 bytes for password reset
		token, err := GenerateRandomToken(32)
		require.NoError(t, err)
		assert.Greater(t, len(token), 40)
	})

	t.Run("API key", func(t *testing.T) {
		// 64 bytes for API key
		token, err := GenerateRandomToken(64)
		require.NoError(t, err)
		assert.Greater(t, len(token), 80)
	})

	t.Run("SMS verification code", func(t *testing.T) {
		// 6 digits for SMS
		code, err := GenerateNumericCode(6)
		require.NoError(t, err)
		assert.Len(t, code, 6)
		assert.Regexp(t, `^\d{6}$`, code)
	})

	t.Run("2FA backup code", func(t *testing.T) {
		// 8 digits for backup code
		code, err := GenerateNumericCode(8)
		require.NoError(t, err)
		assert.Len(t, code, 8)
		assert.Regexp(t, `^\d{8}$`, code)
	})
}

// TestTokenComparison tests that different generation methods produce different results
func TestTokenComparison(t *testing.T) {
	// Generate tokens of different types
	randomToken, err := GenerateRandomToken(32)
	require.NoError(t, err)

	numericCode, err := GenerateNumericCode(32)
	require.NoError(t, err)

	// They should be different in character set
	hasLettersInRandom := regexp.MustCompile(`[a-zA-Z]`).MatchString(randomToken)
	hasLettersInNumeric := regexp.MustCompile(`[a-zA-Z]`).MatchString(numericCode)

	assert.True(t, hasLettersInRandom, "Random token should contain letters")
	assert.False(t, hasLettersInNumeric, "Numeric code should not contain letters")
}

// TestEdgeCase_VeryLongToken tests generating very long token
func TestEdgeCase_VeryLongToken(t *testing.T) {
	token, err := GenerateRandomToken(10000)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Greater(t, len(token), 13000) // 10000 * 4/3
}

// TestEdgeCase_VeryLongNumericCode tests generating very long numeric code
func TestEdgeCase_VeryLongNumericCode(t *testing.T) {
	code, err := GenerateNumericCode(10000)

	assert.NoError(t, err)
	assert.Len(t, code, 10000)
	
	// Verify it's all digits (regex has a limit, so check manually)
	for i, ch := range code {
		if ch < '0' || ch > '9' {
			t.Errorf("Character at position %d is not a digit: %c", i, ch)
			break
		}
	}
}

// BenchmarkGenerateRandomToken benchmarks token generation performance
func BenchmarkGenerateRandomToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateRandomToken(32)
	}
}

// BenchmarkGenerateRandomToken64 benchmarks 64-byte token generation
func BenchmarkGenerateRandomToken64(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateRandomToken(64)
	}
}

// BenchmarkGenerateNumericCode benchmarks numeric code generation
func BenchmarkGenerateNumericCode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateNumericCode(6)
	}
}

// BenchmarkMustGenerateRandomToken benchmarks Must variant
func BenchmarkMustGenerateRandomToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = MustGenerateRandomToken(32)
	}
}
