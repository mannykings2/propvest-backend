package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mannykings2/propvest-backend/internal/audit"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/mailer"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/utils/jwt"
	"github.com/mannykings2/propvest-backend/internal/utils/password"
	"github.com/mannykings2/propvest-backend/internal/validators"
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

	// ── Milestone 1 completion: email verification + password reset ──

	// VerifyEmail consumes an email-verification token and marks the user verified.
	VerifyEmail(ctx context.Context, token string) error
	// ResendVerification issues a fresh verification email (invalidating prior ones).
	ResendVerification(ctx context.Context, email string) error
	// ForgotPassword starts the reset flow by emailing a reset link. Always
	// returns nil (we never reveal whether an email is registered).
	ForgotPassword(ctx context.Context, email string) error
	// ResetPassword consumes a reset token, sets the new password, and revokes
	// all of the user's sessions.
	ResetPassword(ctx context.Context, token, newPassword string) error
}

// authService is the concrete implementation
// It orchestrates multiple repositories to implement business logic
type authService struct {
	userRepo         repositories.UserRepository
	walletRepo       repositories.WalletRepository
	refreshTokenRepo repositories.RefreshTokenRepository
	tokenRepo        repositories.VerificationTokenRepository
	mailer           mailer.Sender
	audit            audit.Recorder
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
	tokenRepo repositories.VerificationTokenRepository,
	emailSender mailer.Sender,
	auditRecorder audit.Recorder,
	cfg *config.Config,
	db *gorm.DB,
) AuthService {
	return &authService{
		userRepo:         userRepo,
		walletRepo:       walletRepo,
		refreshTokenRepo: refreshTokenRepo,
		tokenRepo:        tokenRepo,
		mailer:           emailSender,
		audit:            auditRecorder,
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
	if err := validators.ValidateNigerianPhone(phone); err != nil {
		return nil, err
	}

	// FIX-02: pre-check phone uniqueness. The DB has a unique index on phone,
	// so without this a duplicate phone would surface as a generic transaction
	// error and (previously) be mapped to HTTP 500. Checking here lets us return
	// a clean 409 Conflict. The unique constraint is still the race-safe backstop
	// (handled below by translatePgUniqueViolation).
	phoneExists, err := s.userRepo.ExistsByPhone(ctx, phone)
	if err != nil {
		return nil, errors.ErrInternalServer
	}
	if phoneExists {
		return nil, errors.ErrPhoneAlreadyExists
	}

	// Step 3: Enforce password complexity rules
	// From security requirements:
	//   - Minimum 12 characters
	//   - At least 1 uppercase, 1 lowercase, 1 digit, 1 special char
	if err := validators.ValidatePasswordComplexity(req.Password); err != nil {
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
		// Transaction failed and was rolled back.
		// FIX-02: translate Postgres unique-constraint violations (race between
		// our pre-check and the INSERT) into 409-mapped domain errors instead of
		// a blanket 500. Anything else is a genuine internal error and is logged.
		if mapped := translatePgUniqueViolation(err); mapped != nil {
			return nil, mapped
		}
		logger.Error("register: user+wallet transaction failed", "email", email, "error", err)
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
		// Failed to store token (not critical - user can login again).
		// We don't fail registration just because token storage failed,
		// but we MUST log it (FIX-03: previously this error was silently dropped).
		logger.Error("register: failed to store refresh token", "user_id", user.ID, "error", err)
	}

	// Step 8b: Fire off the email-verification link (best-effort; a failure here
	// must not fail registration — the user can request a resend).
	s.issueVerificationEmail(ctx, user)
	s.audit.Record(ctx, audit.Event{ActorID: user.ID.String(), Action: "user.registered", TargetType: "user", TargetID: user.ID.String()})

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
		// Token storage failed (non-critical, user can login again).
		// FIX-03: log instead of silently swallowing.
		logger.Error("login: failed to store refresh token", "user_id", user.ID, "error", err)
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
	// Old token becomes unusable immediately.
	// FIX-03: this is security-critical. If we cannot revoke the old token we
	// must NOT hand out a new pair while the old one is still usable, so we
	// fail closed (log + error) instead of silently continuing.
	if err := s.refreshTokenRepo.RevokeByTokenHash(ctx, tokenHash); err != nil {
		logger.Error("refresh: failed to revoke old token during rotation", "user_id", claims.UserID, "error", err)
		return nil, errors.ErrInternalServer
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
		// Token storage failed. The old token is already revoked, so the safest
		// outcome is to fail the refresh (FIX-03: log + error, was silently swallowed).
		logger.Error("refresh: failed to store rotated refresh token", "user_id", claims.UserID, "error", err)
		return nil, errors.ErrInternalServer
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
		// Database error (not "token not found").
		// FIX-03: log it. Logout stays idempotent/best-effort from the client's
		// perspective, but the failure is now observable instead of invisible.
		logger.Error("logout: failed to revoke refresh token", "error", err)
		return nil
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
	// FIX-03: this is security-critical (also invoked after password change).
	// Propagate the error so the caller/handler fails closed rather than
	// telling the user "logged out everywhere" when sessions may still be live.
	if err := s.refreshTokenRepo.RevokeAllByUserID(ctx, userID); err != nil {
		logger.Error("logout-all: failed to revoke all refresh tokens", "user_id", userID, "error", err)
		return errors.ErrInternalServer
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

// translatePgUniqueViolation inspects a database error and, if it is a Postgres
// unique-constraint violation (SQLSTATE 23505), returns the matching domain
// error so the HTTP layer maps it to 409 Conflict. It returns nil for any other
// error, letting the caller treat it as a genuine internal error.
//
// This is the race-safe backstop for registration: even if two requests pass the
// ExistsByEmail/ExistsByPhone pre-checks simultaneously, the database's unique
// index still rejects the second INSERT, and we surface a clean conflict.
func translatePgUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if !stderrors.As(err, &pgErr) {
		return nil
	}
	if pgErr.Code != "23505" { // unique_violation
		return nil
	}
	// pgErr.ConstraintName / Detail tells us which column collided.
	detail := strings.ToLower(pgErr.ConstraintName + " " + pgErr.Detail)
	switch {
	case strings.Contains(detail, "email"):
		return errors.ErrEmailAlreadyExists
	case strings.Contains(detail, "phone"):
		return errors.ErrPhoneAlreadyExists
	default:
		return errors.ErrUserExists
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// EMAIL VERIFICATION AND PASSWORD RESET METHODS
// ═══════════════════════════════════════════════════════════════════════════

// VerifyEmail consumes an email-verification token and marks the user verified.
func (s *authService) VerifyEmail(ctx context.Context, token string) error {
	// TODO: Implement email verification logic
	// 1. Validate token format
	// 2. Look up token in verification_tokens table
	// 3. Check if token is expired
	// 4. Mark user's email as verified
	// 5. Delete/invalidate the token
	logger.Info("VerifyEmail not yet implemented", "token", token)
	return errors.ErrNotImplemented
}

// ResendVerification issues a fresh verification email (invalidating prior ones).
func (s *authService) ResendVerification(ctx context.Context, email string) error {
	// TODO: Implement resend verification logic
	// 1. Find user by email
	// 2. Check if already verified
	// 3. Invalidate old tokens
	// 4. Generate new token
	// 5. Send verification email
	logger.Info("ResendVerification not yet implemented", "email", email)
	return errors.ErrNotImplemented
}

// ForgotPassword starts the reset flow by emailing a reset link.
// Always returns nil (we never reveal whether an email is registered).
func (s *authService) ForgotPassword(ctx context.Context, email string) error {
	// TODO: Implement forgot password logic
	// 1. Find user by email (silently fail if not found)
	// 2. Generate password reset token
	// 3. Store token with expiration
	// 4. Send reset email with token link
	// Always return nil to prevent email enumeration
	logger.Info("ForgotPassword not yet implemented", "email", email)
	return nil // Always return nil for security
}

// ResetPassword consumes a reset token, sets the new password, and revokes
// all of the user's sessions.
func (s *authService) ResetPassword(ctx context.Context, token, newPassword string) error {
	// TODO: Implement reset password logic
	// 1. Validate token
	// 2. Look up token in verification_tokens table
	// 3. Check if token is expired
	// 4. Validate new password complexity
	// 5. Hash new password
	// 6. Update user's password
	// 7. Revoke all refresh tokens (force logout everywhere)
	// 8. Invalidate the reset token
	logger.Info("ResetPassword not yet implemented", "token", token)
	return errors.ErrNotImplemented
}

// issueVerificationEmail is a helper that generates a verification token and
// sends it to the user. This is best-effort and should not fail registration.
func (s *authService) issueVerificationEmail(ctx context.Context, user *models.User) {
	// TODO: Implement verification email sending
	// 1. Generate secure random token
	// 2. Store token in verification_tokens table with expiration
	// 3. Build verification URL with token
	// 4. Send email via mailer service
	// Log errors but don't propagate (registration already succeeded)
	logger.Info("issueVerificationEmail not yet implemented", "user_id", user.ID, "email", user.Email)
}
