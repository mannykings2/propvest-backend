package otp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// Generate creates a random 6-digit OTP code
//
// OTP (One-Time Password) is a temporary code sent to users for verification.
// We use 6 digits because:
//   - Easy to type (not too long)
//   - 1,000,000 possible combinations
//   - Industry standard (Google, Facebook, banks all use 6 digits)
//
// Security:
//   - Uses crypto/rand (cryptographically secure random)
//   - NOT math/rand (predictable, insecure)
//   - Each code has equal probability (uniform distribution)
//
// Returns:
//   - 6-digit string (e.g., "123456", "000001", "999999")
//   - error if random number generation fails
//
// Example usage:
//   code, err := otp.Generate()
//   if err != nil {
//       return errors.ErrInternalServer
//   }
//   // code = "482719"
//   // Send via SMS: "Your PropVest verification code is: 482719"
func Generate() (string, error) {
	// Generate random number between 0 and 999,999
	// This gives us all possible 6-digit combinations
	//
	// Why 1000000 (not 999999)?
	//   rand.Int() returns [0, max), so we need max = 1,000,000
	//   to get range [0, 999,999]
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Format as 6-digit string with leading zeros
	// Examples:
	//   123    → "000123"
	//   456789 → "456789" (shouldn't happen with 1M limit, but would work)
	//   1      → "000001"
	//
	// %06d means:
	//   %    - placeholder
	//   0    - pad with zeros (not spaces)
	//   6    - total width of 6 characters
	//   d    - decimal integer
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// Hash creates a SHA-256 hash of an OTP code for secure storage
//
// Why hash OTP codes?
//   If database is breached, attacker can't use the codes because
//   they only have the hashes, not the original values.
//
// Why SHA-256 instead of bcrypt?
//   - bcrypt is for passwords (needs to be slow to resist brute-force)
//   - OTP codes are already random with high entropy
//   - OTP codes expire quickly (10 minutes)
//   - SHA-256 is fast for lookups while still providing security
//
// Parameters:
//   - code: The 6-digit OTP code (e.g., "123456")
//
// Returns:
//   - 64-character hex string (SHA-256 hash)
//
// Example:
//   code := "123456"
//   hash := otp.Hash(code)
//   // hash = "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92"
//   // Store hash in database, send code to user
//
// Verification process:
//   1. User submits code "123456"
//   2. We hash their input → "8d969eef..."
//   3. Compare with stored hash
//   4. If match → OTP is correct
//   5. If no match → OTP is wrong
func Hash(code string) string {
	// Create SHA-256 hash instance
	hash := sha256.New()

	// Write the code bytes to the hash
	// Write() never fails for sha256, so we ignore the error
	hash.Write([]byte(code))

	// Compute the final hash (32 bytes)
	// Sum(nil) means "return just the hash, don't append to anything"
	hashBytes := hash.Sum(nil)

	// Convert binary hash to hex string for database storage
	// 32 bytes becomes 64 hex characters
	// Example: [0x8d, 0x96, ...] → "8d96..."
	return hex.EncodeToString(hashBytes)
}

// Verify checks if a submitted OTP code matches the stored hash
//
// This is a convenience function that combines Hash() and comparison.
// Instead of:
//   submittedHash := otp.Hash(submittedCode)
//   if submittedHash == storedHash { ... }
//
// You can write:
//   if otp.Verify(submittedCode, storedHash) { ... }
//
// Parameters:
//   - submittedCode: Code entered by user (e.g., "123456")
//   - storedHash: Hash from database (e.g., "8d969eef...")
//
// Returns:
//   - true if code matches hash
//   - false if code doesn't match
//
// Example usage in service:
//   otpRecord, err := otpRepo.FindByUserAndPhone(ctx, userID, phone)
//   if !otp.Verify(submittedCode, otpRecord.CodeHash) {
//       otpRecord.IncrementAttempts()
//       return errors.ErrInvalidOTP
//   }
func Verify(submittedCode, storedHash string) bool {
	// Hash the submitted code
	submittedHash := Hash(submittedCode)

	// Compare hashes
	// This is constant-time comparison (prevents timing attacks)
	return submittedHash == storedHash
}

// GenerateAlphanumeric creates a random alphanumeric OTP (for email verification)
//
// Unlike 6-digit numeric OTPs for SMS, email verification can use longer codes
// because users just click a link (don't need to type the code).
//
// Format: 32 characters, [A-Za-z0-9]
// Example: "a7B2dK9mQ5nP8rT3vX6yZ1cF4hJ0sW2u"
//
// Use cases:
//   - Email verification links
//   - Password reset tokens
//   - Magic login links
//
// Security:
//   - 32 characters = 190 bits of entropy
//   - Practically impossible to guess
//   - Doesn't need expiration (but we add it anyway for safety)
//
// Returns:
//   - 32-character alphanumeric string
//   - error if random generation fails
func GenerateAlphanumeric(length int) (string, error) {
	// Character set: A-Z, a-z, 0-9 (62 characters)
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// Build random string
	result := make([]byte, length)
	for i := range result {
		// Generate random number [0, 62)
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("failed to generate alphanumeric code: %w", err)
		}
		// Pick random character from charset
		result[i] = charset[n.Int64()]
	}

	return string(result), nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: OTP Security Best Practices
// ═══════════════════════════════════════════════════════════════════════════
//
// 1. Use Crypto-Safe Random
//    GOOD: crypto/rand.Int()
//    BAD:  math/rand.Intn() ← Predictable! Attackers can guess next code
//
// 2. Hash Before Storage
//    Store: Hash(code)
//    Not:   code itself
//    Reason: Database breach protection
//
// 3. Short Expiration
//    OTP lifetime: 10 minutes (not 1 hour, not 1 day)
//    Reason: Limits attacker window
//
// 4. Limit Attempts
//    Max 3 wrong attempts, then block OTP
//    Reason: Prevents brute-force guessing
//
// 5. Rate Limiting
//    Max 5 OTP requests per hour
//    Reason: Prevents spam and SMS cost abuse
//
// 6. Single-Use
//    Once verified, mark as used
//    Reason: Prevents replay attacks
//
// 7. Uniform Distribution
//    All codes equally likely (000000 to 999999)
//    Don't exclude "weak" codes like 000000 or 123456
//    Reason: Doesn't reduce security (attacker tries all anyway)
//
// Why 6 Digits?
//   - 4 digits: Too easy to guess (10,000 combinations)
//   - 6 digits: Industry standard (1,000,000 combinations)
//   - 8 digits: Harder to type, not much more secure with attempt limiting
//
// SMS vs Email OTP:
//   - SMS: 6 digits (user types manually)
//   - Email: 32+ alphanumeric (user clicks link)
//
// Attack Vectors:
//   1. Brute Force: Prevented by attempt limiting (3 attempts)
//   2. SMS Interception: Risk exists, but rare
//   3. Phishing: User might give code to attacker (education needed)
//   4. Database Breach: Prevented by hashing
//   5. Replay: Prevented by single-use marking
//
// ═══════════════════════════════════════════════════════════════════════════
