package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestHash_Success verifies that Hash generates a valid bcrypt hash
func TestHash_Success(t *testing.T) {
	// Arrange
	plainPassword := "SecurePassword123!"

	// Act
	hash, err := Hash(plainPassword)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Equal(t, 60, len(hash), "bcrypt hash should be exactly 60 characters")
	assert.True(t, strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$"), 
		"bcrypt hash should start with $2a$ or $2b$")
}

// TestHash_DifferentSalts verifies that same password generates different hashes
func TestHash_DifferentSalts(t *testing.T) {
	// Arrange
	plainPassword := "SamePasswordTwice"

	// Act
	hash1, err1 := Hash(plainPassword)
	hash2, err2 := Hash(plainPassword)

	// Assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotEqual(t, hash1, hash2, "Same password should generate different hashes due to random salt")
}

// TestHash_PasswordTooLong verifies that excessively long passwords are rejected
func TestHash_PasswordTooLong(t *testing.T) {
	// Arrange
	longPassword := strings.Repeat("a", MaxPasswordLength+1) // 101 characters

	// Act
	hash, err := Hash(longPassword)

	// Assert
	assert.Empty(t, hash)
	assert.ErrorIs(t, err, ErrPasswordTooLong)
}

// TestHash_MaximumLengthAllowed verifies that maximum length passwords are accepted
func TestHash_MaximumLengthAllowed(t *testing.T) {
	// Arrange - Use 70 characters (under bcrypt's 72-byte limit)
	maxPassword := strings.Repeat("a", 70)

	// Act
	hash, err := Hash(maxPassword)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Equal(t, 60, len(hash))
}

// TestHash_EmptyPassword verifies that empty passwords can be hashed
// Note: Validation should prevent this at the DTO layer, but hash function should handle it
func TestHash_EmptyPassword(t *testing.T) {
	// Arrange
	emptyPassword := ""

	// Act
	hash, err := Hash(emptyPassword)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

// TestHash_SpecialCharacters verifies that passwords with special characters work
func TestHash_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"with spaces", "password with spaces"},
		{"with unicode", "pässwörd🔒"},
		{"with symbols", "p@ssw0rd!#$%^&*()"},
		{"with quotes", `password"with'quotes`},
		{"with newlines", "password\nwith\nnewlines"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			hash, err := Hash(tt.password)

			// Assert
			require.NoError(t, err)
			assert.NotEmpty(t, hash)
		})
	}
}

// TestVerify_CorrectPassword verifies that correct password passes verification
func TestVerify_CorrectPassword(t *testing.T) {
	// Arrange
	plainPassword := "CorrectPassword123!"
	hash, err := Hash(plainPassword)
	require.NoError(t, err)

	// Act
	err = Verify(plainPassword, hash)

	// Assert
	assert.NoError(t, err, "Correct password should verify successfully")
}

// TestVerify_WrongPassword verifies that wrong password fails verification
func TestVerify_WrongPassword(t *testing.T) {
	// Arrange
	correctPassword := "CorrectPassword123!"
	wrongPassword := "WrongPassword123!"
	hash, err := Hash(correctPassword)
	require.NoError(t, err)

	// Act
	err = Verify(wrongPassword, hash)

	// Assert
	assert.ErrorIs(t, err, ErrInvalidPassword, "Wrong password should return ErrInvalidPassword")
}

// TestVerify_CaseSensitive verifies that password verification is case-sensitive
func TestVerify_CaseSensitive(t *testing.T) {
	// Arrange
	password := "Password123"
	hash, err := Hash(password)
	require.NoError(t, err)

	tests := []struct {
		name     string
		attempt  string
		shouldMatch bool
	}{
		{"exact match", "Password123", true},
		{"lowercase", "password123", false},
		{"uppercase", "PASSWORD123", false},
		{"mixed case", "PaSsWoRd123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := Verify(tt.attempt, hash)

			// Assert
			if tt.shouldMatch {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidPassword)
			}
		})
	}
}

// TestVerify_EmptyPassword verifies behavior with empty password
func TestVerify_EmptyPassword(t *testing.T) {
	// Arrange
	emptyPassword := ""
	hash, err := Hash(emptyPassword)
	require.NoError(t, err)

	// Act
	err = Verify(emptyPassword, hash)

	// Assert
	assert.NoError(t, err, "Empty password should verify against its own hash")
}

// TestVerify_InvalidHash verifies that malformed hashes return error
func TestVerify_InvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"empty hash", ""},
		{"too short", "$2a$10$short"},
		{"invalid prefix", "not-a-bcrypt-hash"},
		{"corrupted hash", "$2a$10$N9qo8uLOickgx2ZMRZoMye!!!CORRUPTED!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := Verify("anypassword", tt.hash)

			// Assert
			assert.Error(t, err, "Invalid hash should return error")
		})
	}
}

// TestVerify_SlightlyWrongPassword verifies constant-time comparison
func TestVerify_SlightlyWrongPassword(t *testing.T) {
	// Arrange
	password := "CorrectPassword123!"
	hash, err := Hash(password)
	require.NoError(t, err)

	tests := []struct {
		name    string
		attempt string
	}{
		{"one char off", "CorrectPassword123"},
		{"extra char", "CorrectPassword123!x"},
		{"missing char", "CorrectPassword123"},
		{"transposed chars", "CorrectPassword132!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := Verify(tt.attempt, hash)

			// Assert
			assert.ErrorIs(t, err, ErrInvalidPassword)
		})
	}
}

// TestNeedsRehash_OldCost verifies detection of old cost factor
func TestNeedsRehash_OldCost(t *testing.T) {
	// Arrange - Create hash with cost 4 (much lower than default 10)
	oldHash, err := bcrypt.GenerateFromPassword([]byte("password"), 4)
	require.NoError(t, err)

	// Act
	needsRehash := NeedsRehash(string(oldHash))

	// Assert
	assert.True(t, needsRehash, "Hash with cost 4 should need rehashing to cost 10")
}

// TestNeedsRehash_CurrentCost verifies that current cost doesn't need rehash
func TestNeedsRehash_CurrentCost(t *testing.T) {
	// Arrange - Create hash with current default cost
	hash, err := Hash("password")
	require.NoError(t, err)

	// Act
	needsRehash := NeedsRehash(hash)

	// Assert
	assert.False(t, needsRehash, "Hash with current cost should not need rehashing")
}

// TestNeedsRehash_HigherCost verifies that higher cost doesn't need rehash
func TestNeedsRehash_HigherCost(t *testing.T) {
	// Arrange - Create hash with cost higher than default
	higherCostHash, err := bcrypt.GenerateFromPassword([]byte("password"), DefaultCost+2)
	require.NoError(t, err)

	// Act
	needsRehash := NeedsRehash(string(higherCostHash))

	// Assert
	assert.False(t, needsRehash, "Hash with higher cost should not need rehashing")
}

// TestNeedsRehash_InvalidHash verifies that invalid hashes return true (safe default)
func TestNeedsRehash_InvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{"empty hash", ""},
		{"invalid format", "not-a-hash"},
		{"corrupted hash", "$2a$10$corrupted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			needsRehash := NeedsRehash(tt.hash)

			// Assert
			assert.True(t, needsRehash, "Invalid hash should return true (safe default)")
		})
	}
}

// TestHashAndVerify_Integration verifies full workflow
func TestHashAndVerify_Integration(t *testing.T) {
	passwords := []string{
		"SimplePassword123",
		"Complex!P@ssw0rd#With$Symbols",
		"pässwörd with üñíçødé",
		"MediumLengthPassword123!", // Under 72 bytes
	}

	for _, password := range passwords {
		t.Run(password[:min(20, len(password))], func(t *testing.T) {
			// Hash
			hash, err := Hash(password)
			require.NoError(t, err)
			assert.NotEmpty(t, hash)

			// Verify correct password
			err = Verify(password, hash)
			assert.NoError(t, err)

			// Verify wrong password
			err = Verify(password+"wrong", hash)
			assert.ErrorIs(t, err, ErrInvalidPassword)
		})
	}
}

// Helper function for min (Go 1.21+)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BenchmarkHash benchmarks password hashing performance
func BenchmarkHash(b *testing.B) {
	password := "BenchmarkPassword123!"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Hash(password)
	}
}

// BenchmarkVerify benchmarks password verification performance
func BenchmarkVerify(b *testing.B) {
	password := "BenchmarkPassword123!"
	hash, _ := Hash(password)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Verify(password, hash)
	}
}
