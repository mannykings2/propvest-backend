package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// OTPVerificationRepository defines the interface for OTP data access operations.
// OTP (One-Time Password) codes are used for phone number verification.
//
// Repository responsibilities:
//   - Store new OTP codes
//   - Retrieve codes for validation
//   - Track verification attempts
//   - Clean up expired codes
//   - Enforce rate limiting
type OTPVerificationRepository interface {
	// Create stores a new OTP verification code in the database
	Create(ctx context.Context, otp *models.OTPVerification) error

	// FindByCodeHash retrieves an OTP by its hash (for validation)
	FindByCodeHash(ctx context.Context, codeHash string) (*models.OTPVerification, error)

	// FindActiveByUserAndPhone finds an active OTP for a specific user and phone
	// Used to prevent sending multiple OTP codes to the same phone
	FindActiveByUserAndPhone(ctx context.Context, userID uuid.UUID, phone string) (*models.OTPVerification, error)

	// Update saves changes to an existing OTP record
	// Used to increment attempt count or mark as verified
	Update(ctx context.Context, otp *models.OTPVerification) error

	// DeleteExpired removes expired OTP codes from the database
	// Called by background job to clean up old data
	DeleteExpired(ctx context.Context) error

	// CountRecentByUser returns how many OTPs a user has requested recently
	// Used for rate limiting (prevent spam)
	CountRecentByUser(ctx context.Context, userID uuid.UUID, since time.Time) (int64, error)

	// RevokeByUserAndPhone marks all OTP codes for a user+phone as verified
	// Used when phone change is completed to clean up pending OTPs
	RevokeByUserAndPhone(ctx context.Context, userID uuid.UUID, phone string) error
}

// otpVerificationRepository is the concrete implementation
// It's lowercase (private) so external packages must use the interface
type otpVerificationRepository struct {
	*BaseRepository
}

// NewOTPVerificationRepository creates a new OTP verification repository instance
// Called once at application startup in main.go and injected into UserService
func NewOTPVerificationRepository(db *gorm.DB) OTPVerificationRepository {
	return &otpVerificationRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create inserts a new OTP verification code into the database
//
// Parameters:
//   - ctx: Context for cancellation and deadlines
//   - otp: OTPVerification model with CodeHash, UserID, Phone, and ExpiresAt set
//
// GORM automatically handles:
//   - Setting created_at and updated_at
//   - Generating UUID for otp.ID
//   - Validating NOT NULL constraints
//
// Example usage in service:
//   otp := &models.OTPVerification{
//       UserID: user.ID,
//       Phone: "+2348012345678",
//       CodeHash: hashOTP(code),
//       ExpiresAt: time.Now().Add(10 * time.Minute),
//   }
//   err := repo.Create(ctx, otp)
func (r *otpVerificationRepository) Create(ctx context.Context, otp *models.OTPVerification) error {
	// WithContext(ctx) enables:
	//   - Request cancellation (if client disconnects)
	//   - Query timeout enforcement (if configured)
	//   - Distributed tracing (if using OpenTelemetry)
	//
	// Create(otp) generates SQL:
	//   INSERT INTO otp_verifications (id, user_id, phone, code_hash, expires_at, attempt_count, created_at, updated_at)
	//   VALUES (gen_random_uuid(), $1, $2, $3, $4, 0, NOW(), NOW())
	//
	// .Error returns nil on success, error on failure
	return r.WithContext(ctx).Create(otp).Error
}

// FindByCodeHash retrieves an OTP verification code by its hash
// This is called during OTP verification when user submits a code
//
// Parameters:
//   - ctx: Context for cancellation
//   - codeHash: SHA256 hash of the OTP code string
//
// Returns:
//   - OTP if found
//   - gorm.ErrRecordNotFound if no OTP matches
//   - Other error if database query fails
//
// Why hash the code before lookup?
//   Client sends: "123456"
//   We hash it: "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92"
//   We search: WHERE code_hash = '8d969eef...'
//
// Security: If database is breached, attacker can't use the OTP codes
// because they only have hashes, not original codes
//
// Example usage in service:
//   codeHash := hashOTP(submittedCode)
//   otp, err := repo.FindByCodeHash(ctx, codeHash)
//   if err != nil || !otp.IsValid() {
//       return errors.ErrInvalidOTP
//   }
func (r *otpVerificationRepository) FindByCodeHash(ctx context.Context, codeHash string) (*models.OTPVerification, error) {
	var otp models.OTPVerification

	// First() generates SQL:
	//   SELECT * FROM otp_verifications
	//   WHERE code_hash = $1
	//   LIMIT 1
	//
	// GORM automatically adds:
	//   - No soft-delete filter (this table doesn't use soft-delete)
	//   - LIMIT 1 (First() only needs one record)
	//
	// Index usage: We have an index on code_hash, so this query is O(log n)
	err := r.WithContext(ctx).
		Where("code_hash = ?", codeHash).
		First(&otp).Error

	if err != nil {
		// gorm.ErrRecordNotFound means no matching OTP exists
		// Service layer will convert this to "invalid OTP" error
		return nil, err
	}

	return &otp, nil
}

// FindActiveByUserAndPhone finds an active OTP for a specific user and phone
// "Active" means: not expired, not verified, not blocked by too many attempts
//
// Use cases:
//   - Check if user already has an active OTP (prevent spam)
//   - Rate limiting: max 1 OTP per 2 minutes
//   - Show "OTP sent" message if OTP already exists
//
// Parameters:
//   - ctx: Context for cancellation
//   - userID: UUID of the user
//   - phone: Phone number in E.164 format
//
// Returns:
//   - OTP if active OTP exists
//   - gorm.ErrRecordNotFound if no active OTP
//   - Other error if database query fails
//
// Example usage in service:
//   existingOTP, err := repo.FindActiveByUserAndPhone(ctx, userID, phone)
//   if err == nil {
//       // OTP already exists
//       remainingTime := existingOTP.ExpiresAt.Sub(time.Now())
//       return errors.ErrOTPAlreadySent
//   }
func (r *otpVerificationRepository) FindActiveByUserAndPhone(ctx context.Context, userID uuid.UUID, phone string) (*models.OTPVerification, error) {
	var otp models.OTPVerification

	// Find() generates SQL:
	//   SELECT * FROM otp_verifications
	//   WHERE user_id = $1
	//     AND phone = $2
	//     AND verified_at IS NULL
	//     AND expires_at > NOW()
	//     AND attempt_count < 3
	//   ORDER BY created_at DESC
	//   LIMIT 1
	//
	// Index usage:
	//   - idx_otp_verifications_user_phone (composite index) speeds up this query
	err := r.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("phone = ?", phone).
		Where("verified_at IS NULL").           // Not verified yet
		Where("expires_at > ?", time.Now()).    // Not expired
		Where("attempt_count < ?", 3).          // Not blocked
		Order("created_at DESC").                // Get most recent
		First(&otp).Error

	if err != nil {
		return nil, err
	}

	return &otp, nil
}

// Update saves changes to an existing OTP record
// Used to increment attempt count or mark as verified
//
// Parameters:
//   - ctx: Context for cancellation
//   - otp: OTPVerification model with updated fields
//
// Example usage (increment attempts):
//   otp, err := repo.FindByCodeHash(ctx, hash)
//   otp.IncrementAttempts()
//   repo.Update(ctx, otp)
//
// Example usage (mark verified):
//   otp, err := repo.FindByCodeHash(ctx, hash)
//   otp.MarkVerified()
//   repo.Update(ctx, otp)
func (r *otpVerificationRepository) Update(ctx context.Context, otp *models.OTPVerification) error {
	// Save() updates all fields
	// Generated SQL:
	//   UPDATE otp_verifications
	//   SET user_id = $1, phone = $2, code_hash = $3, 
	//       expires_at = $4, verified_at = $5, attempt_count = $6, 
	//       updated_at = NOW()
	//   WHERE id = $7
	return r.WithContext(ctx).Save(otp).Error
}

// DeleteExpired permanently removes expired OTP codes from the database
// This is called by a background job to clean up old data
//
// Why delete expired OTPs?
//   1. Database size: OTPs accumulate forever if not cleaned
//   2. Performance: Fewer rows = faster queries
//   3. Privacy: No need to keep old verification codes
//
// Strategy:
//   - Delete OTPs expired more than 24 hours ago
//   - Run this job once per day at 3 AM
//   - Keep recent OTPs for debugging purposes
//
// Parameters:
//   - ctx: Context for cancellation (important for long-running deletes)
//
// Returns:
//   - nil on success
//   - error if database operation fails
//
// Example background job:
//   func cleanupExpiredOTPs() {
//       ctx := context.Background()
//       err := otpRepo.DeleteExpired(ctx)
//       if err != nil {
//           log.Error("Failed to cleanup OTPs: %v", err)
//       }
//   }
//   // Run every day at 3 AM
//   cron.Schedule("0 3 * * *", cleanupExpiredOTPs)
func (r *otpVerificationRepository) DeleteExpired(ctx context.Context) error {
	// Delete OTPs that expired more than 24 hours ago
	// This gives us an audit trail of recent verifications
	cutoff := time.Now().Add(-24 * time.Hour)

	// Generated SQL:
	//   DELETE FROM otp_verifications
	//   WHERE expires_at < $1
	//
	// Note: This is a hard delete (permanent removal)
	// OTP table doesn't use soft-delete because:
	//   1. No need to recover deleted OTPs
	//   2. Privacy - old codes should be removed
	//   3. Database size management
	return r.WithContext(ctx).
		Where("expires_at < ?", cutoff).
		Delete(&models.OTPVerification{}).Error
}

// CountRecentByUser returns how many OTPs a user has requested recently
// Used for rate limiting to prevent spam
//
// Rate limiting rule:
//   - Max 5 OTP requests per 1 hour
//   - If user exceeds limit, reject request
//   - Prevents abuse and SMS cost
//
// Parameters:
//   - ctx: Context for cancellation
//   - userID: UUID of the user
//   - since: Time threshold (e.g., 1 hour ago)
//
// Returns:
//   - Count of OTP requests since the threshold
//   - error if database query fails
//
// Example usage in service:
//   oneHourAgo := time.Now().Add(-1 * time.Hour)
//   count, err := repo.CountRecentByUser(ctx, userID, oneHourAgo)
//   if err != nil {
//       return errors.ErrInternalServer
//   }
//   if count >= 5 {
//       return errors.ErrTooManyOTPRequests
//   }
func (r *otpVerificationRepository) CountRecentByUser(ctx context.Context, userID uuid.UUID, since time.Time) (int64, error) {
	var count int64

	// Count() generates SQL:
	//   SELECT COUNT(*) FROM otp_verifications
	//   WHERE user_id = $1
	//     AND created_at > $2
	//
	// Index usage:
	//   - idx_otp_verifications_user_id speeds up this query
	err := r.WithContext(ctx).
		Model(&models.OTPVerification{}).
		Where("user_id = ?", userID).
		Where("created_at > ?", since).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return count, nil
}

// RevokeByUserAndPhone marks all OTP codes for a user+phone as verified
// This is called after successful phone change to clean up pending OTPs
//
// Why revoke instead of delete?
//   - Maintains audit trail
//   - Shows when phone was verified
//   - Debugging friendly
//
// Parameters:
//   - ctx: Context for cancellation
//   - userID: UUID of the user
//   - phone: Phone number in E.164 format
//
// Returns:
//   - nil on success
//   - error if database update fails
//
// Example usage in service (after successful phone change):
//   // Update user's phone
//   user.Phone = newPhone
//   userRepo.Update(ctx, user)
//   // Clean up pending OTPs
//   otpRepo.RevokeByUserAndPhone(ctx, user.ID, newPhone)
func (r *otpVerificationRepository) RevokeByUserAndPhone(ctx context.Context, userID uuid.UUID, phone string) error {
	// Mark all matching OTPs as verified
	// Generated SQL:
	//   UPDATE otp_verifications
	//   SET verified_at = NOW(), updated_at = NOW()
	//   WHERE user_id = $1
	//     AND phone = $2
	//     AND verified_at IS NULL
	return r.WithContext(ctx).
		Model(&models.OTPVerification{}).
		Where("user_id = ?", userID).
		Where("phone = ?", phone).
		Where("verified_at IS NULL").  // Only update unverified OTPs
		Update("verified_at", time.Now()).Error
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: OTP Security Best Practices
// ═══════════════════════════════════════════════════════════════════════════
//
// 1. Hash OTP Codes
//    Like passwords, OTP codes should be hashed before storage
//    Use SHA-256 (fast) instead of bcrypt (slow)
//
// 2. Rate Limiting
//    Prevent abuse by limiting OTP requests
//    Example: Max 5 OTPs per hour per user
//
// 3. Expiration
//    OTP codes should expire quickly (10 minutes)
//    Balance between UX and security
//
// 4. Attempt Limiting
//    Max 3 wrong attempts before blocking
//    Prevents brute-force guessing
//
// 5. Single-Use
//    Once verified, OTP cannot be reused
//    Prevents replay attacks
//
// 6. Cleanup
//    Delete expired OTPs regularly
//    Keeps database size manageable
//
// 7. Audit Trail
//    Keep verified OTPs for 24 hours
//    Useful for debugging and support
//
// ═══════════════════════════════════════════════════════════════════════════
