package validators

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/stretchr/testify/assert"
)

// ═══════════════════════════════════════════════════════════════════════════
// EMAIL VALIDATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateEmail_Valid(t *testing.T) {
	validEmails := []string{
		"user@example.com",
		"john.doe@company.co.uk",
		"admin_123@test-domain.io",
		"test+tag@gmail.com",
		"user.name+tag@example.com",
		"123@456.com",
		"a@b.co",
		"test@subdomain.example.com",
		"user%test@example.com",
	}

	for _, email := range validEmails {
		t.Run(email, func(t *testing.T) {
			err := ValidateEmail(email)
			assert.NoError(t, err, "Expected valid email: %s", email)
		})
	}
}

func TestValidateEmail_Invalid(t *testing.T) {
	invalidEmails := []struct {
		email  string
		reason string
	}{
		{"", "empty string"},
		{"   ", "whitespace only"},
		{"plaintext", "no @ symbol"},
		{"@example.com", "missing local part"},
		{"user@", "missing domain"},
		{"user @example.com", "space in email"},
		{"user@.com", "domain starts with dot"},
		{"user@example", "missing TLD"},
		{"user@@example.com", "double @"},
		{"user@exam ple.com", "space in domain"},
		{"user@example.c", "TLD too short"},
	}

	for _, tc := range invalidEmails {
		t.Run(tc.reason, func(t *testing.T) {
			err := ValidateEmail(tc.email)
			assert.Equal(t, errors.ErrInvalidEmail, err, "Should reject: %s", tc.reason)
		})
	}
}

func TestValidateEmail_Normalization(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"  USER@EXAMPLE.COM  ", "user@example.com"},
		{"User@Example.Com", "user@example.com"},
		{" test@test.com ", "test@test.com"},
		{"ADMIN@COMPANY.COM", "admin@company.com"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			// Validation should pass (normalization happens internally)
			err := ValidateEmail(tc.input)
			assert.NoError(t, err)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// UUID VALIDATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateUUID_Valid(t *testing.T) {
	validUUIDs := []string{
		uuid.New().String(),
		"a3f8b2c1-d4e5-4f67-8901-23456789abcd",
		"00000000-0000-0000-0000-000000000000", // Nil UUID
		"123e4567-e89b-12d3-a456-426614174000",
	}

	for _, id := range validUUIDs {
		t.Run(id, func(t *testing.T) {
			err := ValidateUUID(id)
			assert.NoError(t, err)
		})
	}
}

func TestValidateUUID_Invalid(t *testing.T) {
	invalidUUIDs := []struct {
		uuid   string
		reason string
	}{
		{"", "empty string"},
		{"not-a-uuid", "random string"},
		{"123", "too short"},
		{"a3f8b2c1-d4e5-4f67-8901", "incomplete UUID"},
		{"a3f8b2c1-d4e5-4f67-8901-23456789abcdXXXX", "too long"},
		{"g3f8b2c1-d4e5-4f67-8901-23456789abcd", "invalid character 'g'"},
	}

	for _, tc := range invalidUUIDs {
		t.Run(tc.reason, func(t *testing.T) {
			err := ValidateUUID(tc.uuid)
			assert.Equal(t, errors.ErrInvalidUUID, err, "Should reject: %s", tc.reason)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// NOT EMPTY VALIDATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateNotEmpty_Valid(t *testing.T) {
	validValues := []string{
		"a",
		"test",
		"Hello World",
		"  text  ", // Has content after trim
		"123",
	}

	for _, value := range validValues {
		t.Run(value, func(t *testing.T) {
			err := ValidateNotEmpty(value, "testField")
			assert.NoError(t, err)
		})
	}
}

func TestValidateNotEmpty_Invalid(t *testing.T) {
	invalidValues := []struct {
		value  string
		reason string
	}{
		{"", "empty string"},
		{"   ", "whitespace only"},
		{"\t", "tab only"},
		{"\n", "newline only"},
		{"  \t\n  ", "mixed whitespace"},
	}

	for _, tc := range invalidValues {
		t.Run(tc.reason, func(t *testing.T) {
			err := ValidateNotEmpty(tc.value, "testField")
			assert.Equal(t, errors.ErrMissingField, err)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// STRING LENGTH VALIDATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateStringLength_Valid(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		min   int
		max   int
	}{
		{"exact min", "ab", 2, 10},
		{"exact max", "abcdefghij", 2, 10},
		{"in range", "hello", 2, 10},
		{"single char at min", "a", 1, 5},
		{"with spaces trimmed", "  hello  ", 2, 10}, // "hello" = 5 chars after trim
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStringLength(tc.value, tc.min, tc.max, "testField")
			assert.NoError(t, err)
		})
	}
}

func TestValidateStringLength_Invalid(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		min   int
		max   int
	}{
		{"too short", "a", 2, 10},
		{"too long", "abcdefghijk", 2, 10},
		{"empty vs min", "", 1, 10},
		{"far too long", strings.Repeat("a", 1000), 1, 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStringLength(tc.value, tc.min, tc.max, "testField")
			assert.Equal(t, errors.ErrValidationFailed, err)
		})
	}
}

func TestValidateStringLength_EdgeCases(t *testing.T) {
	// Test boundary conditions
	assert.NoError(t, ValidateStringLength("a", 1, 1, "test"), "Exact length should pass")
	assert.Error(t, ValidateStringLength("ab", 1, 1, "test"), "Longer than exact should fail")
	assert.Error(t, ValidateStringLength("", 1, 1, "test"), "Empty when min=1 should fail")
}

// ═══════════════════════════════════════════════════════════════════════════
// AMOUNT VALIDATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateAmount_Valid(t *testing.T) {
	testCases := []struct {
		name   string
		amount int64
		min    int64
		max    int64
	}{
		{"exact min", 100, 100, 1000},
		{"exact max", 1000, 100, 1000},
		{"in range", 500, 100, 1000},
		{"large amount", 1000000, 1, 10000000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAmount(tc.amount, tc.min, tc.max)
			assert.NoError(t, err)
		})
	}
}

func TestValidateAmount_Invalid(t *testing.T) {
	testCases := []struct {
		name   string
		amount int64
		min    int64
		max    int64
	}{
		{"zero amount", 0, 100, 1000},
		{"negative amount", -100, 100, 1000},
		{"below min", 50, 100, 1000},
		{"above max", 1100, 100, 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAmount(tc.amount, tc.min, tc.max)
			assert.Equal(t, errors.ErrInvalidAmount, err)
		})
	}
}

func TestValidateAmount_EdgeCases(t *testing.T) {
	// Boundary conditions
	assert.NoError(t, ValidateAmount(1, 1, 1), "Exact amount should pass")
	assert.Error(t, ValidateAmount(0, 1, 1), "Zero should fail")
	assert.Error(t, ValidateAmount(-1, 1, 1), "Negative should fail")
	assert.Error(t, ValidateAmount(2, 1, 1), "Above max should fail")
}

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidation_CombinedScenarios(t *testing.T) {
	t.Run("Valid user registration data", func(t *testing.T) {
		email := "john.doe@example.com"
		name := "John Doe"
		
		assert.NoError(t, ValidateEmail(email))
		assert.NoError(t, ValidateNotEmpty(name, "name"))
		assert.NoError(t, ValidateStringLength(name, 2, 100, "name"))
	})

	t.Run("Invalid user registration data", func(t *testing.T) {
		email := "invalid-email"
		name := ""
		
		assert.Error(t, ValidateEmail(email))
		assert.Error(t, ValidateNotEmpty(name, "name"))
	})

	t.Run("Valid transaction data", func(t *testing.T) {
		amount := int64(50000) // 500 NGN in kobo
		min := int64(10000)    // 100 NGN min
		max := int64(1000000)  // 10,000 NGN max
		
		assert.NoError(t, ValidateAmount(amount, min, max))
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// BENCHMARK TESTS
// ═══════════════════════════════════════════════════════════════════════════

func BenchmarkValidateEmail(b *testing.B) {
	email := "test@example.com"
	for i := 0; i < b.N; i++ {
		_ = ValidateEmail(email)
	}
}

func BenchmarkValidateUUID(b *testing.B) {
	id := uuid.New().String()
	for i := 0; i < b.N; i++ {
		_ = ValidateUUID(id)
	}
}

func BenchmarkValidateStringLength(b *testing.B) {
	value := "Hello World"
	for i := 0; i < b.N; i++ {
		_ = ValidateStringLength(value, 2, 100, "test")
	}
}

func BenchmarkValidateAmount(b *testing.B) {
	amount := int64(50000)
	for i := 0; i < b.N; i++ {
		_ = ValidateAmount(amount, 10000, 1000000)
	}
}
