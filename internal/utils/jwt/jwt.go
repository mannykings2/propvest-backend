package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Package jwt provides utilities for creating and validating JSON Web Tokens (JWTs).
// JWTs are used for stateless authentication - the token itself contains the claims
// (user ID, role, expiration) so the server doesn't need to store session data.
//
// Two types of tokens:
//   1. Access Token - Short-lived (15 minutes), used for API requests
//   2. Refresh Token - Long-lived (7-30 days), used to get new access tokens
//
// Security principles:
//   - Access tokens are signed with JWT_SECRET
//   - Refresh tokens are signed with JWT_REFRESH_SECRET (different key!)
//   - Using different secrets means if one is compromised, the other remains secure
//   - Tokens are verified on every request by middleware

// Common errors that can occur during JWT operations
var (
	// ErrInvalidToken means the token string is malformed or signature is wrong
	ErrInvalidToken = errors.New("invalid token")

	// ErrExpiredToken means the token's exp claim is in the past
	ErrExpiredToken = errors.New("token has expired")

	// ErrInvalidClaims means the token is valid but claims are missing or wrong type
	ErrInvalidClaims = errors.New("invalid token claims")
)

// Claims represents the data stored inside a JWT
// This is what we encode into the token when a user logs in
// and what we extract from the token on every authenticated request
type Claims struct {
	// UserID identifies which user this token belongs to
	// We use UUID instead of email because:
	//   1. User can change email, but ID never changes
	//   2. UUID is shorter than email in the token payload
	UserID uuid.UUID `json:"user_id"`

	// Role determines what the user can access
	// Values: investor | developer | staff | admin
	// Middleware checks this to enforce authorization
	Role string `json:"role"`

	// RegisteredClaims are standard JWT fields defined by RFC 7519:
	//   - sub (Subject): Identifies the principal (user ID as string)
	//   - iat (Issued At): When the token was created
	//   - exp (Expiration Time): When the token expires
	//   - iss (Issuer): Who created the token (e.g., "propvest-api")
	//   - aud (Audience): Who the token is intended for
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a short-lived JWT for API authentication
// Parameters:
//   - userID: UUID of the user
//   - role: User's role (investor, developer, admin, etc.)
//   - secret: JWT_SECRET from environment config
//   - ttl: How long the token is valid (typically 15 minutes)
//
// Returns:
//   - Signed JWT string that the client includes in Authorization header
//   - Error if token generation fails
//
// Usage in service layer:
//   token, err := jwt.GenerateAccessToken(user.ID, user.Role, cfg.JWTSecret, 15*time.Minute)
func GenerateAccessToken(userID uuid.UUID, role string, secret string, ttl time.Duration) (string, error) {
	// time.Now() gives current time
	// Add(ttl) calculates expiration time by adding duration
	// Example: if ttl is 15 minutes, expiresAt will be 15 minutes from now
	expiresAt := time.Now().Add(ttl)

	// Create the claims struct with user data and standard JWT fields
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			// Subject is typically the user identifier as a string
			Subject: userID.String(),

			// ExpiresAt tells when the token becomes invalid
			// NewNumericDate converts time.Time to JWT's numeric date format (Unix timestamp)
			ExpiresAt: jwt.NewNumericDate(expiresAt),

			// IssuedAt records when the token was created
			// Useful for debugging and audit logs
			IssuedAt: jwt.NewNumericDate(time.Now()),

			// Issuer identifies our application as the creator
			// This prevents tokens from one system being used in another
			Issuer: "propvest-api",
		},
	}

	// jwt.NewWithClaims creates a token object
	// jwt.SigningMethodHS256 means we use HMAC-SHA256 for signing
	// HMAC is symmetric encryption - same secret signs and verifies
	//
	// Why HS256 instead of RS256?
	//   - HS256 uses one secret key (simpler, faster)
	//   - RS256 uses public/private key pair (more complex, needed only if
	//     multiple services verify tokens but don't issue them)
	//   - For our monolithic API, HS256 is perfect
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// SignedString creates the final JWT string with three parts:
	//   1. Header (base64): {"alg":"HS256","typ":"JWT"}
	//   2. Payload (base64): {user_id, role, exp, iat, sub, iss}
	//   3. Signature (base64): HMAC-SHA256(header + payload, secret)
	//
	// Result looks like: eyJhbGc.eyJ1c2VyX2.SflKxwRJ
	// Client sends this in: Authorization: Bearer eyJhbGc.eyJ1c2VyX2.SflKxwRJ
	return token.SignedString([]byte(secret))
}

// GenerateRefreshToken creates a long-lived JWT for session renewal
// This is identical to GenerateAccessToken but uses a different secret
//
// Why separate function?
//   1. Different secret (JWT_REFRESH_SECRET vs JWT_SECRET)
//   2. Different TTL (30 days vs 15 minutes)
//   3. Different purpose (get new access token vs call APIs)
//
// Security: If access token secret is compromised, refresh tokens remain secure
func GenerateRefreshToken(userID uuid.UUID, role string, secret string, ttl time.Duration) (string, error) {
	expiresAt := time.Now().Add(ttl)

	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "propvest-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken verifies a JWT and extracts its claims
// This is called by authentication middleware on every protected request
//
// Parameters:
//   - tokenString: The JWT from Authorization header (without "Bearer " prefix)
//   - secret: JWT_SECRET or JWT_REFRESH_SECRET depending on token type
//
// Returns:
//   - Claims if token is valid and not expired
//   - Error if token is invalid, expired, or tampered with
//
// Validation checks:
//   1. Token format is correct (three base64 parts separated by dots)
//   2. Signature matches (token hasn't been tampered with)
//   3. Expiration hasn't passed
//   4. Claims can be parsed
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	// jwt.ParseWithClaims does all the heavy lifting:
	//   1. Decodes the base64 parts
	//   2. Verifies the signature using the secret
	//   3. Checks expiration automatically
	//   4. Parses claims into our Claims struct
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// This callback provides the secret key for verification
		// It's called during parsing to get the key

		// Security check: ensure the token uses the expected signing method
		// This prevents an attacker from changing the algorithm to "none"
		// or switching from HS256 to RS256 with a known public key
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}

		// Return the secret as []byte for HMAC verification
		return []byte(secret), nil
	})

	// Handle parsing errors
	if err != nil {
		// Check if the error is specifically about expiration
		// jwt package returns a specific error type for expired tokens
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		// Any other error means invalid token (bad signature, malformed, etc.)
		return nil, ErrInvalidToken
	}

	// Extract and type-assert the claims
	// token.Claims is interface{}, we need to convert it to *Claims
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		// This happens if:
		//   - Claims are wrong type (shouldn't happen with our code)
		//   - Token is marked invalid by jwt library
		return nil, ErrInvalidClaims
	}

	// Success: token is valid and claims are parsed
	return claims, nil
}

// ExtractUserID is a convenience function to get just the user ID from a token
// Useful when you only need the user ID and don't care about other claims
//
// Example usage in middleware:
//   userID, err := jwt.ExtractUserID(tokenString, cfg.JWTSecret)
//   if err != nil {
//       return c.JSON(401, "Unauthorized")
//   }
//   // Now you have the UUID of the authenticated user
func ExtractUserID(tokenString string, secret string) (uuid.UUID, error) {
	// Validate the token first (this checks signature and expiration)
	claims, err := ValidateToken(tokenString, secret)
	if err != nil {
		// Return zero UUID and the error from ValidateToken
		return uuid.Nil, err
	}

	// Return the user ID from claims
	return claims.UserID, nil
}
