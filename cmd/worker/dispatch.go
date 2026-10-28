package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/mailer"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/queue"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// NOTIFICATION DISPATCH
// ═══════════════════════════════════════════════════════════════════════════
//
// These consumers are the far side of every notification the API enqueues:
// the API writes the in-app row and publishes; this process performs the slow,
// flaky part (talking to a provider) where a failure can be retried without
// holding an HTTP request open.
//
// DELIVERY CONTRACT (per queue.Consume): handler error → NACK + requeue with
// no delay and no cap. So handlers here do their own bounded retries
// (mailer.SendWithRetry) and then ACK + log the full payload: a poisoned
// message must not become a hot loop, and an operator must be able to replay
// it from the logs.

// startEmailDispatcher consumes propvest.email.dispatch and actually sends the
// mail through the configured mailer (SMTP/Brevo in production, mock in dev).
func startEmailDispatcher(ctx context.Context, mq *queue.Client, sender mailer.Sender) {
	mq.Consume(queue.QueueEmailDispatch, func(ctx context.Context, body []byte) error {
		log := logger.FromContext(ctx)

		var msg queue.EmailMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			log.Error("invalid email message", "error", err)
			return nil // unparseable → ACK, requeueing can never help
		}
		if strings.TrimSpace(msg.To) == "" {
			log.Error("email message has no recipient; dropping", "subject", msg.Subject)
			return nil
		}

		email := mailer.Email{
			To:       msg.To,
			Subject:  msg.Subject,
			HTMLBody: renderHTML(msg),
			TextBody: msg.TextBody,
		}

		if err := mailer.SendWithRetry(sender, email, emailAttempts, emailRetryDelay); err != nil {
			// Give up (ACK) but leave everything needed to replay it.
			log.Error("email delivery failed after retries; message dropped",
				"to", msg.To, "subject", msg.Subject, "error", err,
				"payload", string(body))
			return nil
		}

		log.Info("email sent", "to", msg.To, "subject", msg.Subject)
		return nil
	})
}

// startReceiptDispatcher consumes propvest.deposit.receipt and emails a
// receipt for a wallet credit.
//
// WHY IT EXISTS: the wallet service publishes this message after every
// verified deposit, and the outbox maps deposit.receipt onto the same queue —
// before this consumer existed the messages simply piled up unread.
func startReceiptDispatcher(ctx context.Context, mq *queue.Client, sender mailer.Sender) {
	mq.Consume(queue.QueueDepositReceipt, func(ctx context.Context, body []byte) error {
		log := logger.FromContext(ctx)

		var msg queue.DepositReceiptMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			log.Error("invalid deposit receipt message", "error", err)
			return nil
		}
		if strings.TrimSpace(msg.Email) == "" {
			log.Error("deposit receipt has no recipient; dropping", "reference", msg.Reference)
			return nil
		}

		amount := formatKobo(msg.AmountKobo)
		text := fmt.Sprintf("We received your deposit of %s.\n\nReference: %s\n\nIf you didn't make this deposit, contact support immediately.", amount, msg.Reference)
		htmlBody := fmt.Sprintf(
			"<p>We received your deposit of <strong>%s</strong>.</p><p>Reference: <code>%s</code></p><p>If you didn't make this deposit, contact support immediately.</p>",
			html.EscapeString(amount), html.EscapeString(msg.Reference),
		)

		email := mailer.Email{
			To:       msg.Email,
			Subject:  "Deposit receipt — " + amount,
			HTMLBody: htmlBody,
			TextBody: text,
		}

		if err := mailer.SendWithRetry(sender, email, emailAttempts, emailRetryDelay); err != nil {
			log.Error("deposit receipt delivery failed after retries; message dropped",
				"to", msg.Email, "reference", msg.Reference, "error", err,
				"payload", string(body))
			return nil
		}

		log.Info("deposit receipt sent", "to", msg.Email, "reference", msg.Reference)
		return nil
	})
}

// startSMSDispatcher consumes propvest.sms.dispatch.
//
// LAUNCH SCOPE DECISION: SMS is deliberately OUT of scope for launch — no
// provider is configured, so there is nothing that can actually deliver a
// message. Rather than pretend, every message is ACKed and logged loudly with
// the recipient and body, so an operator sees exactly what was NOT sent (the
// in-app notification row is still the durable copy). Implementing a provider
// means filling in internal/utils/sms and removing this warning.
func startSMSDispatcher(ctx context.Context, mq *queue.Client) {
	mq.Consume(queue.QueueSMSDispatch, func(ctx context.Context, body []byte) error {
		log := logger.FromContext(ctx)

		var msg queue.SMSMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			log.Error("invalid SMS message", "error", err)
			return nil
		}

		log.Warn("SMS NOT delivered — SMS is out of launch scope (SMS_PROVIDER=mock)",
			"to", msg.To, "message", msg.Message)
		return nil
	})
}

// startBusinessEventDispatcher consumes propvest.business.events — the shared
// queue for investment/property lifecycle events — and turns them into in-app
// notification rows.
//
// WHY THIS EXISTS: the services write these events transactionally (an
// investment row and its event commit together), but until now nothing
// consumed them: every investment produced an event that retried for an hour
// and then failed, and the investor received no confirmation at all. The
// notification row is written here by the worker so the API's realtime relay
// pushes the badge update to the user's open sockets.
func startBusinessEventDispatcher(ctx context.Context, mq *queue.Client, notifier services.NotificationService) {
	mq.Consume(queue.QueueBusinessEvents, func(ctx context.Context, body []byte) error {
		log := logger.FromContext(ctx)

		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Error("invalid business event", "error", err)
			return nil // unparseable → ACK, requeueing can never help
		}

		eventType, _ := payload["event_type"].(string)

		recipient, nType, title, msg, ok := businessEventNotification(payload)
		if !ok {
			// No rule or no addressed user: ACK with a loud log. The event was
			// delivered — it is the notification copy that is missing, and a
			// requeue would loop on the identical payload forever.
			log.Warn("business event produced no notification; acknowledged",
				"event_type", eventType, "payload", string(body))
			return nil
		}

		notifier.Notify(ctx, recipient, nType, title, msg, payload)
		log.Info("notification created from business event",
			"event_type", eventType, "user_id", recipient, "type", nType)
		return nil
	})
}

// businessEventNotification maps an outbox business-event payload onto the
// in-app notification it should produce. ok=false means "no rule for this
// payload" (unknown type, or no user to address).
func businessEventNotification(payload map[string]interface{}) (recipient uuid.UUID, nType, title, body string, ok bool) {
	eventType, _ := payload["event_type"].(string)

	switch eventType {
	case models.EventTypeInvestmentCreated:
		recipient, ok = parseUUID(payload["user_id"])
		if !ok {
			return uuid.Nil, "", "", "", false
		}
		amount := formatKobo(asInt64(payload["amount_kobo"]))
		propertyTitle, _ := payload["property_title"].(string)
		if propertyTitle == "" {
			propertyTitle = "a property"
		}
		slots := asInt64(payload["slots"])
		ref, _ := payload["reference"].(string)
		return recipient, models.NotificationInvestmentCreated, "Investment confirmed",
			fmt.Sprintf("Your %s investment in %s (%d slot(s)) is confirmed. Reference: %s.",
				amount, propertyTitle, slots, ref), true

	case models.EventTypeInvestmentCancelled:
		recipient, ok = parseUUID(payload["user_id"])
		if !ok {
			return uuid.Nil, "", "", "", false
		}
		return recipient, models.NotificationInvestmentCancelled, "Investment cancelled",
			"Your investment was cancelled and the funds returned to your wallet.", true

	case models.EventTypeInvestmentCompleted:
		recipient, ok = parseUUID(payload["user_id"])
		if !ok {
			return uuid.Nil, "", "", "", false
		}
		return recipient, models.NotificationInvestmentCompleted, "Investment completed",
			"Your investment has completed and returns are available.", true

	case models.EventTypePropertyPublished:
		// The publisher is the only addressed user in this payload; investors are
		// not fanned out to (that would be a broadcast, not an event).
		recipient, ok = parseUUID(payload["admin_id"])
		if !ok {
			return uuid.Nil, "", "", "", false
		}
		propertyTitle, _ := payload["title"].(string)
		return recipient, models.NotificationPropertyUpdate, "Property published",
			fmt.Sprintf("%s is now live for investment.", propertyTitle), true

	default:
		// property.funded / property.completed are declared in the model but not
		// written by any service yet; unknown future types land here too.
		return uuid.Nil, "", "", "", false
	}
}

// parseUUID reads a UUID that arrived as JSON (string or already-decoded).
func parseUUID(v interface{}) (uuid.UUID, bool) {
	s, ok := v.(string)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// asInt64 reads a JSON number (decoded as float64) as an integer amount.
func asInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

// renderHTML falls back to a minimal escaped paragraph when the producer sent
// only a text body.
func renderHTML(msg queue.EmailMessage) string {
	if strings.TrimSpace(msg.HTMLBody) != "" {
		return msg.HTMLBody
	}
	// Escape FIRST, then introduce the <br>: the reverse order escapes the tag
	// itself and renders the literal text "<br>" to the reader.
	escaped := html.EscapeString(msg.TextBody)
	return "<p>" + strings.ReplaceAll(escaped, "\n", "<br>") + "</p>"
}

// formatKobo renders kobo as a naira amount, e.g. 750000 → "₦7,500.00".
func formatKobo(kobo int64) string {
	sign := ""
	if kobo < 0 {
		sign, kobo = "-", -kobo
	}
	major, minor := kobo/100, kobo%100
	grouped := strings.Builder{}
	digits := fmt.Sprintf("%d", major)
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteRune(',')
		}
		grouped.WriteRune(r)
	}
	return fmt.Sprintf("%s₦%s.%02d", sign, grouped.String(), minor)
}

// Retry policy for provider calls made from queue handlers. See the contract
// note at the top of this file for why the retry is in-process and bounded.
const (
	emailAttempts   = 3
	emailRetryDelay = 2 * time.Second
)
