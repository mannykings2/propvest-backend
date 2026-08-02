package password

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// Package password provides utilities for securely hashing and verifying passwords.
// We use bcrypt, which is specifically designed for password hashing.
//
// Why bcrypt?
//   1. Slow by design - makes brute-force attacks expensive
//   2. Automatic salt - each password gets a unique random salt
//   3. Configurable cost - can increase difficulty as computers get faster
//   4. Battle-tested - used by millions of applications for 20+ years
//
// Alternative algorithms:
//   - Argon2 (better but newer, less battle-tested)
//   - PBKDF2 (acceptable but slower than bcrypt for same security)
//   - SHA256 (NEVER use for passwords - too fast, no salt)

// Common errors
var (
	// ErrPasswordTooLong prevents DoS attacks where attacker sends massive password
	// bcrypt has a 72-byte limit, but we enforce 100 characters for safety
	ErrPasswordTooLong = errors.New("password exceeds maximum length of 100 characters")

	// ErrInvalidPassword means the password doesn't match the hash
	ErrInvalidPassword = errors.New("invalid password")
)

// Constants for password policy
const (
	// MaxPasswordLength prevents DoS attacks where attacker sends gigabyte passwords
	// This would make bcrypt consume excessive CPU and memory
	MaxPasswordLength = 100

	// MinPasswordLength enforces basic security
	// Enforced at validation layer (DTOs), not here
	MinPasswordLength = 12

	// DefaultCost is the bcrypt work factor
	// Cost 10 = 2^10 = 1,024 iterations
	// Cost 12 = 2^12 = 4,096 iterations (4x slower, 4x more secure)
	//
	// Cost recommendations:
	//   - 10: Fast, acceptable for development
	//   - 12: Recommended for production (2023)
	//   - 14: High security, slower UX
	//
	// Rule of thumb: hash should take ~250ms on your server
	// As computers get faster, increase cost every few years
	DefaultCost = 10
)

// Hash converts a plain-text password into a bcrypt hash
// The hash can be safely stored in the database
//
// Parameters:
//   - plainPassword: The user's password in plain text
//
// Returns:
//   - Bcrypt hash string (60 characters, starts with "$2a$" or "$2b$")
//   - Error if password is too long or hashing fails
//
// Security properties:
//   1. Same password generates different hash each time (random salt)
//   2. Hash cannot be reversed to get original password
//   3. Only way to verify is to hash the input and compare
//
// Example:
//   hash1, _ := Hash("password123")  // $2a$10$N9qo8uLOickgx2ZMRZoMye...
//   hash2, _ := Hash("password123")  // $2a$10$DifferentSaltDifferentHash...
//   // Same password, different hashes - this is correct!
func Hash(plainPassword string) (string, error) {
	// Enforce maximum length to prevent DoS
	// bcrypt internally truncates at 72 bytes, but we want to reject early
	if len(plainPassword) > MaxPasswordLength {
		return "", ErrPasswordTooLong
	}

	// bcrypt.GenerateFromPassword does all the work:
	//   1. Generates a random salt (16 bytes)
	//   2. Combines password + salt
	//   3. Runs bcrypt key derivation function 2^cost times
	//   4. Encodes result in standard bcrypt format
	//
	// The hash includes:
	//   - Algorithm identifier ($2a$, $2b$, or $2y$)
	//   - Cost parameter (10 = 2^10 iterations)
	//   - Salt (22 base64 characters)
	//   - Hash (31 base64 characters)
	//
	// Format: $2a$10$salt22chars$hash31chars
	// Total length: 60 characters
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(plainPassword), DefaultCost)
	if err != nil {
		return "", err
	}

	// Convert []byte to string for database storage
	return string(hashedBytes), nil
}

// Verify checks if a plain-text password matches a bcrypt hash
// This is called during login to authenticate users
//
// Parameters:
//   - plainPassword: The password the user submitted (from login form)
//   - hashedPassword: The bcrypt hash from the database (users.password_hash)
//
// Returns:
//   - nil if password matches (authentication success)
//   - ErrInvalidPassword if password doesn't match
//   - Other error if hash is malformed
//
// How it works:
//   1. Extract salt from the stored hash
//   2. Hash the plain password with the same salt and cost
//   3. Compare the new hash with the stored hash
//   4. Return match result
//
// Security properties:
//   1. Constant-time comparison (prevents timing attacks)
//   2. No information leakage if password is wrong
//   3. Cannot extract salt to precompute rainbow tables (salt is public but unique per hash)
//
// Example usage in service:
//   err := password.Verify(loginRequest.Password, user.PasswordHash)
//   if err != nil {
//       return errors.New("invalid email or password")  // Don't reveal which is wrong!
//   }
//   // Password correct, proceed with login
func Verify(plainPassword, hashedPassword string) error {
	// bcrypt.CompareHashAndPassword does:
	//   1. Parse the hash to extract algorithm, cost, and salt
	//   2. Hash the plain password with those same parameters
	//   3. Compare the result with the stored hash in constant time
	//
	// Constant-time comparison means:
	//   Even if attacker can measure response time, they can't tell
	//   how close their guess was. "wrong" password and "close" password
	//   take the same time to reject.
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
	if err != nil {
		// bcrypt returns bcrypt.ErrMismatchedHashAndPassword if password is wrong
		// We convert this to our own error type for consistency
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrInvalidPassword
		}
		// Any other error means the hash is malformed or corrupted
		return err
	}

	// Password matches!
	return nil
}

// NeedsRehash checks if a hash was created with an old cost factor
// This allows gradual migration to stronger hashing as computers get faster
//
// Usage:
//   if password.NeedsRehash(user.PasswordHash) {
//       // User logged in successfully, but hash is old
//       // Rehash with current cost and update database
//       newHash, _ := password.Hash(plainPassword)
//       db.UpdateUserPassword(user.ID, newHash)
//   }
//
// When to use:
//   - After successful login (user just proved they know the password)
//   - During password change (you have the plain password anyway)
//
// This is how you upgrade from cost 10 to cost 12 without forcing password resets
func NeedsRehash(hashedPassword string) bool {
	// bcrypt.Cost extracts the cost parameter from the hash string
	// For "$2a$10$...", it returns 10
	// For "$2a$12$...", it returns 12
	cost, err := bcrypt.Cost([]byte(hashedPassword))
	if err != nil {
		// If we can't read the cost, assume rehash is needed
		return true
	}

	// If current hash uses a cost lower than our default, it should be rehashed
	return cost < DefaultCost
}
