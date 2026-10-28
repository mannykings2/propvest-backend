package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/queue"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AdminMessagingService handles bulk/broadcast messaging from admin dashboard.
// Allows administrators to send emails, SMS, and in-app notifications to:
// - Single user
// - Segment of users (filtered by criteria)
// - All users (platform-wide broadcast)
//
// Safety features:
// - Admin-only (enforced by handler/middleware)
// - Max 10,000 users per broadcast
// - Confirmation required for "all users"
// - Audit logging
type AdminMessagingService interface {
	// SendBulkMessage sends bulk messages to targeted users.
	// Returns message ID, target count, and queued counts.
	SendBulkMessage(ctx context.Context, adminID uuid.UUID, req dto.SendBulkMessageRequest) (*dto.BulkMessageResponse, error)
}

type adminMessagingService struct {
	userRepo         repositories.UserRepository
	notificationRepo repositories.NotificationRepository
	mq               *queue.Client
	db               *gorm.DB // Direct DB access for bulk operations (temporary)
}

// NewAdminMessagingService constructs the service.
func NewAdminMessagingService(
	userRepo repositories.UserRepository,
	notificationRepo repositories.NotificationRepository,
	mq *queue.Client,
	db *gorm.DB,
) AdminMessagingService {
	return &adminMessagingService{
		userRepo:         userRepo,
		notificationRepo: notificationRepo,
		mq:               mq,
		db:               db,
	}
}

func (s *adminMessagingService) SendBulkMessage(ctx context.Context, adminID uuid.UUID, req dto.SendBulkMessageRequest) (*dto.BulkMessageResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, apperrors.WrapError(err, "invalid bulk message request", "validation_error")
	}

	// Resolve target users
	users, err := s.resolveTargetUsers(ctx, req)
	if err != nil {
		return nil, err
	}

	// Safety limit
	if len(users) > 10000 {
		return nil, apperrors.NewAppError(
			fmt.Sprintf("target exceeds safety limit: %d users (max 10,000)", len(users)),
			"target_too_large",
		)
	}

	if len(users) == 0 {
		return nil, apperrors.NewAppError("no users match the targeting criteria", "no_targets")
	}

	// Generate unique message ID for tracking
	messageID := uuid.New().String()

	// Audit log
	logger.FromContext(ctx).Info("admin bulk message initiated",
		"admin_id", adminID,
		"message_id", messageID,
		"target_type", req.TargetType,
		"message_type", req.MessageType,
		"user_count", len(users),
	)

	// Initialize counters
	queuedEmails := 0
	queuedSMS := 0
	inAppCreated := 0

	// Create in-app notifications if requested
	if req.CreateInApp {
		count, err := s.createBulkInAppNotifications(ctx, users, req.NotifType, req.NotifTitle, req.NotifBody)
		if err != nil {
			logger.FromContext(ctx).Error("failed to create in-app notifications", "error", err)
			// Continue - don't fail the whole operation
		} else {
			inAppCreated = count
		}
	}

	// Publish emails if requested
	if req.MessageType == "email" || req.MessageType == "both" {
		count, err := s.publishBulkEmails(ctx, users, req.Subject, req.EmailBody)
		if err != nil {
			logger.FromContext(ctx).Error("failed to publish emails", "error", err)
			// Continue - don't fail the whole operation
		} else {
			queuedEmails = count
		}
	}

	// Publish SMS if requested
	if req.MessageType == "sms" || req.MessageType == "both" {
		count, err := s.publishBulkSMS(ctx, users, req.SMSMessage)
		if err != nil {
			logger.FromContext(ctx).Error("failed to publish SMS", "error", err)
			// Continue - don't fail the whole operation
		} else {
			queuedSMS = count
		}
	}

	// Build response
	resp := &dto.BulkMessageResponse{
		MessageID:     messageID,
		TargetType:    req.TargetType,
		MessageType:   req.MessageType,
		TargetedUsers: len(users),
		QueuedEmails:  queuedEmails,
		QueuedSMS:     queuedSMS,
		InAppCreated:  inAppCreated,
		CreatedAt:     time.Now(),
	}

	return resp, nil
}

// resolveTargetUsers returns the list of users based on targeting criteria.
func (s *adminMessagingService) resolveTargetUsers(ctx context.Context, req dto.SendBulkMessageRequest) ([]*models.User, error) {
	switch req.TargetType {
	case "single":
		return s.getUsersByIDs(ctx, req.UserIDs)
	case "segment":
		return s.getUsersByFilters(ctx, req.Filters)
	case "all":
		return s.getAllUsers(ctx)
	default:
		return nil, apperrors.NewAppError("invalid target_type", "invalid_target")
	}
}

// getUsersByIDs fetches users by their IDs.
func (s *adminMessagingService) getUsersByIDs(ctx context.Context, userIDs []string) ([]*models.User, error) {
	users := make([]*models.User, 0, len(userIDs))
	for _, idStr := range userIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue // Skip invalid UUIDs (already validated in Validate())
		}
		
		user, err := s.userRepo.FindByID(ctx, id)
		if err != nil {
			logger.FromContext(ctx).Warn("user not found in bulk message targeting", "user_id", id)
			continue // Skip not found users
		}
		users = append(users, user)
	}
	return users, nil
}

// getUsersByFilters fetches users matching filter criteria.
func (s *adminMessagingService) getUsersByFilters(ctx context.Context, filters *dto.UserFilters) ([]*models.User, error) {
	// TODO: Implement repository method for filtered user fetch
	// For now, fetch all and filter in memory (not scalable for production)
	allUsers, err := s.getAllUsers(ctx)
	if err != nil {
		return nil, err
	}

	// Apply filters
	filtered := make([]*models.User, 0)
	for _, user := range allUsers {
		if s.matchesFilters(user, filters) {
			filtered = append(filtered, user)
		}
	}

	return filtered, nil
}

// matchesFilters checks if a user matches filter criteria.
func (s *adminMessagingService) matchesFilters(user *models.User, filters *dto.UserFilters) bool {
	if filters.Role != "" && user.Role != filters.Role {
		return false
	}
	if filters.KYCStatus != "" && user.KYCStatus != filters.KYCStatus {
		return false
	}
	// TODO: Add wallet balance filter (requires wallet join/query)
	// TODO: Add has_invested filter (requires investment count query)
	return true
}

// getAllUsers fetches all users.
func (s *adminMessagingService) getAllUsers(ctx context.Context) ([]*models.User, error) {
	// TODO: Implement repository method to fetch all users efficiently
	// For MVP, we use direct DB access (not ideal but functional)
	// In production, add UserRepository.ListAll(ctx, limit, offset) method
	
	var users []models.User
	if err := s.db.WithContext(ctx).Limit(10000).Find(&users).Error; err != nil {
		logger.FromContext(ctx).Error("failed to fetch all users", "error", err)
		return nil, apperrors.ErrInternalServer
	}
	
	// Convert []models.User to []*models.User
	userPtrs := make([]*models.User, len(users))
	for i := range users {
		userPtrs[i] = &users[i]
	}
	
	return userPtrs, nil
}

// createBulkInAppNotifications creates in-app notifications for all users.
func (s *adminMessagingService) createBulkInAppNotifications(ctx context.Context, users []*models.User, notifType, title, body string) (int, error) {
	count := 0
	for _, user := range users {
		notif := &models.Notification{
			UserID: user.ID,
			Type:   notifType,
			Title:  title,
			Body:   body,
			Read:   false,
			Metadata: datatypes.JSON([]byte("{}")),
		}
		
		if err := s.notificationRepo.Create(ctx, notif); err != nil {
			logger.FromContext(ctx).Error("failed to create in-app notification", "user_id", user.ID, "error", err)
			continue // Skip failed notifications
		}
		count++
	}
	return count, nil
}

// suppressedAddresses loads the marketing-suppression set (NDPA
// objection-to-direct-marketing + FCCPC unsubscribe baseline). Every marketing
// channel — email and SMS — filters through it before a send.
func (s *adminMessagingService) suppressedAddresses(ctx context.Context) map[string]bool {
	var rows []models.MarketingSuppression
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		// A broken suppression read must NOT silently become an open gate to
		// suppressed addresses — fail the send instead.
		logger.FromContext(ctx).Error("suppression list unavailable; aborting bulk send", "error", err)
		return nil
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.Channel+"|"+strings.ToLower(r.Address)] = true
	}
	return out
}

// publishBulkEmails publishes email messages to the queue for all users.
func (s *adminMessagingService) publishBulkEmails(ctx context.Context, users []*models.User, subject, htmlBody string) (int, error) {
	if s.mq == nil || !s.mq.Enabled() {
		return 0, apperrors.NewAppError("queue is disabled", "queue_disabled")
	}

	suppressed := s.suppressedAddresses(ctx)
	if suppressed == nil {
		return 0, apperrors.NewAppError("suppression list unavailable; send aborted", "suppression_unavailable")
	}

	count := 0
	for _, user := range users {
		if user.Email == "" {
			continue // Skip users without email
		}
		if suppressed["email|"+strings.ToLower(user.Email)] {
			continue // Opted out (NDPA objection) — never send
		}

		// One-click unsubscribe on EVERY bulk message (Phase B2): the link is
		// per-user and resolves to the SPA landing page, which calls the same
		// /unsubscribe endpoint an API client would.
		html := htmlBody
		text := stripHTML(htmlBody) // Simple text version
		if link := UnsubscribeLink(user.ID); link != "" {
			html += `<hr><p style="font-size:12px;color:#666">You are receiving this because you opted in to PropVest updates. ` +
				`<a href="` + link + `">Unsubscribe from marketing</a></p>`
			text += "\n\nUnsubscribe: " + link
		}

		msg := queue.EmailMessage{
			To:       user.Email,
			Subject:  subject,
			HTMLBody: html,
			TextBody: text,
		}

		if err := s.mq.Publish(ctx, queue.QueueEmailDispatch, msg); err != nil {
			logger.FromContext(ctx).Error("failed to publish email", "user_id", user.ID, "error", err)
			continue // Skip failed publishes
		}
		count++
	}
	return count, nil
}

// publishBulkSMS publishes SMS messages to the queue for all users.
func (s *adminMessagingService) publishBulkSMS(ctx context.Context, users []*models.User, message string) (int, error) {
	if s.mq == nil || !s.mq.Enabled() {
		return 0, apperrors.NewAppError("queue is disabled", "queue_disabled")
	}

	suppressed := s.suppressedAddresses(ctx)
	if suppressed == nil {
		return 0, apperrors.NewAppError("suppression list unavailable; send aborted", "suppression_unavailable")
	}

	count := 0
	for _, user := range users {
		if user.Phone == "" {
			continue // Skip users without phone
		}
		if suppressed["sms|"+strings.ToLower(user.Phone)] {
			continue // Opted out (NDPA objection) — never send
		}

		body := message
		if link := UnsubscribeLink(user.ID); link != "" {
			body += " Opt-out: " + link
		}

		msg := queue.SMSMessage{
			To:      user.Phone,
			Message: body,
		}

		if err := s.mq.Publish(ctx, queue.QueueSMSDispatch, msg); err != nil {
			logger.FromContext(ctx).Error("failed to publish SMS", "user_id", user.ID, "error", err)
			continue // Skip failed publishes
		}
		count++
	}
	return count, nil
}

// stripHTML is a simple HTML tag stripper for creating plain text version.
// TODO: Use a proper HTML-to-text library for production.
func stripHTML(html string) string {
	// Simple implementation: remove all HTML tags
	// For production, use a library like github.com/PuerkitoBio/goquery
	text := html
	// Remove common tags (very basic implementation)
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<p>", "")
	text = strings.ReplaceAll(text, "</p>", "\n\n")
	text = strings.ReplaceAll(text, "<h1>", "")
	text = strings.ReplaceAll(text, "</h1>", "\n")
	text = strings.ReplaceAll(text, "<h2>", "")
	text = strings.ReplaceAll(text, "</h2>", "\n")
	// Remove all remaining tags
	for strings.Contains(text, "<") && strings.Contains(text, ">") {
		start := strings.Index(text, "<")
		end := strings.Index(text, ">")
		if start < end {
			text = text[:start] + text[end+1:]
		} else {
			break
		}
	}
	return strings.TrimSpace(text)
}
