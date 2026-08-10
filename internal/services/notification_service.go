package services

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/queue"
	"github.com/mannykings2/propvest-backend/internal/realtime"
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// NotificationService is the fan-out point for user notifications. When a
// business event happens, other services call Notify(). This service then:
//
//  1. Persists an in-app notification row (durable copy + unread badge).
//  2. Pushes it to the user's live WebSocket connections (instant UI update),
//     both directly via the hub AND by publishing to the realtime queue so a
//     separate worker process could trigger the same push.
//  3. Optionally enqueues an email/SMS for the worker to deliver asynchronously.
//
// This split is deliberate (see pivotal decisions): realtime/in-app is handled
// synchronously and cheaply in-process; slow, flaky email/SMS delivery is
// offloaded to RabbitMQ so it can be retried without blocking the request.
type NotificationService interface {
	// Notify records + pushes an in-app notification.
	Notify(ctx context.Context, userID uuid.UUID, nType, title, body string, metadata map[string]any)
	// NotifyWithEmail additionally enqueues an email to the worker.
	NotifyWithEmail(ctx context.Context, userID uuid.UUID, email, nType, title, body string, metadata map[string]any)

	// HTTP-facing reads/writes for the notification centre.
	List(ctx context.Context, userID uuid.UUID, unreadOnly bool, page, limit int) (*dto.NotificationListResponse, int64, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	Delete(ctx context.Context, userID, id uuid.UUID) error

	// StartRealtimeConsumer wires the API process to consume realtime messages
	// from RabbitMQ and push them to WebSocket clients.
	StartRealtimeConsumer(ctx context.Context)
}

type notificationService struct {
	repo repositories.NotificationRepository
	hub  *realtime.Hub
	mq   *queue.Client
}

// NewNotificationService constructs the service.
func NewNotificationService(repo repositories.NotificationRepository, hub *realtime.Hub, mq *queue.Client) NotificationService {
	return &notificationService{repo: repo, hub: hub, mq: mq}
}

func (s *notificationService) Notify(ctx context.Context, userID uuid.UUID, nType, title, body string, metadata map[string]any) {
	s.create(ctx, userID, nType, title, body, metadata)
}

func (s *notificationService) NotifyWithEmail(ctx context.Context, userID uuid.UUID, email, nType, title, body string, metadata map[string]any) {
	s.create(ctx, userID, nType, title, body, metadata)
	// Enqueue the email for the worker (best-effort; disabled queue just logs).
	if email != "" {
		_ = s.mq.Publish(ctx, queue.QueueEmailDispatch, queue.EmailMessage{
			To:       email,
			Subject:  title,
			HTMLBody: "<p>" + body + "</p>",
			TextBody: body,
		})
	}
}

// create is the shared path: persist the row, then push realtime.
func (s *notificationService) create(ctx context.Context, userID uuid.UUID, nType, title, body string, metadata map[string]any) {
	var meta datatypes.JSON
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			meta = b
		}
	}

	n := &models.Notification{
		UserID:   userID,
		Type:     nType,
		Title:    title,
		Body:     body,
		Metadata: meta,
	}
	if err := s.repo.Create(ctx, n); err != nil {
		// Notifications are non-critical; log and continue (don't fail the caller's
		// business operation just because a notification couldn't be stored).
		logger.FromContext(ctx).Error("failed to store notification", "user_id", userID, "error", err)
		return
	}

	s.pushRealtime(ctx, userID, n)
}

// pushRealtime sends the notification to the user's sockets. It pushes locally
// (this process) AND publishes to the realtime queue so other processes (the
// worker) can trigger pushes too. Duplicate delivery is harmless for the UI.
func (s *notificationService) pushRealtime(ctx context.Context, userID uuid.UUID, n *models.Notification) {
	payload := dto.NotificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		Read:      n.Read,
		Metadata:  json.RawMessage(n.Metadata),
		CreatedAt: n.CreatedAt,
	}

	// Local push (fast path).
	if s.hub != nil {
		if b, err := json.Marshal(map[string]any{"event": "notification.created", "payload": payload}); err == nil {
			s.hub.PushToUser(userID.String(), b)
		}
	}

	// Queue push (so a worker-originated event still reaches the socket via the
	// API process's realtime consumer). Skipped automatically if the queue is
	// disabled.
	if s.mq != nil && s.mq.Enabled() {
		_ = s.mq.Publish(ctx, queue.QueueRealtimePush, queue.RealtimeMessage{
			UserID:  userID.String(),
			Event:   "notification.created",
			Payload: payload,
		})
	}
}

// StartRealtimeConsumer consumes QueueRealtimePush and fans messages out to the
// hub. Runs in the API process (which owns the WebSocket connections).
func (s *notificationService) StartRealtimeConsumer(ctx context.Context) {
	if s.mq == nil || !s.mq.Enabled() {
		return
	}
	s.mq.Consume(queue.QueueRealtimePush, func(_ context.Context, body []byte) error {
		var msg queue.RealtimeMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return nil // bad message; drop (don't requeue a poison message forever)
		}
		out, err := json.Marshal(map[string]any{"event": msg.Event, "payload": msg.Payload})
		if err != nil {
			return nil
		}
		s.hub.PushToUser(msg.UserID, out)
		return nil
	})
}

// ── HTTP-facing reads/writes ────────────────────────────────────────────────

func (s *notificationService) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, page, limit int) (*dto.NotificationListResponse, int64, error) {
	offset := (page - 1) * limit
	rows, total, err := s.repo.ListByUser(ctx, userID, unreadOnly, limit, offset)
	if err != nil {
		return nil, 0, apperrors.ErrInternalServer
	}
	unread, err := s.repo.CountUnread(ctx, userID)
	if err != nil {
		return nil, 0, apperrors.ErrInternalServer
	}

	items := make([]dto.NotificationResponse, 0, len(rows))
	for _, n := range rows {
		items = append(items, dto.NotificationResponse{
			ID:        n.ID,
			Type:      n.Type,
			Title:     n.Title,
			Body:      n.Body,
			Read:      n.Read,
			Metadata:  json.RawMessage(n.Metadata),
			CreatedAt: n.CreatedAt,
		})
	}
	return &dto.NotificationListResponse{Notifications: items, UnreadCount: unread}, total, nil
}

func (s *notificationService) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	if err := s.repo.MarkRead(ctx, id, userID); err != nil {
		return apperrors.ErrInternalServer
	}
	return nil
}

func (s *notificationService) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if err := s.repo.MarkAllRead(ctx, userID); err != nil {
		return apperrors.ErrInternalServer
	}
	return nil
}

func (s *notificationService) Delete(ctx context.Context, userID, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		return apperrors.ErrInternalServer
	}
	return nil
}
