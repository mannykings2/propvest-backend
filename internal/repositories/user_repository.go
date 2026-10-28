package repositories

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// UserRepository defines the interface for user data access operations.
// Using an interface allows us to:
//   1. Mock the repository in service tests
//   2. Swap implementations without changing service code
//   3. Document the repository's contract clearly
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	// HasPendingDeletion reports whether the user has a LIVE erasure request
	// (Phase 9.3) — the money-moving gate uses it to block new activity the
	// moment deletion is filed.
	HasPendingDeletion(ctx context.Context, userID uuid.UUID) (bool, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByPhone(ctx context.Context, phone string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	// UpdatePartial updates specific columns of one user.
	// Pass an optional transaction handle to make the update part of a larger
	// atomic operation (see admin_user_service.SuspendUser, which commits the
	// change and its audit row together).
	UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]interface{}, txs ...*gorm.DB) error
	Delete(ctx context.Context, id uuid.UUID, txs ...*gorm.DB) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	FindByIDWithWallet(ctx context.Context, id uuid.UUID) (*models.User, error)
	
	// Admin operations (Milestone 7)
	List(ctx context.Context, filter UserFilter, limit, offset int) ([]*models.User, int64, error)
	FindByIDWithWalletAndInvestments(ctx context.Context, id uuid.UUID) (*models.User, error)

	// FindSessionState loads the narrow account state the auth middleware
	// re-checks on every authenticated request: RBAC role, active/suspended
	// flags, and the verification flags the launch policy gates read.
	// It selects only those columns (no password hash) because it runs on the
	// hot path. Soft-deleted users are reported as not found.
	FindSessionState(ctx context.Context, id uuid.UUID) (*AccountState, error)
}

// AccountState is the per-request account state returned by
// UserRepository.FindSessionState. It is deliberately a narrow projection of
// users: enough to decide whether a presented access token may act *right now*,
// without loading credentials or profile PII.
type AccountState struct {
	UserID        uuid.UUID
	Role          string
	IsActive      bool
	Suspended     bool // suspended_at IS NOT NULL
	EmailVerified bool
	KYCStatus     string
}

// userRepository is the concrete implementation of UserRepository.
// It's lowercase (private) so external packages must use the interface.
type userRepository struct {
	*BaseRepository
}

// NewUserRepository creates a new user repository instance.
// This is called once at application startup in main.go.
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create inserts a new user into the database.
// GORM automatically handles:
//   - Setting created_at and updated_at
//   - Generating UUID (via database default)
//   - Running BeforeCreate hooks (e.g., UserCode generation)
// HasPendingDeletion checks for a live (not cancelled/executed) erasure
// request. One indexed lookup on the partial unique index's target column.
func (r *userRepository) HasPendingDeletion(ctx context.Context, userID uuid.UUID) (bool, error) {
	var n int64
	if err := r.WithContext(ctx).Model(&models.AccountDeletionRequest{}).
		Where("user_id = ? AND status = ?", userID, models.DeletionStatusRequested).
		Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	return r.WithContext(ctx).Create(user).Error
}

// FindByID retrieves a user by their UUID.
// Returns gorm.ErrRecordNotFound if the user doesn't exist.
func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByEmail retrieves a user by their email address.
// Returns gorm.ErrRecordNotFound if no user has that email.
//
// This is used during login to verify credentials.
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByPhone retrieves a user by their phone number.
// Returns gorm.ErrRecordNotFound if no user has that phone.
//
// Used for:
//   - Phone uniqueness validation during registration
//   - Phone update validation (prevent duplicates)
//   - Phone-based authentication (future feature)
//
// Example usage:
//   user, err := repo.FindByPhone(ctx, "+2348012345678")
//   if err == nil {
//       return errors.ErrPhoneAlreadyExists
//   }
func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*models.User, error) {
	var user models.User
	// WHERE phone = ? uses the unique index for fast lookup
	err := r.WithContext(ctx).Where("phone = ?", phone).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update saves changes to an existing user record.
// GORM automatically updates the updated_at field via the database trigger.
//
// Important: This updates ALL fields. For partial updates, use:
//   db.Model(&user).Updates(map[string]interface{}{"field": value})
func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	return r.WithContext(ctx).Save(user).Error
}

// UpdatePartial updates only specific fields of a user record.
// This is more efficient than Update() when you only want to change a few fields.
//
// Parameters:
//   - id: UUID of the user to update
//   - updates: Map of field names to new values
//
// Example usage (update just the phone):
//   updates := map[string]interface{}{
//       "phone": "+2348012345678",
//       "updated_at": time.Now(),
//   }
//   err := repo.UpdatePartial(ctx, userID, updates)
//
// Example usage (update name and avatar):
//   updates := map[string]interface{}{
//       "full_name": "New Name",
//       "avatar_url": "https://cloudinary.com/image.jpg",
//   }
//   err := repo.UpdatePartial(ctx, userID, updates)
//
// Why use UpdatePartial instead of Update?
//   - Update() requires loading the full user first
//   - UpdatePartial() directly executes UPDATE query
//   - More efficient for single-field changes
//   - Atomic operation (no race conditions)
//
// Generated SQL:
//   UPDATE users 
//   SET phone = $1, updated_at = NOW() 
//   WHERE id = $2 AND deleted_at IS NULL
func (r *userRepository) UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]interface{}, txs ...*gorm.DB) error {
	// Model(&models.User{}) tells GORM which table to update
	// Where("id = ?", id) specifies which row
	// Updates(updates) sets the fields from the map
	//
	// GORM automatically adds:
	//   - updated_at = NOW()
	//   - AND deleted_at IS NULL (soft-delete check)
	//
	// txs[0], when supplied, is the caller's transaction: the update then joins
	// that transaction instead of running on the base pool (same pattern as
	// wallet_repository.CreateTransaction after P0-1).
	q := r.db
	if len(txs) > 0 && txs[0] != nil {
		q = txs[0]
	}
	return q.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// Delete performs a soft delete on a user.
// The record remains in the database but deleted_at is set to the current time.
// GORM automatically excludes soft-deleted records from queries.
//
// To include soft-deleted records:
//   db.Unscoped().Where("email = ?", email).First(&user)
func (r *userRepository) Delete(ctx context.Context, id uuid.UUID, txs ...*gorm.DB) error {
	q := r.db
	if len(txs) > 0 && txs[0] != nil {
		q = txs[0]
	}
	return q.WithContext(ctx).Delete(&models.User{}, id).Error
}

// FindSessionState fetches the auth middleware's account-state projection.
//
// WHY THIS EXISTS: access tokens are stateless, so "did an admin suspend this
// account 30 seconds ago?" can only be answered by looking at the row. Selecting
// a handful of columns (instead of SELECT *) keeps that lookup cheap enough to
// run on every authenticated request, and GORM's soft-delete filter means a
// deleted account answers ErrRecordNotFound.
func (r *userRepository) FindSessionState(ctx context.Context, id uuid.UUID) (*AccountState, error) {
	var u models.User

	err := r.WithContext(ctx).
		Model(&models.User{}).
		Select("id", "role", "is_active", "suspended_at", "email_verified", "kyc_status").
		Where("id = ?", id).
		First(&u).Error
	if err != nil {
		return nil, err
	}

	return &AccountState{
		UserID:        u.ID,
		Role:          u.Role,
		IsActive:      u.IsActive,
		Suspended:     u.IsSuspended(),
		EmailVerified: u.EmailVerified,
		KYCStatus:     u.KYCStatus,
	}, nil
}

// ExistsByEmail checks if a user with the given email already exists.
// Returns true if exists, false otherwise.
//
// This is more efficient than FindByEmail when you only need to check existence.
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.WithContext(ctx).Model(&models.User{}).Where("email = ?", email).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsByPhone checks if a user with the given phone number already exists.
// Returns true if exists, false otherwise.
//
// Used for:
//   - Registration validation (prevent duplicate phones)
//   - Phone update validation (prevent taking another user's phone)
//
// Example usage:
//   exists, err := repo.ExistsByPhone(ctx, "+2348012345678")
//   if err != nil {
//       return errors.ErrInternalServer
//   }
//   if exists {
//       return errors.ErrPhoneAlreadyExists
//   }
//
// Why Count() instead of First()?
//   - Count() only queries the index, not the full row
//   - More efficient when you don't need the data
//   - Standard pattern for existence checks
func (r *userRepository) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	var count int64
	// Count() generates SQL:
	//   SELECT COUNT(*) FROM users 
	//   WHERE phone = $1 AND deleted_at IS NULL
	err := r.WithContext(ctx).
		Model(&models.User{}).
		Where("phone = ?", phone).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// FindByIDWithWallet retrieves a user and eagerly loads their wallet.
// This is more efficient than separate queries when you need both.
//
// Usage:
//   user, err := repo.FindByIDWithWallet(ctx, userID)
//   balance := user.Wallet.MainBalance
func (r *userRepository) FindByIDWithWallet(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.WithContext(ctx).Preload("Wallet").Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// IsErrRecordNotFound checks if an error is "record not found".
// This helper makes service layer code cleaner.
//
// Usage:
//   user, err := repo.FindByEmail(ctx, email)
//   if repositories.IsErrRecordNotFound(err) {
//       return ErrInvalidCredentials
//   }
func IsErrRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}


// ═══════════════════════════════════════════════════════════════════════════
// ADMIN USER MANAGEMENT (Milestone 7 - Phase 4)
// ═══════════════════════════════════════════════════════════════════════════

// UserFilter defines filter criteria for admin user list queries.
type UserFilter struct {
	Search      string  // Search in full_name, email, user_code
	Role        *string // Filter by role
	KYCStatus   *string // Filter by KYC status
	IsActive    *bool   // Filter by active status
	IsSuspended *bool   // Filter by suspension status
}

// List returns a paginated list of users matching the filter criteria.
//
// This is used for the admin user management dashboard.
// Supports filtering by role, KYC status, active status, and text search.
//
// Parameters:
//   - ctx: Request context
//   - filter: Search and filter criteria
//   - limit: Max results per page (bounded to prevent abuse)
//   - offset: Number of records to skip (for pagination)
//
// Returns:
//   - []*models.User: Slice of matching users
//   - int64: Total count (for pagination metadata)
//   - error: Database error if any
//
// Query pattern:
//   SELECT * FROM users
//   WHERE <filters>
//   ORDER BY created_at DESC
//   LIMIT ? OFFSET ?
func (r *userRepository) List(ctx context.Context, filter UserFilter, limit, offset int) ([]*models.User, int64, error) {
	var users []*models.User
	var total int64

	// Start with base query
	query := r.WithContext(ctx).Model(&models.User{})

	// Apply filters
	if filter.Search != "" {
		// Search in name, email, or user code
		searchPattern := "%" + filter.Search + "%"
		query = query.Where(
			"full_name ILIKE ? OR email ILIKE ? OR user_code ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	if filter.Role != nil {
		query = query.Where("role = ?", *filter.Role)
	}

	if filter.KYCStatus != nil {
		query = query.Where("kyc_status = ?", *filter.KYCStatus)
	}

	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}

	if filter.IsSuspended != nil {
		if *filter.IsSuspended {
			query = query.Where("suspended_at IS NOT NULL")
		} else {
			query = query.Where("suspended_at IS NULL")
		}
	}

	// Get total count for pagination
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	// Order by created_at DESC (newest first)
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error

	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// FindByIDWithWalletAndInvestments retrieves a user with their wallet and investment summary.
//
// This is used for the admin user detail view which shows:
//   - User profile
//   - Wallet balances
//   - Investment count and total invested
//
// Note: This does NOT load all investments (could be hundreds).
// Investment count and total are calculated separately in the service layer.
func (r *userRepository) FindByIDWithWalletAndInvestments(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.WithContext(ctx).
		Preload("Wallet").
		Where("id = ?", id).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
