package queue

// QueueConfig defines the configuration for a RabbitMQ queue.
// This includes queue declaration parameters, message TTL, dead-letter queue
// routing, and consumer behavior settings.
//
// Queue Configuration Requirements (Req 12.x):
// - Durable queues survive broker restarts (Req 12.1)
// - Max-length prevents memory exhaustion via backpressure (Req 12.2, 12.3)
// - Message TTL ensures stale messages expire (48 hours for primary queues)
// - Priority queues allow withdrawal events to jump ahead of deposits (Req 12.4)
// - DLX routing sends failed messages to dead-letter queues for manual investigation
// - Prefetch and concurrency settings balance throughput vs resource usage (Req 12.6)
//
// Environment-Specific Settings (Req 14.x):
// Development defaults: max_retries=1, concurrency=2, max_length=100
// Production defaults: max_retries=3, concurrency=10, max_length=10000
type QueueConfig struct {
	// Queue declaration
	Name       string // Queue name (e.g., "propvest.deposit.confirmed")
	Durable    bool   // Survive broker restart (Req 12.1)
	AutoDelete bool   // Delete when unused (should be false for production)
	Exclusive  bool   // Used by only one connection (should be false for shared queues)

	// Queue arguments
	MaxLength int   // Backpressure limit - reject new messages when reached (Req 12.2, 12.3)
	TTL       int64 // Message TTL in milliseconds (48h = 172800000ms)
	Priority  uint8 // Queue priority level (0-10, higher = more important) (Req 12.4)

	// Dead-letter queue configuration (Req 3.x)
	DLXName string // Dead letter exchange name
	DLXKey  string // Dead letter routing key

	// Consumer configuration
	PrefetchCount int // Messages to fetch at once per consumer (Req 12.6)
	Concurrency   int // Concurrent handlers per consumer instance (Req 13.3, 14.2, 14.3)
}

// Queue configuration constants for all queue types.
// These configurations are used in declareTopology to set up the RabbitMQ
// queue infrastructure with appropriate settings for each queue type.
var (
	// DepositQueueConfig configures the deposit confirmation queue.
	// Deposits are medium priority (5) and processed with moderate concurrency (10).
	// Requirements: 12.1, 12.2, 12.3, 12.4, 12.6, 14.3
	DepositQueueConfig = QueueConfig{
		Name:       "propvest.deposit.confirmed",
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		MaxLength:  10000,                      // Production default (Req 14.3)
		TTL:        48 * 60 * 60 * 1000,        // 48 hours in milliseconds
		Priority:   5,                          // Medium priority (Req 12.4)
		DLXName:    "propvest.dlx",             // Dead letter exchange
		DLXKey:     "deposit.dlq",              // DLQ routing key
		PrefetchCount: 10,                      // Fetch 10 messages at a time (Req 12.6)
		Concurrency:   10,                      // 10 concurrent handlers (Req 14.3)
	}

	// WithdrawalQueueConfig configures the withdrawal processing queue.
	// Withdrawals are highest priority (10) because user funds are locked.
	// Lower concurrency (5) due to bank API rate limits.
	// Requirements: 12.1, 12.2, 12.3, 12.4, 12.6, 14.3
	WithdrawalQueueConfig = QueueConfig{
		Name:       "propvest.withdrawal.requested",
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		MaxLength:  10000,                      // Production default (Req 14.3)
		TTL:        48 * 60 * 60 * 1000,        // 48 hours
		Priority:   10,                         // Highest priority - money locked (Req 12.4)
		DLXName:    "propvest.dlx",
		DLXKey:     "withdrawal.dlq",
		PrefetchCount: 5,                       // Lower prefetch for rate-limited operations
		Concurrency:   5,                       // Lower concurrency for bank API (Req 14.3)
	}

	// NotificationQueueConfig configures the notification dispatch queue.
	// Notifications are lowest priority (1) - can be delayed if system is busy.
	// Higher concurrency (20) since email/SMS services can handle parallel requests.
	// Requirements: 12.1, 12.2, 12.3, 12.4, 12.6, 14.3
	NotificationQueueConfig = QueueConfig{
		Name:       "propvest.notification.dispatch",
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		MaxLength:  10000,                      // Production default (Req 14.3)
		TTL:        48 * 60 * 60 * 1000,        // 48 hours
		Priority:   1,                          // Lowest priority (Req 12.4)
		DLXName:    "propvest.dlx",
		DLXKey:     "notification.dlq",
		PrefetchCount: 20,                      // High prefetch for high-throughput
		Concurrency:   20,                      // High concurrency (Req 14.3)
	}

	// ReceiptQueueConfig configures the receipt generation queue.
	// Receipts are low priority (1) - generated after transaction completion.
	// Medium concurrency (10) for PDF generation and email delivery.
	// Requirements: 12.1, 12.2, 12.3, 12.4, 12.6, 14.3
	ReceiptQueueConfig = QueueConfig{
		Name:       "propvest.receipt.generate",
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		MaxLength:  10000,                      // Production default (Req 14.3)
		TTL:        48 * 60 * 60 * 1000,        // 48 hours
		Priority:   1,                          // Low priority (Req 12.4)
		DLXName:    "propvest.dlx",
		DLXKey:     "receipt.dlq",
		PrefetchCount: 10,                      // Standard prefetch
		Concurrency:   10,                      // Standard concurrency (Req 14.3)
	}
)

// GetDevelopmentConfig returns queue configurations suitable for local development.
// Development settings use lower limits to conserve resources and speed up testing.
// Requirement 14.2: Development defaults
func GetDevelopmentConfig(baseConfig QueueConfig) QueueConfig {
	config := baseConfig
	config.MaxLength = 100        // Lower queue depth for dev (Req 14.2)
	config.Concurrency = 2        // Lower concurrency for dev (Req 14.2)
	return config
}

// GetProductionConfig returns queue configurations suitable for production use.
// Production settings use higher limits for scalability and throughput.
// Requirement 14.3: Production defaults
func GetProductionConfig(baseConfig QueueConfig) QueueConfig {
	config := baseConfig
	config.MaxLength = 10000      // Higher queue depth for production (Req 14.3)
	// Concurrency is already set to production values in base configs
	return config
}

// AllQueueConfigs returns all queue configurations as a slice.
// This is useful for iterating over all queues during topology setup.
func AllQueueConfigs() []QueueConfig {
	return []QueueConfig{
		DepositQueueConfig,
		WithdrawalQueueConfig,
		NotificationQueueConfig,
		ReceiptQueueConfig,
	}
}

// GetQueueConfigByName retrieves a queue configuration by name.
// Returns the config and true if found, or zero config and false if not found.
func GetQueueConfigByName(name string) (QueueConfig, bool) {
	configs := map[string]QueueConfig{
		DepositQueueConfig.Name:      DepositQueueConfig,
		WithdrawalQueueConfig.Name:   WithdrawalQueueConfig,
		NotificationQueueConfig.Name: NotificationQueueConfig,
		ReceiptQueueConfig.Name:      ReceiptQueueConfig,
	}
	config, ok := configs[name]
	return config, ok
}

// DLQTTLMilliseconds is the TTL for dead-letter queues (7 days).
// Messages in DLQs are kept for manual investigation before expiry.
// Requirement 3.8: DLQ message expiry to 7 days
const DLQTTLMilliseconds int64 = 7 * 24 * 60 * 60 * 1000 // 7 days in milliseconds

// MaxRetries is the default maximum number of retries before routing to DLQ.
// Requirement 4.x: Retry policy with max 3 attempts
const MaxRetries = 3

// RetryDelays defines exponential backoff delays for each retry attempt.
// Requirement 4.1: Exponential backoff with delays: 5s, 15s, 45s
var RetryDelays = []int{5, 15, 45} // seconds

// GetRetryDelay returns the delay in seconds for a given retry attempt (0-indexed).
// If retry count exceeds max, returns the last delay value.
func GetRetryDelay(retryCount int) int {
	if retryCount < 0 {
		return RetryDelays[0]
	}
	if retryCount >= len(RetryDelays) {
		return RetryDelays[len(RetryDelays)-1]
	}
	return RetryDelays[retryCount]
}
