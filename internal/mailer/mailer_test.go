package mailer

import (
	"errors"
	"testing"
	"time"
)

// flakySender fails a fixed number of times before succeeding.
type flakySender struct {
	failures int
	calls    int
	last     Email
}

func (f *flakySender) Send(e Email) error {
	f.calls++
	f.last = e
	if f.calls <= f.failures {
		return errors.New("smtp unavailable")
	}
	return nil
}

func TestSendWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	s := &flakySender{failures: 2}
	if err := SendWithRetry(s, Email{To: "a@b.com", Subject: "hi"}, 3, time.Millisecond); err != nil {
		t.Fatalf("expected success on attempt 3, got %v", err)
	}
	if s.calls != 3 {
		t.Errorf("calls = %d, want 3", s.calls)
	}
	if s.last.To != "a@b.com" {
		t.Errorf("recipient not preserved: %q", s.last.To)
	}
}

func TestSendWithRetry_GivesUpAfterBoundedAttempts(t *testing.T) {
	s := &flakySender{failures: 99}
	if err := SendWithRetry(s, Email{To: "a@b.com"}, 3, time.Millisecond); err == nil {
		t.Fatal("expected an error after exhausting attempts")
	}
	// Bounded: never more than the configured attempts, otherwise the queue's
	// NACK/requeue loop would spin forever on a poison message.
	if s.calls != 3 {
		t.Errorf("calls = %d, want exactly 3", s.calls)
	}
}

func TestSendWithRetry_SingleAttempt(t *testing.T) {
	s := &flakySender{failures: 1}
	if err := SendWithRetry(s, Email{To: "a@b.com"}, 1, time.Millisecond); err == nil {
		t.Fatal("expected the single attempt to fail")
	}
	if s.calls != 1 {
		t.Errorf("calls = %d, want 1", s.calls)
	}
}
