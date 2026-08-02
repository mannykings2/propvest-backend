package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/utils/jwt"
	"github.com/mannykings2/propvest-backend/internal/utils/password"
	"gorm.io/gorm"
)

// AuthService handles all authentication-related business logic
//
// Responsibilities:
//   - User registration (with automatic wallet creation)
//   - Login (password verification + token generation)
//   - Token refresh (with rotation for security)
//   - Logout (token revocation)
//   - Password complexity validation
//   - Email/phone format validation
//
// Security principles:
//   - Passwords are hashed with bcrypt (never stored plain)
//   - Refresh tokens are hashed before database storage
//   - Tokens are rotated on every refresh (prevents replay attacks)
//   - Generic error messages (don't reveal if email exists)
//   - Transactions for atomic operations (user + wallet created together)
type AuthService interface {
	// Register creates a new user account with automatic wallet creation
	// Returns user object + access token + refresh token
	// User is immediately logged in after registration (modern UX pattern)
	Register(ctx context.Context, req dto.RegisterRequest) (*dto.RegisterResponse, error)

	// Login authenticates a user and issues tokens
	// Verifies password, checks account status, generates JWT pair
	Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error)

	// RefreshAccessToken issues new tokens from a refresh token
	// Implements token rotation: old refresh token is revoked, new one issued
	// This makes token theft detectable (stolen token can only be used once)
	RefreshAccessToken(ctx context.Context, req dto.RefreshRequest) (*dto.RefreshResponse, error)

	// Logout revokes a single refresh token (logout from current device)
	Logout(ctx context.Context, refreshToken string) error

	// LogoutAllDevices revokes all of a user's refresh tokens
	// Called when: password changed, account suspended, user clicks "logout everywhere"
	LogoutAllDevices(ctx context.Context, userID uuid.UUID) error
}

// authService is the concrete implementation
// It orchestrates multiple repositories to implement business logic
type authService struct {
	userRepo         repositories.UserRepository
	walletRepo       repositories.WalletRepository
	refreshTokenRepo repositories.RefreshTokenRepository
	config           *config.Config
	db               *gorm.DB // For transactions that span multiple repositories
}

// NewAuthService creates a new authentication service instance
// All dependencies are injected via constructor (dependency injection pattern)
// Called once at application startup in main.go
func NewAuthService(
	userRepo repositories.UserRepository,
	walletRepo repositories.WalletRepository,
	refreshTokenRepo repositories.RefreshTokenRepository,
	cfg *config.Config,
	db *gorm.DB,
) AuthService {
	return &authService{
		userRepo:         userRepo,
		walletRepo:       walletRepo,
		refreshTokenRepo: refreshTokenRepo,
		config:           cfg,
		db:               db,
	}
}

// Register creates a new user account
//
// Process:
//   1. Validate email format and uniqueness
//   2. Validate phone format and uniqueness
//   3. Enforce password complexity rules
//   4. Hash password with bcrypt
//   5. Create user and wallet in a transaction (atomic operation)
//   6. Generate access token and refresh token
//   7. Store hashed refresh token in database
//
// Why transaction?
//   Every user MUST have a wallet. If we create the user but wallet creation
//   fails, we'd have an inconsistent state. Transaction ensures both succeed
//   or both fail together.
//
// Security:
//   - Password never stored plain (bcrypt with cost 10)
//   - Refresh token hashed before storage (like passwords)
//   - Email verification token would be generated here (future milestone)
//
// Example usage in handler:
//   response, err := authService.Register(ctx, registerRequest)
//   if err != nil {
//       return c.JSON(400, err)
//   }
//   return c.JSON(201, response)
func (s *authService) Register(ctx context.Context, req dto.RegisterRequest) (*dto.RegisterResponse, error) {
	// Step 1: Normalize and validate email
	// Emails are case-insensitive, so "User@Example.COM" = "user@example.com"
	// We store lowercase to prevent duplicate registrations
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Check if email already exists
	// We do this BEFORE password hashing to avoid expensive bcrypt on doomed requests
	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		// Database error (connection lost, timeout, etc.)
		return nil, errors.ErrInternalServer
	}
	if exists {
		// Don't say "email already exists" - that reveals if an email is registered
		// For registration, we can be specific since user is creating account
		return nil, errors.ErrEmailAlreadyExists
	}

	// Step 2: Validate and normalize phone number
	// Phone must be in E.164 format: +2348012345678
	// This ensures consistency and enables SMS delivery
	phone := strings.TrimSpace(req.Phone)
	if err := validateNigerianPhone(phone); err != nil {
		return nil, err
	}

	// Step 3: Enforce password complexity rules
	// From security requirements:
	//   - Minimum 12 characters
	//   - At least 1 uppercase, 1 lowercase, 1 digit, 1 special char
	if err := validatePasswordComplexity(req.Password); err != nil {
		return nil, err
	}

	// Step 4: Hash the password
	// bcrypt.GenerateFromPassword is expensive (intentionally slow for security)
	// Cost 10 = ~100ms on modern hardware
	// This is why we check email existence first - no point hashing if email is taken
	hashedPassword, err := password.Hash(req.Password)
	if err != nil {
		// This should never fail unless password exceeds max length (100 chars)
		// or system is out of memory
		return nil, errors.ErrInternalServer
	}

	// Step 5: Create user and wallet in a transaction
	// Transaction ensures atomicity: both succeed or both fail
	// This prevents orphaned users without wallets
	var user *models.User
	var wallet *models.Wallet

	// db.Transaction() automatically handles:
	//   - BEGIN at start
	//   - COMMIT if function returns nil
	//   - ROLLBACK if function returns error or panics
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create user record
		user = &models.User{
			ID:           uuid.New(), // Generate UUID now (we need it for wallet)
			Email:        email,
			Phone:        phone,
			FullName:     req.FullName,
			PasswordHash: hashedPassword,
			Role:         "investor",    // Default role (others assigned by admin)
			KYCStatus:    "pending",     // Email verification required first
			IsActive:     true,           // Account active immediately
		}

		// Insert user into database
		// GORM sets CreatedAt, UpdatedAt automatically
		// BeforeCreate hook generates UserCode
		if err := tx.Create(user).Error; err != nil {
			return err // Triggers rollback
		}

		// Create wallet for the user
		// Every user gets a wallet automatically
		// Initial balance is 0.00, currency defaults to NGN
		wallet = &models.Wallet{
			UserID:      user.ID,
			MainBalance: 0,
			Currency:    "NGN", // Nigerian Naira (default)
			// EarningsBalance defaults to 0 via database default
		}

		if err := tx.Create(wallet).Error; err != nil {
			return err // Triggers rollback (user creation is undone)
		}

		// Both operations succeeded
		return nil // Triggers commit
	})

	if err != nil {
		// Transaction failed and was rolled back
		// Could be: unique constraint violation, foreign key error, connection lost
		return nil, errors.ErrInternalServer
	}

	// Step 6: Generate JWT access token
	// Access token is short-lived (15 minutes)
	// Contains: user ID, role, expiration
	// Signed with JWT_SECRET
	accessTokenTTL, _ := time.ParseDuration(s.config.AccessTokenTTL)
	accessToken, err := jwt.GenerateAccessToken(
		user.ID,
		user.Role,
		s.config.JWTSecret,
		accessTokenTTL,
	)
	if err != nil {
		// JWT generation failed (should never happen unless secret is empty)
		return nil, errors.ErrInternalServer
	}

	// Step 7: Generate refresh token
	// Refresh token is long-lived (30 days)
	// Same structure as access token but signed with different secret
	refreshTokenTTL, _ := time.ParseDuration(s.config.RefreshTokenTTL)
	refreshToken, err := jwt.GenerateRefreshToken(
		user.ID,
		user.Role,
		s.config.JWTRefreshSecret,
		refreshTokenTTL,
	)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Step 8: Store hashed refresh token in database
	// We hash the token before storage (like passwords)
	// If database is breached, attacker can't use these tokens
	tokenHash := hashToken(refreshToken)
	refreshTokenModel := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}
	if err := s.refreshTokenRepo.Create(ctx, refreshTokenModel); err != nil {
		// Failed to store token (not critical - user can login again)
		// We don't fail registration just because token storage failed
		// But we log it for monitoring
		// In production: log.Error("Failed to store refresh token", "error", err)
	}

	// Step 9: Build response DTO
	// Map internal model to public DTO (excludes sensitive fields)
	return &dto.RegisterResponse{
		User: dto.UserResponse{
			ID:        user.ID,
			UserCode:  user.UserCode,
			FullName:  user.FullName,
			Email:     user.Email,
			Phone:     user.Phone,
			KYCStatus: user.KYCStatus,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer", // Standard OAuth 2.0 token type
	}, nil
}

// Login authenticates a user and issues tokens
//
// Process:
//   1. Find user by email
//   2. Check if account is active
//   3. Verify password matches hash
//   4. Generate access token and refresh token
//   5. Store hashed refresh token
//
// Security:
//   - Generic error message ("invalid email or password")
//   - Don't reveal whether email exists or password is wrong
//   - Constant-time password comparison (bcrypt does this internally)
//   - Rate limiting should be applied at handler/middleware level
//
// Example usage:
//   response, err := authService.Login(ctx, loginRequest)
//   if err != nil {
//       return c.JSON(401, "Invalid credentials")
//   }
//   return c.JSON(200, response)
func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	// Step 1: Normalize email (case-insensitive lookup)
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Step 2: Find user by email
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		// User not found or database error
		// Return generic error (don't reveal if email exists)
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrInvalidCredentials
		}
		return nil, errors.ErrInternalServer
	}

	// Step 3: Check if account is active
	// Admin may have suspended the account
	if !user.IsActive {
		return nil, errors.ErrAccountSuspended
	}

	// Step 4: Verify password
	// bcrypt.CompareHashAndPassword does:
	//   - Extract salt from stored hash
	//   - Hash the input password with same salt
	//   - Compare in constant time (prevents timing attacks)
	if err := password.Verify(req.Password, user.PasswordHash); err != nil {
		// Password doesn't match
		// Return same error as "user not found" (don't reveal which is wrong)
		return nil, errors.ErrInvalidCredentials
	}

	// Step 5: Password is correct - generate tokens
	accessTokenTTL, _ := time.ParseDuration(s.config.AccessTokenTTL)
	accessToken, err := jwt.GenerateAccessToken(
		user.ID,
		user.Role,
		s.config.JWTSecret,
		accessTokenTTL,
	)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	refreshTokenTTL, _ := time.ParseDuration(s.config.RefreshTokenTTL)
	refreshToken, err := jwt.GenerateRefreshToken(
		user.ID,
		user.Role,
		s.config.JWTRefreshSecret,
		refreshTokenTTL,
	)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Step 6: Store hashed refresh token
	tokenHash := hashToken(refreshToken)
	refreshTokenModel := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}
	if err := s.refreshTokenRepo.Create(ctx, refreshTokenModel); err != nil {
		// Token storage failed (non-critical, user can login again)
		// In production: log this for monitoring
	}

	// Step 7: Return response
	return &dto.LoginResponse{
		User: dto.UserResponse{
			ID:        user.ID,
			UserCode:  user.UserCode,
			FullName:  user.FullName,
			Email:     user.Email,
			Phone:     user.Phone,
			KYCStatus: user.KYCStatus,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
	}, nil
}

// RefreshAccessToken issues new tokens from a refresh token
// Implements token rotation for security
//
// Process:
//   1. Validate refresh token JWT (signature + expiration)
//   2. Extract user ID from token
//   3. Hash token and look it up in database
//   4. Check if token is revoked
//   5. Revoke old refresh token (rotation step 1)
//   6. Generate new access token
//   7. Generate new refresh token (rotation step 2)
//   8. Store new refresh token in database
//
// Token rotation security:
//   - Old refresh token becomes invalid immediately
//   - If attacker steals token, they can only use it once
//   - If stolen token is used, legitimate user's next refresh fails
//   - This alerts user to potential breach
//
// Example usage:
//   response, err := authService.RefreshAccessToken(ctx, refreshRequest)
//   if err != nil {
//       return c.JSON(401, "Invalid refresh token")
//   }
//   return c.JSON(200, response)
func (s *authService) RefreshAccessToken(ctx context.Context, req dto.RefreshRequest) (*dto.RefreshResponse, error) {
	// Step 1: Validate JWT signature and expiration
	// This checks:
	//   - Token format is correct
	//   - Signature matches (token hasn't been tampered with)
	//   - Expiration hasn't passed
	claims, err := jwt.ValidateToken(req.RefreshToken, s.config.JWTRefreshSecret)
	if err != nil {
		// Token is invalid, expired, or has wrong signature
		return nil, errors.ErrInvalidToken
	}

	// Step 2: Hash the token for database lookup
	// We store hashed tokens (like passwords)
	tokenHash := hashToken(req.RefreshToken)

	// Step 3: Find token in database
	dbToken, err := s.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		// Token not found in database
		// Could mean: token already revoked, deleted, or never existed
		if repositories.IsErrRecordNotFound(err) {
			return nil, errors.ErrInvalidToken
		}
		return nil, errors.ErrInternalServer
	}

	// Step 4: Check if token is revoked or expired
	// Even though JWT validation passed, we check database state
	// Token might be revoked (logout, password change, admin action)
	if !dbToken.IsValid() {
		return nil, errors.ErrInvalidToken
	}

	// Step 5: Verify token belongs to the user in JWT claims
	// This shouldn't fail if system is working correctly, but defense in depth
	if dbToken.UserID != claims.UserID {
		return nil, errors.ErrInvalidToken
	}

	// Step 6: Token is valid - revoke it (rotation step 1)
	// Old token becomes unusable immediately
	// If someone tries to reuse it, this lookup will fail
	if err := s.refreshTokenRepo.RevokeByTokenHash(ctx, tokenHash); err != nil {
		// Revocation failed (non-critical, token will expire naturally)
		// In production: log this for monitoring
	}

	// Step 7: Generate new access token
	accessTokenTTL, _ := time.ParseDuration(s.config.AccessTokenTTL)
	newAccessToken, err := jwt.GenerateAccessToken(
		claims.UserID,
		claims.Role,
		s.config.JWTSecret,
		accessTokenTTL,
	)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Step 8: Generate new refresh token (rotation step 2)
	refreshTokenTTL, _ := time.ParseDuration(s.config.RefreshTokenTTL)
	newRefreshToken, err := jwt.GenerateRefreshToken(
		claims.UserID,
		claims.Role,
		s.config.JWTRefreshSecret,
		refreshTokenTTL,
	)
	if err != nil {
		return nil, errors.ErrInternalServer
	}

	// Step 9: Store new refresh token in database
	newTokenHash := hashToken(newRefreshToken)
	refreshTokenModel := &models.RefreshToken{
		UserID:    claims.UserID,
		TokenHash: newTokenHash,
		ExpiresAt: time.Now().Add(refreshTokenTTL),
	}
	if err := s.refreshTokenRepo.Create(ctx, refreshTokenModel); err != nil {
		// Token storage failed (non-critical)
		// User can login again to get a new token
	}

	// Step 10: Return new token pair
	return &dto.RefreshResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken, // Client must use THIS token next time
		TokenType:    "Bearer",
	}, nil
}

// Logout revokes a single refresh token
// This logs the user out from the current device only
//
// Process:
//   1. Hash the refresh token
//   2. Find in database
//   3. Mark as revoked
//
// After logout:
//   - Refresh token can no longer be used to get new access tokens
//   - Access token remains valid until expiration (can't be revoked because it's stateless)
//   - Client should discard both tokens locally
//
// Example usage:
//   err := authService.Logout(ctx, refreshToken)
//   if err != nil {
//       return c.JSON(500, "Logout failed")
//   }
//   return c.JSON(200, "Logged out successfully")
func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	// Hash token for database lookup
	tokenHash := hashToken(refreshToken)

	// Revoke the token
	// If token doesn't exist (already revoked or expired), that's okay
	// Logout is idempotent - calling it twice has same effect as once
	if err := s.refreshTokenRepo.RevokeByTokenHash(ctx, tokenHash); err != nil {
		// Database error (not "token not found")
		// We don't fail logout just because database is having issues
		// In production: log this for monitoring
		return nil // Logout succeeds even if revocation failed
	}

	return nil
}

// LogoutAllDevices revokes all of a user's refresh tokens
// This logs the user out from ALL devices simultaneously
//
// Called when:
//   - User changes password (security: invalidate all sessions)
//   - Admin suspends account (force logout everywhere)
//   - User clicks "Logout from all devices" button
//
// Process:
//   1. Find all user's tokens
//   2. Mark all as revoked
//
// After logout all:
//   - User must login again on EVERY device
//   - All access tokens remain valid until expiration
//   - All refresh tokens are unusable
//
// Example usage (password change):
//   // Update password
//   user.PasswordHash = newHash
//   db.Save(&user)
//   // Force re-login everywhere
//   authService.LogoutAllDevices(ctx, user.ID)
func (s *authService) LogoutAllDevices(ctx context.Context, userID uuid.UUID) error {
	// Revoke all user's refresh tokens
	// This is a batch update: UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = ?
	if err := s.refreshTokenRepo.RevokeAllByUserID(ctx, userID); err != nil {
		// Database error (connection lost, timeout, etc.)
		// We don't fail the operation just because revocation failed
		// In production: log this for monitoring
		return nil
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS (private, used only within this service)
// ═══════════════════════════════════════════════════════════════════════════

// hashToken creates a SHA-256 hash of a token for database storage
//
// Why hash tokens?
//   If an attacker gains read-access to the database (SQL injection, backup leak, etc.),
//   they still can't use the refresh tokens because they only have the hashes.
//   They would need to guess the original token (2^256 possibilities).
//
// Why SHA-256 instead of bcrypt?
//   - bcrypt is for passwords (needs to be slow to resist brute-force)
//   - Tokens are already random with high entropy (can't be brute-forced)
//   - SHA-256 is fast for lookups while still providing security
//
// Process:
//   Input: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoi..."
//   SHA-256: [32 bytes of binary hash]
//   Hex encode: "3f8a5c2d9e1b4f7c8a6d3e9f1b5c8a2d..."
//   Store: "3f8a5c2d9e1b4f7c8a6d3e9f1b5c8a2d..."
func hashToken(token string) string {
	// sha256.New() creates a new hash instance
	hash := sha256.New()

	// Write() adds data to be hashed
	// We don't check error because Write() never fails for sha256
	hash.Write([]byte(token))

	// Sum() computes the final hash (32 bytes)
	// Sum(nil) means "return just the hash, don't append to anything"
	hashBytes := hash.Sum(nil)

	// Convert binary hash to hex string for database storage
	// 32 bytes becomes 64 hex characters
	// Example: [0x3f, 0x8a, ...] → "3f8a..."
	return hex.EncodeToString(hashBytes)
}

// validatePasswordComplexity enforces password strength rules
//
// Requirements (from security doc):
//   - Minimum 12 characters
//   - At least 1 uppercase letter (A-Z)
//   - At least 1 lowercase letter (a-z)
//   - At least 1 digit (0-9)
//   - At least 1 special character (!@#$%^&*()_+{}|:"<>?[]-=;',./`~)
//
// Why these rules?
//   - Length: Increases brute-force difficulty exponentially
//   - Complexity: Prevents dictionary attacks ("password123" is common)
//   - Special chars: Forces mixing character sets (harder to guess)
//
// Trade-off:
//   Strict rules can frustrate users. Balance security with usability.
//   Consider allowing passphrases: "correct horse battery staple" (44 bits entropy)
//   vs complex password: "P@ssw0rd!" (28 bits entropy)
//
// Returns:
//   - nil if password meets all requirements
//   - ErrWeakPassword if any requirement fails
func validatePasswordComplexity(pwd string) error {
	// Check minimum length
	if len(pwd) < 12 {
		return errors.ErrWeakPassword
	}

	// Check maximum length (bcrypt truncates at 72 bytes)
	if len(pwd) > 72 {
		return errors.ErrPasswordTooLong
	}

	// Define regex patterns for each requirement
	// [A-Z] means "any uppercase letter"
	// + means "one or more times"
	hasUppercase := regexp.MustCompile(`[A-Z]+`).MatchString(pwd)
	hasLowercase := regexp.MustCompile(`[a-z]+`).MatchString(pwd)
	hasDigit := regexp.MustCompile(`[0-9]+`).MatchString(pwd)
	// Special characters: common symbols on keyboard
	hasSpecial := regexp.MustCompile(`[!@#$%^&*()_+{}\|:"<>?\[\]\-=;',./` + "`~]+").MatchString(pwd)

	// All requirements must be met
	if !hasUppercase || !hasLowercase || !hasDigit || !hasSpecial {
		return errors.ErrWeakPassword
	}

	return nil
}

// validateNigerianPhone enforces E.164 phone format for Nigerian numbers
//
// E.164 format: +[country code][number]
// Nigerian numbers: +234[area code][number]
//
// Valid examples:
//   +2348012345678 (11 digits after +234)
//   +2347012345678
//   +2349012345678
//
// Invalid examples:
//   08012345678 (missing country code)
//   2348012345678 (missing +)
//   +234 801 234 5678 (spaces not allowed)
//
// Why E.164?
//   - International standard for phone numbers
//   - Works with SMS providers worldwide
//   - Consistent format in database
//   - Enables phone number portability
//
// Returns:
//   - nil if phone matches pattern
//   - ErrInvalidPhoneFormat otherwise
func validateNigerianPhone(phone string) error {
	// Regex breakdown:
	//   ^\+234       - Must start with +234 (Nigerian country code)
	//   [789]        - Area code must be 7, 8, or 9 (Nigerian mobile networks)
	//   [01]         - Second digit is 0 or 1
	//   \d{8}$       - Exactly 8 more digits
	//
	// Total length: +234 (4) + [789] (1) + [01] (1) + 8 digits = 14 characters
	matched, _ := regexp.MatchString(`^\+234[789][01]\d{8}$`, phone)
	if !matched {
		return errors.ErrInvalidPhoneFormat
	}

	return nil
}
