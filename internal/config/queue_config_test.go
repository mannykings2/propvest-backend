package config

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadQueueConfig_DevelopmentDefaults(t *testing.T) {
	// Reset viper state
	viper.Reset()

	cfg := LoadQueueConfig("development")

	// Development defaults
	assert.Equal(t, 1, cfg.MaxRetries, "development should have max_retries=1")
	assert.Equal(t, 2, cfg.Concurrency, "development should have concurrency=2")
	assert.Equal(t, 100, cfg.MaxLength, "development should have max_length=100")
	assert.Equal(t, 5, cfg.PrefetchCount, "development should have prefetch=5")

	// Common defaults
	assert.Equal(t, 5*time.Second, cfg.RetryDelayBase)
	assert.Equal(t, 48, cfg.MessageTTL)
	assert.Equal(t, 7, cfg.DLQMessageTTL)
	assert.Equal(t, 5*time.Minute, cfg.ReconciliationInterval)
	assert.Equal(t, 10*time.Minute, cfg.ReconciliationPendingAge)
	assert.Equal(t, 30*time.Minute, cfg.ReconciliationProcessingAge)
	assert.Equal(t, 100, cfg.ReconciliationBatchSize)
	assert.Equal(t, 10, cfg.CircuitBreakerThreshold)
	assert.Equal(t, 60*time.Second, cfg.CircuitBreakerTimeout)
}

func TestLoadQueueConfig_ProductionDefaults(t *testing.T) {
	// Reset viper state
	viper.Reset()

	cfg := LoadQueueConfig("production")

	// Production defaults
	assert.Equal(t, 3, cfg.MaxRetries, "production should have max_retries=3")
	assert.Equal(t, 10, cfg.Concurrency, "production should have concurrency=10")
	assert.Equal(t, 10000, cfg.MaxLength, "production should have max_length=10000")
	assert.Equal(t, 10, cfg.PrefetchCount, "production should have prefetch=10")
}

func TestLoadQueueConfig_EnvironmentOverrides(t *testing.T) {
	// Reset viper state and set up environment variables
	viper.Reset()
	os.Setenv("RABBITMQ_URL", "amqp://test:test@localhost:5672/test")
	os.Setenv("QUEUE_MAX_RETRIES", "5")
	os.Setenv("QUEUE_CONSUMER_CONCURRENCY", "20")
	defer func() {
		os.Unsetenv("RABBITMQ_URL")
		os.Unsetenv("QUEUE_MAX_RETRIES")
		os.Unsetenv("QUEUE_CONSUMER_CONCURRENCY")
	}()

	viper.AutomaticEnv()
	cfg := LoadQueueConfig("production")

	assert.Equal(t, "amqp://test:test@localhost:5672/test", cfg.URL)
	assert.Equal(t, 5, cfg.MaxRetries)
	assert.Equal(t, 20, cfg.Concurrency)
}

func TestQueueConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &QueueConfig{
		MaxRetries:                  3,
		RetryDelayBase:              5 * time.Second,
		Concurrency:                 10,
		PrefetchCount:               10,
		MaxLength:                   10000,
		MessageTTL:                  48,
		DLQMessageTTL:               7,
		ReconciliationInterval:      5 * time.Minute,
		ReconciliationPendingAge:    10 * time.Minute,
		ReconciliationProcessingAge: 30 * time.Minute,
		ReconciliationBatchSize:     100,
		CircuitBreakerThreshold:     10,
		CircuitBreakerTimeout:       60 * time.Second,
	}

	err := cfg.Validate()
	assert.NoError(t, err, "valid config should pass validation")
}

func TestQueueConfig_Validate_InvalidMaxRetries(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		wantErr    bool
	}{
		{"negative retries", -1, true},
		{"too many retries", 11, true},
		{"zero retries valid", 0, false},
		{"max retries valid", 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validQueueConfig()
			cfg.MaxRetries = tt.maxRetries

			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "QUEUE_MAX_RETRIES")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQueueConfig_Validate_InvalidConcurrency(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		wantErr     bool
	}{
		{"zero concurrency", 0, true},
		{"too high concurrency", 101, true},
		{"min concurrency valid", 1, false},
		{"max concurrency valid", 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validQueueConfig()
			cfg.Concurrency = tt.concurrency

			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "QUEUE_CONSUMER_CONCURRENCY")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQueueConfig_Validate_InvalidRetryDelayBase(t *testing.T) {
	tests := []struct {
		name  string
		delay time.Duration
		want  bool
	}{
		{"too short", 500 * time.Millisecond, true},
		{"too long", 61 * time.Second, true},
		{"min valid", 1 * time.Second, false},
		{"max valid", 60 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validQueueConfig()
			cfg.RetryDelayBase = tt.delay

			err := cfg.Validate()
			if tt.want {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "QUEUE_RETRY_DELAY_BASE")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQueueConfig_Validate_InvalidReconciliationInterval(t *testing.T) {
	cfg := validQueueConfig()
	cfg.ReconciliationInterval = 30 * time.Second

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "QUEUE_RECONCILIATION_INTERVAL")
}

func TestQueueConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"enabled with URL", "amqp://localhost:5672", true},
		{"disabled without URL", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &QueueConfig{URL: tt.url}
			assert.Equal(t, tt.want, cfg.IsEnabled())
		})
	}
}

func TestQueueConfig_GetConcurrency(t *testing.T) {
	cfg := &QueueConfig{
		Concurrency:            10,
		DepositConcurrency:     15,
		WithdrawalConcurrency:  5,
		NotificationConcurrency: 0, // use default
	}

	tests := []struct {
		queueType string
		want      int
	}{
		{"deposit", 15},
		{"withdrawal", 5},
		{"notification", 10}, // uses default
		{"receipt", 10},      // uses default
		{"unknown", 10},      // uses default
	}

	for _, tt := range tests {
		t.Run(tt.queueType, func(t *testing.T) {
			got := cfg.GetConcurrency(tt.queueType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQueueConfig_GetMessageTTL(t *testing.T) {
	cfg := &QueueConfig{MessageTTL: 48} // 48 hours

	ttl := cfg.GetMessageTTL()
	expected := int64(48 * 60 * 60 * 1000) // 48 hours in milliseconds
	assert.Equal(t, expected, ttl)
}

func TestQueueConfig_GetDLQMessageTTL(t *testing.T) {
	cfg := &QueueConfig{DLQMessageTTL: 7} // 7 days

	ttl := cfg.GetDLQMessageTTL()
	expected := int64(7 * 24 * 60 * 60 * 1000) // 7 days in milliseconds
	assert.Equal(t, expected, ttl)
}

func TestQueueConfig_GetRetryDelay(t *testing.T) {
	cfg := &QueueConfig{RetryDelayBase: 5 * time.Second}

	tests := []struct {
		retryCount int
		want       time.Duration
	}{
		{0, 5 * time.Second},   // base delay
		{1, 5 * time.Second},   // 5s * 3^0 = 5s
		{2, 15 * time.Second},  // 5s * 3^1 = 15s
		{3, 45 * time.Second},  // 5s * 3^2 = 45s
		{4, 135 * time.Second}, // 5s * 3^3 = 135s
		{5, 5 * time.Minute},   // capped at 5 minutes
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.retryCount)), func(t *testing.T) {
			got := cfg.GetRetryDelay(tt.retryCount)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestQueueConfig_GetRetryDelay_MaxCap(t *testing.T) {
	cfg := &QueueConfig{RetryDelayBase: 60 * time.Second}

	// Even with high base delay and retry count, should cap at 5 minutes
	delay := cfg.GetRetryDelay(10)
	assert.Equal(t, 5*time.Minute, delay)
}

// Helper function to create a valid QueueConfig for testing
func validQueueConfig() *QueueConfig {
	return &QueueConfig{
		MaxRetries:                  3,
		RetryDelayBase:              5 * time.Second,
		Concurrency:                 10,
		PrefetchCount:               10,
		MaxLength:                   10000,
		MessageTTL:                  48,
		DLQMessageTTL:               7,
		ReconciliationInterval:      5 * time.Minute,
		ReconciliationPendingAge:    10 * time.Minute,
		ReconciliationProcessingAge: 30 * time.Minute,
		ReconciliationBatchSize:     100,
		CircuitBreakerThreshold:     10,
		CircuitBreakerTimeout:       60 * time.Second,
		DepositConcurrency:          0,
		WithdrawalConcurrency:       0,
		NotificationConcurrency:     0,
		ReceiptConcurrency:          0,
	}
}
