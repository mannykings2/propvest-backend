package otp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerate_Success tests successful OTP generation
func TestGenerate_Success(t *testing.T) {
	code, err := Generate()

	assert.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.Len(t, code, 6, "OTP should be exactly 6 characters")

	// Verify it's all digits
	matched, _ := regexp.MatchString(`^\d{6}$`, code)
	assert.True(t, matched, "OTP should be 6 digits")
}

// TestGenerate_Format tests that OTP has correct format with leading zeros
func TestGenerate_Format(t *testing.T) {
	// Generate multiple OTPs to test format consistency
	for i := 0; i < 10; i++ {
		code, err := Generate()
		assert.NoError(t, err)

		// Must be exactly 6 characters
		assert.Len(t, code, 6)

		// Must be all numeric
		matched, _ := regexp.MatchString(`^\d{6}$`, code)
		assert.True(t, matched, "Code should be numeric: %s", code)
	}
}

// TestGenerate_Uniqueness tests that multiple OTPs are different
func TestGenerate_Uniqueness(t *testing.T) {
	// Generate multiple OTPs
	codes := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		code, err := Generate()
		require.NoError(t, err)
		codes[code] = true
	}

	// With 100 generations and 1,000,000 possible codes,
	// we should have mostly unique codes (very unlikely to get duplicates)
	// We'll be lenient and just check we got more than 90 unique codes
	assert.Greater(t, len(codes), 90, "Should generate mostly unique OTPs")
}

// TestGenerate_Range tests that generated OTPs are within valid range
func TestGenerate_Range(t *testing.T) {
	for i := 0; i < 50; i++ {
		code, err := Generate()
		require.NoError(t, err)

		// Convert to integer
		var num int
		_, err = fmt.Sscanf(code, "%d", &num)
		require.NoError(t, err)

		// Should be between 0 and 999999
		assert.GreaterOrEqual(t, num, 0)
		assert.LessOrEqual(t, num, 999999)
	}
}

// TestGenerate_LeadingZeros tests that codes with leading zeros are formatted correctly
func TestGenerate_LeadingZeros(t *testing.T) {
	// Generate many codes to eventually get ones with leading zeros
	foundLeadingZero := false

	for i := 0; i < 200; i++ {
		code, err := Generate()
		require.NoError(t, err)

		// Check if it starts with 0
		if code[0] == '0' {
			foundLeadingZero = true

			// Verify format is still 6 digits
			assert.Len(t, code, 6)
			matched, _ := regexp.MatchString(`^\d{6}$`, code)
			assert.True(t, matched)

			// At least one leading zero found, test passes
			break
		}
	}

	// We should eventually find a code starting with 0
	// Probability is ~10% per generation, so 200 tries should be enough
	assert.True(t, foundLeadingZero, "Should generate codes with leading zeros")
}

// TestHash_Consistency tests that hashing the same code produces same hash
func TestHash_Consistency(t *testing.T) {
	code := "123456"

	hash1 := Hash(code)
	hash2 := Hash(code)

	assert.Equal(t, hash1, hash2, "Same code should produce same hash")
}

// TestHash_DifferentCodes tests that different codes produce different hashes
func TestHash_DifferentCodes(t *testing.T) {
	codes := []string{"123456", "654321", "000000", "999999", "111111"}
	hashes := make(map[string]bool)

	for _, code := range codes {
		hash := Hash(code)
		hashes[hash] = true

		// Verify hash format (64 hex characters for SHA-256)
		assert.Len(t, hash, 64)
		matched, _ := regexp.MatchString(`^[a-f0-9]{64}$`, hash)
		assert.True(t, matched, "Hash should be 64 hex characters")
	}

	// All hashes should be unique
	assert.Equal(t, len(codes), len(hashes), "Different codes should produce different hashes")
}

// TestHash_KnownValue tests hash against a known SHA-256 value
func TestHash_KnownValue(t *testing.T) {
	code := "123456"

	// Calculate expected hash manually
	hasher := sha256.New()
	hasher.Write([]byte(code))
	expected := hex.EncodeToString(hasher.Sum(nil))

	actual := Hash(code)

	assert.Equal(t, expected, actual, "Hash should match known SHA-256 value")
}

// TestHash_EmptyString tests hashing an empty string
func TestHash_EmptyString(t *testing.T) {
	hash := Hash("")

	// Should still produce a valid hash
	assert.Len(t, hash, 64)
	matched, _ := regexp.MatchString(`^[a-f0-9]{64}$`, hash)
	assert.True(t, matched)

	// Empty string has a known SHA-256 hash
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	assert.Equal(t, expected, hash)
}

// TestHash_SpecialCharacters tests hashing codes with special characters
func TestHash_SpecialCharacters(t *testing.T) {
	testCases := []string{
		"123!@#",
		"abc def",
		"test\ncode",
		"unicode日本語",
	}

	for _, code := range testCases {
		hash := Hash(code)

		// Should produce valid hash even for non-numeric input
		assert.Len(t, hash, 64)
		matched, _ := regexp.MatchString(`^[a-f0-9]{64}$`, hash)
		assert.True(t, matched, "Hash should be valid for: %s", code)
	}
}

// TestVerify_CorrectCode tests verification with correct code
func TestVerify_CorrectCode(t *testing.T) {
	code := "123456"
	hash := Hash(code)

	result := Verify(code, hash)

	assert.True(t, result, "Verification should succeed with correct code")
}

// TestVerify_IncorrectCode tests verification with wrong code
func TestVerify_IncorrectCode(t *testing.T) {
	correctCode := "123456"
	wrongCode := "654321"
	hash := Hash(correctCode)

	result := Verify(wrongCode, hash)

	assert.False(t, result, "Verification should fail with incorrect code")
}

// TestVerify_SlightlyWrongCode tests verification with slight variations
func TestVerify_SlightlyWrongCode(t *testing.T) {
	correctCode := "123456"
	hash := Hash(correctCode)

	testCases := []struct {
		name string
		code string
	}{
		{"one digit wrong", "123455"},
		{"first digit wrong", "023456"},
		{"last digit wrong", "123457"},
		{"transposed digits", "123465"},
		{"extra digit", "1234567"},
		{"missing digit", "12345"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := Verify(tc.code, hash)
			assert.False(t, result, "Should fail for: %s", tc.code)
		})
	}
}

// TestVerify_CaseSensitivity tests that verification is case-sensitive for alphanumeric codes
func TestVerify_CaseSensitivity(t *testing.T) {
	code := "Abc123"
	hash := Hash(code)

	// Same case should work
	assert.True(t, Verify("Abc123", hash))

	// Different case should fail
	assert.False(t, Verify("abc123", hash))
	assert.False(t, Verify("ABC123", hash))
	assert.False(t, Verify("aBc123", hash))
}

// TestVerify_EmptyInputs tests verification with empty inputs
func TestVerify_EmptyInputs(t *testing.T) {
	code := "123456"
	hash := Hash(code)

	// Empty code against valid hash
	assert.False(t, Verify("", hash))

	// Valid code against empty hash
	assert.False(t, Verify(code, ""))

	// Empty code against empty hash (technically should match)
	emptyHash := Hash("")
	assert.True(t, Verify("", emptyHash))
}

// TestVerify_InvalidHash tests verification with malformed hash
func TestVerify_InvalidHash(t *testing.T) {
	code := "123456"

	testCases := []struct {
		name string
		hash string
	}{
		{"short hash", "abc123"},
		{"wrong length", "1234567890123456789012345678901234567890123456789012345678901234"},
		{"non-hex characters", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"empty hash", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := Verify(code, tc.hash)
			assert.False(t, result, "Should fail for invalid hash: %s", tc.name)
		})
	}
}

// TestGenerateAlphanumeric_Success tests successful alphanumeric code generation
func TestGenerateAlphanumeric_Success(t *testing.T) {
	length := 32
	code, err := GenerateAlphanumeric(length)

	assert.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.Len(t, code, length)

	// Verify it's alphanumeric only
	matched, _ := regexp.MatchString(`^[A-Za-z0-9]+$`, code)
	assert.True(t, matched, "Code should be alphanumeric")
}

// TestGenerateAlphanumeric_DifferentLengths tests various lengths
func TestGenerateAlphanumeric_DifferentLengths(t *testing.T) {
	testLengths := []int{8, 16, 32, 64, 128}

	for _, length := range testLengths {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			code, err := GenerateAlphanumeric(length)

			assert.NoError(t, err)
			assert.Len(t, code, length)

			// Verify alphanumeric
			matched, _ := regexp.MatchString(`^[A-Za-z0-9]+$`, code)
			assert.True(t, matched)
		})
	}
}

// TestGenerateAlphanumeric_Uniqueness tests that codes are unique
func TestGenerateAlphanumeric_Uniqueness(t *testing.T) {
	length := 32
	codes := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		code, err := GenerateAlphanumeric(length)
		require.NoError(t, err)
		codes[code] = true
	}

	// All codes should be unique (32 chars = huge entropy)
	assert.Equal(t, iterations, len(codes), "All alphanumeric codes should be unique")
}

// TestGenerateAlphanumeric_CharacterDistribution tests that all character types appear
func TestGenerateAlphanumeric_CharacterDistribution(t *testing.T) {
	// Generate a long code to ensure we get diverse characters
	code, err := GenerateAlphanumeric(1000)
	require.NoError(t, err)

	hasLowercase := regexp.MustCompile(`[a-z]`).MatchString(code)
	hasUppercase := regexp.MustCompile(`[A-Z]`).MatchString(code)
	hasDigits := regexp.MustCompile(`[0-9]`).MatchString(code)

	assert.True(t, hasLowercase, "Should contain lowercase letters")
	assert.True(t, hasUppercase, "Should contain uppercase letters")
	assert.True(t, hasDigits, "Should contain digits")
}

// TestGenerateAlphanumeric_ZeroLength tests generating zero-length code
func TestGenerateAlphanumeric_ZeroLength(t *testing.T) {
	code, err := GenerateAlphanumeric(0)

	assert.NoError(t, err)
	assert.Empty(t, code)
	assert.Len(t, code, 0)
}

// TestGenerateAlphanumeric_SingleCharacter tests generating single character
func TestGenerateAlphanumeric_SingleCharacter(t *testing.T) {
	code, err := GenerateAlphanumeric(1)

	assert.NoError(t, err)
	assert.Len(t, code, 1)

	// Should be alphanumeric
	matched, _ := regexp.MatchString(`^[A-Za-z0-9]$`, code)
	assert.True(t, matched)
}

// TestIntegration_GenerateHashVerify tests full OTP workflow
func TestIntegration_GenerateHashVerify(t *testing.T) {
	// Step 1: Generate OTP
	code, err := Generate()
	require.NoError(t, err)

	// Step 2: Hash for storage
	hash := Hash(code)
	assert.Len(t, hash, 64)

	// Step 3: Verify correct code
	assert.True(t, Verify(code, hash), "Should verify correct code")

	// Step 4: Verify wrong code fails
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "111111" // Make sure it's different
	}
	assert.False(t, Verify(wrongCode, hash), "Should reject wrong code")
}

// TestIntegration_AlphanumericHashVerify tests alphanumeric code workflow
func TestIntegration_AlphanumericHashVerify(t *testing.T) {
	// Generate alphanumeric code
	code, err := GenerateAlphanumeric(32)
	require.NoError(t, err)

	// Hash it
	hash := Hash(code)

	// Should verify successfully
	assert.True(t, Verify(code, hash))

	// Wrong code should fail
	wrongCode, err := GenerateAlphanumeric(32)
	require.NoError(t, err)
	assert.False(t, Verify(wrongCode, hash))
}

// TestEdgeCase_AllZeros tests handling of "000000" code
func TestEdgeCase_AllZeros(t *testing.T) {
	code := "000000"
	hash := Hash(code)

	// Should hash and verify correctly
	assert.Len(t, hash, 64)
	assert.True(t, Verify(code, hash))
	assert.False(t, Verify("000001", hash))
}

// TestEdgeCase_AllNines tests handling of "999999" code
func TestEdgeCase_AllNines(t *testing.T) {
	code := "999999"
	hash := Hash(code)

	// Should hash and verify correctly
	assert.Len(t, hash, 64)
	assert.True(t, Verify(code, hash))
	assert.False(t, Verify("999998", hash))
}

// TestEdgeCase_SequentialDigits tests codes with sequential digits
func TestEdgeCase_SequentialDigits(t *testing.T) {
	codes := []string{
		"123456",
		"654321",
		"111111",
		"000000",
		"999999",
	}

	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			hash := Hash(code)
			assert.True(t, Verify(code, hash))
		})
	}
}

// BenchmarkGenerate benchmarks OTP generation performance
func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = Generate()
	}
}

// BenchmarkHash benchmarks OTP hashing performance
func BenchmarkHash(b *testing.B) {
	code := "123456"
	for i := 0; i < b.N; i++ {
		_ = Hash(code)
	}
}

// BenchmarkVerify benchmarks OTP verification performance
func BenchmarkVerify(b *testing.B) {
	code := "123456"
	hash := Hash(code)
	for i := 0; i < b.N; i++ {
		_ = Verify(code, hash)
	}
}

// BenchmarkGenerateAlphanumeric benchmarks alphanumeric generation
func BenchmarkGenerateAlphanumeric(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = GenerateAlphanumeric(32)
	}
}
