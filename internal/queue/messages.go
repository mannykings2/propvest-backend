package queue

// This file defines the JSON message contracts that travel through RabbitMQ.
// Producers (API/worker) and consumers (worker/API) both import these types, so
// the shape of a message can never drift between the two sides.

// EmailMessage is dispatched to QueueEmailDispatch; the worker sends the email.
type EmailMessage struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`
}

// SMSMessage is dispatched to QueueSMSDispatch; the worker sends the SMS.
type SMSMessage struct {
	To      string `json:"to"`
	Message string `json:"message"`
}

// WithdrawalMessage is dispatched to QueueWithdrawalProcess after a withdrawal
// has been recorded (wallet already debited). The worker performs the actual
// payout (mocked here) and marks the transaction completed/failed.
type WithdrawalMessage struct {
	TransactionID string `json:"transaction_id"`
	UserID        string `json:"user_id"`
	AmountKobo    int64  `json:"amount_kobo"`
	Reference     string `json:"reference"`
}

// DepositReceiptMessage is dispatched to QueueDepositReceipt after a wallet has
// been credited from a verified payment; the worker emails a receipt.
type DepositReceiptMessage struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	AmountKobo int64  `json:"amount_kobo"`
	Reference  string `json:"reference"`
}

// RealtimeMessage is published to QueueRealtimePush by any process; the API
// process consumes it and pushes the payload to the target user's connected
// WebSocket clients. This is how a background WORKER can trigger a realtime UI
// update even though it holds no WebSocket connections itself.
type RealtimeMessage struct {
	UserID  string `json:"user_id"`
	Event   string `json:"event"`   // e.g. "notification.created"
	Payload any    `json:"payload"` // arbitrary JSON for the client
}
