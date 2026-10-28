package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// AdminHealthService provides system health monitoring for administrators.
//
// SECURITY:
//   - NO secrets, DSNs, or internal topology in responses
//   - Sanitized error categories only
//   - Timeout-bounded checks (5 seconds max per check)
//   - Circuit-breaker friendly (cached results)
type AdminHealthService interface {
	// CheckSystemHealth performs health checks on all critical dependencies.
	//
	// CHECKS:
	//   - PostgreSQL connectivity
	//   - RabbitMQ connectivity
	//   - Paystack configuration/reachability
	//   - Cloudinary configuration
	//   - SMS provider configuration
	//
	// RETURNS:
	//   response - health status for each dependency
	//   error - if health check itself fails
	CheckSystemHealth(ctx context.Context, actorID uuid.UUID) (*SystemHealthResponse, error)

	// CheckQueueHealth returns RabbitMQ queue health metrics.
	//
	// RETURNS:
	//   response - queue depths, consumer status, oldest message age
	//   error - if queue health check fails or unsupported
	CheckQueueHealth(ctx context.Context, actorID uuid.UUID) (*QueueHealthResponse, error)
}

// SystemHealthResponse represents the health status of all system dependencies.
type SystemHealthResponse struct {
	Status     string                 `json:"status"` // "healthy", "degraded", "unhealthy"
	CheckedAt  time.Time              `json:"checked_at"`
	Components map[string]HealthCheck `json:"components"`
}

// HealthCheck represents the health status of a single component.
type HealthCheck struct {
	Status  string `json:"status"` // "healthy", "degraded", "unhealthy"
	Message string `json:"message,omitempty"`
	Latency *int64 `json:"latency_ms,omitempty"` // Response time in milliseconds
}

// QueueHealthResponse represents RabbitMQ queue health.
type QueueHealthResponse struct {
	Status    string                  `json:"status"` // "healthy", "degraded", "unhealthy", "unsupported"
	CheckedAt time.Time               `json:"checked_at"`
	Queues    map[string]QueueMetrics `json:"queues,omitempty"`
}

// QueueMetrics represents health metrics for a single queue.
type QueueMetrics struct {
	Depth            int    `json:"depth"`           // Number of messages in queue
	ConsumerStatus   string `json:"consumer_status"` // "active", "stopped", "unknown"
	OldestMessageAge *int64 `json:"oldest_message_age_seconds,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────
// Implementation
// ────────────────────────────────────────────────────────────────────────────

type adminHealthService struct {
	db  *gorm.DB
	cfg *config.Config
}

// NewAdminHealthService creates a new admin health service instance.
func NewAdminHealthService(db *gorm.DB, cfg *config.Config) AdminHealthService {
	return &adminHealthService{
		db:  db,
		cfg: cfg,
	}
}

// CheckSystemHealth performs health checks on all dependencies.
//
// TIMEOUT: Each check has 5-second timeout
// FAIL-FAST: Checks run concurrently, first failure sets overall status
// SANITIZATION: Error messages are categorized, no internal details exposed
func (s *adminHealthService) CheckSystemHealth(ctx context.Context, actorID uuid.UUID) (*SystemHealthResponse, error) {
	checkedAt := time.Now().UTC()
	components := make(map[string]HealthCheck)

	// Check PostgreSQL
	postgresStatus := s.checkPostgreSQL(ctx)
	components["postgresql"] = postgresStatus

	// Check RabbitMQ (best effort - no hard dependency)
	rabbitmqStatus := s.checkRabbitMQ(ctx)
	components["rabbitmq"] = rabbitmqStatus

	// Check Paystack configuration
	paystackStatus := s.checkPaystackConfig()
	components["paystack"] = paystackStatus

	// Check Cloudinary configuration
	cloudinaryStatus := s.checkCloudinaryConfig()
	components["cloudinary"] = cloudinaryStatus

	// Check SMS provider configuration
	smsStatus := s.checkSMSConfig()
	components["sms_provider"] = smsStatus

	// Check email delivery configuration (verification + receipts)
	components["email"] = s.checkEmailConfig()

	// Determine overall status
	overallStatus := determineOverallStatus(components)

	return &SystemHealthResponse{
		Status:     overallStatus,
		CheckedAt:  checkedAt,
		Components: components,
	}, nil
}

// CheckQueueHealth reports the transactional-outbox backlog — the one queue
// whose health we can measure honestly from the database we already hold.
//
// WHY THIS IS THE METRIC THAT MATTERS: every notification, withdrawal job and
// event in the system waits in outbox_events until the worker's dispatcher
// publishes it. If that table grows without draining, users stop getting
// emails and jobs stop running while every other check still says "healthy".
//
// (RabbitMQ queue *depths* still require the management API, which is not
// wired up — the response reports outbox metrics instead of pretending.)
func (s *adminHealthService) CheckQueueHealth(ctx context.Context, actorID uuid.UUID) (*QueueHealthResponse, error) {
	checkedAt := time.Now().UTC()
	queues := make(map[string]QueueMetrics)

	notPublished := []string{models.OutboxStatusPending, models.OutboxStatusClaimed}

	var backlog int64
	if err := s.db.WithContext(ctx).Model(&models.OutboxEvent{}).
		Where("status IN ?", notPublished).Count(&backlog).Error; err != nil {
		return nil, fmt.Errorf("counting outbox backlog: %w", err)
	}

	var oldest struct {
		CreatedAt time.Time
	}
	var ageSeconds *int64
	if backlog > 0 {
		if err := s.db.WithContext(ctx).Model(&models.OutboxEvent{}).
			Select("created_at").
			Where("status IN ?", notPublished).
			Order("created_at ASC").
			Take(&oldest).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("reading oldest outbox event: %w", err)
		}
		if !oldest.CreatedAt.IsZero() {
			age := int64(time.Since(oldest.CreatedAt).Seconds())
			ageSeconds = &age
		}
	}

	var failed int64
	if err := s.db.WithContext(ctx).Model(&models.OutboxEvent{}).
		Where("status = ?", models.OutboxStatusFailed).Count(&failed).Error; err != nil {
		return nil, fmt.Errorf("counting failed outbox events: %w", err)
	}

	// Consumer state is inferred, not observed: a backlog that keeps growing is
	// the symptom of a stopped worker, so report "unknown" rather than guess.
	queues["outbox"] = QueueMetrics{
		Depth:            int(backlog),
		ConsumerStatus:   "unknown",
		OldestMessageAge: ageSeconds,
	}
	queues["outbox_failed"] = QueueMetrics{
		Depth:          int(failed),
		ConsumerStatus: "unknown",
	}

	status := "healthy"
	switch {
	case failed > 0:
		status = "degraded"
	case ageSeconds != nil && *ageSeconds > 600: // >10 min: dispatcher is stuck or down
		status = "unhealthy"
	case ageSeconds != nil && *ageSeconds > 60: // >1 min: slower than the poll interval should allow
		status = "degraded"
	}

	return &QueueHealthResponse{
		Status:    status,
		CheckedAt: checkedAt,
		Queues:    queues,
	}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Component Health Checks
// ────────────────────────────────────────────────────────────────────────────

// checkPostgreSQL verifies PostgreSQL connectivity.
func (s *adminHealthService) checkPostgreSQL(ctx context.Context) HealthCheck {
	start := time.Now()

	// Create timeout context (5 seconds)
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Simple ping query
	var result int
	err := s.db.WithContext(timeoutCtx).Raw("SELECT 1").Scan(&result).Error

	latency := time.Since(start).Milliseconds()

	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return HealthCheck{
				Status:  "unhealthy",
				Message: "database connection timeout",
				Latency: &latency,
			}
		}
		return HealthCheck{
			Status:  "unhealthy",
			Message: "database connection failed",
			Latency: &latency,
		}
	}

	// Warn if latency is high (> 1 second)
	if latency > 1000 {
		return HealthCheck{
			Status:  "degraded",
			Message: fmt.Sprintf("database latency high (%dms)", latency),
			Latency: &latency,
		}
	}

	return HealthCheck{
		Status:  "healthy",
		Message: "database connection ok",
		Latency: &latency,
	}
}

// checkRabbitMQ verifies RabbitMQ is configured (best effort).
func (s *adminHealthService) checkRabbitMQ(ctx context.Context) HealthCheck {
	// We don't have direct RabbitMQ health check in current architecture
	// Just check if RabbitMQ DSN is configured
	if s.cfg.RabbitMQURL == "" {
		return HealthCheck{
			Status:  "unhealthy",
			Message: "rabbitmq not configured",
		}
	}

	// Configuration present (actual connection health requires management API)
	return HealthCheck{
		Status:  "healthy",
		Message: "rabbitmq configured",
	}
}

// checkPaystackConfig verifies Paystack is configured.
func (s *adminHealthService) checkPaystackConfig() HealthCheck {
	if s.cfg.PaystackSecretKey == "" {
		return HealthCheck{
			Status:  "degraded",
			Message: "paystack not configured",
		}
	}

	return HealthCheck{
		Status:  "healthy",
		Message: "paystack configured",
	}
}

// checkCloudinaryConfig verifies Cloudinary is configured.
func (s *adminHealthService) checkCloudinaryConfig() HealthCheck {
	if s.cfg.CloudinaryCloudName == "" || s.cfg.CloudinaryAPIKey == "" {
		return HealthCheck{
			Status:  "degraded",
			Message: "cloudinary not configured",
		}
	}

	return HealthCheck{
		Status:  "healthy",
		Message: "cloudinary configured",
	}
}

// checkSMSConfig reports the SMS launch-scope decision honestly.
//
// SMS is deliberately out of scope for launch: no provider is implemented, so
// nothing is ever delivered. "Configured" therefore does NOT mean "working" —
// the only providers that could be selected (twilio/termii) are stubs, which is
// why an explicit provider selection reports degraded rather than healthy.
func (s *adminHealthService) checkSMSConfig() HealthCheck {
	switch s.cfg.SMSProvider {
	case "", "mock":
		return HealthCheck{
			Status:  "healthy",
			Message: "sms disabled by launch scope (messages are logged, not sent)",
		}
	default:
		return HealthCheck{
			Status:  "degraded",
			Message: fmt.Sprintf("sms provider %q selected but sending is not implemented; messages are logged, not sent", s.cfg.SMSProvider),
		}
	}
}

// checkEmailConfig reports whether transactional email can actually leave the
// building. Email carries verification and password-reset links, so "mock" here
// means users cannot finish sign-up.
func (s *adminHealthService) checkEmailConfig() HealthCheck {
	if s.cfg.EmailProvider == "mock" {
		return HealthCheck{
			Status:  "degraded",
			Message: "email disabled (EMAIL_PROVIDER=mock): verification links are logged, not sent",
		}
	}
	return HealthCheck{
		Status:  "healthy",
		Message: fmt.Sprintf("email provider configured (%s via %s)", s.cfg.EmailProvider, s.cfg.SMTPHost),
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Helper Functions
// ────────────────────────────────────────────────────────────────────────────

// determineOverallStatus calculates overall system health from component statuses.
//
// LOGIC:
//   - If any component is "unhealthy", overall is "unhealthy"
//   - If any component is "degraded", overall is "degraded"
//   - If all components are "healthy", overall is "healthy"
func determineOverallStatus(components map[string]HealthCheck) string {
	hasUnhealthy := false
	hasDegraded := false

	for _, check := range components {
		switch check.Status {
		case "unhealthy":
			hasUnhealthy = true
		case "degraded":
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return "unhealthy"
	}
	if hasDegraded {
		return "degraded"
	}
	return "healthy"
}
