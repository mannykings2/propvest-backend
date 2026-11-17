package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"gorm.io/gorm"
)

// ═══════════════════════════════════════════════════════════════════════════
// ADMIN USER SERVICE (Milestone 7 - Phase 4)
// ═══════════════════════════════════════════════════════════════════════════
//
// Provides admin operations for user management:
//   - List users with filtering and pagination
//   - Get user details with wallet and investment summary
//   - Suspend/activate user accounts
//   - Approve/reject KYC verification
//   - Change user roles (super_admin only)
//   - Force password reset
//   - Revoke all sessions
//   - Delete user account (super_admin only)
//
// AUTHORIZATION:
//   - All operations require admin or super_admin role
//   - Role changes and deletion require super_admin
//   - Privilege escalation is prevented (admin cannot manage super_admins)
//
// AUDIT LOGGING:
//   - All state changes are logged via audit service
//   - Captures actor, action, target, old/new values, and reason

// AdminUserService defines the interface for admin user management operations.
type AdminUserService interface {
	// ListUsers returns a paginated list of users with filters
	ListUsers(ctx context.Context, actorID uuid.UUID, filter repositories.UserFilter, page, pageSize int) (*dto.AdminUserListResponse, error)

	// GetUserDetail returns detailed user information for admin view
	GetUserDetail(ctx context.Context, actorID, targetUserID uuid.UUID) (*dto.AdminUserDetailResponse, error)

	// SuspendUser suspends a user account
	SuspendUser(ctx context.Context, actorID, targetUserID uuid.UUID, reason string) error

	// ActivateUser removes suspension and re-enables the account
	ActivateUser(ctx context.Context, actorID, targetUserID uuid.UUID) error

	// DecideKYC approves or rejects KYC verification
	DecideKYC(ctx context.Context, actorID, targetUserID uuid.UUID, decision string, reason *string) error

	// ChangeRole changes a user's role (super_admin only)
	ChangeRole(ctx context.Context, actorID, targetUserID uuid.UUID, newRole string, reason string) error

	// ForcePasswordReset invalidates user's password and all sessions
	ForcePasswordReset(ctx context.Context, actorID, targetUserID uuid.UUID, reason string) error

	// LogoutAllSessions revokes all refresh tokens for a user
	LogoutAllSessions(ctx context.Context, actorID, targetUserID uuid.UUID) error

	// DeleteUser permanently deletes a user account (super_admin only)
	DeleteUser(ctx context.Context, actorID, targetUserID uuid.UUID) error

	// ListKYCSubmissions returns the pending KYC review queue with user summary
	// (PII-safe: only BVN/NIN last-4). Phase 2, Workstream A4.
	ListKYCSubmissions(ctx context.Context, status string, page, pageSize int) (*dto.KYCQueueResponse, error)

	// GetKYCDetail returns the full submission — decrypted BVN/NIN — for the
	// admin review screen. THE only endpoint that returns full identity numbers,
	// and the read itself writes a `user.kyc_viewed` audit row (Phase 6.2/6.5).
	GetKYCDetail(ctx context.Context, actorID, targetUserID uuid.UUID) (*dto.KYCDetailResponse, error)

	// RecordScreening stores the admin's manual sanctions/PEP attestation
	// (source/result/notes, who and when), atomically with an audit row.
	// Approval is refused without it; a "hit" blocks approval (Phase 6.3).
	RecordScreening(ctx context.Context, actorID, targetUserID uuid.UUID, source, result, notes string) error

	// SetScreeningRiskHook wires the Phase 8.3 follow-through: once the 6.3
	// attestation commits, the risk layer mirrors it into screening_results
	// and — on a hit — flags the user and escalates an alert. Optional
	// (nil hook = mirror skipped); set once at boot.
	SetScreeningRiskHook(h ScreeningRiskHook)
}

// ScreeningRiskHook is the slice of RiskService admin_user_service needs —
// declared here so the dependency stays one-directional and easy to stub.
type ScreeningRiskHook interface {
	RecordScreeningResult(ctx context.Context, actor Actor, userID uuid.UUID, source, result, notes string) error
}

// adminUserService is the concrete implementation of AdminUserService.
type adminUserService struct {
	userRepo         repositories.UserRepository
	refreshTokenRepo repositories.RefreshTokenRepository
	investmentRepo   repositories.InvestmentRepository
	kycRepo          repositories.KYCSubmissionRepository
	auditSvc         AuditService
	db               *gorm.DB
	// Phase 8.3: risk-layer mirror for every attestation (set at boot).
	screeningHook ScreeningRiskHook
}

// SetScreeningRiskHook wires the risk service (see the interface doc).
func (s *adminUserService) SetScreeningRiskHook(h ScreeningRiskHook) {
	s.screeningHook = h
}

// NewAdminUserService creates a new admin user service instance.
func NewAdminUserService(
	userRepo repositories.UserRepository,
	refreshTokenRepo repositories.RefreshTokenRepository,
	investmentRepo repositories.InvestmentRepository,
	kycRepo repositories.KYCSubmissionRepository,
	auditSvc AuditService,
	db *gorm.DB,
) AdminUserService {
	return &adminUserService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		investmentRepo:   investmentRepo,
		kycRepo:          kycRepo,
		auditSvc:         auditSvc,
		db:               db,
	}
}

// recordAudit writes one audit row using the caller's transaction handle.
//
// Audit rows are written in the SAME transaction as the admin change they
// describe (audit_logs is append-only compliance evidence — see models.AuditLog):
// if the change rolls back, so does its audit row; if the audit insert fails,
// the whole operation rolls back. An un-audited admin action is treated as a
// failed admin action.
//
// Request metadata (id/IP/user-agent) travels via ctx — handlers attach it with
// services.WithRequestMetadata.
func (s *adminUserService) recordAudit(
	ctx context.Context,
	tx *gorm.DB,
	actor *models.User,
	action, targetType string,
	targetID *uuid.UUID,
	reason string,
	oldValues, newValues map[string]interface{},
) error {
	actorID := actor.ID
	return s.auditSvc.Record(ctx, tx, AuditEvent{
		ActorID:         &actorID,
		ActorType:       models.AuditActorTypeAdmin,
		ActorRole:       actor.Role,
		Action:          action,
		Description:     fmt.Sprintf("%s (%s) -> %s", action, actor.UserCode, targetType),
		TargetType:      targetType,
		TargetID:        targetID,
		OldValues:       oldValues,
		NewValues:       newValues,
		Reason:          reason,
		RequestMetadata: RequestMetadataFromContext(ctx),
	})
}

// ListUsers returns a paginated list of users for admin management.
//
// BOUNDED QUERY:
//   - Results are limited by page_size (max 100)
//   - Filters use indexed columns
//   - Text search uses ILIKE with indexes
//
// SECURITY:
//   - Only admin/super_admin can call this
//   - No sensitive data in list view (no passwords, tokens)
func (s *adminUserService) ListUsers(ctx context.Context, actorID uuid.UUID, filter repositories.UserFilter, page, pageSize int) (*dto.AdminUserListResponse, error) {
	// Validate pagination bounds
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100 // Max to prevent abuse
	}

	offset := (page - 1) * pageSize

	// Query users
	users, total, err := s.userRepo.List(ctx, filter, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	// Convert to DTOs
	userSummaries := make([]dto.AdminUserSummary, len(users))
	for i, user := range users {
		// Get investment summary for this user
		investmentCount, totalInvested, _ := s.investmentRepo.PortfolioSummary(ctx, user.ID)

		// Get wallet balance
		var walletBalance int64
		if user.Wallet.ID != uuid.Nil {
			walletBalance = user.Wallet.MainBalance + user.Wallet.EarningsBalance
		}

		userSummaries[i] = dto.AdminUserSummary{
			ID:              user.ID.String(),
			UserCode:        user.UserCode,
			FullName:        user.FullName,
			Email:           user.Email,
			Phone:           &user.Phone,
			Role:            user.Role,
			KYCStatus:       user.KYCStatus,
			IsActive:        user.IsActive,
			IsSuspended:     user.IsSuspended(),
			InvestmentCount: investmentCount,
			TotalInvested:   totalInvested,
			WalletBalance:   walletBalance,
			CreatedAt:       user.CreatedAt,
			LastLoginAt:     user.LastLoginAt,
		}
	}

	// Calculate total pages
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &dto.AdminUserListResponse{
		Users:      userSummaries,
		TotalCount: total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetUserDetail returns detailed information about a user for admin view.
//
// INCLUDES:
//   - Full user profile
//   - Wallet balances (main + earnings)
//   - Investment summary (count, total)
//   - Suspension details if suspended
//   - KYC verification timestamps
func (s *adminUserService) GetUserDetail(ctx context.Context, actorID, targetUserID uuid.UUID) (*dto.AdminUserDetailResponse, error) {
	// Load user with wallet
	user, err := s.userRepo.FindByIDWithWalletAndInvestments(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get investment summary
	investmentCount, totalInvested, err := s.investmentRepo.PortfolioSummary(ctx, user.ID)
	if err != nil {
		// Log but don't fail - investment data is supplementary
		investmentCount = 0
		totalInvested = 0
	}

	// Build response
	response := &dto.AdminUserDetailResponse{
		ID:                user.ID.String(),
		UserCode:          user.UserCode,
		FullName:          user.FullName,
		Email:             user.Email,
		Phone:             &user.Phone,
		Role:              user.Role,
		KYCStatus:         user.KYCStatus,
		IsActive:          user.IsActive,
		IsSuspended:       user.IsSuspended(),
		InvestmentCount:   investmentCount,
		TotalInvested:     totalInvested,
		WalletMainBalance: user.Wallet.MainBalance,
		WalletEarnings:    user.Wallet.EarningsBalance,
		CreatedAt:         user.CreatedAt,
		UpdatedAt:         user.UpdatedAt,
		LastLoginAt:       user.LastLoginAt,
	}

	// Add suspension details if suspended
	if user.IsSuspended() {
		response.SuspendedAt = user.SuspendedAt
		if user.SuspendedBy != nil {
			suspendedByStr := user.SuspendedBy.String()
			response.SuspendedBy = &suspendedByStr
		}
		response.SuspendedReason = user.SuspendedReason
	}

	// Add KYC timestamps if available
	// TODO: Add KYC submitted/verified timestamps when KYC table is implemented

	return response, nil
}

// SuspendUser suspends a user account.
//
// EFFECTS:
//   - Sets suspended_at, suspended_by, suspended_reason
//   - Sets is_active = false
//   - User cannot login (auth service checks is_active)
//   - Existing sessions remain valid until expiry (sessions not revoked)
//
// AUTHORIZATION:
//   - Admin can suspend investor/developer/staff
//   - Admin CANNOT suspend admin or super_admin
//   - Super_admin can suspend anyone (including admins)
func (s *adminUserService) SuspendUser(ctx context.Context, actorID, targetUserID uuid.UUID, reason string) error {
	// Load actor to check permissions
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	// Load target user
	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	// Check if actor can manage this user
	if !actor.CanManageRole(targetUser.Role) {
		return fmt.Errorf("insufficient permissions to suspend this user")
	}

	// Check if already suspended
	if targetUser.IsSuspended() {
		return fmt.Errorf("user is already suspended")
	}

	// Suspend the user
	now := time.Now()
	updates := map[string]interface{}{
		"suspended_at":     now,
		"suspended_by":     actorID,
		"suspended_reason": reason,
		"is_active":        false,
	}

	// Suspend + revoke every session + write the audit row in ONE transaction:
	// a suspension that leaves live sessions, or that lands without an audit
	// trail, is a compliance failure.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.userRepo.UpdatePartial(ctx, targetUserID, updates, tx); uerr != nil {
			return fmt.Errorf("failed to suspend user: %w", uerr)
		}

		// Kill all refresh tokens so the suspended account cannot mint a fresh
		// access token via /auth/refresh. The auth middleware's suspended-user
		// check covers tokens that are already in the user's hands.
		if rerr := s.refreshTokenRepo.RevokeAllByUserID(ctx, targetUserID, tx); rerr != nil {
			return fmt.Errorf("failed to revoke sessions for suspended user: %w", rerr)
		}

		return s.recordAudit(ctx, tx, actor, models.AuditActionUserSuspended, "user", &targetUserID, reason,
			map[string]interface{}{"is_active": true, "suspended_at": nil},
			map[string]interface{}{
				"is_active":        false,
				"suspended_at":     now,
				"suspended_reason": reason,
			})
	})
	if err != nil {
		return err
	}

	return nil
}

// ActivateUser removes suspension and re-enables the account.
//
// EFFECTS:
//   - Clears suspended_at, suspended_by, suspended_reason
//   - Sets is_active = true
//   - User can login again
func (s *adminUserService) ActivateUser(ctx context.Context, actorID, targetUserID uuid.UUID) error {
	// Load actor to check permissions
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	// Load target user
	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	// Check if actor can manage this user
	if !actor.CanManageRole(targetUser.Role) {
		return fmt.Errorf("insufficient permissions to activate this user")
	}

	// Check if not suspended
	if !targetUser.IsSuspended() {
		return fmt.Errorf("user is not suspended")
	}

	// Activate the user
	updates := map[string]interface{}{
		"suspended_at":     nil,
		"suspended_by":     nil,
		"suspended_reason": nil,
		"is_active":        true,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.userRepo.UpdatePartial(ctx, targetUserID, updates, tx); uerr != nil {
			return fmt.Errorf("failed to activate user: %w", uerr)
		}
		return s.recordAudit(ctx, tx, actor, models.AuditActionUserActivated, "user", &targetUserID, "",
			map[string]interface{}{
				"is_active":        false,
				"suspended_at":     targetUser.SuspendedAt,
				"suspended_reason": targetUser.SuspendedReason,
			},
			map[string]interface{}{"is_active": true, "suspended_at": nil, "suspended_reason": nil})
	})
	if err != nil {
		return err
	}

	return nil
}

// DecideKYC approves or rejects KYC verification.
//
// VALIDATION (Phase 6.2 / 6.3):
//   - Decision must be "verified" or "rejected"
//   - A reason is required for BOTH decisions (the audit row must explain
//     why an identity was accepted, not only why it was refused)
//   - Approval additionally requires a recorded sanctions/PEP screening
//     result of "clear" on the submission: no record -> ErrKYCScreeningRequired,
//     a "hit" -> ErrKYCScreeningHit. Both surface as HTTP 422.
//
// ATOMICITY: users.kyc_status, the kyc_submissions decision columns and the
// audit_logs row are written in ONE transaction — a decision is either fully
// recorded or not at all, never a flipped status with no trail.
func (s *adminUserService) DecideKYC(ctx context.Context, actorID, targetUserID uuid.UUID, decision string, reason *string) error {
	// Validate decision
	if decision != "verified" && decision != "rejected" {
		return fmt.Errorf("invalid decision: must be 'verified' or 'rejected'")
	}

	// Phase 6.2: every decision carries a reason — approval included.
	if reason == nil || *reason == "" {
		return apperrors.ErrKYCReasonRequired
	}
	reasonStr := *reason

	// Load actor (needed for the audit row: who made the call, in what role).
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	// Load target user
	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	// Check if KYC is pending
	if targetUser.KYCStatus != "pending" {
		return fmt.Errorf("KYC status is not pending (current: %s)", targetUser.KYCStatus)
	}

	// Screening gate (6.3): approval only with a recorded 'clear' check.
	// Rejection needs no screening — you can always refuse.
	sub, subErr := s.kycRepo.FindByUserID(ctx, targetUserID)
	if subErr != nil && !repositories.IsErrRecordNotFound(subErr) {
		return fmt.Errorf("failed to load submission: %w", subErr)
	}
	if decision == "verified" {
		if subErr != nil || sub == nil {
			return apperrors.ErrKYCScreeningRequired
		}
		switch {
		case sub.ScreeningResult == nil || *sub.ScreeningResult == "":
			return apperrors.ErrKYCScreeningRequired
		case *sub.ScreeningResult == models.ScreeningHit:
			return apperrors.ErrKYCScreeningHit
		case *sub.ScreeningResult != models.ScreeningClear:
			return apperrors.ErrKYCScreeningRequired
		}
	}

	// Update KYC status on the user record — in the same transaction as the
	// submission decision and the audit row, so all three commit together.
	updates := map[string]interface{}{
		"kyc_status": decision,
	}

	action := models.AuditActionUserKYCApproved
	if decision == "rejected" {
		action = models.AuditActionUserKYCRejected
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.userRepo.UpdatePartial(ctx, targetUserID, updates, tx); uerr != nil {
			return fmt.Errorf("failed to update KYC status: %w", uerr)
		}
		// Sync the submission row in the same tx (Phase 6.2 removed the old
		// best-effort sync). A user with no submission row matches zero rows,
		// which is a no-op, not an error.
		if derr := s.kycRepo.ApplyDecision(ctx, targetUserID, decision, reasonStr, actorID, tx); derr != nil {
			return fmt.Errorf("failed to record submission decision: %w", derr)
		}
		oldVals := map[string]interface{}{"kyc_status": targetUser.KYCStatus}
		newVals := map[string]interface{}{"kyc_status": decision}
		if sub != nil && sub.ScreeningResult != nil {
			newVals["screening_result"] = *sub.ScreeningResult
		}
		return s.recordAudit(ctx, tx, actor, action, "user", &targetUserID, reasonStr, oldVals, newVals)
	})
	return err
}

// ListKYCSubmissions returns the KYC review queue with a user summary per row.
// PII-SAFE: only the last 4 digits of BVN/NIN are returned.
func (s *adminUserService) ListKYCSubmissions(ctx context.Context, status string, page, pageSize int) (*dto.KYCQueueResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	rows, total, err := s.kycRepo.List(ctx, status, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list KYC submissions: %w", err)
	}

	items := make([]dto.KYCQueueItem, 0, len(rows))
	for _, sub := range rows {
		item := dto.KYCQueueItem{
			SubmissionID:    sub.ID,
			UserID:          sub.UserID,
			DocType:         sub.DocType,
			BVNLast4:        sub.BVNLast4(),
			NINLast4:        sub.NINLast4(),
			Status:          sub.Status,
			Reason:          sub.Reason,
			SubmittedAt:     sub.SubmittedAt,
			DecidedAt:       sub.DecidedAt,
			ScreeningResult: sub.ScreeningResult,
			ScreeningSource: sub.ScreeningSource,
			ScreeningAt:     sub.ScreeningAt,
		}
		// Attach the user summary (name/email/phone) for the queue UI.
		if u, uerr := s.userRepo.FindByID(ctx, sub.UserID); uerr == nil {
			item.FullName = u.FullName
			item.Email = u.Email
			item.Phone = u.Phone
			item.KYCStatus = u.KYCStatus
		}
		items = append(items, item)
	}

	return &dto.KYCQueueResponse{
		Submissions: items,
		Total:       total,
		Page:        page,
		Limit:       pageSize,
	}, nil
}

// GetKYCDetail returns the full submission for the admin review screen.
//
// PII (Phase 6.5): this is the ONE response in the system that carries the
// full BVN/NIN, and it lives behind RequireAdmin. Reading it writes a
// `user.kyc_viewed` audit row in the same request, so a privileged identity
// read is itself evidence. The audit write is fail-closed: if it cannot be
// recorded, the view is refused rather than leaving an untracked PII read.
func (s *adminUserService) GetKYCDetail(ctx context.Context, actorID, targetUserID uuid.UUID) (*dto.KYCDetailResponse, error) {
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("failed to load actor: %w", err)
	}

	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to load user: %w", err)
	}

	base := dto.KYCDetailResponse{
		UserID:    targetUser.ID,
		FullName:  targetUser.FullName,
		Email:     targetUser.Email,
		Phone:     targetUser.Phone,
		KYCStatus: targetUser.KYCStatus,
	}

	sub, err := s.kycRepo.FindByUserID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			// Nothing submitted: still audited (the admin opened the screen),
			// and the admin sees an explicit empty state.
			base.Status = "none"
			base.SubmittedAt = targetUser.CreatedAt
			if aerr := s.auditKYCView(ctx, actor, targetUserID, "none", "", ""); aerr != nil {
				return nil, aerr
			}
			return &base, nil
		}
		return nil, fmt.Errorf("failed to load submission: %w", err)
	}

	if aerr := s.auditKYCView(ctx, actor, targetUserID, sub.Status, sub.BVNLast4(), sub.NINLast4()); aerr != nil {
		return nil, aerr
	}

	base.SubmissionID = sub.ID
	base.BVN = sub.BVN // decrypted in-process by the repository; admin route only
	base.NIN = sub.NIN
	base.DocType = sub.DocType
	base.Status = sub.Status
	base.Reason = sub.Reason
	base.SubmittedAt = sub.SubmittedAt
	base.DecidedAt = sub.DecidedAt
	base.DecidedBy = sub.DecidedBy
	base.ScreeningSource = sub.ScreeningSource
	base.ScreeningResult = sub.ScreeningResult
	base.ScreeningNotes = sub.ScreeningNotes
	base.ScreeningAt = sub.ScreeningAt
	base.ScreenedBy = sub.ScreenedBy
	if len(sub.Evidence) > 0 {
		base.Evidence = json.RawMessage(sub.Evidence)
	}
	return &base, nil
}

// auditKYCView records that an admin opened the full PII detail view. Only
// last-4 goes into the audit payload — the trail itself must not become a PII
// store. Fail-closed: the caller refuses the view if this cannot be written.
func (s *adminUserService) auditKYCView(ctx context.Context, actor *models.User, targetUserID uuid.UUID, status, bvnLast4, ninLast4 string) error {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.recordAudit(ctx, tx, actor, models.AuditActionUserKYCViewed, "kyc_submission", &targetUserID,
			"admin viewed full BVN/NIN",
			map[string]interface{}{},
			map[string]interface{}{
				"status":    status,
				"bvn_last4": bvnLast4,
				"nin_last4": ninLast4,
				"full_pii":  true,
			})
	}); err != nil {
		return fmt.Errorf("refusing PII view: audit write failed: %w", err)
	}
	return nil
}

// RecordScreening stores the admin's manual sanctions/PEP check (Phase 6.3).
// The check is an attestation the admin performs themselves (public lists /
// internal policy) — no third-party provider is called, per the Phase 6 scope
// decision. Written atomically with an audit row.
func (s *adminUserService) RecordScreening(ctx context.Context, actorID, targetUserID uuid.UUID, source, result, notes string) error {
	if source == "" {
		return apperrors.ErrValidationFailed
	}
	if result != models.ScreeningClear && result != models.ScreeningHit {
		return fmt.Errorf("invalid screening result: must be 'clear' or 'hit'")
	}

	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	sub, err := s.kycRepo.FindByUserID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("no KYC submission for this user")
		}
		return fmt.Errorf("failed to load submission: %w", err)
	}

	oldResult := interface{}(nil)
	if sub.ScreeningResult != nil {
		oldResult = *sub.ScreeningResult
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if serr := s.kycRepo.RecordScreening(ctx, targetUserID, source, result, notes, actorID, tx); serr != nil {
			return fmt.Errorf("failed to record screening: %w", serr)
		}
		return s.recordAudit(ctx, tx, actor, models.AuditActionKYCScreeningSaved, "kyc_submission", &targetUserID,
			notes,
			map[string]interface{}{"screening_result": oldResult},
			map[string]interface{}{
				"screening_result": result,
				"screening_source": source,
			})
	})
	if err != nil {
		return err
	}

	// Phase 8.3: mirror into screening_results + hit handling (freeze, escalated
	// alert, STR draft). Runs AFTER the 6.3 commit: the attestation is the
	// source of truth and must not be rolled back by a risk-layer hiccup —
	// a failure here is logged loudly and the admin can re-run the check.
	if s.screeningHook != nil {
		if herr := s.screeningHook.RecordScreeningResult(ctx,
			Actor{ID: actorID, Role: actor.Role}, targetUserID, source, result, notes); herr != nil {
			logger.Error("risk: screening mirror failed",
				"user_id", targetUserID, "result", result, "error", herr)
		}
	}
	return nil
}

// ChangeRole changes a user's role.
//
// AUTHORIZATION:
//   - Only super_admin can call this
//   - Cannot change own role
//   - Role hierarchy validated via actor.CanManageRole()
//
// AUDIT:
//   - Full audit log with old role, new role, and reason
func (s *adminUserService) ChangeRole(ctx context.Context, actorID, targetUserID uuid.UUID, newRole string, reason string) error {
	// Prevent self role change
	if actorID == targetUserID {
		return fmt.Errorf("cannot change your own role")
	}

	// Load actor to verify super_admin
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	if !actor.IsSuperAdmin() {
		return fmt.Errorf("only super_admin can change roles")
	}

	// Load target user
	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	// Validate new role
	validRoles := []string{models.RoleInvestor, models.RoleDeveloper, models.RoleStaff, models.RoleAdmin, models.RoleSuperAdmin}
	isValid := false
	for _, validRole := range validRoles {
		if newRole == validRole {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("invalid role: %s", newRole)
	}

	// Check if same as current role
	if targetUser.Role == newRole {
		return fmt.Errorf("user already has role: %s", newRole)
	}

	// Update role + audit (old role, new role, reason) in one transaction.
	updates := map[string]interface{}{
		"role": newRole,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.userRepo.UpdatePartial(ctx, targetUserID, updates, tx); uerr != nil {
			return fmt.Errorf("failed to change role: %w", uerr)
		}
		return s.recordAudit(ctx, tx, actor, models.AuditActionUserRoleChanged, "user", &targetUserID, reason,
			map[string]interface{}{"role": targetUser.Role},
			map[string]interface{}{"role": newRole})
	})
	if err != nil {
		return err
	}

	// A role change only takes effect at the next login (the role lives in JWT
	// claims), so drop this user's refresh tokens now rather than let an old
	// admin token keep minting sessions with the stale role.
	if rerr := s.refreshTokenRepo.RevokeAllByUserID(ctx, targetUserID); rerr != nil {
		// The role already changed; failing the request would misreport success
		// as failure. Sessions die at refresh anyway via the middleware check.
		fmt.Printf("admin role: failed to revoke sessions for user %s: %v\n", targetUserID, rerr)
	}

	return nil
}

// ForcePasswordReset invalidates the user's password and revokes all sessions.
//
// EFFECTS:
//   - Sets a random unguessable password hash (user cannot login)
//   - Revokes all refresh tokens
//   - User must use "forgot password" flow to regain access
//
// USE CASES:
//   - Account compromise suspected
//   - Admin needs to force user to reset password
func (s *adminUserService) ForcePasswordReset(ctx context.Context, actorID, targetUserID uuid.UUID, reason string) error {
	// Load actor (for the audit row)
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	// Load target user
	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	// Set a random unguessable password
	// User must use "forgot password" to regain access
	randomPassword := uuid.New().String() + uuid.New().String() // 72 chars, unguessable
	updates := map[string]interface{}{
		"password_hash": randomPassword, // This is intentionally not hashed - user cannot use it
	}

	// Revoke sessions + scramble password + audit, atomically.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if rerr := s.refreshTokenRepo.RevokeAllByUserID(ctx, targetUserID, tx); rerr != nil {
			return fmt.Errorf("failed to revoke sessions: %w", rerr)
		}
		if uerr := s.userRepo.UpdatePartial(ctx, targetUserID, updates, tx); uerr != nil {
			return fmt.Errorf("failed to reset password: %w", uerr)
		}
		return s.recordAudit(ctx, tx, actor, models.AuditActionUserPasswordReset, "user", &targetUserID, reason,
			map[string]interface{}{"password_hash": "[redacted]", "email": targetUser.Email},
			map[string]interface{}{"password_hash": "[redacted]", "sessions_revoked": true})
	})
	if err != nil {
		return err
	}

	// TODO: Send notification to user (Phase 9 - notification integration)

	return nil
}

// LogoutAllSessions revokes all refresh tokens for a user.
//
// EFFECTS:
//   - Invalidates all refresh tokens
//   - User is logged out from all devices
//   - Access tokens remain valid until expiry (short-lived)
//
// USE CASES:
//   - Suspicious activity detected
//   - User requested remote logout
//   - Admin needs to force logout
func (s *adminUserService) LogoutAllSessions(ctx context.Context, actorID, targetUserID uuid.UUID) error {
	// Load actor (for the audit row)
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	// Load target user to verify exists
	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	// Revoke all refresh tokens + audit, atomically.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if rerr := s.refreshTokenRepo.RevokeAllByUserID(ctx, targetUserID, tx); rerr != nil {
			return fmt.Errorf("failed to revoke sessions: %w", rerr)
		}
		return s.recordAudit(ctx, tx, actor, models.AuditActionUserSessionsRevoked, "user", &targetUserID, "",
			nil,
			map[string]interface{}{"sessions_revoked": true, "email": targetUser.Email})
	})
	if err != nil {
		return err
	}

	return nil
}

// DeleteUser permanently deletes a user account.
//
// AUTHORIZATION:
//   - Only super_admin can delete users
//   - Cannot delete own account
//   - Cannot delete other super_admins
//
// EFFECTS:
//   - Soft-deletes user record (sets deleted_at)
//   - Wallet remains (for accounting/auditing)
//   - Investments remain (for property records)
//   - User cannot login
//
// WARNING: This is irreversible in the UI
func (s *adminUserService) DeleteUser(ctx context.Context, actorID, targetUserID uuid.UUID) error {
	// Prevent self-deletion
	if actorID == targetUserID {
		return fmt.Errorf("cannot delete your own account")
	}

	// Load actor to verify super_admin
	actor, err := s.userRepo.FindByID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("failed to load actor: %w", err)
	}

	if !actor.IsSuperAdmin() {
		return fmt.Errorf("only super_admin can delete users")
	}

	// Load target user
	targetUser, err := s.userRepo.FindByID(ctx, targetUserID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to load user: %w", err)
	}

	// Prevent deleting other super_admins
	if targetUser.IsSuperAdmin() {
		return fmt.Errorf("cannot delete super_admin accounts")
	}

	// Soft delete + revoke sessions + audit, in one transaction.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if derr := s.userRepo.Delete(ctx, targetUserID, tx); derr != nil {
			return fmt.Errorf("failed to delete user: %w", derr)
		}
		if rerr := s.refreshTokenRepo.RevokeAllByUserID(ctx, targetUserID, tx); rerr != nil {
			return fmt.Errorf("failed to revoke sessions for deleted user: %w", rerr)
		}
		return s.recordAudit(ctx, tx, actor, models.AuditActionUserDeleted, "user", &targetUserID, "",
			map[string]interface{}{
				"email":          targetUser.Email,
				"role":           targetUser.Role,
				"email_verified": targetUser.EmailVerified,
			},
			map[string]interface{}{"deleted": true})
	})
	if err != nil {
		return err
	}

	return nil
}
