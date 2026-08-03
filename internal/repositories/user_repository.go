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
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByPhone(ctx context.Context, phone string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
	Delete(ctx context.Context, id uuid.UUID) error
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	FindByIDWithWallet(ctx context.Context, id uuid.UUID) (*models.User, error)
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
func (r *userRepository) UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	// Model(&models.User{}) tells GORM which table to update
	// Where("id = ?", id) specifies which row
	// Updates(updates) sets the fields from the map
	//
	// GORM automatically adds:
	//   - updated_at = NOW()
	//   - AND deleted_at IS NULL (soft-delete check)
	return r.WithContext(ctx).
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
func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.WithContext(ctx).Delete(&models.User{}, id).Error
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
