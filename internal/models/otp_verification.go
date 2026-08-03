package models

import (
	"time"

	"github.com/google/uuid"
)

// OTPVerification represents a one-time password sent for phone verification
//
// Purpose:
//   When a user wants to change their phone number, we need to verify
//   they actually own the new phone number. We do this by:
//   1. Generating a random 6-digit code (e.g., "123456")
//   2. Sending it via SMS to the new phone
//   3. User enters the code in the app
//   4. We verify the code matches what we sent
//
// Security model:
//   - OTP codes are hashed before storage (like passwords)
//   - Codes expire after 10 minutes
//   - Each code can only be used once (verified_at prevents reuse)
//   - After 3 failed attempts, code is blocked
//   - Rate limiting prevents spam (max 1 OTP per 2 minutes per user)
//
// Example flow:
//   User: "I want to change my phone to +2348012345678"
//   System: Generates OTP "123456", sends SMS, stores hash
//   User: Enters "123456"
//   System: Hashes input, compares with stored hash
//   System: If match → update phone, mark verified_at
//   System: If no match → increment attempt_count
type OTPVerification struct {
	// Primary key - unique identifier for this OTP record
	// type:uuid tells GORM to use PostgreSQL's UUID type
	// primaryKey marks this as the table's primary key
	// default:gen_random_uuid() tells PostgreSQL to auto-generate UUIDs
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// Foreign key linking to the users table
	// This establishes who requested this OTP
	// not null ensures every OTP belongs to someone
	// ON DELETE CASCADE means: if user is deleted, delete their OTP records too
	UserID uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`

	// Phone number this OTP was sent to
	// This is the NEW phone number the user wants to verify
	// Format: E.164 (+2348012345678)
	// We store this separately from User.Phone because:
	//   1. User's current phone might be different
	//   2. We need to know which phone to send SMS to
	//   3. Verification might fail, and we don't want to update User.Phone yet
	Phone string `gorm:"not null" json:"phone"`

	// CodeHash stores the hashed version of the OTP code
	// NEVER store plain OTP codes - always hash them
	//
	// Why hash OTP codes?
	//   If an attacker gets database access, they can't use the OTP codes
	//   because they only have the hashes, not the original values.
	//
	// Example:
	//   Original OTP: "123456"
	//   SHA-256 hash: "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92"
	//   Stored: "8d969eef..."
	//
	// When user submits "123456":
	//   1. We hash their input → "8d969eef..."
	//   2. Compare with stored hash
	//   3. If match → OTP is correct
	CodeHash string `gorm:"not null" json:"-"` // "-" means don't include in JSON

	// When this OTP expires and can no longer be used
	// Typically 10 minutes from creation
	// Application validates: if time.Now() > ExpiresAt, reject the OTP
	//
	// Why 10 minutes?
	//   - Long enough for user to receive SMS and enter code
	//   - Short enough to minimize security window
	//   - Industry standard for OTP expiration
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`

	// VerifiedAt is NULL for unused OTP codes
	// Set to a timestamp when the OTP is successfully verified
	//
	// Once VerifiedAt is set:
	//   - OTP cannot be used again (prevents replay attacks)
	//   - User's phone number is updated
	//   - Frontend can proceed with phone change confirmation
	//
	// Why use *time.Time instead of time.Time?
	//   Plain time.Time defaults to zero-time (0001-01-01), not NULL
	//   *time.Time can be nil, which maps to SQL NULL
	VerifiedAt *time.Time `gorm:"index" json:"verified_at,omitempty"`

	// AttemptCount tracks how many times user tried to verify this OTP
	// Incremented on each failed verification attempt
	// If AttemptCount >= 3, OTP is blocked (prevents brute-force attacks)
	//
	// Example flow:
	//   User enters "111111" → Wrong! AttemptCount = 1
	//   User enters "222222" → Wrong! AttemptCount = 2
	//   User enters "333333" → Wrong! AttemptCount = 3 → BLOCKED
	//   User must request new OTP
	//
	// Why limit to 3 attempts?
	//   - 6-digit OTP has 1,000,000 possible values
	//   - With 3 attempts, attacker has 0.0003% chance of guessing
	//   - Industry standard for OTP verification
	AttemptCount int `gorm:"default:0;not null" json:"attempt_count"`

	// CreatedAt is automatically set by GORM on INSERT
	// Records when this OTP was generated
	CreatedAt time.Time `gorm:"not null" json:"created_at"`

	// UpdatedAt is automatically set by GORM on every UPDATE
	// For OTP, this tracks when verification was attempted
	UpdatedAt time.Time `json:"updated_at"`

	// User is the association to the User model
	// GORM will automatically populate this when you use Preload("User")
	// Example: db.Preload("User").Find(&otp)
	//
	// foreignKey:UserID tells GORM that UserID field references users.id
	// This creates a "belongs to" relationship: OTPVerification belongs to User
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// IsValid checks if the OTP can currently be used for verification
// Returns true only if:
//   1. Not expired (current time is before ExpiresAt)
//   2. Not already verified (VerifiedAt is nil)
//   3. Not blocked by too many attempts (AttemptCount < 3)
//
// This encapsulates the validation business rules in one place
// so every part of the codebase uses the same logic.
//
// Example usage:
//   otp, err := otpRepo.FindByCodeHash(ctx, hash)
//   if err != nil || !otp.IsValid() {
//       return errors.ErrInvalidOTP
//   }
func (otp *OTPVerification) IsValid() bool {
	// Check if OTP is expired
	// time.Now() returns current time in UTC
	// After() returns true if time.Now() is after ExpiresAt
	if time.Now().After(otp.ExpiresAt) {
		return false
	}

	// Check if OTP was already used
	// VerifiedAt is *time.Time, so it can be nil
	// If it's not nil, the OTP has already been verified
	if otp.VerifiedAt != nil {
		return false
	}

	// Check if too many failed attempts
	// After 3 wrong attempts, we block the OTP
	// User must request a new OTP
	if otp.AttemptCount >= 3 {
		return false
	}

	// OTP is valid if it's not expired, not used, and not blocked
	return true
}

// MarkVerified marks the OTP as successfully verified
// Sets VerifiedAt to current time
// After verification, IsValid() will return false
//
// This is called when:
//   - User submits correct OTP code
//   - We've verified the hash matches
//   - Phone number update is successful
//
// Example usage in service:
//   otp.MarkVerified()
//   otpRepo.Update(ctx, otp)
//   user.Phone = otp.Phone
//   userRepo.Update(ctx, user)
func (otp *OTPVerification) MarkVerified() {
	now := time.Now()
	otp.VerifiedAt = &now
}

// IncrementAttempts increases the attempt counter
// Called when user submits wrong OTP code
// After 3 attempts, IsValid() will return false
//
// Example usage in service:
//   // User submitted wrong code
//   otp.IncrementAttempts()
//   otpRepo.Update(ctx, otp)
//   if otp.AttemptCount >= 3 {
//       return errors.ErrTooManyAttempts
//   }
func (otp *OTPVerification) IncrementAttempts() {
	otp.AttemptCount++
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: Why Hash OTP Codes?
// ═══════════════════════════════════════════════════════════════════════════
//
// You might think: "OTP codes expire in 10 minutes, why hash them?"
//
// Reasons:
//
// 1. Database Breach Protection
//    If attacker gets read-access to database (SQL injection, backup leak),
//    they can't use the OTP codes because they only have hashes.
//
// 2. Insider Threat
//    Database administrators or developers can't see OTP codes.
//    They can't help a friend bypass verification.
//
// 3. Compliance
//    Some regulations require all sensitive data to be encrypted/hashed.
//    OTP codes are authentication credentials.
//
// 4. Defense in Depth
//    Even if OTP expires soon, hashing adds extra security layer.
//    Small cost (SHA-256 is fast), significant security benefit.
//
// 5. Audit Trail
//    If security incident happens, hashed OTPs prove no plain codes were leaked.
//
// ═══════════════════════════════════════════════════════════════════════════
