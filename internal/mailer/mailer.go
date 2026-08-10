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
func New(cfg *config.Config) Sender {
	switch cfg.EmailProvider {
	case "smtp":
		logger.Info("email provider: SMTP", "host", cfg.SMTPHost, "port", cfg.SMTPPort)
		return &SMTPSender{cfg: cfg}
	default:
		logger.Info("email provider: mock (emails logged to console)")
		return &MockSender{}
	}
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
