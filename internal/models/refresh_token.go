package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RefreshToken represents a long-lived token that can be exchanged for a new access token.
// Access tokens are short-lived (15 minutes) for security.
// Refresh tokens live longer (7-30 days) so users don't have to login repeatedly.
//
// Security model:
//   - Tokens are stored HASHED in the database (like passwords)
//   - Tokens are rotated on every use (old token invalidated, new one issued)
//   - Tokens can be revoked manually (logout, password change, suspicious activity)
//   - Expired tokens are automatically rejected by validation logic
type RefreshToken struct {
	// Primary key - unique identifier for this token record
	// type:uuid tells GORM to use PostgreSQL's UUID type
	// primaryKey marks this as the table's primary key
	// default:gen_random_uuid() tells PostgreSQL to auto-generate UUIDs
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// Foreign key linking to the users table
	// This establishes who owns this refresh token
	// not null ensures every token belongs to someone
	UserID uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`

	// TokenHash stores the hashed version of the refresh token
	// NEVER store plain tokens - always hash them (like passwords)
	//
	// Why hash tokens?
	//   If an attacker gets database access, they can't use the tokens
	//   because they only have the hashes, not the original values.
	//
	// uniqueIndex creates both:
	//   1. UNIQUE constraint (prevents duplicate tokens)
	//   2. B-tree index (makes lookups fast)
	TokenHash string `gorm:"uniqueIndex;not null" json:"-"`

	// When this token expires and can no longer be used
	// Typically 7-30 days from creation
	// Application validates: if time.Now() > ExpiresAt, reject the token
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`

	// RevokedAt is NULL for active tokens
	// Set to a timestamp when the token is manually revoked
	//
	// Revocation happens when:
	//   - User logs out
	//   - User changes password
	//   - Admin suspends account
	//   - Suspicious activity detected
	//
	// Why use *time.Time instead of time.Time?
	//   Plain time.Time defaults to zero-time (0001-01-01), not NULL
	//   *time.Time can be nil, which maps to SQL NULL
	RevokedAt *time.Time `gorm:"index" json:"revoked_at,omitempty"`

	// CreatedAt is automatically set by GORM on INSERT
	// Records when this token was first issued
	CreatedAt time.Time `gorm:"not null" json:"created_at"`

	// UpdatedAt is automatically set by GORM on every UPDATE
	// For refresh tokens, this tracks when token was last rotated
	UpdatedAt time.Time `json:"updated_at"`

	// DeletedAt enables soft-delete behavior
	// When you call db.Delete(&token), GORM sets DeletedAt to time.Now()
	// instead of running DELETE FROM refresh_tokens
	//
	// Soft-delete means:
	//   - Row stays in database but is invisible to normal queries
	//   - Can be recovered later if needed
	//   - Audit trail preserved
	//
	// gorm.DeletedAt is a special type that automatically filters deleted records
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// User is the association to the User model
	// GORM will automatically populate this when you use Preload("User")
	// Example: db.Preload("User").Find(&token)
	//
	// foreignKey:UserID tells GORM that UserID field references users.id
	// This creates a "belongs to" relationship: RefreshToken belongs to User
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// IsValid checks if the refresh token can currently be used
// Returns true only if:
//   1. Not expired (current time is before ExpiresAt)
//   2. Not revoked (RevokedAt is nil)
//
// This encapsulates the validation business rules in one place
// so every part of the codebase uses the same logic.
func (rt *RefreshToken) IsValid() bool {
	// Check if token is expired
	// time.Now() returns current time in UTC
	// After() returns true if time.Now() is after ExpiresAt
	if time.Now().After(rt.ExpiresAt) {
		return false
	}

	// Check if token is revoked
	// RevokedAt is *time.Time, so it can be nil
	// If it's not nil, the token has been revoked
	if rt.RevokedAt != nil {
		return false
	}

	// Token is valid if it's not expired and not revoked
	return true
}

// Revoke marks the token as revoked by setting RevokedAt to current time
// After revocation, IsValid() will return false
// This is called when:
//   - User logs out
//   - User changes password (revoke all tokens)
//   - Admin suspends account (revoke all tokens)
//   - Suspicious activity detected
func (rt *RefreshToken) Revoke() {
	now := time.Now()
	rt.RevokedAt = &now
}
