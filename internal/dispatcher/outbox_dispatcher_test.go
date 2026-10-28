package dispatcher

import (
	"fmt"
	"testing"

	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/queue"
)

// TestGetQueueName_CoversEveryDeclaredEventType guards the failure this test
// exists for: investment.created had no route, so every investment wrote an
// outbox event that retried for an hour and then failed — silently, because
// nothing checked that the declared event types actually mapped.
func TestGetQueueName_CoversEveryDeclaredEventType(t *testing.T) {
	d := &OutboxDispatcher{}

	declared := map[string]string{
		models.EventTypeWithdrawalProcess:   queue.QueueWithdrawalProcess,
		models.EventTypeDepositReceipt:      queue.QueueDepositReceipt,
		models.EventTypeEmailDispatch:       queue.QueueEmailDispatch,
		models.EventTypeSMSDispatch:         queue.QueueSMSDispatch,
		models.EventTypeRealtimePush:        queue.QueueRealtimePush,
		models.EventTypeInvestmentCreated:   queue.QueueBusinessEvents,
		models.EventTypeInvestmentCancelled: queue.QueueBusinessEvents,
		models.EventTypeInvestmentCompleted: queue.QueueBusinessEvents,
		models.EventTypePropertyPublished:   queue.QueueBusinessEvents,
		models.EventTypePropertyFunded:      queue.QueueBusinessEvents,
		models.EventTypePropertyCompleted:   queue.QueueBusinessEvents,
	}

	for eventType, wantQueue := range declared {
		if got := d.getQueueName(eventType); got != wantQueue {
			t.Errorf("getQueueName(%q) = %q, want %q", eventType, got, wantQueue)
		}
	}
}

// TestGetQueueName_UnknownTypeHasNoRoute: an unrecognized type must map to no
// queue, which is what triggers the permanent (non-retrying) failure path.
func TestGetQueueName_UnknownTypeHasNoRoute(t *testing.T) {
	d := &OutboxDispatcher{}

	if got := d.getQueueName("never.heard.of.it"); got != "" {
		t.Fatalf("expected no queue for an unknown type, got %q", got)
	}

	// And the sentinel processEvent wraps it with is detectable, which is what
	// handleFailure branches on.
	wrapped := fmt.Errorf("wrapper: %w: never.heard.of.it", errUnknownEventType)
	if !isUnknownEventType(wrapped) {
		t.Fatal("unknown event type must be detectable with errors.Is")
	}
}
