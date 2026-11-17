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
//   - Update user profile (name + address/occupation/employer)
//   - Upload avatar image
//   - Change password (with session revocation)
//   - Update phone number (with OTP verification)
//   - Request phone change OTP
//   - Verify phone change OTP
//   - Submit + read KYC verification (Phase 2)
type UserService interface {
	// GetProfile retrieves user profile by ID
	GetProfile(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error)

	// UpdateProfile updates user's name and optional profile detail fields
	UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.UserResponse, error)

	// UploadAvatar uploads and sets user's avatar image
	UploadAvatar(ctx context.Context, userID uuid.UUID, file *multipart.FileHeader) (*dto.UserResponse, error)

	// ChangePassword changes user's password and revokes all sessions
	ChangePassword(ctx context.Context, userID uuid.UUID, req dto.ChangePasswordRequest) error

	// RequestPhoneChange sends OTP to new phone number
	RequestPhoneChange(ctx context.Context, userID uuid.UUID, req dto.RequestPhoneChangeRequest) (*dto.PhoneChangeResponse, error)

	// VerifyPhoneChange verifies OTP and updates phone number
	VerifyPhoneChange(ctx context.Context, userID uuid.UUID, req dto.VerifyPhoneChangeRequest) (*dto.UserResponse, error)

	// SubmitKYC records a KYC submission (upserts the user's single row) and
	// returns the PII-safe status view. Sets users.kyc_status to 'pending'.
	SubmitKYC(ctx context.Context, userID uuid.UUID, req dto.SubmitKYCRequest) (*dto.KYCStatusResponse, error)

	// GetKYCStatus returns the user's current submission state.
	GetKYCStatus(ctx context.Context, userID uuid.UUID) (*dto.KYCStatusResponse, error)
}

// userService is the concrete implementation
type userService struct {
	userRepo         repositories.UserRepository
	otpRepo          repositories.OTPVerificationRepository
	refreshTokenRepo repositories.RefreshTokenRepository
	kycRepo          repositories.KYCSubmissionRepository
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
	kycRepo repositories.KYCSubmissionRepository,
	smsService sms.SMSService,
	cloudinaryService *cloudinary.CloudinaryService,
	cfg *config.Config,
) UserService {
	return &userService{
		userRepo:         userRepo,
		otpRepo:          otpRepo,
		refreshTokenRepo: refreshTokenRepo,
		kycRepo:          kycRepo,
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
		Address:       user.Address,
		Occupation:    user.Occupation,
		Employer:      user.Employer,
		CreatedAt:     user.CreatedAt,
	}, nil
}

// UpdateProfile updates user's full name and optional profile detail fields.
// Only the fields the client actually sent are written (nil pointers are
// skipped), so a name-only edit never clobbers a previously saved address.
func (s *userService) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.UserResponse, error) {
	// Validate full name
	fullName := strings.TrimSpace(req.FullName)
	if fullName == "" {
		return nil, errors.ErrValidationFailed
	}

	if len(fullName) < 2 || len(fullName) > 100 {
		return nil, errors.ErrValidationFailed
	}

	updates := map[string]interface{}{
		"full_name": fullName,
	}
	if req.Address != nil {
		updates["address"] = *req.Address
	}
	if req.Occupation != nil {
		updates["occupation"] = *req.Occupation
	}
	if req.Employer != nil {
		updates["employer"] = *req.Employer
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
	hashedPassword, err := password.HashWithCost(req.NewPassword, s.config.BcryptCost)
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
			Message:        "OTP already sent to this number",
			ExpiresIn:      int(remainingTime.Seconds()),
			CanResendAfter: int(remainingTime.Seconds()),
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

// SubmitKYC records the user's identity verification (BVN + NIN + doc type).
//
// One row per user: an existing submission is replaced so a re-submit updates
// rather than duplicates the queue entry. users.kyc_status is reset to
// 'pending' so the admin DecideKYC guard (which requires pending) accepts it.
//
// SECURITY: BVN/NIN are PII — written but never returned or logged. The
// response only carries the last 4 digits. Encryption at rest is a follow-up.
func (s *userService) SubmitKYC(ctx context.Context, userID uuid.UUID, req dto.SubmitKYCRequest) (*dto.KYCStatusResponse, error) {
	bvn := strings.TrimSpace(req.BVN)
	nin := strings.TrimSpace(req.NIN)
	docType := strings.TrimSpace(req.DocType)

	if bvn == "" || nin == "" || docType == "" {
		return nil, errors.ErrValidationFailed
	}

	sub := &models.KYCSubmission{
		UserID:      userID,
		BVN:         bvn,
		NIN:         nin,
		DocType:     docType,
		Status:      models.KYCStatusPending,
		SubmittedAt: time.Now(),
	}
	if err := s.kycRepo.Upsert(ctx, sub); err != nil {
		logger.Error("kyc-submit: failed to upsert submission", "user_id", userID, "error", err)
		return nil, errors.ErrInternalServer
	}

	// Reset the user's KYC flag to pending so the admin can decide it.
	if err := s.userRepo.UpdatePartial(ctx, userID, map[string]interface{}{"kyc_status": "pending"}); err != nil {
		logger.Error("kyc-submit: failed to set kyc_status", "user_id", userID, "error", err)
		return nil, errors.ErrInternalServer
	}

	return &dto.KYCStatusResponse{
		KYCStatus:   "pending",
		Status:      models.KYCStatusPending,
		DocType:     docType,
		BVNLast4:    sub.BVNLast4(),
		NINLast4:    sub.NINLast4(),
		SubmittedAt: sub.SubmittedAt,
	}, nil
}

// GetKYCStatus returns the user's current submission state (PII-safe).
func (s *userService) GetKYCStatus(ctx context.Context, userID uuid.UUID) (*dto.KYCStatusResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrUserNotFound
		}
		return nil, errors.ErrInternalServer
	}

	sub, err := s.kycRepo.FindByUserID(ctx, userID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			// No submission yet — report the user-level status only.
			return &dto.KYCStatusResponse{
				KYCStatus:   user.KYCStatus,
				Status:      "none",
				SubmittedAt: user.CreatedAt,
			}, nil
		}
		return nil, errors.ErrInternalServer
	}

	return &dto.KYCStatusResponse{
		KYCStatus:   user.KYCStatus,
		Status:      sub.Status,
		DocType:     sub.DocType,
		BVNLast4:    sub.BVNLast4(),
		NINLast4:    sub.NINLast4(),
		Reason:      sub.Reason,
		SubmittedAt: sub.SubmittedAt,
		DecidedAt:   sub.DecidedAt,
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
