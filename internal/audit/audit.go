// Package audit provides append-only security/audit logging.
//
// WHY A SEPARATE PACKAGE FROM `logger`?
// -------------------------------------
// The docs (4.2 §17, 6.2 §19, 1.2 §15) draw a hard line between *application
// logs* (for engineers, debugging) and *audit logs* (for security/compliance:
// "who did what, when, from where"). Audit records must be append-only and
// immutable. Mixing them with normal logs is explicitly called an anti-pattern
// (6.2 §27).
//
// DESIGN
// ------
// This package exposes a small Recorder interface with one method, Record. It
// ships with two implementations:
//
//   - LogRecorder: writes the audit event as a structured log line tagged
//     `audit=true`. This works from day one, before any audit table exists.
//   - (A DB-backed recorder is added in Milestone 7 alongside the audit_logs
//     table; it will implement this same interface so nothing else changes.)
//
// Services depend on the Recorder interface, so we can swap the log-only
// implementation for the DB-backed one without touching call sites.
package audit

import (
	"context"

	"github.com/mannykings2/propvest-backend/internal/logger"
)

// Event is a single security-relevant action worth recording.
// Keep field names stable — downstream tooling may parse them.
type Event struct {
	Action     string // e.g. "login_success", "password_changed", "wallet_withdraw"
	ActorID    string // user id performing the action (may be empty for anonymous)
	TargetType string // e.g. "user", "property", "wallet"
	TargetID   string // id of the affected entity
	IP         string // client IP, when available
	UserAgent  string // client user agent, when available
	RequestID  string // correlation id
	Metadata   map[string]any
}

// Recorder records audit events. Implementations MUST treat records as
// append-only (never update/delete an existing audit entry).
type Recorder interface {
	Record(ctx context.Context, e Event)
}

// LogRecorder is a Recorder that emits audit events as structured log lines.
// It is the default until the DB-backed recorder (Milestone 7) is wired in.
type LogRecorder struct{}

// NewLogRecorder returns a log-only audit recorder.
func NewLogRecorder() *LogRecorder { return &LogRecorder{} }

// Record writes the event as a single INFO log line tagged audit=true so it can
// be filtered/shipped to a separate sink later.
func (r *LogRecorder) Record(ctx context.Context, e Event) {
	l := logger.FromContext(ctx)
	l.Info("audit_event",
		"audit", true,
		"action", e.Action,
		"actor_id", e.ActorID,
		"target_type", e.TargetType,
		"target_id", e.TargetID,
		"ip", e.IP,
		"user_agent", e.UserAgent,
		"request_id", e.RequestID,
		"metadata", e.Metadata,
	)
}
