package validators

import (
	"testing"

	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/stretchr/testify/assert"
)

// ═══════════════════════════════════════════════════════════════════════════
// NIGERIAN PHONE VALIDATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestValidateNigerianPhone_Valid(t *testing.T) {
	validPhones := []struct {
		phone   string
		network string
	}{
		// MTN numbers
		{"+2348012345678", "MTN 080"},
		{"+2347012345678", "MTN 070"},
		{"+2349012345678", "MTN 090"},
		{"+2348112345678", "MTN 081"},
		{"+2347112345678", "MTN 071"},

		// Airtel numbers
		{"+2348012345678", "Airtel 080"},
		{"+2347012345678", "Airtel 070"},
		{"+2349012345678", "Airtel 090"},
		{"+2348112345678", "Airtel 081"},
		{"+2349112345678", "Airtel 091"},

		// Glo numbers
		{"+2348012345678", "Glo 080"},
		{"+2347012345678", "Glo 070"},
		{"+2348112345678", "Glo 081"},

		// 9mobile/Etisalat numbers
		{"+2348092345678", "9mobile 809"},
		{"+2348172345678", "9mobile 817"},
		{"+2349082345678", "9mobile 908"},
		{"+2349092345678", "9mobile 909"},
	}

	for _, tc := range validPhones {
		t.Run(tc.network, func(t *testing.T) {
			err := ValidateNigerianPhone(tc.phone)
			assert.NoError(t, err, "Expected valid phone: %s (%s)", tc.phone, tc.network)
		})
	}
}

func TestValidateNigerianPhone_Invalid(t *testing.T) {
	invalidPhones := []struct {
		phone  string
		reason string
	}{
		// Format errors
		{"", "empty string"},
		{"   ", "whitespace only"},
		{"08012345678", "missing country code (+234)"},
		{"2348012345678", "missing plus sign"},
		{"+234 801 234 5678", "contains spaces"},
		{"+234-801-234-5678", "contains hyphens"},
		{"+234(801)234-5678", "contains parentheses"},
		
		// Length errors
		{"+23480123456", "too short (10 digits instead of 10)"},
		{"+2348012345", "too short (7 digits)"},
		{"+234801234567890", "too long (12 digits)"},
		{"+23480123456789", "too long (11 digits)"},
		
		// Invalid area codes (first digit after +234)
		{"+2346012345678", "starts with 6 (invalid)"},
		{"+2345012345678", "starts with 5 (invalid)"},
		{"+2344012345678", "starts with 4 (invalid)"},
		{"+2343012345678", "starts with 3 (invalid)"},
		{"+2342012345678", "starts with 2 (invalid)"},
		{"+2341012345678", "starts with 1 (invalid)"},
		{"+2340012345678", "starts with 0 (invalid)"},
		
		// Invalid second digit (must be 0 or 1)
		{"+2348212345678", "second digit is 2 (must be 0 or 1)"},
		{"+2347312345678", "second digit is 3 (must be 0 or 1)"},
		{"+2349512345678", "second digit is 5 (must be 0 or 1)"},
		{"+2348912345678", "second digit is 9 (must be 0 or 1)"},
		
		// Wrong country code
		{"+1234567890", "US number (wrong country)"},
		{"+4412345678", "UK number (wrong country)"},
		{"+254701234567", "Kenya number (wrong country)"},
		{"+27123456789", "South Africa (wrong country)"},
		
		// Non-numeric characters
		{"+234801234567a", "contains letter 'a'"},
		{"+234801234567!", "contains special char"},
		{"+234801234567?", "contains question mark"},
		
		// Incomplete numbers
		{"+234", "just country code"},
		{"+2348", "just country code + first digit"},
		{"+23480", "just country code + area code"},
	}

	for _, tc := range invalidPhones {
		t.Run(tc.reason, func(t *testing.T) {
			err := ValidateNigerianPhone(tc.phone)
			assert.Equal(t, errors.ErrInvalidPhoneFormat, err, "Should reject: %s", tc.reason)
		})
	}
}

func TestValidateNigerianPhone_AllValidAreaCodes(t *testing.T) {
	// Test all valid first digits (7, 8, 9) with second digits (0, 1)
	validCombos := []string{
		"+2347012345678", // 70
		"+2347112345678", // 71
		"+2348012345678", // 80
		"+2348112345678", // 81
		"+2349012345678", // 90
		"+2349112345678", // 91
	}

	for _, phone := range validCombos {
		t.Run(phone, func(t *testing.T) {
			err := ValidateNigerianPhone(phone)
			assert.NoError(t, err)
		})
	}
}

func TestValidateNigerianPhone_EdgeCases(t *testing.T) {
	t.Run("All zeros after area code", func(t *testing.T) {
		err := ValidateNigerianPhone("+2348000000000")
		assert.NoError(t, err, "Technically valid (though unlikely real number)")
	})

	t.Run("All nines after area code", func(t *testing.T) {
		err := ValidateNigerianPhone("+2348099999999")
		assert.NoError(t, err)
	})

	t.Run("Sequential digits", func(t *testing.T) {
		err := ValidateNigerianPhone("+2348012345678")
		assert.NoError(t, err)
	})

	t.Run("Repeated digit pattern", func(t *testing.T) {
		err := ValidateNigerianPhone("+2348011111111")
		assert.NoError(t, err, "Valid pattern even if looks fake")
	})
}

func TestValidateNigerianPhone_CommonMistakes(t *testing.T) {
	t.Run("User types local format", func(t *testing.T) {
		err := ValidateNigerianPhone("08012345678")
		assert.Error(t, err, "Should reject local format without +234")
	})

	t.Run("User adds spaces", func(t *testing.T) {
		err := ValidateNigerianPhone("+234 801 234 5678")
		assert.Error(t, err, "Should reject formatted number with spaces")
	})

	t.Run("User adds country code without plus", func(t *testing.T) {
		err := ValidateNigerianPhone("2348012345678")
		assert.Error(t, err, "Should reject number without + prefix")
	})

	t.Run("User types number with parentheses", func(t *testing.T) {
		err := ValidateNigerianPhone("+234(801)234-5678")
		assert.Error(t, err, "Should reject formatted number with parentheses")
	})
}

func TestValidateNigerianPhone_SecurityTests(t *testing.T) {
	t.Run("SQL injection attempt", func(t *testing.T) {
		err := ValidateNigerianPhone("+234801234567'; DROP TABLE users--")
		assert.Error(t, err, "Should reject SQL injection")
	})

	t.Run("Script injection attempt", func(t *testing.T) {
		err := ValidateNigerianPhone("<script>alert('xss')</script>")
		assert.Error(t, err, "Should reject script injection")
	})

	t.Run("Extremely long input", func(t *testing.T) {
		longInput := "+234801234567" + string(make([]byte, 10000))
		err := ValidateNigerianPhone(longInput)
		assert.Error(t, err, "Should reject extremely long input")
	})
}

func TestValidateNigerianPhone_BoundaryConditions(t *testing.T) {
	t.Run("Exactly 14 characters (valid)", func(t *testing.T) {
		phone := "+2348012345678" // Exactly 14 chars
		assert.Len(t, phone, 14)
		err := ValidateNigerianPhone(phone)
		assert.NoError(t, err)
	})

	t.Run("13 characters (too short)", func(t *testing.T) {
		phone := "+234801234567" // 13 chars
		assert.Len(t, phone, 13)
		err := ValidateNigerianPhone(phone)
		assert.Error(t, err)
	})

	t.Run("15 characters (too long)", func(t *testing.T) {
		phone := "+23480123456789" // 15 chars
		assert.Len(t, phone, 15)
		err := ValidateNigerianPhone(phone)
		assert.Error(t, err)
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// BENCHMARK TESTS
// ═══════════════════════════════════════════════════════════════════════════

func BenchmarkValidateNigerianPhone_Valid(b *testing.B) {
	phone := "+2348012345678"
	for i := 0; i < b.N; i++ {
		_ = ValidateNigerianPhone(phone)
	}
}

func BenchmarkValidateNigerianPhone_Invalid(b *testing.B) {
	phone := "08012345678" // Invalid (missing +234)
	for i := 0; i < b.N; i++ {
		_ = ValidateNigerianPhone(phone)
	}
}
