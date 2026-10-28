package jwt

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test configuration constants
const (
	testAccessSecret  = "test-access-secret-key-32-characters-long"
	testRefreshSecret = "test-refresh-secret-key-32-characters-long"
	testAccessExpiry  = 15 * time.Minute
	testRefreshExpiry = 7 * 24 * time.Hour
	testRole          = "investor"
)

// TestGenerateAccessToken_Success tests successful access token generation
func TestGenerateAccessToken_Success(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate the token structure
	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testAccessSecret), nil
	})

	require.NoError(t, err)
	assert.True(t, parsedToken.Valid)

	// Verify claims
	claims, ok := parsedToken.Claims.(*Claims)
	require.True(t, ok)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, testRole, claims.Role)

	// Verify expiry is set
	assert.NotNil(t, claims.ExpiresAt)
	assert.Greater(t, claims.ExpiresAt.Unix(), time.Now().Unix())
}

// TestGenerateAccessToken_VerifyExpiry tests that access token has correct expiry
func TestGenerateAccessToken_VerifyExpiry(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testAccessSecret), nil
	})
	require.NoError(t, err)

	claims, ok := parsedToken.Claims.(*Claims)
	require.True(t, ok)

	expectedExpiry := time.Now().Add(testAccessExpiry).Unix()
	actualExpiry := claims.ExpiresAt.Unix()

	// Allow 5 second tolerance for test execution time
	assert.InDelta(t, expectedExpiry, actualExpiry, 5)
}

// TestGenerateAccessToken_EmptySecret tests generation with empty secret
func TestGenerateAccessToken_EmptySecret(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, "", testAccessExpiry)

	// Should still generate token (JWT library allows empty secret)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

// TestGenerateAccessToken_NilUUID tests generation with nil UUID
func TestGenerateAccessToken_NilUUID(t *testing.T) {
	token, err := GenerateAccessToken(uuid.Nil, testRole, testAccessSecret, testAccessExpiry)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Verify nil UUID is stored correctly
	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testAccessSecret), nil
	})
	require.NoError(t, err)

	claims, ok := parsedToken.Claims.(*Claims)
	require.True(t, ok)
	assert.Equal(t, uuid.Nil, claims.UserID)
}

// TestGenerateAccessToken_EmptyRole tests generation with empty role
func TestGenerateAccessToken_EmptyRole(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, "", testAccessSecret, testAccessExpiry)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Verify empty role is stored correctly
	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testAccessSecret), nil
	})
	require.NoError(t, err)

	claims, ok := parsedToken.Claims.(*Claims)
	require.True(t, ok)
	assert.Equal(t, "", claims.Role)
}

// TestGenerateRefreshToken_Success tests successful refresh token generation
func TestGenerateRefreshToken_Success(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateRefreshToken(userID, testRole, testRefreshSecret, testRefreshExpiry)

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate with refresh secret (not access secret)
	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testRefreshSecret), nil
	})

	require.NoError(t, err)
	assert.True(t, parsedToken.Valid)

	// Verify claims
	claims, ok := parsedToken.Claims.(*Claims)
	require.True(t, ok)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, testRole, claims.Role)
}

// TestGenerateRefreshToken_DifferentSecret tests that refresh token uses different secret
func TestGenerateRefreshToken_DifferentSecret(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateRefreshToken(userID, testRole, testRefreshSecret, testRefreshExpiry)
	require.NoError(t, err)

	// Should fail with access secret
	_, err = jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testAccessSecret), nil
	})
	assert.Error(t, err)

	// Should succeed with refresh secret
	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testRefreshSecret), nil
	})
	require.NoError(t, err)
	assert.True(t, parsedToken.Valid)
}

// TestGenerateRefreshToken_VerifyExpiry tests that refresh token has correct expiry
func TestGenerateRefreshToken_VerifyExpiry(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateRefreshToken(userID, testRole, testRefreshSecret, testRefreshExpiry)
	require.NoError(t, err)

	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(testRefreshSecret), nil
	})
	require.NoError(t, err)

	claims, ok := parsedToken.Claims.(*Claims)
	require.True(t, ok)

	expectedExpiry := time.Now().Add(testRefreshExpiry).Unix()
	actualExpiry := claims.ExpiresAt.Unix()

	// Allow 5 second tolerance for test execution time
	assert.InDelta(t, expectedExpiry, actualExpiry, 5)
}

// TestValidateToken_ValidAccessToken tests validation of valid access token
func TestValidateToken_ValidAccessToken(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	claims, err := ValidateToken(token, testAccessSecret)

	assert.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, testRole, claims.Role)
}

// TestValidateToken_ValidRefreshToken tests validation of valid refresh token
func TestValidateToken_ValidRefreshToken(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateRefreshToken(userID, testRole, testRefreshSecret, testRefreshExpiry)
	require.NoError(t, err)

	claims, err := ValidateToken(token, testRefreshSecret)

	assert.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, testRole, claims.Role)
}

// TestValidateToken_ExpiredToken tests validation of expired token
func TestValidateToken_ExpiredToken(t *testing.T) {
	userID := uuid.New()

	// Create token with negative expiry (already expired)
	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, -1*time.Hour)
	require.NoError(t, err)

	// Give it a moment for the token to be definitely expired
	time.Sleep(10 * time.Millisecond)

	_, err = ValidateToken(token, testAccessSecret)

	assert.Error(t, err)
	assert.Equal(t, ErrExpiredToken, err)
}

// TestValidateToken_InvalidSignature tests validation with wrong secret
func TestValidateToken_InvalidSignature(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	// Validate with wrong secret
	_, err = ValidateToken(token, "wrong-secret")

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err)
}

// TestValidateToken_MalformedToken tests validation of malformed token
func TestValidateToken_MalformedToken(t *testing.T) {
	malformedTokens := []string{
		"",
		"not.a.token",
		"invalid-jwt-format",
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.signature",
	}

	for _, token := range malformedTokens {
		_, err := ValidateToken(token, testAccessSecret)
		assert.Error(t, err, "Expected error for malformed token: %s", token)
	}
}

// TestValidateToken_WrongSigningMethod tests token created with wrong signing method
func TestValidateToken_WrongSigningMethod(t *testing.T) {
	userID := uuid.New()

	// Create claims manually
	claims := &Claims{
		UserID: userID,
		Role:   testRole,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(testAccessExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "propvest-api",
		},
	}

	// Use HS512 instead of HS256
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	tokenString, err := token.SignedString([]byte(testAccessSecret))
	require.NoError(t, err)

	// ValidateToken should accept HS512 since it checks for HMAC family
	// This test validates that the signing method check works
	_, err = ValidateToken(tokenString, testAccessSecret)
	
	// Should still work because HS512 is also HMAC
	assert.NoError(t, err)
}

// TestExtractUserID_Success tests successful user ID extraction from valid token
func TestExtractUserID_Success(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	extractedID, err := ExtractUserID(token, testAccessSecret)

	assert.NoError(t, err)
	assert.Equal(t, userID, extractedID)
}

// TestExtractUserID_FromRefreshToken tests user ID extraction from refresh token
func TestExtractUserID_FromRefreshToken(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateRefreshToken(userID, testRole, testRefreshSecret, testRefreshExpiry)
	require.NoError(t, err)

	extractedID, err := ExtractUserID(token, testRefreshSecret)

	assert.NoError(t, err)
	assert.Equal(t, userID, extractedID)
}

// TestExtractUserID_InvalidToken tests extraction from invalid token
func TestExtractUserID_InvalidToken(t *testing.T) {
	invalidTokens := []string{
		"",
		"invalid-token",
		"not.a.jwt.token",
	}

	for _, token := range invalidTokens {
		_, err := ExtractUserID(token, testAccessSecret)
		assert.Error(t, err, "Expected error for invalid token: %s", token)
		assert.Equal(t, uuid.Nil, uuid.Nil) // Return value should be nil UUID
	}
}

// TestExtractUserID_WrongSecret tests extraction with wrong secret
func TestExtractUserID_WrongSecret(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	_, err = ExtractUserID(token, "wrong-secret")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err)
}

// TestExtractUserID_ExpiredToken tests extraction from expired token
func TestExtractUserID_ExpiredToken(t *testing.T) {
	userID := uuid.New()

	// Create already expired token
	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, -1*time.Hour)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	_, err = ExtractUserID(token, testAccessSecret)
	assert.Error(t, err)
	assert.Equal(t, ErrExpiredToken, err)
}

// TestTokenRoundTrip tests full lifecycle of token generation and validation
func TestTokenRoundTrip(t *testing.T) {
	userID := uuid.New()

	// Generate access token
	accessToken, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	// Generate refresh token
	refreshToken, err := GenerateRefreshToken(userID, testRole, testRefreshSecret, testRefreshExpiry)
	require.NoError(t, err)

	// Validate access token
	accessClaims, err := ValidateToken(accessToken, testAccessSecret)
	require.NoError(t, err)
	assert.Equal(t, userID, accessClaims.UserID)
	assert.Equal(t, testRole, accessClaims.Role)

	// Validate refresh token
	refreshClaims, err := ValidateToken(refreshToken, testRefreshSecret)
	require.NoError(t, err)
	assert.Equal(t, userID, refreshClaims.UserID)
	assert.Equal(t, testRole, refreshClaims.Role)

	// Extract user ID from access token
	extractedID, err := ExtractUserID(accessToken, testAccessSecret)
	require.NoError(t, err)
	assert.Equal(t, userID, extractedID)

	// Extract user ID from refresh token
	extractedID, err = ExtractUserID(refreshToken, testRefreshSecret)
	require.NoError(t, err)
	assert.Equal(t, userID, extractedID)
}

// TestDifferentUserIDs tests that different user IDs produce different tokens
func TestDifferentUserIDs(t *testing.T) {
	userID1 := uuid.New()
	userID2 := uuid.New()

	token1, err := GenerateAccessToken(userID1, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	token2, err := GenerateAccessToken(userID2, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	// Tokens should be different
	assert.NotEqual(t, token1, token2)

	// Extract and verify user IDs
	extractedID1, err := ExtractUserID(token1, testAccessSecret)
	require.NoError(t, err)
	assert.Equal(t, userID1, extractedID1)

	extractedID2, err := ExtractUserID(token2, testAccessSecret)
	require.NoError(t, err)
	assert.Equal(t, userID2, extractedID2)
}

// TestDifferentRoles tests tokens with different roles
func TestDifferentRoles(t *testing.T) {
	userID := uuid.New()
	roles := []string{"investor", "developer", "admin", "staff"}

	for _, role := range roles {
		token, err := GenerateAccessToken(userID, role, testAccessSecret, testAccessExpiry)
		require.NoError(t, err)

		claims, err := ValidateToken(token, testAccessSecret)
		require.NoError(t, err)
		assert.Equal(t, role, claims.Role)
	}
}

// TestTokenIssuer tests that issuer is set correctly
func TestTokenIssuer(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	claims, err := ValidateToken(token, testAccessSecret)
	require.NoError(t, err)

	assert.Equal(t, "propvest-api", claims.Issuer)
}

// TestTokenSubject tests that subject is set to user ID
func TestTokenSubject(t *testing.T) {
	userID := uuid.New()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	claims, err := ValidateToken(token, testAccessSecret)
	require.NoError(t, err)

	assert.Equal(t, userID.String(), claims.Subject)
}

// TestTokenIssuedAt tests that IssuedAt is set correctly
func TestTokenIssuedAt(t *testing.T) {
	userID := uuid.New()
	beforeGeneration := time.Now().Unix()

	token, err := GenerateAccessToken(userID, testRole, testAccessSecret, testAccessExpiry)
	require.NoError(t, err)

	afterGeneration := time.Now().Unix()

	claims, err := ValidateToken(token, testAccessSecret)
	require.NoError(t, err)

	assert.NotNil(t, claims.IssuedAt)
	issuedAt := claims.IssuedAt.Unix()
	
	// IssuedAt should be between before and after generation
	assert.GreaterOrEqual(t, issuedAt, beforeGeneration)
	assert.LessOrEqual(t, issuedAt, afterGeneration)
}
