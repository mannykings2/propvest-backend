// Package queue is the application's asynchronous messaging layer, backed by
// RabbitMQ (AMQP 0-9-1) via github.com/rabbitmq/amqp091-go.
//
// WHY A QUEUE AT ALL?
// -------------------
// Some work must NOT happen inside the HTTP request:
//   - Sending emails/SMS (slow, flaky third-party calls; must be retryable).
//   - Processing a withdrawal payout (talks to a bank/provider; can be slow).
//   - Fanning a realtime notification out to WebSocket clients that may be
//     connected to a DIFFERENT process than the one that produced the event.
//
// A message queue lets the producer "fire and forget": it drops a small JSON
// message on a durable queue and returns immediately, while a separate WORKER
// process consumes and does the slow/failure-prone work with retries. This is
// the classic way to keep the API fast and resilient (docs 7.3 Background Jobs).
//
// WHY RABBITMQ?
// ------------
// It is free and open-source (self-hosted via Docker), battle-tested, supports
// durable queues, acknowledgements, and redelivery — everything we need for
// reliable financial/notification workflows — without any paid service.
//
// GRACEFUL DEGRADATION
// --------------------
// A junior dev cloning this repo may not have RabbitMQ running. So New() never
// fails hard: if RABBITMQ_URL is empty or the broker is unreachable, the client
// runs in a DISABLED mode where Publish logs a warning and drops the message,
// and Consume is a no-op. The API and its synchronous features keep working;
// only the async side effects are skipped. In production you WILL run RabbitMQ,
// and the same code lights up automatically.
package queue

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/mannykings2/propvest-backend/internal/logger"
)

// Queue names. Keeping them as constants avoids typos between publisher and
// consumer (a mismatched name silently drops messages).
const (
	QueueEmailDispatch      = "propvest.email.dispatch"      // worker sends emails
	QueueSMSDispatch        = "propvest.sms.dispatch"        // worker sends SMS
	QueueWithdrawalProcess  = "propvest.withdrawal.process"  // worker processes payouts
	QueueRealtimePush       = "propvest.realtime.push"       // API pushes to WebSocket clients
	QueueDepositReceipt     = "propvest.deposit.receipt"     // worker emails deposit receipts
)

// Client is the queue façade used by the rest of the app. It is safe for
// concurrent use.
type Client struct {
	url      string
	mu       sync.RWMutex
	conn     *amqp.Connection
	channel  *amqp.Channel
	enabled  bool // false => disabled/no-op mode
	closed   bool
}

// New creates a queue client and attempts to connect. It NEVER returns an error:
// on failure it returns a disabled client (see package doc) so the app can run
// without RabbitMQ during local development.
func New(url string) *Client {
	c := &Client{url: url}
	if url == "" {
		logger.Warn("RABBITMQ_URL not set; async queue disabled (emails/SMS/withdrawals will be skipped)")
		return c
	}
	if err := c.connect(); err != nil {
		logger.Warn("could not connect to RabbitMQ; running with queue disabled", "error", err)
		return c
	}
	c.enabled = true
	logger.Info("connected to RabbitMQ")
	return c
}

// connect dials RabbitMQ, opens a channel, and declares all durable queues.
func (c *Client) connect() error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}

	// Declare every queue as durable so messages survive a broker restart.
	for _, q := range []string{
		QueueEmailDispatch, QueueSMSDispatch, QueueWithdrawalProcess,
		QueueRealtimePush, QueueDepositReceipt,
	} {
		if _, err := ch.QueueDeclare(q, true /*durable*/, false, false, false, nil); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return err
		}
	}

	c.mu.Lock()
	c.conn = conn
	c.channel = ch
	c.mu.Unlock()
	return nil
}

// Enabled reports whether the client is actually connected to a broker.
func (c *Client) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

// Publish marshals v to JSON and publishes it to the named queue as a persistent
// message. In disabled mode it logs and returns nil (best-effort semantics).
func (c *Client) Publish(ctx context.Context, queueName string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}

	c.mu.RLock()
	enabled, ch := c.enabled, c.channel
	c.mu.RUnlock()

	if !enabled || ch == nil {
		logger.Warn("queue disabled; dropping message", "queue", queueName)
		return nil
	}

	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return ch.PublishWithContext(pctx,
		"",        // default exchange: route by queue name
		queueName, // routing key == queue name
		false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // survive broker restart
			Timestamp:    time.Now(),
		},
	)
}

// Consume registers handler for messages on queueName and blocks-consuming in a
// background goroutine. The handler returns an error to NACK+requeue (retry) or
// nil to ACK. In disabled mode Consume is a no-op.
//
// This is deliberately simple (at-least-once delivery, requeue-on-error). For
// production you would add a dead-letter queue and capped retries; that is noted
// in the completion plan as a Milestone 8 hardening task.
func (c *Client) Consume(queueName string, handler func(ctx context.Context, body []byte) error) {
	c.mu.RLock()
	enabled, ch := c.enabled, c.channel
	c.mu.RUnlock()

	if !enabled || ch == nil {
		logger.Warn("queue disabled; not consuming", "queue", queueName)
		return
	}

	deliveries, err := ch.Consume(queueName, "", false /*autoAck=false*/, false, false, false, nil)
	if err != nil {
		logger.Error("failed to start consumer", "queue", queueName, "error", err)
		return
	}

	go func() {
		logger.Info("consumer started", "queue", queueName)
		for d := range deliveries {
			ctx := context.Background()
			if err := handler(ctx, d.Body); err != nil {
				logger.Error("message handler failed; requeuing", "queue", queueName, "error", err)
				_ = d.Nack(false, true) // requeue for another attempt
				continue
			}
			_ = d.Ack(false)
		}
		logger.Warn("consumer channel closed", "queue", queueName)
	}()
}

// Close tears down the channel and connection.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}
