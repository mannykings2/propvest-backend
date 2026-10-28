package mailer

import (
	"context"
	"time"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/queue"
)

// Publisher is the slice of *queue.Client this sender needs. Declared as an
// interface (satisfied by *queue.Client) so tests can fake a broker.
type Publisher interface {
	Publish(ctx context.Context, queueName string, v any) error
	Enabled() bool
}

// QueuedSender hands every email to the dispatch queue instead of dialing
// SMTP inside the request path.
//
// Why it exists (Phase 5 load smoke): net/smtp.SendMail dials synchronously
// with NO timeout, and register fires the verification email before
// responding. Measured on this box: register p95 = 2.4s, of which ~2.2s was
// the single SMTP connect attempt to smtp-relay.brevo.com (which never
// succeeded from here — the error was logged and swallowed as best-effort).
// With the send moved to QueueEmailDispatch, the request path only pays for
// a local publish (~1ms); the worker sends with SendWithRetry + NACK backoff.
//
// Degraded modes keep today's delivery semantics:
//   - broker disabled (no RabbitMQ in local dev) → send directly, as before;
//   - publish error/timeout → fall back to a direct send (slower, but a lost
//     verification or password-reset email is worse than 2 slow seconds).
type QueuedSender struct {
	mq    Publisher
	inner Sender
}

// NewQueuedSender wraps inner (the real SMTP/mock sender) so Send publishes
// asynchronously when a broker is available.
func NewQueuedSender(mq Publisher, inner Sender) Sender {
	return &QueuedSender{mq: mq, inner: inner}
}

func (q *QueuedSender) Send(e Email) error {
	if q.mq == nil || !q.mq.Enabled() {
		return q.inner.Send(e)
	}

	// Short bound: Publish itself allows 5s, but this is a request path —
	// beyond ~1s the direct-send fallback is no slower than being here at all.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := q.mq.Publish(ctx, queue.QueueEmailDispatch, queue.EmailMessage{
		To:       e.To,
		Subject:  e.Subject,
		TextBody: e.TextBody,
		HTMLBody: e.HTMLBody,
	})
	if err != nil {
		logger.Warn("email publish failed; sending directly", "to", e.To, "subject", e.Subject, "error", err)
		return q.inner.Send(e)
	}
	return nil
}
