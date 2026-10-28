package queue

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
// EMAIL MESSAGE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestEmailMessage_JSONSerialization tests EmailMessage JSON marshaling
func TestEmailMessage_JSONSerialization(t *testing.T) {
	msg := EmailMessage{
		To:       "user@example.com",
		Subject:  "Welcome to PropVest",
		HTMLBody: "<p>Hello, welcome!</p>",
		TextBody: "Hello, welcome!",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Should contain expected fields
	assert.Contains(t, string(data), "user@example.com")
	assert.Contains(t, string(data), "Welcome to PropVest")
}

// TestEmailMessage_JSONDeserialization tests EmailMessage JSON unmarshaling
func TestEmailMessage_JSONDeserialization(t *testing.T) {
	jsonData := `{
		"to": "user@example.com",
		"subject": "Test Subject",
		"html_body": "<p>Test HTML</p>",
		"text_body": "Test Text"
	}`

	var msg EmailMessage
	err := json.Unmarshal([]byte(jsonData), &msg)
	require.NoError(t, err)

	assert.Equal(t, "user@example.com", msg.To)
	assert.Equal(t, "Test Subject", msg.Subject)
	assert.Equal(t, "<p>Test HTML</p>", msg.HTMLBody)
	assert.Equal(t, "Test Text", msg.TextBody)
}

// TestEmailMessage_EmptyFields tests EmailMessage with empty fields
func TestEmailMessage_EmptyFields(t *testing.T) {
	msg := EmailMessage{}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded EmailMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "", decoded.To)
	assert.Equal(t, "", decoded.Subject)
}

// TestEmailMessage_SpecialCharacters tests EmailMessage with UTF-8
func TestEmailMessage_SpecialCharacters(t *testing.T) {
	msg := EmailMessage{
		To:       "用户@example.com",
		Subject:  "Test with émojis 🎉",
		HTMLBody: "<p>Hello 世界</p>",
		TextBody: "Hello 世界",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded EmailMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.To, decoded.To)
	assert.Equal(t, msg.Subject, decoded.Subject)
	assert.Equal(t, msg.HTMLBody, decoded.HTMLBody)
}

// ═══════════════════════════════════════════════════════════════════════════
// SMS MESSAGE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestSMSMessage_JSONSerialization tests SMSMessage JSON marshaling
func TestSMSMessage_JSONSerialization(t *testing.T) {
	msg := SMSMessage{
		To:      "+2348012345678",
		Message: "Your OTP is 123456",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	assert.Contains(t, string(data), "+2348012345678")
	assert.Contains(t, string(data), "Your OTP is 123456")
}

// TestSMSMessage_JSONDeserialization tests SMSMessage JSON unmarshaling
func TestSMSMessage_JSONDeserialization(t *testing.T) {
	jsonData := `{
		"to": "+2348012345678",
		"message": "Test SMS message"
	}`

	var msg SMSMessage
	err := json.Unmarshal([]byte(jsonData), &msg)
	require.NoError(t, err)

	assert.Equal(t, "+2348012345678", msg.To)
	assert.Equal(t, "Test SMS message", msg.Message)
}

// TestSMSMessage_LongMessage tests SMSMessage with long content
func TestSMSMessage_LongMessage(t *testing.T) {
	longMessage := string(make([]byte, 500)) // 500 character message

	msg := SMSMessage{
		To:      "+2348012345678",
		Message: longMessage,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded SMSMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.Message, decoded.Message)
}

// TestSMSMessage_SpecialCharacters tests SMSMessage with special chars
func TestSMSMessage_SpecialCharacters(t *testing.T) {
	msg := SMSMessage{
		To:      "+2348012345678",
		Message: "Hello! Your código is: ₦1,000 🎉",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded SMSMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.Message, decoded.Message)
}

// ═══════════════════════════════════════════════════════════════════════════
// WITHDRAWAL MESSAGE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestWithdrawalMessage_JSONSerialization tests WithdrawalMessage marshaling
func TestWithdrawalMessage_JSONSerialization(t *testing.T) {
	msg := WithdrawalMessage{
		TransactionID: "tx-abc-123",
		UserID:        "user-456",
		AmountKobo:    50000,
		Reference:     "WD-20260906-001",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	assert.Contains(t, string(data), "tx-abc-123")
	assert.Contains(t, string(data), "user-456")
	assert.Contains(t, string(data), "50000")
}

// TestWithdrawalMessage_JSONDeserialization tests unmarshaling
func TestWithdrawalMessage_JSONDeserialization(t *testing.T) {
	jsonData := `{
		"transaction_id": "tx-xyz-789",
		"user_id": "user-999",
		"amount_kobo": 100000,
		"reference": "WD-REF-001"
	}`

	var msg WithdrawalMessage
	err := json.Unmarshal([]byte(jsonData), &msg)
	require.NoError(t, err)

	assert.Equal(t, "tx-xyz-789", msg.TransactionID)
	assert.Equal(t, "user-999", msg.UserID)
	assert.Equal(t, int64(100000), msg.AmountKobo)
	assert.Equal(t, "WD-REF-001", msg.Reference)
}

// TestWithdrawalMessage_LargeAmount tests large withdrawal amounts
func TestWithdrawalMessage_LargeAmount(t *testing.T) {
	msg := WithdrawalMessage{
		TransactionID: "tx-123",
		UserID:        "user-456",
		AmountKobo:    100000000000, // 1 billion naira in kobo
		Reference:     "REF-001",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded WithdrawalMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, msg.AmountKobo, decoded.AmountKobo)
}

// TestWithdrawalMessage_ZeroAmount tests zero amount edge case
func TestWithdrawalMessage_ZeroAmount(t *testing.T) {
	msg := WithdrawalMessage{
		TransactionID: "tx-123",
		UserID:        "user-456",
		AmountKobo:    0,
		Reference:     "REF-001",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded WithdrawalMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(0), decoded.AmountKobo)
}

// TestWithdrawalMessage_NegativeAmount tests negative amount handling
func TestWithdrawalMessage_NegativeAmount(t *testing.T) {
	msg := WithdrawalMessage{
		TransactionID: "tx-123",
		UserID:        "user-456",
		AmountKobo:    -50000, // Negative (shouldn't happen, but test serialization)
		Reference:     "REF-001",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded WithdrawalMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, int64(-50000), decoded.AmountKobo)
}

// ═══════════════════════════════════════════════════════════════════════════
// DEPOSIT RECEIPT MESSAGE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestDepositReceiptMessage_JSONSerialization tests marshaling
func TestDepositReceiptMessage_JSONSerialization(t *testing.T) {
	msg := DepositReceiptMessage{
		UserID:     "user-123",
		Email:      "user@example.com",
		AmountKobo: 50000,
		Reference:  "DEP-20260906-001",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	assert.Contains(t, string(data), "user-123")
	assert.Contains(t, string(data), "user@example.com")
	assert.Contains(t, string(data), "50000")
}

// TestDepositReceiptMessage_JSONDeserialization tests unmarshaling
func TestDepositReceiptMessage_JSONDeserialization(t *testing.T) {
	jsonData := `{
		"user_id": "user-789",
		"email": "test@example.com",
		"amount_kobo": 100000,
		"reference": "DEP-REF-999"
	}`

	var msg DepositReceiptMessage
	err := json.Unmarshal([]byte(jsonData), &msg)
	require.NoError(t, err)

	assert.Equal(t, "user-789", msg.UserID)
	assert.Equal(t, "test@example.com", msg.Email)
	assert.Equal(t, int64(100000), msg.AmountKobo)
	assert.Equal(t, "DEP-REF-999", msg.Reference)
}

// TestDepositReceiptMessage_RoundTrip tests full serialization cycle
func TestDepositReceiptMessage_RoundTrip(t *testing.T) {
	original := DepositReceiptMessage{
		UserID:     "user-abc",
		Email:      "test@propvest.ng",
		AmountKobo: 250000,
		Reference:  "DEP-2026-0001",
	}

	// Marshal
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	var decoded DepositReceiptMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Compare
	assert.Equal(t, original, decoded)
}

// ═══════════════════════════════════════════════════════════════════════════
// REALTIME MESSAGE TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestRealtimeMessage_JSONSerialization tests marshaling
func TestRealtimeMessage_JSONSerialization(t *testing.T) {
	msg := RealtimeMessage{
		UserID: "user-123",
		Event:  "notification.created",
		Payload: map[string]interface{}{
			"title":   "New Notification",
			"message": "You have a new message",
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	assert.Contains(t, string(data), "user-123")
	assert.Contains(t, string(data), "notification.created")
}

// TestRealtimeMessage_JSONDeserialization tests unmarshaling
func TestRealtimeMessage_JSONDeserialization(t *testing.T) {
	jsonData := `{
		"user_id": "user-456",
		"event": "wallet.credited",
		"payload": {
			"amount": 50000,
			"balance": 150000
		}
	}`

	var msg RealtimeMessage
	err := json.Unmarshal([]byte(jsonData), &msg)
	require.NoError(t, err)

	assert.Equal(t, "user-456", msg.UserID)
	assert.Equal(t, "wallet.credited", msg.Event)
	assert.NotNil(t, msg.Payload)
}

// TestRealtimeMessage_NilPayload tests nil payload
func TestRealtimeMessage_NilPayload(t *testing.T) {
	msg := RealtimeMessage{
		UserID:  "user-123",
		Event:   "test.event",
		Payload: nil,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded RealtimeMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Nil(t, decoded.Payload)
}

// TestRealtimeMessage_ComplexPayload tests complex nested payload
func TestRealtimeMessage_ComplexPayload(t *testing.T) {
	msg := RealtimeMessage{
		UserID: "user-123",
		Event:  "investment.completed",
		Payload: map[string]interface{}{
			"investment": map[string]interface{}{
				"id":       "inv-123",
				"property": "Lekki Gardens Phase 2",
				"units":    10,
				"amount":   5000000,
			},
			"timestamp": "2026-09-06T10:00:00Z",
			"metadata": map[string]string{
				"source":  "api",
				"version": "1.0",
			},
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded RealtimeMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotNil(t, decoded.Payload)
}

// TestRealtimeMessage_ArrayPayload tests array as payload
func TestRealtimeMessage_ArrayPayload(t *testing.T) {
	msg := RealtimeMessage{
		UserID: "user-123",
		Event:  "notifications.batch",
		Payload: []map[string]interface{}{
			{"id": "1", "title": "First"},
			{"id": "2", "title": "Second"},
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded RealtimeMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotNil(t, decoded.Payload)
}

// TestRealtimeMessage_StringPayload tests string as payload
func TestRealtimeMessage_StringPayload(t *testing.T) {
	msg := RealtimeMessage{
		UserID:  "user-123",
		Event:   "message.received",
		Payload: "Simple string payload",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded RealtimeMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotNil(t, decoded.Payload)
}

// TestRealtimeMessage_NumberPayload tests number as payload
func TestRealtimeMessage_NumberPayload(t *testing.T) {
	msg := RealtimeMessage{
		UserID:  "user-123",
		Event:   "balance.updated",
		Payload: 150000,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded RealtimeMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotNil(t, decoded.Payload)
}

// ═══════════════════════════════════════════════════════════════════════════
// INTEGRATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

// TestAllMessages_RoundTrip tests all message types can round-trip
func TestAllMessages_RoundTrip(t *testing.T) {
	testCases := []struct {
		name    string
		message interface{}
	}{
		{
			"EmailMessage",
			EmailMessage{
				To:       "user@example.com",
				Subject:  "Test",
				HTMLBody: "<p>Test</p>",
				TextBody: "Test",
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
				Reference:     "REF-001",
			},
		},
		{
			"DepositReceiptMessage",
			DepositReceiptMessage{
				UserID:     "user-123",
				Email:      "user@example.com",
				AmountKobo: 100000,
				Reference:  "DEP-001",
			},
		},
		{
			"RealtimeMessage",
			RealtimeMessage{
				UserID:  "user-123",
				Event:   "test.event",
				Payload: map[string]string{"key": "value"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tc.message)
			require.NoError(t, err, "Should marshal to JSON")
			assert.NotEmpty(t, data)

			// Unmarshal back to map to verify structure
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			require.NoError(t, err, "Should unmarshal from JSON")
			assert.NotEmpty(t, result)
		})
	}
}

// TestMessages_EmptyStructs tests all message types as empty structs
func TestMessages_EmptyStructs(t *testing.T) {
	messages := []interface{}{
		EmailMessage{},
		SMSMessage{},
		WithdrawalMessage{},
		DepositReceiptMessage{},
		RealtimeMessage{},
	}

	for _, msg := range messages {
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		assert.NotEmpty(t, data, "Empty struct should serialize to {}")
	}
}

// TestMessages_JSONFieldNames tests JSON field naming conventions
func TestMessages_JSONFieldNames(t *testing.T) {
	// EmailMessage uses snake_case
	email := EmailMessage{HTMLBody: "test", TextBody: "test"}
	data, _ := json.Marshal(email)
	assert.Contains(t, string(data), "html_body")
	assert.Contains(t, string(data), "text_body")

	// WithdrawalMessage uses snake_case
	withdrawal := WithdrawalMessage{TransactionID: "tx-1", AmountKobo: 1000}
	data, _ = json.Marshal(withdrawal)
	assert.Contains(t, string(data), "transaction_id")
	assert.Contains(t, string(data), "amount_kobo")

	// DepositReceiptMessage uses snake_case
	receipt := DepositReceiptMessage{UserID: "u1", AmountKobo: 1000}
	data, _ = json.Marshal(receipt)
	assert.Contains(t, string(data), "user_id")
	assert.Contains(t, string(data), "amount_kobo")

	// RealtimeMessage uses snake_case
	realtime := RealtimeMessage{UserID: "u1", Event: "test"}
	data, _ = json.Marshal(realtime)
	assert.Contains(t, string(data), "user_id")
}
