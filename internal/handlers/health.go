package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/queue"
	"github.com/mannykings2/propvest-backend/internal/response"
	"gorm.io/gorm"
)

// HealthCheck is the simplest possible endpoint (LIVENESS).
// Its only job is to confirm the server process is running and reachable.
// In production systems, load balancers and monitoring tools
// (like AWS ALB or Kubernetes) ping this endpoint every few seconds
// to decide whether to route traffic to this instance.
//
// It deliberately checks NO dependencies: a dead database must not kill the
// process (that is /health/ready's job), or a dependency blip would restart
// every pod at once.
func HealthCheck(c *gin.Context) {
	response.Success(c, gin.H{
		"status":  "healthy",
		"service": "propvest-api",
		"version": "1.0.0",
	})
}

// ReadinessCheck probes one dependency. It returns nil when the dependency can
// serve traffic, and a non-nil error (safe to show in the response body) when
// it cannot.
type ReadinessCheck func(ctx context.Context) error

// ReadinessProbe is a named dependency wired into /health/ready.
//
// Critical probes flip the endpoint to 503 when they fail — the instance is
// pulled out of rotation. Non-critical probes are still reported, but a
// failure only marks that component "degraded": the API can serve the request
// without them (the outbox pattern exists precisely so RabbitMQ being down
// does not block a withdrawal).
type ReadinessProbe struct {
	Name     string
	Critical bool
	Check    ReadinessCheck
}

// DatabaseReadiness pings PostgreSQL and verifies the schema is at a known
// migration version. A reachable-but-unmigrated (or half-migrated "dirty")
// database is NOT ready: it would serve queries against the wrong schema.
func DatabaseReadiness(db *gorm.DB) ReadinessCheck {
	return func(ctx context.Context) error {
		if db == nil {
			return errors.New("database not configured")
		}
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("connection pool unavailable: %w", err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("ping failed: %w", err)
		}

		// golang-migrate keeps the applied version in schema_migrations.
		var state struct {
			Version uint64
			Dirty   bool
		}
		if err := db.WithContext(ctx).
			Raw("SELECT version, dirty FROM schema_migrations").
			Scan(&state).Error; err != nil {
			return fmt.Errorf("migration state unreadable (run migrations): %w", err)
		}
		if state.Dirty {
			return fmt.Errorf("migrations in dirty state at version %d", state.Version)
		}
		return nil
	}
}

// QueueReadiness reports the live state of the RabbitMQ connection.
func QueueReadiness(mq *queue.Client) ReadinessCheck {
	return func(ctx context.Context) error {
		if mq == nil {
			return errors.New("queue client not initialised")
		}
		return mq.Ping(ctx)
	}
}

// NewReadinessHandler builds the READINESS probe.
//
// Unlike HealthCheck, this inspects real dependencies: the database (ping +
// migration version) and, when configured, the message broker. Orchestrators
// (Kubernetes readiness, ALB target health, deploy gates) call it to decide
// whether this instance should receive traffic.
//
// Contract:
//   - 200 → every critical dependency is serving traffic
//   - 503 → a critical dependency is down; body names the failing component
//
// Usage:
//	r.GET("/health/ready", handlers.NewReadinessHandler(
//	    handlers.ReadinessProbe{Name: "database", Critical: true,
//	        Check: handlers.DatabaseReadiness(database.DB)},
//	    handlers.ReadinessProbe{Name: "queue",
//	        Check: handlers.QueueReadiness(mq)},
//	))
func NewReadinessHandler(probes ...ReadinessProbe) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Bound the whole probe so a hung dependency can't hang the health
		// endpoint itself (and so the orchestrator gets its answer).
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		components := make(map[string]gin.H, len(probes))
		ready := true

		for _, p := range probes {
			start := time.Now()
			err := p.Check(ctx)
			latency := time.Since(start).Milliseconds()

			status := "healthy"
			message := "ok"
			if err != nil {
				message = err.Error()
				if p.Critical {
					status = "unhealthy"
					ready = false
				} else {
					status = "degraded"
				}
			}
			components[p.Name] = gin.H{
				"status":     status,
				"message":    message,
				"latency_ms": latency,
				"critical":   p.Critical,
			}
		}

		body := gin.H{
			"status":     map[bool]string{true: "ready", false: "not_ready"}[ready],
			"service":    "propvest-api",
			"version":    "1.0.0",
			"components": components,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		}

		if !ready {
			c.JSON(http.StatusServiceUnavailable, response.StandardResponse{
				Success:   false,
				Message:   "service not ready: a critical dependency is unavailable",
				Code:      "not_ready",
				Data:      body,
				RequestID: c.GetString("request_id"),
			})
			return
		}

		c.JSON(http.StatusOK, response.StandardResponse{
			Success:   true,
			Message:   "ready",
			Code:      "ready",
			Data:      body,
			RequestID: c.GetString("request_id"),
		})
	}
}
