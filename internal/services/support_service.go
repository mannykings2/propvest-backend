package services

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"gorm.io/gorm"
)

// TicketCategories is the allowlist shown on the Support page. Keeping it here
// (not in a binding tag) means one place decides what a valid category is, and
// the service rejects anything else with 422 instead of storing junk.
var TicketCategories = []string{
	"Rental Income",
	"Distribution",
	"Investment / Slots",
	"KYC / Verification",
	"Documents",
	"Withdrawal",
	"Off-Plan",
	"Other",
}

func validCategory(c string) bool {
	for _, v := range TicketCategories {
		if v == c {
			return true
		}
	}
	return false
}

// SupportService manages support tickets: the user-facing conversation
// (create/list/get/reply, always scoped to the owner) and the agent queue
// (list/get/reply/status, called from RBAC-gated admin routes).
type SupportService interface {
	// ── User-facing (owner-scoped) ──
	Create(ctx context.Context, userID uuid.UUID, req *dto.CreateTicketRequest) (*dto.TicketDetailResponse, error)
	List(ctx context.Context, userID uuid.UUID, status string, page, limit int) (*dto.TicketListResponse, error)
	Get(ctx context.Context, userID, ticketID uuid.UUID) (*dto.TicketDetailResponse, error)
	Reply(ctx context.Context, userID, ticketID uuid.UUID, message string) (*dto.TicketReplyResponse, error)

	// ── Agent queue (RBAC-gated at the route) ──
	AdminList(ctx context.Context, status, category string, page, limit int) (*dto.TicketListResponse, error)
	AdminGet(ctx context.Context, ticketID uuid.UUID) (*dto.TicketDetailResponse, error)
	AdminReply(ctx context.Context, agentID, ticketID uuid.UUID, message string) (*dto.TicketReplyResponse, error)
	AdminUpdateStatus(ctx context.Context, agentID, ticketID uuid.UUID, status string) (*dto.TicketDetailResponse, error)
}

type supportService struct {
	repo          repositories.SupportRepository
	notifications NotificationService // may be nil in tests
}

// NewSupportService constructs the service. notifications may be nil (tests):
// ticket notifications are best-effort, exactly like the notification module's
// own failure policy.
func NewSupportService(repo repositories.SupportRepository, notifications NotificationService) SupportService {
	return &supportService{repo: repo, notifications: notifications}
}

// ── User-facing ─────────────────────────────────────────────────────────────

func (s *supportService) Create(ctx context.Context, userID uuid.UUID, req *dto.CreateTicketRequest) (*dto.TicketDetailResponse, error) {
	if !validCategory(req.Category) {
		return nil, apperrors.NewAppError(
			"unsupported ticket category: "+req.Category, "validation_error")
	}

	ticket := &models.SupportTicket{
		UserID:   userID,
		Subject:  strings.TrimSpace(req.Subject),
		Category: req.Category,
		Status:   models.SupportStatusOpen,
		Priority: req.Priority,
	}
	if ticket.Priority == "" {
		ticket.Priority = "medium"
	}
	if err := s.repo.Create(ctx, ticket); err != nil {
		logger.FromContext(ctx).Error("failed to create support ticket", "user_id", userID, "error", err)
		return nil, apperrors.ErrInternalServer
	}

	// The opening message is the first reply — same table, same thread shape,
	// so the detail view renders create + reply identically.
	first := &models.SupportReply{
		TicketID:   ticket.ID,
		AuthorID:   userID,
		AuthorRole: models.SupportAuthorUser,
		Message:    strings.TrimSpace(req.Message),
	}
	if err := s.repo.AddReply(ctx, ticket, first); err != nil {
		logger.FromContext(ctx).Error("failed to store ticket opening message", "ticket_id", ticket.ID, "error", err)
		return nil, apperrors.ErrInternalServer
	}

	return s.Get(ctx, userID, ticket.ID)
}

func (s *supportService) List(ctx context.Context, userID uuid.UUID, status string, page, limit int) (*dto.TicketListResponse, error) {
	if status != "" && !contains(models.ValidSupportStatuses(), status) {
		return nil, apperrors.NewAppError("invalid status filter: "+status, "invalid_status")
	}
	offset := (page - 1) * limit
	rows, total, err := s.repo.ListByUser(ctx, userID, status, limit, offset)
	if err != nil {
		return nil, apperrors.ErrInternalServer
	}

	items := make([]dto.TicketResponse, 0, len(rows))
	for _, t := range rows {
		items = append(items, ticketToListResponse(t))
	}
	return &dto.TicketListResponse{
		Tickets:    items,
		Pagination: dto.NewInvestmentPaginationMetadata(page, limit, total),
	}, nil
}

func (s *supportService) Get(ctx context.Context, userID, ticketID uuid.UUID) (*dto.TicketDetailResponse, error) {
	ticket, err := s.repo.FindByIDAndUserID(ctx, ticketID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTicketNotFound
		}
		return nil, apperrors.ErrInternalServer
	}
	return ticketToDetailResponse(ticket, false), nil
}

func (s *supportService) Reply(ctx context.Context, userID, ticketID uuid.UUID, message string) (*dto.TicketReplyResponse, error) {
	// Ownership is enforced by the lookup itself: someone else's ticket comes
	// back as not-found, so a stranger can't even learn that it exists.
	ticket, err := s.repo.FindByIDAndUserID(ctx, ticketID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTicketNotFound
		}
		return nil, apperrors.ErrInternalServer
	}

	if ticket.Status == models.SupportStatusClosed {
		return nil, apperrors.ErrTicketClosed
	}

	reply := &models.SupportReply{
		TicketID:   ticket.ID,
		AuthorID:   userID,
		AuthorRole: models.SupportAuthorUser,
		Message:    strings.TrimSpace(message),
	}
	if reply.Message == "" {
		return nil, apperrors.NewAppError("message cannot be empty", "validation_error")
	}

	if err := s.repo.AddReply(ctx, ticket, reply); err != nil {
		logger.FromContext(ctx).Error("failed to add ticket reply", "ticket_id", ticketID, "error", err)
		return nil, apperrors.ErrInternalServer
	}
	resp := replyToResponse(reply)
	return &resp, nil
}

// ── Agent queue ─────────────────────────────────────────────────────────────

func (s *supportService) AdminList(ctx context.Context, status, category string, page, limit int) (*dto.TicketListResponse, error) {
	if status != "" && !contains(models.ValidSupportStatuses(), status) {
		return nil, apperrors.NewAppError("invalid status filter: "+status, "invalid_status")
	}
	offset := (page - 1) * limit
	rows, total, err := s.repo.ListAll(ctx, status, category, limit, offset)
	if err != nil {
		return nil, apperrors.ErrInternalServer
	}

	items := make([]dto.TicketResponse, 0, len(rows))
	for _, t := range rows {
		items = append(items, ticketToListResponse(t))
	}
	return &dto.TicketListResponse{
		Tickets:    items,
		Pagination: dto.NewInvestmentPaginationMetadata(page, limit, total),
	}, nil
}

func (s *supportService) AdminGet(ctx context.Context, ticketID uuid.UUID) (*dto.TicketDetailResponse, error) {
	ticket, err := s.repo.FindByIDWithReplies(ctx, ticketID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTicketNotFound
		}
		return nil, apperrors.ErrInternalServer
	}
	return ticketToDetailResponse(ticket, true), nil
}

func (s *supportService) AdminReply(ctx context.Context, agentID, ticketID uuid.UUID, message string) (*dto.TicketReplyResponse, error) {
	ticket, err := s.repo.FindByIDWithReplies(ctx, ticketID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTicketNotFound
		}
		return nil, apperrors.ErrInternalServer
	}
	if ticket.Status == models.SupportStatusClosed {
		return nil, apperrors.ErrTicketClosed
	}

	reply := &models.SupportReply{
		TicketID:   ticket.ID,
		AuthorID:   agentID,
		AuthorRole: models.SupportAuthorAgent,
		Message:    strings.TrimSpace(message),
	}
	if reply.Message == "" {
		return nil, apperrors.NewAppError("message cannot be empty", "validation_error")
	}

	if err := s.repo.AddReply(ctx, ticket, reply); err != nil {
		logger.FromContext(ctx).Error("failed to add agent reply", "ticket_id", ticketID, "error", err)
		return nil, apperrors.ErrInternalServer
	}

	// An agent picking up an open ticket moves it into the queue's working
	// state automatically — no separate status click required.
	if ticket.Status == models.SupportStatusOpen {
		if err := s.repo.UpdateStatus(ctx, ticket.ID, models.SupportStatusInProgress); err != nil {
			logger.FromContext(ctx).Warn("failed to mark ticket in_progress", "ticket_id", ticketID, "error", err)
		} else {
			ticket.Status = models.SupportStatusInProgress
		}
	}

	s.notifyOwner(ctx, ticket, "support_reply",
		"Support replied to "+ticket.TicketNumber,
		truncate(reply.Message, 140))

	resp := replyToResponse(reply)
	return &resp, nil
}

func (s *supportService) AdminUpdateStatus(ctx context.Context, agentID, ticketID uuid.UUID, status string) (*dto.TicketDetailResponse, error) {
	ticket, err := s.repo.FindByIDWithReplies(ctx, ticketID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTicketNotFound
		}
		return nil, apperrors.ErrInternalServer
	}

	if ticket.Status == status {
		return ticketToDetailResponse(ticket, true), nil // idempotent no-op
	}

	if err := s.repo.UpdateStatus(ctx, ticketID, status); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrTicketNotFound
		}
		logger.FromContext(ctx).Error("failed to update ticket status",
			"ticket_id", ticketID, "status", status, "error", err)
		return nil, apperrors.ErrInternalServer
	}

	s.notifyOwner(ctx, ticket, "support_status",
		"Support ticket "+ticket.TicketNumber+" is now "+statusLabel(status),
		truncate(ticket.Subject, 140))

	return s.AdminGet(ctx, ticketID)
}

// notifyOwner is best-effort, mirroring the notification module's own policy:
// a missing in-app notification must never fail a support action.
func (s *supportService) notifyOwner(ctx context.Context, ticket *models.SupportTicket, nType, title, body string) {
	if s.notifications == nil {
		return
	}
	s.notifications.Notify(ctx, ticket.UserID, nType, title, body, map[string]any{
		"ticket_id":     ticket.ID.String(),
		"ticket_number": ticket.TicketNumber,
	})
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func ticketToListResponse(t models.SupportTicket) dto.TicketResponse {
	return dto.TicketResponse{
		ID:           t.ID,
		TicketNumber: t.TicketNumber,
		Subject:      t.Subject,
		Category:     t.Category,
		Status:       t.Status,
		Priority:     t.Priority,
		ReplyCount:   t.ReplyCount,
		LastReplyAt:  t.LastReplyAt,
		ClosedAt:     t.ClosedAt,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		User:         userSummary(t.User),
	}
}

func ticketToDetailResponse(t *models.SupportTicket, withUser bool) *dto.TicketDetailResponse {
	replies := make([]dto.TicketReplyResponse, 0, len(t.Replies))
	for _, r := range t.Replies {
		replies = append(replies, replyToResponse(&r))
	}
	resp := ticketToListResponse(*t)
	resp.ReplyCount = len(replies)
	if !withUser {
		resp.User = nil
	}
	return &dto.TicketDetailResponse{TicketResponse: resp, Replies: replies}
}

func replyToResponse(r *models.SupportReply) dto.TicketReplyResponse {
	return dto.TicketReplyResponse{
		ID:         r.ID,
		AuthorID:   r.AuthorID,
		AuthorRole: r.AuthorRole,
		Message:    r.Message,
		CreatedAt:  r.CreatedAt,
	}
}

func userSummary(u *models.User) *dto.TicketUserSummary {
	if u == nil {
		return nil
	}
	return &dto.TicketUserSummary{ID: u.ID, FullName: u.FullName, Email: u.Email}
}

func statusLabel(status string) string {
	switch status {
	case models.SupportStatusInProgress:
		return "in progress"
	case models.SupportStatusResolved:
		return "resolved"
	case models.SupportStatusClosed:
		return "closed"
	default:
		return status
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
