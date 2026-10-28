// Package mailer sends transactional emails (verification links, password reset
// links, receipts).
//
// WHY AN INTERFACE + TWO IMPLEMENTATIONS?
// ---------------------------------------
// The rest of the app should not care HOW an email is sent, only that it can ask
// for one. So we define a small Sender interface and provide:
//
//   - MockSender: logs the email (and any link) to the console. Perfect for
//     local development — a junior dev can copy the verification link straight
//     from their terminal without configuring SMTP.
//   - SMTPSender: sends a real email over SMTP (works with Mailpit locally or a
//     real SMTP provider in production).
//
// New() picks the implementation from EMAIL_PROVIDER ("mock" by default). This
// is the Strategy pattern: swap behaviour via config without touching callers.
package mailer

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"time"

	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/logger"
)

// Email is a single message to send.
type Email struct {
	To       string
	Subject  string
	HTMLBody string
	TextBody string
}

// Sender is the abstraction the app depends on.
type Sender interface {
	Send(email Email) error
}

// New returns the configured Sender. Defaults to the mock sender so the app
// runs out of the box.
//
// config.Load() normalizes EMAIL_PROVIDER first ("brevo" and friends become
// "smtp" once BREVO_SMTP_* is folded into SMTP_*), so "brevo" is accepted here
// too as a belt-and-braces guard: an unrecognized provider name must never
// silently select the mock — that is how delivery went missing entirely.
func New(cfg *config.Config) Sender {
	switch cfg.EmailProvider {
	case "smtp", "brevo":
		if cfg.SMTPHost == "" {
			logger.Error("email provider is SMTP but SMTP_HOST is empty; falling back to mock (emails will NOT be delivered)",
				"provider", cfg.EmailProvider)
			return &MockSender{}
		}
		logger.Info("email provider: SMTP", "host", cfg.SMTPHost, "port", cfg.SMTPPort, "from", cfg.SMTPFromEmail)
		return &SMTPSender{cfg: cfg}
	default:
		logger.Info("email provider: mock (emails logged to console)", "configured_provider", cfg.EmailProvider)
		return &MockSender{}
	}
}

// SendWithRetry delivers one email, retrying transient failures a bounded
// number of times with a fixed delay.
//
// WHY BOUNDED: the queue consumer NACKs-and-requeues on error, so an
// unbounded/hot retry loop would spin on a permanently rejected address (bad
// recipient, provider outage) forever. Three in-process attempts, then give up
// loudly — the message is ACKed and the payload is logged so an operator can
// replay it.
func SendWithRetry(s Sender, e Email, attempts int, delay time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 1; i <= attempts; i++ {
		if err = s.Send(e); err == nil {
			return nil
		}
		if i < attempts {
			logger.Warn("email send failed; retrying",
				"attempt", i, "of", attempts, "to", e.To, "error", err)
			time.Sleep(delay)
		}
	}
	return err
}

// MockSender logs emails instead of sending them.
type MockSender struct{}

// Send logs the email. In dev this is where you read the verification/reset link.
func (m *MockSender) Send(e Email) error {
	logger.Info("MOCK EMAIL",
		"to", e.To,
		"subject", e.Subject,
		"text_body", e.TextBody,
	)
	return nil
}

// SMTPSender sends real email over SMTP.
type SMTPSender struct {
	cfg *config.Config
}

// Send delivers the email via the configured SMTP server. It builds a minimal
// MIME message with both text and HTML parts.
func (s *SMTPSender) Send(e Email) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)
	from := s.cfg.SMTPFromEmail
	fromName := s.cfg.SMTPFromName
	if fromName == "" {
		fromName = "PropVest"
	}

	// Compose a simple multipart/alternative message.
	boundary := "propvest-boundary-42"
	msg := fmt.Sprintf("From: %s <%s>\r\n", fromName, from) +
		fmt.Sprintf("To: %s\r\n", e.To) +
		fmt.Sprintf("Subject: %s\r\n", e.Subject) +
		"MIME-Version: 1.0\r\n" +
		fmt.Sprintf("Content-Type: multipart/alternative; boundary=%s\r\n\r\n", boundary) +
		fmt.Sprintf("--%s\r\n", boundary) +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + e.TextBody + "\r\n" +
		fmt.Sprintf("--%s\r\n", boundary) +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" + e.HTMLBody + "\r\n" +
		fmt.Sprintf("--%s--\r\n", boundary)

	// Auth is optional (Mailpit needs none). Only authenticate if a username is set.
	var auth smtp.Auth
	if s.cfg.SMTPUsername != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	}

	// For simplicity we use net/smtp's SendMail (STARTTLS negotiated by the
	// server where supported). TLS config is only relevant for explicit TLS.
	_ = tls.Config{}
	if err := smtp.SendMail(addr, auth, from, []string{e.To}, []byte(msg)); err != nil {
		logger.Error("SMTP send failed", "to", e.To, "error", err)
		return err
	}
	return nil
}
