package token

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// Package token provides utilities for generating random tokens
// These are different from JWTs - they are simple random strings
// used for one-time operations like email verification and password reset
//
// Use cases:
//   - Email verification codes: sent in verification email links
//   - Password reset tokens: sent in password reset email links
//   - API keys: long-lived tokens for service-to-service auth
//   - Invite codes: unique codes for user invitations
//
// Security properties:
//   - Cryptographically random (not predictable)
//   - URL-safe (no special characters that need escaping)
//   - High entropy (difficult to guess even with many attempts)

// GenerateRandomToken creates a cryptographically secure random token
// The token is URL-safe and base64 encoded
//
// Parameters:
//   - length: Number of random bytes to generate (NOT the final string length)
//
// Returns:
//   - URL-safe base64 string (actual length will be ~4/3 * input length)
//   - Error if random generation fails (extremely rare)
//
// Length recommendations:
//   - 16 bytes = 22 characters: Short codes (email verification)
//   - 32 bytes = 43 characters: Standard tokens (password reset)
//   - 64 bytes = 86 characters: High security (API keys)
//
// Example:
//   token, _ := GenerateRandomToken(32)
//   // Result: "kJ8jxY2pN9fM3hR7sT4vW6xZ1aB5cD8eF"
//
// Why base64?
//   Converts binary random bytes to text that can be:
//     - Included in URLs
//     - Stored in VARCHAR columns
//     - Sent in JSON
//     - Copied and pasted by users
func GenerateRandomToken(length int) (string, error) {
	// Create a byte slice to hold the random bytes
	// Example: if length is 32, this creates space for 32 random bytes
	bytes := make([]byte, length)

	// crypto/rand.Read fills the slice with cryptographically secure random bytes
	//
	// Why crypto/rand instead of math/rand?
	//   - math/rand is predictable (pseudo-random, for games/simulations)
	//   - crypto/rand uses OS entropy sources (unpredictable, for security)
	//
	// Entropy sources (where randomness comes from):
	//   - Windows: CryptGenRandom API
	//   - Linux: /dev/urandom
	//   - macOS: /dev/random
	//
	// These sources collect randomness from:
	//   - Hardware interrupts (mouse movements, keyboard timing)
	//   - Network packet arrival times
	//   - Disk seek times
	//   - Thermal noise from CPU
	_, err := rand.Read(bytes)
	if err != nil {
		// This error is extremely rare and indicates:
		//   - OS entropy pool is depleted (shouldn't happen on modern systems)
		//   - System call failed
		//   - Hardware RNG failed
		//
		// If this happens, DO NOT fall back to math/rand - fail securely
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// base64.RawURLEncoding converts bytes to URL-safe base64 string
	//
	// Base64 encoding:
	//   - Converts 3 bytes → 4 characters
	//   - Uses: A-Z, a-z, 0-9, -, _ (URL-safe alphabet)
	//   - No padding (= characters removed with "Raw" encoding)
	//
	// Example transformation:
	//   Binary: 01010101 11001100 10101010
	//   Base64: VcyqKw
	//
	// URL-safe vs standard base64:
	//   Standard: Uses + and / (must be URL-encoded as %2B and %2F)
	//   URL-safe: Uses - and _ (can be used directly in URLs)
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// GenerateNumericCode creates a numeric-only code for user-friendly verification
// These codes are easier to type but have lower entropy than random tokens
//
// Parameters:
//   - length: Number of digits (e.g., 6 for a 6-digit code)
//
// Returns:
//   - String of random digits (e.g., "428391")
//   - Error if random generation fails
//
// Use cases:
//   - SMS verification codes: 6 digits
//   - Two-factor authentication: 6 digits
//   - Backup codes: 8 digits
//
// Security trade-off:
//   - Easier for users to type (no letters, case-sensitivity, or special chars)
//   - Lower entropy: 6 digits = 1 million possibilities (vs 32-byte token = 2^256)
//   - Must be combined with rate limiting (max 5 attempts per hour)
//   - Should expire quickly (5-10 minutes)
//
// Example:
//   code, _ := GenerateNumericCode(6)
//   // Result: "749203"
func GenerateNumericCode(length int) (string, error) {
	// Create byte slice for random data
	// We need 1 random byte per digit
	bytes := make([]byte, length)

	// Fill with cryptographically random bytes
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Convert each random byte to a digit (0-9)
	// We do this by taking modulo 10 of each byte value
	//
	// Example:
	//   Byte value: 157 → 157 % 10 = 7 → digit '7'
	//   Byte value: 42  → 42 % 10  = 2 → digit '2'
	//   Byte value: 201 → 201 % 10 = 1 → digit '1'
	//
	// Note: This introduces slight bias (bytes 0-249 map evenly, 250-255 don't)
	// For security-critical applications, reject bytes >= 250 and regenerate
	// For verification codes, the bias is negligible
	code := make([]byte, length)
	for i := 0; i < length; i++ {
		// bytes[i] is 0-255, modulo 10 gives 0-9
		// Add '0' (ASCII 48) to convert number to character
		// Example: 7 + '0' = 55 (ASCII code for '7')
		code[i] = '0' + (bytes[i] % 10)
	}

	return string(code), nil
}

// MustGenerateRandomToken is like GenerateRandomToken but panics on error
// Use this only in initialization code where you want the app to crash
// if token generation fails (which indicates serious system problems)
//
// Example usage:
//   var apiKey = token.MustGenerateRandomToken(64)
//   // This runs at package initialization
//   // If it fails, the application should not start
//
// Do NOT use this in request handlers - always handle errors gracefully there
func MustGenerateRandomToken(length int) string {
	token, err := GenerateRandomToken(length)
	if err != nil {
		// panic crashes the application with an error message
		// Appropriate for initialization code, never for request handlers
		panic(fmt.Sprintf("failed to generate random token: %v", err))
	}
	return token
}
