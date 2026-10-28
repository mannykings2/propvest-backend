package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// SupportRepository is the data-access contract for support tickets.
// (Phase 4.2 — the Support page used to keep tickets in component state.)
type SupportRepository interface {
	// Create inserts a ticket and allocates its human-friendly number
	// (TKT-000123) from the support_ticket_seq sequence in the SAME
	// transaction, so numbers are unique and gap-free under concurrency.
	Create(ctx context.Context, ticket *models.SupportTicket) error

	// ListByUser returns a user's tickets (newest first) with a total count.
	// status may be "" for all.
	ListByUser(ctx context.Context, userID uuid.UUID, status string, limit, offset int) ([]models.SupportTicket, int64, error)

	// ListAll returns every ticket for the agent queue (newest first).
	ListAll(ctx context.Context, status, category string, limit, offset int) ([]models.SupportTicket, int64, error)

	// FindByIDAndUserID loads a ticket and its ordered replies, enforcing
	// ownership (WHERE id = ? AND user_id = ?).
	FindByIDAndUserID(ctx context.Context, id, userID uuid.UUID) (*models.SupportTicket, error)

	// FindByIDWithReplies loads a ticket + replies without an ownership filter
	// — admin path only (the route already carries the RBAC gate).
	FindByIDWithReplies(ctx context.Context, id uuid.UUID) (*models.SupportTicket, error)

	// AddReply appends a reply and advances the ticket's last_reply_at.
	// When the ticket is 'resolved', a USER reply reopens it to 'open'
	// (the customer answered, so the queue must show it again).
	// A 'closed' ticket accepts no replies — the service rejects that first.
	AddReply(ctx context.Context, ticket *models.SupportTicket, reply *models.SupportReply) error

	// UpdateStatus moves the ticket through the lifecycle, stamping closed_at
	// when it closes and clearing it if it reopens.
	UpdateStatus(ctx context.Context, ticketID uuid.UUID, status string) error
}

type supportRepository struct {
	*BaseRepository
}

// replyCountSelect keeps list rows cheap (one extra scalar subquery per row)
// while still telling the UI how long the thread is — without loading replies.
const replyCountSelect = `support_tickets.*, (
	SELECT count(*) FROM support_replies r WHERE r.ticket_id = support_tickets.id
) AS reply_count`

// NewSupportRepository constructs the repository.
func NewSupportRepository(db *gorm.DB) SupportRepository {
	return &supportRepository{BaseRepository: NewBaseRepository(db)}
}

func (r *supportRepository) Create(ctx context.Context, ticket *models.SupportTicket) error {
	return r.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var next int64
		if err := tx.Raw("SELECT nextval('support_ticket_seq')").Scan(&next).Error; err != nil {
			return err
		}
		ticket.TicketNumber = fmt.Sprintf("TKT-%06d", next)
		return tx.Create(ticket).Error
	})
}

func (r *supportRepository) ListByUser(ctx context.Context, userID uuid.UUID, status string, limit, offset int) ([]models.SupportTicket, int64, error) {
	q := r.WithContext(ctx).Model(&models.SupportTicket{}).Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}

	// Count on a SESSION CLONE: Count mutates the chain (it rewrites SELECT),
	// which would silently drop replyCountSelect from the Find below.
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tickets []models.SupportTicket
	if err := q.Select(replyCountSelect).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&tickets).Error; err != nil {
		return nil, 0, err
	}
	return tickets, total, nil
}

func (r *supportRepository) ListAll(ctx context.Context, status, category string, limit, offset int) ([]models.SupportTicket, int64, error) {
	q := r.WithContext(ctx).Model(&models.SupportTicket{}).Preload("User")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if category != "" {
		q = q.Where("category = ?", category)
	}

	// See ListByUser: count on a clone so the reply-count select survives.
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tickets []models.SupportTicket
	if err := q.Select(replyCountSelect).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&tickets).Error; err != nil {
		return nil, 0, err
	}
	return tickets, total, nil
}

func (r *supportRepository) FindByIDAndUserID(ctx context.Context, id, userID uuid.UUID) (*models.SupportTicket, error) {
	var ticket models.SupportTicket
	err := r.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		First(&ticket).Error
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (r *supportRepository) FindByIDWithReplies(ctx context.Context, id uuid.UUID) (*models.SupportTicket, error) {
	var ticket models.SupportTicket
	err := r.WithContext(ctx).
		Where("id = ?", id).
		Preload("User").
		Preload("Replies", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		First(&ticket).Error
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (r *supportRepository) AddReply(ctx context.Context, ticket *models.SupportTicket, reply *models.SupportReply) error {
	return r.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(reply).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{
			"last_reply_at": reply.CreatedAt,
			"updated_at":    gorm.Expr("NOW()"),
		}
		// A user answering a resolved ticket reopens it — the queue must show
		// that the customer came back. Agents don't reopen: moving the ticket
		// is their explicit status action.
		if ticket.Status == models.SupportStatusResolved && reply.AuthorRole == models.SupportAuthorUser {
			updates["status"] = models.SupportStatusOpen
			ticket.Status = models.SupportStatusOpen
		}

		if err := tx.Model(&models.SupportTicket{}).
			Where("id = ?", ticket.ID).
			Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *supportRepository) UpdateStatus(ctx context.Context, ticketID uuid.UUID, status string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": gorm.Expr("NOW()"),
	}
	if status == models.SupportStatusClosed {
		updates["closed_at"] = gorm.Expr("NOW()")
	} else {
		updates["closed_at"] = nil
	}

	result := r.WithContext(ctx).
		Model(&models.SupportTicket{}).
		Where("id = ?", ticketID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
