package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// QueueConfig holds all configuration parameters for the RabbitMQ queue system.
// It provides environment-specific defaults for development and production,
// enabling safe testing with low concurrency and high throughput in production.
type QueueConfig struct {
	// Connection
	URL string `mapstructure:"RABBITMQ_URL"` // amqp://user:pass@host:port/vhost

	// Retry policy
	MaxRetries     int           `mapstructure:"QUEUE_MAX_RETRIES"`      // Maximum retry attempts before DLQ
	RetryDelayBase time.Duration `mapstructure:"QUEUE_RETRY_DELAY_BASE"` // Base delay for exponential backoff

	// Consumer configuration
	Concurrency    int `mapstructure:"QUEUE_CONSUMER_CONCURRENCY"`     // Concurrent message handlers per consumer
	PrefetchCount  int `mapstructure:"QUEUE_PREFETCH_COUNT"`           // Messages to fetch at once
	MaxLength      int `mapstructure:"QUEUE_MAX_LENGTH"`               // Maximum queue depth before backpressure
	MessageTTL     int `mapstructure:"QUEUE_MESSAGE_TTL_HOURS"`        // Message TTL in hours (default 48h)
	DLQMessageTTL  int `mapstructure:"QUEUE_DLQ_MESSAGE_TTL_DAYS"`     // DLQ message TTL in days (default 7d)

	// Reconciliation
	ReconciliationInterval      time.Duration `mapstructure:"QUEUE_RECONCILIATION_INTERVAL"`       // How often to run reconciliation (default 5m)
	ReconciliationPendingAge    time.Duration `mapstructure:"QUEUE_RECONCILIATION_PENDING_AGE"`    // Age threshold for pending events (default 10m)
	ReconciliationProcessingAge time.Duration `mapstructure:"QUEUE_RECONCILIATION_PROCESSING_AGE"` // Age threshold for stuck processing events (default 30m)
	ReconciliationBatchSize     int           `mapstructure:"QUEUE_RECONCILIATION_BATCH_SIZE"`     // Batch size for reconciliation (default 100)

	// Circuit breaker
	CircuitBreakerThreshold int           `mapstructure:"QUEUE_CIRCUIT_BREAKER_THRESHOLD"` // Failures before opening circuit (default 10)
	CircuitBreakerTimeout   time.Duration `mapstructure:"QUEUE_CIRCUIT_BREAKER_TIMEOUT"`   // Time before attempting half-open (default 60s)

	// Per-queue overrides (optional, use defaults if not set)
	DepositConcurrency     int `mapstructure:"QUEUE_DEPOSIT_CONCURRENCY"`
	WithdrawalConcurrency  int `mapstructure:"QUEUE_WITHDRAWAL_CONCURRENCY"`
	NotificationConcurrency int `mapstructure:"QUEUE_NOTIFICATION_CONCURRENCY"`
	ReceiptConcurrency     int `mapstructure:"QUEUE_RECEIPT_CONCURRENCY"`
}

// LoadQueueConfig reads queue configuration from environment variables and
// applies environment-specific defaults (development vs production).
func LoadQueueConfig(appEnv string) *QueueConfig {
	// Set defaults based on environment
	setQueueDefaults(appEnv)

	// Read from viper using GetString, GetInt, GetDuration for proper type conversion
	cfg := &QueueConfig{
		URL:                         viper.GetString("RABBITMQ_URL"),
		MaxRetries:                  viper.GetInt("QUEUE_MAX_RETRIES"),
		RetryDelayBase:              viper.GetDuration("QUEUE_RETRY_DELAY_BASE"),
		Concurrency:                 viper.GetInt("QUEUE_CONSUMER_CONCURRENCY"),
		PrefetchCount:               viper.GetInt("QUEUE_PREFETCH_COUNT"),
		MaxLength:                   viper.GetInt("QUEUE_MAX_LENGTH"),
		MessageTTL:                  viper.GetInt("QUEUE_MESSAGE_TTL_HOURS"),
		DLQMessageTTL:               viper.GetInt("QUEUE_DLQ_MESSAGE_TTL_DAYS"),
		ReconciliationInterval:      viper.GetDuration("QUEUE_RECONCILIATION_INTERVAL"),
		ReconciliationPendingAge:    viper.GetDuration("QUEUE_RECONCILIATION_PENDING_AGE"),
		ReconciliationProcessingAge: viper.GetDuration("QUEUE_RECONCILIATION_PROCESSING_AGE"),
		ReconciliationBatchSize:     viper.GetInt("QUEUE_RECONCILIATION_BATCH_SIZE"),
		CircuitBreakerThreshold:     viper.GetInt("QUEUE_CIRCUIT_BREAKER_THRESHOLD"),
		CircuitBreakerTimeout:       viper.GetDuration("QUEUE_CIRCUIT_BREAKER_TIMEOUT"),
		DepositConcurrency:          viper.GetInt("QUEUE_DEPOSIT_CONCURRENCY"),
		WithdrawalConcurrency:       viper.GetInt("QUEUE_WITHDRAWAL_CONCURRENCY"),
		NotificationConcurrency:     viper.GetInt("QUEUE_NOTIFICATION_CONCURRENCY"),
		ReceiptConcurrency:          viper.GetInt("QUEUE_RECEIPT_CONCURRENCY"),
	}

	return cfg
}

// setQueueDefaults registers fallback values based on the environment.
// Development uses conservative settings (low concurrency, small queues)
// for safe local testing. Production uses higher throughput settings.
func setQueueDefaults(appEnv string) {
	isDev := appEnv != "production"

	// Development defaults (conservative)
	if isDev {
		viper.SetDefault("QUEUE_MAX_RETRIES", 1)
		viper.SetDefault("QUEUE_CONSUMER_CONCURRENCY", 2)
		viper.SetDefault("QUEUE_MAX_LENGTH", 100)
		viper.SetDefault("QUEUE_PREFETCH_COUNT", 5)
	} else {
		// Production defaults (high throughput)
		viper.SetDefault("QUEUE_MAX_RETRIES", 3)
		viper.SetDefault("QUEUE_CONSUMER_CONCURRENCY", 10)
		viper.SetDefault("QUEUE_MAX_LENGTH", 10000)
		viper.SetDefault("QUEUE_PREFETCH_COUNT", 10)
	}

	// Common defaults (same for all environments)
	viper.SetDefault("QUEUE_RETRY_DELAY_BASE", "5s")
	viper.SetDefault("QUEUE_MESSAGE_TTL_HOURS", 48)
	viper.SetDefault("QUEUE_DLQ_MESSAGE_TTL_DAYS", 7)

	// Reconciliation defaults
	viper.SetDefault("QUEUE_RECONCILIATION_INTERVAL", "5m")
	viper.SetDefault("QUEUE_RECONCILIATION_PENDING_AGE", "10m")
	viper.SetDefault("QUEUE_RECONCILIATION_PROCESSING_AGE", "30m")
	viper.SetDefault("QUEUE_RECONCILIATION_BATCH_SIZE", 100)

	// Circuit breaker defaults
	viper.SetDefault("QUEUE_CIRCUIT_BREAKER_THRESHOLD", 10)
	viper.SetDefault("QUEUE_CIRCUIT_BREAKER_TIMEOUT", "60s")

	// Per-queue concurrency overrides (optional, defaults to QUEUE_CONSUMER_CONCURRENCY)
	// Leave unset by default; consumer will use main concurrency setting if these are 0
	viper.SetDefault("QUEUE_DEPOSIT_CONCURRENCY", 0)
	viper.SetDefault("QUEUE_WITHDRAWAL_CONCURRENCY", 0)
	viper.SetDefault("QUEUE_NOTIFICATION_CONCURRENCY", 0)
	viper.SetDefault("QUEUE_RECEIPT_CONCURRENCY", 0)
}

// Validate checks that queue configuration is valid and returns an error
// if any values are out of acceptable ranges. This enables fail-fast startup.
func (c *QueueConfig) Validate() error {
	// Retry configuration
	if c.MaxRetries < 0 || c.MaxRetries > 10 {
		return fmt.Errorf("QUEUE_MAX_RETRIES must be between 0 and 10 (got %d)", c.MaxRetries)
	}
	if c.RetryDelayBase < 1*time.Second || c.RetryDelayBase > 60*time.Second {
		return fmt.Errorf("QUEUE_RETRY_DELAY_BASE must be between 1s and 60s (got %v)", c.RetryDelayBase)
	}

	// Consumer configuration
	if c.Concurrency < 1 || c.Concurrency > 100 {
		return fmt.Errorf("QUEUE_CONSUMER_CONCURRENCY must be between 1 and 100 (got %d)", c.Concurrency)
	}
	if c.PrefetchCount < 1 || c.PrefetchCount > 1000 {
		return fmt.Errorf("QUEUE_PREFETCH_COUNT must be between 1 and 1000 (got %d)", c.PrefetchCount)
	}
	if c.MaxLength < 10 || c.MaxLength > 1000000 {
		return fmt.Errorf("QUEUE_MAX_LENGTH must be between 10 and 1,000,000 (got %d)", c.MaxLength)
	}
	if c.MessageTTL < 1 || c.MessageTTL > 168 {
		return fmt.Errorf("QUEUE_MESSAGE_TTL_HOURS must be between 1 and 168 hours (got %d)", c.MessageTTL)
	}
	if c.DLQMessageTTL < 1 || c.DLQMessageTTL > 30 {
		return fmt.Errorf("QUEUE_DLQ_MESSAGE_TTL_DAYS must be between 1 and 30 days (got %d)", c.DLQMessageTTL)
	}

	// Reconciliation configuration
	if c.ReconciliationInterval < 1*time.Minute {
		return fmt.Errorf("QUEUE_RECONCILIATION_INTERVAL must be at least 1 minute (got %v)", c.ReconciliationInterval)
	}
	if c.ReconciliationPendingAge < 1*time.Minute {
		return fmt.Errorf("QUEUE_RECONCILIATION_PENDING_AGE must be at least 1 minute (got %v)", c.ReconciliationPendingAge)
	}
	if c.ReconciliationProcessingAge < 1*time.Minute {
		return fmt.Errorf("QUEUE_RECONCILIATION_PROCESSING_AGE must be at least 1 minute (got %v)", c.ReconciliationProcessingAge)
	}
	if c.ReconciliationBatchSize < 1 || c.ReconciliationBatchSize > 1000 {
		return fmt.Errorf("QUEUE_RECONCILIATION_BATCH_SIZE must be between 1 and 1000 (got %d)", c.ReconciliationBatchSize)
	}

	// Circuit breaker configuration
	if c.CircuitBreakerThreshold < 1 || c.CircuitBreakerThreshold > 100 {
		return fmt.Errorf("QUEUE_CIRCUIT_BREAKER_THRESHOLD must be between 1 and 100 (got %d)", c.CircuitBreakerThreshold)
	}
	if c.CircuitBreakerTimeout < 10*time.Second || c.CircuitBreakerTimeout > 600*time.Second {
		return fmt.Errorf("QUEUE_CIRCUIT_BREAKER_TIMEOUT must be between 10s and 600s (got %v)", c.CircuitBreakerTimeout)
	}

	// Per-queue concurrency overrides (optional, 0 means use default)
	if c.DepositConcurrency < 0 || c.DepositConcurrency > 100 {
		return fmt.Errorf("QUEUE_DEPOSIT_CONCURRENCY must be between 0 and 100 (got %d)", c.DepositConcurrency)
	}
	if c.WithdrawalConcurrency < 0 || c.WithdrawalConcurrency > 100 {
		return fmt.Errorf("QUEUE_WITHDRAWAL_CONCURRENCY must be between 0 and 100 (got %d)", c.WithdrawalConcurrency)
	}
	if c.NotificationConcurrency < 0 || c.NotificationConcurrency > 100 {
		return fmt.Errorf("QUEUE_NOTIFICATION_CONCURRENCY must be between 0 and 100 (got %d)", c.NotificationConcurrency)
	}
	if c.ReceiptConcurrency < 0 || c.ReceiptConcurrency > 100 {
		return fmt.Errorf("QUEUE_RECEIPT_CONCURRENCY must be between 0 and 100 (got %d)", c.ReceiptConcurrency)
	}

	return nil
}

// IsEnabled returns true if RabbitMQ is configured and should be used.
// If URL is empty, the queue system is disabled and webhook handlers
// will fall back to event store only (reconciliation will handle processing).
func (c *QueueConfig) IsEnabled() bool {
	return c.URL != ""
}

// GetConcurrency returns the concurrency setting for a specific queue type.
// If a queue-specific override is set (non-zero), it returns that; otherwise
// it returns the default concurrency setting.
func (c *QueueConfig) GetConcurrency(queueType string) int {
	switch queueType {
	case "deposit":
		if c.DepositConcurrency > 0 {
			return c.DepositConcurrency
		}
	case "withdrawal":
		if c.WithdrawalConcurrency > 0 {
			return c.WithdrawalConcurrency
		}
	case "notification":
		if c.NotificationConcurrency > 0 {
			return c.NotificationConcurrency
		}
	case "receipt":
		if c.ReceiptConcurrency > 0 {
			return c.ReceiptConcurrency
		}
	}
	return c.Concurrency
}

// GetMessageTTL returns the message TTL in milliseconds for RabbitMQ configuration.
func (c *QueueConfig) GetMessageTTL() int64 {
	return int64(c.MessageTTL) * 60 * 60 * 1000 // hours to milliseconds
}

// GetDLQMessageTTL returns the DLQ message TTL in milliseconds for RabbitMQ configuration.
func (c *QueueConfig) GetDLQMessageTTL() int64 {
	return int64(c.DLQMessageTTL) * 24 * 60 * 60 * 1000 // days to milliseconds
}

// GetRetryDelay calculates the delay for a given retry attempt using exponential backoff.
// Returns the delay duration for the specified retry number (1, 2, 3, etc.).
func (c *QueueConfig) GetRetryDelay(retryCount int) time.Duration {
	if retryCount <= 0 {
		return c.RetryDelayBase
	}

	// Exponential backoff: base * 3^(retryCount-1)
	// retry 1: base * 3^0 = base (e.g., 5s)
	// retry 2: base * 3^1 = base * 3 (e.g., 15s)
	// retry 3: base * 3^2 = base * 9 (e.g., 45s)
	multiplier := 1
	for i := 1; i < retryCount; i++ {
		multiplier *= 3
	}

	delay := c.RetryDelayBase * time.Duration(multiplier)

	// Cap at 5 minutes to prevent excessive delays
	maxDelay := 5 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}
