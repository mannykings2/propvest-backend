package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// RefreshTokenRepository defines the interface for refresh token data access operations.
// Refresh tokens are used to issue new access tokens without requiring re-login.
//
// Repository responsibilities:
//   - Store new refresh tokens
//   - Retrieve tokens for validation
//   - Revoke tokens (logout, password change)
//   - Clean up expired tokens
type RefreshTokenRepository interface {
	// Create stores a new refresh token in the database
	Create(ctx context.Context, token *models.RefreshToken) error

	// FindByTokenHash retrieves a token by its hash (for validation during refresh)
	FindByTokenHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)

	// FindActiveByUserID retrieves all non-revoked, non-expired tokens for a user
	// Used to display active sessions or revoke all sessions
	FindActiveByUserID(ctx context.Context, userID uuid.UUID) ([]*models.RefreshToken, error)

	// RevokeByTokenHash marks a single token as revoked (logout from one device)
	RevokeByTokenHash(ctx context.Context, tokenHash string) error

	// RevokeAllByUserID marks all user's tokens as revoked (logout all devices)
	// Called when: password changed, account suspended, user requests logout everywhere
	RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error

	// DeleteExpired removes expired tokens from the database
	// This is called by a background job to clean up old data
	DeleteExpired(ctx context.Context) error

	// CountActiveByUserID returns how many active sessions a user has
	// Useful for displaying session count in UI or enforcing session limits
	CountActiveByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
}

// refreshTokenRepository is the concrete implementation of RefreshTokenRepository
// It's lowercase (private) so external packages must use the interface
type refreshTokenRepository struct {
	*BaseRepository
}

// NewRefreshTokenRepository creates a new refresh token repository instance
// Called once at application startup in main.go and injected into AuthService
func NewRefreshTokenRepository(db *gorm.DB) RefreshTokenRepository {
	return &refreshTokenRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create inserts a new refresh token into the database
//
// Parameters:
//   - ctx: Context for cancellation and deadlines
//   - token: RefreshToken model with TokenHash, UserID, and ExpiresAt set
//
// GORM automatically handles:
//   - Setting created_at and updated_at
//   - Generating UUID for token.ID
//   - Validating NOT NULL constraints
//
// Business rules enforced here:
//   - TokenHash must be unique (database constraint)
//   - UserID must reference existing user (foreign key constraint)
//
// Example usage in service:
//   token := &models.RefreshToken{
//       UserID: user.ID,
//       TokenHash: hashToken(refreshToken),
//       ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
//   }
//   err := repo.Create(ctx, token)
func (r *refreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	// WithContext(ctx) enables:
	//   - Request cancellation (if client disconnects)
	//   - Query timeout enforcement (if configured)
	//   - Distributed tracing (if using OpenTelemetry)
	//
	// Create(token) generates SQL:
	//   INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at, updated_at)
	//   VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())
	//
	// .Error returns nil on success, error on failure
	return r.WithContext(ctx).Create(token).Error
}

// FindByTokenHash retrieves a refresh token by its hash
// This is called during token refresh to validate the token
//
// Parameters:
//   - ctx: Context for cancellation
//   - tokenHash: SHA256 hash of the refresh token string
//
// Returns:
//   - Token if found
//   - gorm.ErrRecordNotFound if no token matches
//   - Other error if database query fails
//
// Why hash the token before lookup?
//   Client sends: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
//   We hash it: "3f8a5c2d9e1b..."
//   We search: WHERE token_hash = '3f8a5c2d9e1b...'
//
// Security: If database is breached, attacker can't use the tokens
// because they only have hashes, not original JWT strings
//
// Example usage in service:
//   tokenHash := hashToken(clientToken)
//   dbToken, err := repo.FindByTokenHash(ctx, tokenHash)
//   if err != nil || !dbToken.IsValid() {
//       return ErrInvalidToken
//   }
func (r *refreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	var token models.RefreshToken

	// First() generates SQL:
	//   SELECT * FROM refresh_tokens
	//   WHERE token_hash = $1 AND deleted_at IS NULL
	//   LIMIT 1
	//
	// GORM automatically adds:
	//   - deleted_at IS NULL (soft-delete filter)
	//   - LIMIT 1 (First() only needs one record)
	//
	// Index usage: We have an index on token_hash, so this query is O(log n)
	err := r.WithContext(ctx).
		Where("token_hash = ?", tokenHash).
		First(&token).Error

	if err != nil {
		// gorm.ErrRecordNotFound means no matching token exists
		// Service layer will convert this to "invalid token" error
		return nil, err
	}

	return &token, nil
}

// FindActiveByUserID retrieves all active (non-revoked, non-expired) tokens for a user
//
// "Active" means:
//   1. revoked_at IS NULL (not manually revoked)
//   2. expires_at > NOW() (not expired by time)
//
// Use cases:
//   - Display user's active sessions in account settings
//   - Revoke all sessions when password changes
//   - Enforce session limits (max 5 devices)
//
// Example usage in service:
//   sessions, err := repo.FindActiveByUserID(ctx, userID)
//   for _, session := range sessions {
//       fmt.Printf("Device logged in at: %v\n", session.CreatedAt)
//   }
func (r *refreshTokenRepository) FindActiveByUserID(ctx context.Context, userID uuid.UUID) ([]*models.RefreshToken, error) {
	var tokens []*models.RefreshToken

	// Find() generates SQL:
	//   SELECT * FROM refresh_tokens
	//   WHERE user_id = $1
	//     AND revoked_at IS NULL
	//     AND expires_at > NOW()
	//     AND deleted_at IS NULL
	//   ORDER BY created_at DESC
	//
	// Index usage:
	//   - idx_refresh_tokens_user_id speeds up user_id lookup
	//   - idx_refresh_tokens_revoked_at speeds up revoked_at filter
	//   - idx_refresh_tokens_expires_at speeds up expires_at filter
	err := r.WithContext(ctx).
		Where("user_id = ?", userID).
		Where("revoked_at IS NULL").              // Not manually revoked
		Where("expires_at > ?", time.Now()).      // Not expired by time
		Order("created_at DESC").                  // Newest first
		Find(&tokens).Error

	if err != nil {
		return nil, err
	}

	return tokens, nil
}

// RevokeByTokenHash marks a single token as revoked
// This is called during logout to invalidate the current session
//
// "Revoked" means setting revoked_at to the current timestamp
// Once revoked, the token fails validation even if not expired
//
// Parameters:
//   - ctx: Context for cancellation
//   - tokenHash: Hash of the token to revoke
//
// Returns:
//   - nil on success
//   - error if token doesn't exist or database error
//
// Why update instead of delete?
//   - Audit trail: We keep history of all tokens for security analysis
//   - Recovery: Can see when sessions were terminated
//   - Debugging: Can investigate "why was I logged out?" issues
//
// Example usage in service (logout):
//   tokenHash := hashToken(clientToken)
//   err := repo.RevokeByTokenHash(ctx, tokenHash)
//   if err != nil {
//       // Token might already be expired/deleted, that's okay
//       return nil  // Logout succeeds even if token not found
//   }
func (r *refreshTokenRepository) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	// Model(&models.RefreshToken{}) tells GORM which table to update
	// Where() adds the condition
	// Update() sets revoked_at to current time
	//
	// Generated SQL:
	//   UPDATE refresh_tokens
	//   SET revoked_at = NOW(), updated_at = NOW()
	//   WHERE token_hash = $1 AND deleted_at IS NULL
	//
	// Note: If token doesn't exist, this returns no error (0 rows affected)
	// Service layer should check RowsAffected if it needs to know
	return r.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Update("revoked_at", time.Now()).Error
}

// RevokeAllByUserID marks all of a user's tokens as revoked
// This is called when:
//   - User changes password (invalidate all sessions for security)
//   - Admin suspends account (force logout everywhere)
//   - User clicks "Logout from all devices" button
//
// Parameters:
//   - ctx: Context for cancellation
//   - userID: UUID of the user whose tokens to revoke
//
// Returns:
//   - nil on success
//   - error if database update fails
//
// Business impact:
//   - User must login again on all devices
//   - All access tokens become useless (can't refresh them)
//   - Forces immediate security when credentials may be compromised
//
// Example usage in service (password change):
//   // Update password in database
//   user.PasswordHash = newHash
//   db.Save(&user)
//   // Revoke all sessions
//   repo.RevokeAllByUserID(ctx, user.ID)
//   // User must login again with new password
func (r *refreshTokenRepository) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	// Generated SQL:
	//   UPDATE refresh_tokens
	//   SET revoked_at = NOW(), updated_at = NOW()
	//   WHERE user_id = $1
	//     AND revoked_at IS NULL
	//     AND deleted_at IS NULL
	//
	// We only update tokens that aren't already revoked (efficiency)
	return r.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("user_id = ?", userID).
		Where("revoked_at IS NULL").  // Only update active tokens
		Update("revoked_at", time.Now()).Error
}

// DeleteExpired permanently removes expired tokens from the database
// This is called by a background job (cron or scheduled task) to clean up old data
//
// Why delete expired tokens?
//   1. Database size: Tokens accumulate forever if not cleaned
//   2. Performance: Fewer rows = faster queries
//   3. Compliance: GDPR/privacy laws may require deleting old sessions
//
// Strategy:
//   - Keep tokens for 90 days after expiration (for audit)
//   - Delete anything older than that
//   - Run this job once per day at 3 AM
//
// Parameters:
//   - ctx: Context for cancellation (important for long-running deletes)
//
// Returns:
//   - nil on success
//   - error if database operation fails
//
// Example background job:
//   func cleanupExpiredTokens() {
//       ctx := context.Background()
//       err := refreshTokenRepo.DeleteExpired(ctx)
//       if err != nil {
//           log.Error("Failed to cleanup tokens: %v", err)
//       }
//   }
//   // Run every day at 3 AM
//   cron.Schedule("0 3 * * *", cleanupExpiredTokens)
func (r *refreshTokenRepository) DeleteExpired(ctx context.Context) error {
	// Calculate cutoff: 90 days ago
	// Tokens expired before this are permanently deleted
	cutoff := time.Now().AddDate(0, 0, -90)

	// Unscoped() bypasses soft-delete (performs hard DELETE)
	// Without Unscoped(), this would just set deleted_at
	// With Unscoped(), rows are actually removed from disk
	//
	// Generated SQL:
	//   DELETE FROM refresh_tokens
	//   WHERE expires_at < $1
	//
	// No deleted_at IS NULL check because Unscoped() removes it
	//
	// Warning: This is a permanent operation - cannot be undone
	// Make sure cutoff logic is correct before running in production
	return r.WithContext(ctx).
		Unscoped().  // Hard delete (remove rows permanently)
		Where("expires_at < ?", cutoff).
		Delete(&models.RefreshToken{}).Error
}

// CountActiveByUserID returns the number of active sessions for a user
//
// Use cases:
//   - Display in UI: "You have 3 active sessions"
//   - Enforce limits: "Maximum 5 devices allowed"
//   - Security monitoring: "Alert if user has > 10 sessions"
//
// Parameters:
//   - ctx: Context for cancellation
//   - userID: UUID of the user
//
// Returns:
//   - Number of active tokens (non-revoked, non-expired)
//   - error if database query fails
//
// Example usage in handler:
//   count, _ := repo.CountActiveByUserID(ctx, user.ID)
//   response := map[string]interface{}{
//       "active_sessions": count,
//   }
func (r *refreshTokenRepository) CountActiveByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64

	// Count() generates SQL:
	//   SELECT COUNT(*) FROM refresh_tokens
	//   WHERE user_id = $1
	//     AND revoked_at IS NULL
	//     AND expires_at > NOW()
	//     AND deleted_at IS NULL
	//
	// Count() is more efficient than Find() when you only need the number
	// Database can use index-only scan without reading actual rows
	err := r.WithContext(ctx).
		Model(&models.RefreshToken{}).
		Where("user_id = ?", userID).
		Where("revoked_at IS NULL").
		Where("expires_at > ?", time.Now()).
		Count(&count).Error

	if err != nil {
		return 0, err
	}

	return count, nil
}
