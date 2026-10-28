package mailer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mannykings2/propvest-backend/internal/queue"
)

// fakePublisher records what was published and simulates the three broker
// states the sender must distinguish: enabled, disabled, failing.
type fakePublisher struct {
	enabled   bool
	publishErr error
	gotQueue  string
	gotMsg    any
}

func (f *fakePublisher) Publish(_ context.Context, queueName string, v any) error {
	f.gotQueue = queueName
	f.gotMsg = v
	return f.publishErr
}

func (f *fakePublisher) Enabled() bool { return f.enabled }

// recordingSender stands in for the direct SMTP sender (the fallback path).
type recordingSender struct {
	calls []Email
	err   error
}

func (r *recordingSender) Send(e Email) error {
	r.calls = append(r.calls, e)
	return r.err
}

func testEmail() Email {
	return Email{
		To:       "new.user@example.com",
		Subject:  "Verify your PropVest email",
		TextBody: "Confirm: https://propvest.test/verify?token=abc",
		HTMLBody: "Confirm: https://propvest.test/verify?token=abc",
	}
}

// Enabled broker: the request path must NOT touch the direct sender, and the
// message must land on QueueEmailDispatch in the worker's contract shape.
func TestQueuedSender_PublishesWhenBrokerEnabled(t *testing.T) {
	pub := &fakePublisher{enabled: true}
	inner := &recordingSender{}

	if err := NewQueuedSender(pub, inner).Send(testEmail()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if len(inner.calls) != 0 {
		t.Fatalf("direct sender must not be called when publish succeeds, got %d calls", len(inner.calls))
	}
	if pub.gotQueue != queue.QueueEmailDispatch {
		t.Fatalf("published to %q, want %q", pub.gotQueue, queue.QueueEmailDispatch)
	}
	msg, ok := pub.gotMsg.(queue.EmailMessage)
	if !ok {
		t.Fatalf("published payload is %T, want queue.EmailMessage", pub.gotMsg)
	}
	if msg.To != "new.user@example.com" || msg.Subject == "" || msg.TextBody == "" {
		t.Fatalf("message missing contract fields: %+v", msg)
	}
}

// Broker disabled (local dev without RabbitMQ): behave exactly as before —
// send directly so the verification email still goes out.
func TestQueuedSender_FallsBackToDirectSendWhenDisabled(t *testing.T) {
	pub := &fakePublisher{enabled: false}
	inner := &recordingSender{}

	if err := NewQueuedSender(pub, inner).Send(testEmail()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("direct sender must handle disabled-broker sends, got %d calls", len(inner.calls))
	}
	if pub.gotQueue != "" {
		t.Fatalf("nothing must be published when disabled, got queue %q", pub.gotQueue)
	}
}

// Publish failure (timeout, dead channel): fall back to the direct send and
// surface the inner result — an email must never vanish silently.
func TestQueuedSender_FallsBackToDirectSendOnPublishError(t *testing.T) {
	pub := &fakePublisher{enabled: true, publishErr: errors.New("publish timeout")}
	inner := &recordingSender{}

	if err := NewQueuedSender(pub, inner).Send(testEmail()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if len(inner.calls) != 1 {
		t.Fatalf("publish failure must fall back to direct send, got %d calls", len(inner.calls))
	}
}

// The fallback must propagate the inner sender's error (no swallowing).
func TestQueuedSender_PropagatesFallbackError(t *testing.T) {
	pub := &fakePublisher{enabled: true, publishErr: errors.New("boom")}
	inner := &recordingSender{err: errors.New("smtp down")}

	err := NewQueuedSender(pub, inner).Send(testEmail())
	if err == nil || !strings.Contains(err.Error(), "smtp down") {
		t.Fatalf("want inner error propagated, got %v", err)
	}
}
