package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a registered account in the PropVest system.
// This struct maps directly to the `users` table in PostgreSQL.
// Every field tag tells GORM how to translate the struct to SQL.
type User struct {
	// uuid.UUID gives us a globally unique identifier.
	// Why UUID instead of auto-increment integer IDs?
	//   1. Security: sequential IDs (1, 2, 3...) let attackers enumerate
	//      users by just incrementing the number in the URL.
	//   2. Distribution: if you ever shard the database or merge datasets,
	//      UUIDs never collide. Integer IDs do.
	//   3. Predictability: you can generate the ID before inserting the row,
	//      which is important for our transaction in Step 7 (we create the
	//      wallet using the user's ID before the user row is committed).
	//
	// `default:gen_random_uuid()` tells PostgreSQL to generate the UUID
	// itself if we don't provide one. gen_random_uuid() is a built-in
	// PostgreSQL function — no extension needed in Postgres 13+.
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	// UserCode is a human-readable reference like "USR-00001".
	// UUIDs are not user-friendly ("show me order a3f8..."). A short code
	// gives customer support and users a readable identifier.
	UserCode string `gorm:"uniqueIndex;not null" json:"user_code"`

	// uniqueIndex creates a UNIQUE constraint + index on this column.
	// The index makes `WHERE email = ?` queries fast.
	// The constraint means the database rejects duplicate emails even if
	// our application code has a bug and tries to insert one.
	Email string `gorm:"uniqueIndex;not null" json:"email"`

	// Phone has a pointer type (*string) meaning it is nullable.
	// A plain string in Go is never nil — it defaults to "".
	// A *string can be nil, which maps to NULL in PostgreSQL.
	// We make phone optional at registration but unique if provided.
	Phone *string `gorm:"uniqueIndex" json:"phone,omitempty"`

	// PasswordHash stores the bcrypt hash, never the plain password.
	// json:"-" means this field is NEVER included in any JSON response,
	// even if a developer accidentally passes the whole User struct to c.JSON().
	// This is a critical security guardrail.
	PasswordHash string `gorm:"not null" json:"-"`

	FullName string `gorm:"not null" json:"full_name"`

	// KYCStatus tracks the identity verification state.
	// Using a string with a default is simpler than an enum for now.
	// Valid values: pending | in_review | approved | rejected
	KYCStatus string `gorm:"default:'pending';not null" json:"kyc_status"`

	// Role controls what the user can access.
	// Valid values: investor | developer | staff | admin
	Role string `gorm:"default:'investor';not null" json:"role"`

	// IsActive allows admins to suspend an account without deleting it.
	IsActive bool `gorm:"default:true;not null" json:"is_active"`

	// GORM automatically manages these three fields when you use
	// gorm.Model or declare them explicitly like this.
	// CreatedAt: set once on INSERT, never updated
	// UpdatedAt: updated on every UPDATE
	// DeletedAt: set on soft-delete, NULL for active records
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Association: GORM will automatically join to the wallets table
	// when you use Preload("Wallet"). The foreign key is inferred
	// as wallet.user_id matching user.id.
	Wallet Wallet `gorm:"foreignKey:UserID" json:"wallet,omitempty"`
}

// BeforeCreate is a GORM hook — it runs automatically before every INSERT.
// We use it to generate the human-readable UserCode.
// GORM hooks are how you add automatic behaviour without cluttering
// your service layer with boilerplate.
func (u *User) BeforeCreate(tx *gorm.DB) error {
	// If UserCode is already set (e.g. in tests), don't overwrite it.
	if u.UserCode == "" {
		// We use the first 8 characters of the UUID as the code suffix.
		// In production you would use a database sequence for a cleaner
		// incrementing number, but this is solid for Phase 1.
		u.UserCode = "USR-" + u.ID.String()[:8]
	}
	return nil
}