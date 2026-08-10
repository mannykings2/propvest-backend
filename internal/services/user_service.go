package services

import (
	"context"
	"mime/multipart"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/utils/cloudinary"
	"github.com/mannykings2/propvest-backend/internal/utils/otp"
	"github.com/mannykings2/propvest-backend/internal/utils/password"
	"github.com/mannykings2/propvest-backend/internal/utils/sms"
	"github.com/mannykings2/propvest-backend/internal/validators"
)

// UserService handles user management business logic
//
// Responsibilities:
//   - Get user profile
//   - Update user profile (name)
//   - Upload avatar image
//   - Change password (with session revocation)
//   - Update phone number (with OTP verification)
//   - Request phone change OTP
//   - Verify phone change OTP
type UserService interface {
	// GetProfile retrieves user profile by ID
	GetProfile(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error)

	// UpdateProfile updates user's name
	UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.UserResponse, error)

	// UploadAvatar uploads and sets user's avatar image
	UploadAvatar(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (*dto.UserResponse, error)

	// ChangePassword changes user's password and revokes all sessions
	ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequest) error

	// RequestPhoneChange sends OTP to new phone number
	RequestPhoneChange(ctx context.Context, userID uuid.UUID, req dto.RequestPhoneChangeRequest) (*dto.PhoneChangeResponse, error)

	// VerifyPhoneChange verifies OTP and updates phone number
	VerifyPhoneChange(ctx context.Context, userID uuid.UUID, req dto.VerifyPhoneChangeRequest) (*dto.UserResponse, error)
}

// userService is the concrete implementation
type userService struct {
	userRepo         repositories.UserRepository
	otpRepo          repositories.OTPVerificationRepository
	refreshTokenRepo repositories.RefreshTokenRepository
	smsService       sms.SMSService
	cloudinary       *cloudinary.CloudinaryService
	config           *config.Config
}

// NewUserService creates a new user service instance
// All dependencies are injected via constructor
func NewUserService(
	userRepo repositories.UserRepository,
	otpRepo repositories.OTPVerificationRepository,
	refreshTokenRepo repositories.RefreshTokenRepository,
	smsService sms.SMSService,
	cloudinaryService *cloudinary.CloudinaryService,
	cfg *config.Config,
) UserService {
	return &userService{
		userRepo:         userRepo,
		otpRepo:          otpRepo,
		refreshTokenRepo: refreshTokenRepo,
		smsService:       smsService,
		cloudinary:       cloudinaryService,
		config:           cfg,
	}
}

// GetProfile retrieves a user's profile
func (s *userService) GetProfile(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrUserNotFound
		}
		return nil, errors.ErrInternalServer
	}

	return &dto.UserResponse{
		ID:            user.ID,
		UserCode:      user.UserCode,
		FullName:      user.FullName,
		Email:         user.Email,
		Phone:         user.Phone,
		AvatarURL:     user.AvatarURL,
		EmailVerified: user.EmailVerified,
		KYCStatus:     user.KYCStatus,
		Role:          user.Role,
		IsActive:      user.IsActive,
		CreatedAt:     user.CreatedAt,
	}, nil
}

// UpdateProfile updates user's full name
func (s *userService) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.UserResponse, error) {
	// Validate full name
	fullName := strings.TrimSpace(req.FullName)
	if fullName == "" {
		return nil, errors.ErrValidationFailed
	}

	if len(fullName) < 2 || len(fullName) > 100 {
		return nil, errors.ErrValidationFailed
	}

	// Update only the full name field
	updates := map[string]interface{}{
		"full_name": fullName,
	}

	err := s.userRepo.UpdatePartial(ctx, userID, updates)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Return updated profile
	return s.GetProfile(ctx, userID)
}

// UploadAvatar uploads user's avatar to Cloudinary and updates profile
func (s *userService) UploadAvatar(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (*dto.UserResponse, error) {
	// Upload to Cloudinary
	result, err := s.cloudinary.UploadAvatar(ctx, file, userID)
	if err != nil {
		return nil, errors.ErrImageUploadFailed
	}

	// Update user's avatar URL
	updates := map[string]interface{}{
		"avatar_url": result.SecureURL,
	}

	err = s.userRepo.UpdatePartial(ctx, userID, updates)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Return updated profile
	return s.GetProfile(ctx, userID)
}

// ChangePassword changes user's password and revokes all active sessions
func (s *userService) ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequest) error {
	// Get user
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return errors.ErrUserNotFound
		}
		return errors.ErrInternalServer
	}

	// Verify current password
	if err := password.Verify(req.CurrentPassword, user.PasswordHash); err != nil {
		return errors.ErrPasswordWrong
	}

	// Check that new password is different
	if req.NewPassword == req.CurrentPassword {
		return errors.ErrSamePassword
	}

	// Validate new password complexity
	if err := validators.ValidatePasswordComplexity(req.NewPassword); err != nil {
		return err
	}

	// Hash new password
	hashedPassword, err := password.Hash(req.NewPassword)
	if err != nil {
		return errors.ErrInternalServer
	}

	// Update password
	updates := map[string]interface{}{
		"password_hash": hashedPassword,
	}

	err = s.userRepo.UpdatePartial(ctx, userID, updates)
	if err != nil {
		return errors.ErrInternalServer
	}

	// Revoke all refresh tokens (force re-login on all devices).
	// FIX-03: this is security-critical after a password change. If we cannot
	// revoke sessions we must NOT report success — otherwise a stolen refresh
	// token would survive the password change. Fail closed.
	if err := s.refreshTokenRepo.RevokeAllByUserID(ctx, userID); err != nil {
		logger.Error("change-password: failed to revoke sessions", "user_id", userID, "error", err)
		return errors.ErrInternalServer
	}

	return nil
}

// RequestPhoneChange sends OTP to new phone number
func (s *userService) RequestPhoneChange(ctx context.Context, userID uuid.UUID, req dto.RequestPhoneChangeRequest) (*dto.PhoneChangeResponse, error) {
	// Validate phone format
	phone := strings.TrimSpace(req.NewPhone)
	if err := validators.ValidateNigerianPhone(phone); err != nil {
		return nil, err
	}

	// Check if phone is already taken by another user
	existingUser, err := s.userRepo.FindByPhone(ctx, phone)
	if err == nil && existingUser.ID != userID {
		return nil, errors.ErrPhoneAlreadyExists
	}

	// Rate limiting: Check if user has requested too many OTPs recently
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	recentCount, err := s.otpRepo.CountRecentByUser(ctx, userID, oneHourAgo)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	if recentCount >= 5 {
		return nil, errors.ErrTooManyOTPRequests
	}

	// Check if active OTP already exists for this phone
	existingOTP, err := s.otpRepo.FindActiveByUserAndPhone(ctx, userID, phone)
	if err == nil && existingOTP.IsValid() {
		// OTP already sent and still valid
		remainingTime := existingOTP.ExpiresAt.Sub(time.Now())
		return &dto.PhoneChangeResponse{
			Message:          "OTP already sent to this number",
			ExpiresIn:        int(remainingTime.Seconds()),
			CanResendAfter:   int(remainingTime.Seconds()),
		}, nil
	}

	// Generate OTP code
	code, err := otp.Generate()
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Hash OTP code for storage
	codeHash := otp.Hash(code)

	// Create OTP verification record
	otpVerification := &models.OTPVerification{
		UserID:    userID,
		Phone:     phone,
		CodeHash:  codeHash,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}

	err = s.otpRepo.Create(ctx, otpVerification)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Send OTP via SMS
	err = s.smsService.SendOTP(ctx, phone, code)
	if err != nil {
		// Log error but don't fail (OTP is stored, user can try again).
		// FIX-03: use the structured logger instead of fmt.Printf.
		logger.Error("phone-change: failed to send OTP SMS", "user_id", userID, "error", err)
	}

	return &dto.PhoneChangeResponse{
		Message:        "OTP sent successfully",
		ExpiresIn:      600, // 10 minutes
		CanResendAfter: 120, // 2 minutes
	}, nil
}

// VerifyPhoneChange verifies OTP and updates phone number
func (s *userService) VerifyPhoneChange(ctx context.Context, userID uuid.UUID, req dto.VerifyPhoneChangeRequest) (*dto.UserResponse, error) {
	// Validate inputs
	phone := strings.TrimSpace(req.NewPhone)
	code := strings.TrimSpace(req.OTPCode)

	if phone == "" || code == "" {
		return nil, errors.ErrValidationFailed
	}

	// Find active OTP for this user and phone
	otpVerification, err := s.otpRepo.FindActiveByUserAndPhone(ctx, userID, phone)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrOTPNotFound
		}
		return nil, errors.ErrInternalServer
	}

	// Validate OTP is still usable
	if !otpVerification.IsValid() {
		if time.Now().After(otpVerification.ExpiresAt) {
			return nil, errors.ErrOTPExpired
		}
		if otpVerification.VerifiedAt != nil {
			return nil, errors.ErrOTPAlreadyUsed
		}
		if otpVerification.AttemptCount >= 3 {
			return nil, errors.ErrTooManyOTPAttempts
		}
		return nil, errors.ErrInvalidOTP
	}

	// Verify OTP code
	if !otp.Verify(code, otpVerification.CodeHash) {
		// Wrong code - increment attempts
		otpVerification.IncrementAttempts()
		_ = s.otpRepo.Update(ctx, otpVerification)

		if otpVerification.AttemptCount >= 3 {
			return nil, errors.ErrTooManyOTPAttempts
		}

		return nil, errors.ErrInvalidOTP
	}

	// OTP is correct - update phone number
	updates := map[string]interface{}{
		"phone": phone,
	}

	err = s.userRepo.UpdatePartial(ctx, userID, updates)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Mark OTP as verified
	otpVerification.MarkVerified()
	_ = s.otpRepo.Update(ctx, otpVerification)

	// Return updated profile
	return s.GetProfile(ctx, userID)
}

