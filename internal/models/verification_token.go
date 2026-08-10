package models

import (
	"time"

	"github.com/google/uuid"
)

// VerificationToken is a single-use, expiring token used for BOTH email
// verification and password reset. One table with a Purpose column keeps things
// simple (docs 4.2 §9/§10 both need "single-use, expiring, resend invalidates
// previous").
//
// SECURITY: like refresh tokens, we store only the SHA-256 HASH of the token,
// never the token itself. The plaintext token is emailed to the user; if our DB
// leaks, the hashes are useless to an attacker.
type VerificationToken struct {
	ID     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	// TokenHash is the SHA-256 hex of the plaintext token we emailed.
	TokenHash string `gorm:"uniqueIndex;not null" json:"-"`

	// Purpose is "email_verification" or "password_reset".
	Purpose string `gorm:"not null;index" json:"purpose"`

	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"` // nil until consumed
	CreatedAt time.Time  `json:"created_at"`
}

// Verification-token purposes.
const (
	PurposeEmailVerification = "email_verification"
	PurposePasswordReset     = "password_reset"
)

// IsUsable reports whether the token can still be consumed: not expired and not
// already used.
func (t *VerificationToken) IsUsable() bool {
	if t.UsedAt != nil {
		return false
	}
	return time.Now().Before(t.ExpiresAt)
}
