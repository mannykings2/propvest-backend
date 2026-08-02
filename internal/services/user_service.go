package services

import (
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// UserService handles user profile management.
// This will be fully implemented in Milestone 2.
type UserService interface {
	// GetProfile retrieves the authenticated user's profile
	// TODO: Implement in Milestone 2
	// GetProfile(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error)

	// UpdateProfile updates user profile information
	// TODO: Implement in Milestone 2
	// UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.UserResponse, error)

	// UploadAvatar handles profile picture upload
	// TODO: Implement in Milestone 2
	// UploadAvatar(ctx context.Context, userID uuid.UUID, file multipart.File) (string, error)

	// ChangePassword allows users to update their password
	// TODO: Implement in Milestone 2
	// ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error
}

type userService struct {
	userRepo repositories.UserRepository
}

// NewUserService creates a new user service instance.
func NewUserService(userRepo repositories.UserRepository) UserService {
	return &userService{
		userRepo: userRepo,
	}
}

// Milestone 2 will implement:
//   - GetProfile: retrieve and return user profile
//   - UpdateProfile: validate and update user fields
//   - UploadAvatar: integrate Cloudinary, update avatar_url
//   - ChangePassword: verify old password, hash new password, update
