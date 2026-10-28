package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
// CLIENT INITIALIZATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestNew_EmptyURL tests that client works in disabled mode with empty URL
func TestNew_EmptyURL(t *testing.T) {
	client := New("")

	assert.NotNil(t, client)
	assert.False(t, client.Enabled(), "Client should be disabled with empty URL")
	assert.Equal(t, "", client.url)
}

// TestNew_InvalidURL tests graceful degradation with invalid RabbitMQ URL
func TestNew_InvalidURL(t *testing.T) {
	client := New("amqp://invalid-host:5672/")

	assert.NotNil(t, client)
	assert.False(t, client.Enabled(), "Client should be disabled with invalid URL")
	assert.Equal(t, "amqp://invalid-host:5672/", client.url)
}

// TestNew_Disabled tests that disabled client doesn't crash
func TestNew_Disabled(t *testing.T) {
	client := New("")
	defer client.Close()

	// Should not panic
	assert.NotPanics(t, func() {
		ctx := context.Background()
		_ = client.Publish(ctx, QueueEmailDispatch, EmailMessage{
			To:      "test@example.com",
			Subject: "Test",
		})
	})
}

// TestEnabled_DisabledClient tests Enabled() returns false for disabled client
func TestEnabled_DisabledClient(t *testing.T) {
	client := New("")
	assert.False(t, client.Enabled())
}

// TestClient_ConcurrentEnabled tests thread-safe Enabled() calls
func TestClient_ConcurrentEnabled(t *testing.T) {
	client := New("")
	defer client.Close()

	done := make(chan bool)

	// Spawn multiple goroutines checking Enabled()
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = client.Enabled()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// PUBLISH TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestPublish_DisabledClient tests publishing with disabled client
func TestPublish_DisabledClient(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()
	msg := EmailMessage{
		To:       "user@example.com",
		Subject:  "Welcome",
		HTMLBody: "<p>Hello</p>",
		TextBody: "Hello",
	}

	// Should not error (graceful degradation)
	err := client.Publish(ctx, QueueEmailDispatch, msg)
	assert.NoError(t, err, "Disabled client should not error on publish")
}

// TestPublish_InvalidJSON tests publishing non-serializable data
func TestPublish_InvalidJSON(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()

	// channels cannot be marshaled to JSON
	invalidMsg := make(chan int)

	err := client.Publish(ctx, QueueEmailDispatch, invalidMsg)
	assert.Error(t, err, "Should error on invalid JSON")
}

// TestPublish_AllQueueNames tests publishing to all defined queues
func TestPublish_AllQueueNames(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()

	testCases := []struct {
		queueName string
		message   interface{}
	}{
		{QueueEmailDispatch, EmailMessage{To: "test@example.com", Subject: "Test"}},
		{QueueSMSDispatch, SMSMessage{To: "+2348012345678", Message: "Test"}},
		{QueueWithdrawalProcess, WithdrawalMessage{TransactionID: "tx-123", UserID: "user-1", AmountKobo: 50000}},
		{QueueRealtimePush, RealtimeMessage{UserID: "user-1", Event: "test", Payload: map[string]string{"key": "value"}}},
		{QueueDepositReceipt, DepositReceiptMessage{UserID: "user-1", Email: "user@example.com", AmountKobo: 100000}},
	}

	for _, tc := range testCases {
		t.Run(tc.queueName, func(t *testing.T) {
			err := client.Publish(ctx, tc.queueName, tc.message)
			assert.NoError(t, err)
		})
	}
}

// TestPublish_ContextCancellation tests context cancellation during publish
func TestPublish_ContextCancellation(t *testing.T) {
	client := New("")
	defer client.Close()

	// Create already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := EmailMessage{To: "test@example.com", Subject: "Test"}

	// Should handle canceled context gracefully (disabled client)
	err := client.Publish(ctx, QueueEmailDispatch, msg)
	assert.NoError(t, err, "Disabled client should ignore context cancellation")
}

// TestPublish_ConcurrentPublishes tests concurrent publishing
func TestPublish_ConcurrentPublishes(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()
	done := make(chan bool)

	// Spawn multiple goroutines publishing concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				msg := EmailMessage{
					To:      "test@example.com",
					Subject: "Test",
				}
				_ = client.Publish(ctx, QueueEmailDispatch, msg)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CONSUME TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestConsume_DisabledClient tests consuming with disabled client
func TestConsume_DisabledClient(t *testing.T) {
	client := New("")
	defer client.Close()

	handlerCalled := false
	handler := func(ctx context.Context, body []byte) error {
		handlerCalled = true
		return nil
	}

	// Should not panic or block
	client.Consume(QueueEmailDispatch, handler)

	// Give it a moment to potentially call handler (it shouldn't)
	time.Sleep(50 * time.Millisecond)

	assert.False(t, handlerCalled, "Handler should not be called with disabled client")
}

// TestConsume_HandlerSignature tests handler with correct signature
func TestConsume_HandlerSignature(t *testing.T) {
	client := New("")
	defer client.Close()

	// This should compile and not panic
	handler := func(ctx context.Context, body []byte) error {
		return nil
	}

	assert.NotPanics(t, func() {
		client.Consume(QueueEmailDispatch, handler)
	})
}

// TestConsume_MultipleQueues tests consuming from multiple queues
func TestConsume_MultipleQueues(t *testing.T) {
	client := New("")
	defer client.Close()

	queues := []string{
		QueueEmailDispatch,
		QueueSMSDispatch,
		QueueWithdrawalProcess,
		QueueRealtimePush,
		QueueDepositReceipt,
	}

	for _, q := range queues {
		handler := func(ctx context.Context, body []byte) error {
			return nil
		}

		assert.NotPanics(t, func() {
			client.Consume(q, handler)
		}, "Should not panic for queue: %s", q)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CLOSE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestClose_DisabledClient tests closing disabled client
func TestClose_DisabledClient(t *testing.T) {
	client := New("")

	assert.NotPanics(t, func() {
		client.Close()
	})
}

// TestClose_MultipleCalls tests calling Close() multiple times
func TestClose_MultipleCalls(t *testing.T) {
	client := New("")

	// First close
	assert.NotPanics(t, func() {
		client.Close()
	})

	// Second close should be idempotent
	assert.NotPanics(t, func() {
		client.Close()
	})

	// Third close
	assert.NotPanics(t, func() {
		client.Close()
	})

	assert.True(t, client.closed, "Client should be marked as closed")
}

// TestClose_ConcurrentCalls tests concurrent Close() calls
func TestClose_ConcurrentCalls(t *testing.T) {
	client := New("")
	done := make(chan bool)

	// Spawn multiple goroutines calling Close()
	for i := 0; i < 10; i++ {
		go func() {
			client.Close()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.True(t, client.closed)
}

// TestClose_AfterPublish tests closing after publishing
func TestClose_AfterPublish(t *testing.T) {
	client := New("")

	ctx := context.Background()
	msg := EmailMessage{To: "test@example.com", Subject: "Test"}

	_ = client.Publish(ctx, QueueEmailDispatch, msg)

	assert.NotPanics(t, func() {
		client.Close()
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// QUEUE NAME CONSTANTS TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestQueueNameConstants tests that queue names are defined correctly
func TestQueueNameConstants(t *testing.T) {
	testCases := []struct {
		name     string
		constant string
		expected string
	}{
		{"Email Dispatch", QueueEmailDispatch, "propvest.email.dispatch"},
		{"SMS Dispatch", QueueSMSDispatch, "propvest.sms.dispatch"},
		{"Withdrawal Process", QueueWithdrawalProcess, "propvest.withdrawal.process"},
		{"Realtime Push", QueueRealtimePush, "propvest.realtime.push"},
		{"Deposit Receipt", QueueDepositReceipt, "propvest.deposit.receipt"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.constant)
		})
	}
}

// TestQueueNameConstants_Uniqueness tests that all queue names are unique
func TestQueueNameConstants_Uniqueness(t *testing.T) {
	queues := []string{
		QueueEmailDispatch,
		QueueSMSDispatch,
		QueueWithdrawalProcess,
		QueueRealtimePush,
		QueueDepositReceipt,
	}

	seen := make(map[string]bool)
	for _, q := range queues {
		assert.False(t, seen[q], "Duplicate queue name: %s", q)
		seen[q] = true
	}
}

// TestQueueNameConstants_Format tests naming convention
func TestQueueNameConstants_Format(t *testing.T) {
	queues := []string{
		QueueEmailDispatch,
		QueueSMSDispatch,
		QueueWithdrawalProcess,
		QueueRealtimePush,
		QueueDepositReceipt,
	}

	for _, q := range queues {
		assert.Contains(t, q, "propvest.", "Queue name should start with 'propvest.'")
		assert.NotContains(t, q, " ", "Queue name should not contain spaces")
		assert.NotContains(t, q, "_", "Queue name should use dots, not underscores (by convention)")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION/WORKFLOW TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestClient_PublishConsumeWorkflow tests publish-consume workflow with disabled client
func TestClient_PublishConsumeWorkflow(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()

	// Publish a message
	msg := EmailMessage{
		To:       "user@example.com",
		Subject:  "Test Email",
		HTMLBody: "<p>Hello World</p>",
		TextBody: "Hello World",
	}

	err := client.Publish(ctx, QueueEmailDispatch, msg)
	assert.NoError(t, err)

	// Try to consume (won't actually receive anything with disabled client)
	handlerCalled := false
	handler := func(ctx context.Context, body []byte) error {
		handlerCalled = true
		return nil
	}

	client.Consume(QueueEmailDispatch, handler)

	time.Sleep(50 * time.Millisecond)

	assert.False(t, handlerCalled, "Handler should not be called with disabled client")
}

// TestClient_Lifecycle tests full client lifecycle
func TestClient_Lifecycle(t *testing.T) {
	// 1. Create client
	client := New("")
	assert.NotNil(t, client)

	// 2. Check enabled status
	assert.False(t, client.Enabled())

	// 3. Publish some messages
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		msg := EmailMessage{
			To:      "test@example.com",
			Subject: "Test",
		}
		err := client.Publish(ctx, QueueEmailDispatch, msg)
		assert.NoError(t, err)
	}

	// 4. Setup consumer
	client.Consume(QueueEmailDispatch, func(ctx context.Context, body []byte) error {
		return nil
	})

	// 5. Close client
	client.Close()

	// 6. Verify closed
	assert.True(t, client.closed)
}

// TestClient_ErrorHandling tests error handling with disabled client
func TestClient_ErrorHandling(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()

	// Invalid JSON should error
	err := client.Publish(ctx, QueueEmailDispatch, make(chan int))
	assert.Error(t, err)

	// Valid message should not error (disabled mode)
	err = client.Publish(ctx, QueueEmailDispatch, EmailMessage{To: "test@example.com"})
	assert.NoError(t, err)
}

// TestClient_MessageSerialization tests that messages serialize correctly
func TestClient_MessageSerialization(t *testing.T) {
	testCases := []struct {
		name    string
		message interface{}
	}{
		{
			"EmailMessage",
			EmailMessage{
				To:       "user@example.com",
				Subject:  "Test",
				HTMLBody: "<p>Hello</p>",
				TextBody: "Hello",
			},
		},
		{
			"SMSMessage",
			SMSMessage{
				To:      "+2348012345678",
				Message: "Test SMS",
			},
		},
		{
			"WithdrawalMessage",
			WithdrawalMessage{
				TransactionID: "tx-123",
				UserID:        "user-456",
				AmountKobo:    50000,
				Reference:     "ref-789",
			},
		},
		{
			"RealtimeMessage",
			RealtimeMessage{
				UserID:  "user-123",
				Event:   "notification.created",
				Payload: map[string]string{"title": "New Notification"},
			},
		},
		{
			"DepositReceiptMessage",
			DepositReceiptMessage{
				UserID:     "user-123",
				Email:      "user@example.com",
				AmountKobo: 100000,
				Reference:  "ref-abc",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should be able to marshal to JSON
			data, err := json.Marshal(tc.message)
			require.NoError(t, err, "Message should serialize to JSON")
			assert.NotEmpty(t, data)

			// Should be valid JSON
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			require.NoError(t, err, "Should deserialize back from JSON")
		})
	}
}

// TestClient_NilMessageHandling tests handling of nil messages
func TestClient_NilMessageHandling(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()

	// nil pointer should serialize to "null" (valid JSON)
	err := client.Publish(ctx, QueueEmailDispatch, (*EmailMessage)(nil))
	assert.NoError(t, err, "Should handle nil message (disabled client)")
}

// TestClient_EmptyMessage tests publishing empty messages
func TestClient_EmptyMessage(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()

	// Empty struct is valid JSON: {}
	err := client.Publish(ctx, QueueEmailDispatch, EmailMessage{})
	assert.NoError(t, err)
}

// TestClient_LargeMessage tests publishing large messages
func TestClient_LargeMessage(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()

	// Create a large HTML body (10KB)
	largeBody := string(make([]byte, 10*1024))
	msg := EmailMessage{
		To:       "user@example.com",
		Subject:  "Large Email",
		HTMLBody: largeBody,
		TextBody: largeBody,
	}

	err := client.Publish(ctx, QueueEmailDispatch, msg)
	assert.NoError(t, err, "Should handle large messages")
}

// ═══════════════════════════════════════════════════════════════════════════
// EDGE CASE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestClient_PublishAfterClose tests publishing after closing client
func TestClient_PublishAfterClose(t *testing.T) {
	client := New("")
	client.Close()

	ctx := context.Background()
	msg := EmailMessage{To: "test@example.com", Subject: "Test"}

	// Should not panic (disabled client gracefully handles this)
	err := client.Publish(ctx, QueueEmailDispatch, msg)
	assert.NoError(t, err)
}

// TestClient_ConsumeAfterClose tests consuming after closing client
func TestClient_ConsumeAfterClose(t *testing.T) {
	client := New("")
	client.Close()

	handler := func(ctx context.Context, body []byte) error {
		return nil
	}

	// Should not panic
	assert.NotPanics(t, func() {
		client.Consume(QueueEmailDispatch, handler)
	})
}

// TestClient_EmptyQueueName tests publishing to empty queue name
func TestClient_EmptyQueueName(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()
	msg := EmailMessage{To: "test@example.com"}

	// Should not panic (disabled client)
	err := client.Publish(ctx, "", msg)
	assert.NoError(t, err)
}

// TestClient_SpecialCharactersInMessage tests special characters
func TestClient_SpecialCharactersInMessage(t *testing.T) {
	client := New("")
	defer client.Close()

	ctx := context.Background()
	msg := EmailMessage{
		To:       "user@example.com",
		Subject:  "Test with émojis 🎉 and spëcial çhars",
		HTMLBody: "<p>Hello 世界 🌍</p>",
		TextBody: "Hello 世界 🌍",
	}

	err := client.Publish(ctx, QueueEmailDispatch, msg)
	assert.NoError(t, err, "Should handle UTF-8 and special characters")
}
