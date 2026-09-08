package queue

import (
	"testing"
)

// TestQueueConfigConstants verifies all queue configuration constants are properly defined
// Requirements: 12.1, 12.2, 12.3, 12.4, 12.6, 14.3
func TestQueueConfigConstants(t *testing.T) {
	tests := []struct {
		name   string
		config QueueConfig
		want   struct {
			durable       bool
			maxLength     int
			ttl           int64
			priority      uint8
			prefetchCount int
			concurrency   int
		}
	}{
		{
			name:   "DepositQueueConfig",
			config: DepositQueueConfig,
			want: struct {
				durable       bool
				maxLength     int
				ttl           int64
				priority      uint8
				prefetchCount int
				concurrency   int
			}{
				durable:       true,
				maxLength:     10000,
				ttl:           48 * 60 * 60 * 1000,
				priority:      5,
				prefetchCount: 10,
				concurrency:   10,
			},
		},
		{
			name:   "WithdrawalQueueConfig",
			config: WithdrawalQueueConfig,
			want: struct {
				durable       bool
				maxLength     int
				ttl           int64
				priority      uint8
				prefetchCount int
				concurrency   int
			}{
				durable:       true,
				maxLength:     10000,
				ttl:           48 * 60 * 60 * 1000,
				priority:      10, // Highest priority
				prefetchCount: 5,
				concurrency:   5,
			},
		},
		{
			name:   "NotificationQueueConfig",
			config: NotificationQueueConfig,
			want: struct {
				durable       bool
				maxLength     int
				ttl           int64
				priority      uint8
				prefetchCount int
				concurrency   int
			}{
				durable:       true,
				maxLength:     10000,
				ttl:           48 * 60 * 60 * 1000,
				priority:      1, // Lowest priority
				prefetchCount: 20,
				concurrency:   20,
			},
		},
		{
			name:   "ReceiptQueueConfig",
			config: ReceiptQueueConfig,
			want: struct {
				durable       bool
				maxLength     int
				ttl           int64
				priority      uint8
				prefetchCount int
				concurrency   int
			}{
				durable:       true,
				maxLength:     10000,
				ttl:           48 * 60 * 60 * 1000,
				priority:      1,
				prefetchCount: 10,
				concurrency:   10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.Durable != tt.want.durable {
				t.Errorf("Durable = %v, want %v", tt.config.Durable, tt.want.durable)
			}
			if tt.config.MaxLength != tt.want.maxLength {
				t.Errorf("MaxLength = %v, want %v", tt.config.MaxLength, tt.want.maxLength)
			}
			if tt.config.TTL != tt.want.ttl {
				t.Errorf("TTL = %v, want %v", tt.config.TTL, tt.want.ttl)
			}
			if tt.config.Priority != tt.want.priority {
				t.Errorf("Priority = %v, want %v", tt.config.Priority, tt.want.priority)
			}
			if tt.config.PrefetchCount != tt.want.prefetchCount {
				t.Errorf("PrefetchCount = %v, want %v", tt.config.PrefetchCount, tt.want.prefetchCount)
			}
			if tt.config.Concurrency != tt.want.concurrency {
				t.Errorf("Concurrency = %v, want %v", tt.config.Concurrency, tt.want.concurrency)
			}
			if tt.config.AutoDelete != false {
				t.Errorf("AutoDelete = %v, want false", tt.config.AutoDelete)
			}
			if tt.config.Exclusive != false {
				t.Errorf("Exclusive = %v, want false", tt.config.Exclusive)
			}
			if tt.config.DLXName != "propvest.dlx" {
				t.Errorf("DLXName = %v, want 'propvest.dlx'", tt.config.DLXName)
			}
		})
	}
}

// TestGetDevelopmentConfig verifies development environment defaults
// Requirement 14.2: Development defaults (max_retries=1, concurrency=2, max_length=100)
func TestGetDevelopmentConfig(t *testing.T) {
	devConfig := GetDevelopmentConfig(DepositQueueConfig)

	if devConfig.MaxLength != 100 {
		t.Errorf("Dev MaxLength = %v, want 100", devConfig.MaxLength)
	}
	if devConfig.Concurrency != 2 {
		t.Errorf("Dev Concurrency = %v, want 2", devConfig.Concurrency)
	}
	// Name should remain the same
	if devConfig.Name != DepositQueueConfig.Name {
		t.Errorf("Dev Name changed unexpectedly")
	}
	// Other properties should be preserved
	if devConfig.Durable != DepositQueueConfig.Durable {
		t.Errorf("Dev Durable changed unexpectedly")
	}
}

// TestGetProductionConfig verifies production environment defaults
// Requirement 14.3: Production defaults (max_retries=3, concurrency=10, max_length=10000)
func TestGetProductionConfig(t *testing.T) {
	prodConfig := GetProductionConfig(DepositQueueConfig)

	if prodConfig.MaxLength != 10000 {
		t.Errorf("Prod MaxLength = %v, want 10000", prodConfig.MaxLength)
	}
	// Concurrency should remain as defined in base config
	if prodConfig.Concurrency != DepositQueueConfig.Concurrency {
		t.Errorf("Prod Concurrency = %v, want %v", prodConfig.Concurrency, DepositQueueConfig.Concurrency)
	}
}

// TestAllQueueConfigs verifies all queue configurations are returned
func TestAllQueueConfigs(t *testing.T) {
	configs := AllQueueConfigs()

	if len(configs) != 4 {
		t.Errorf("AllQueueConfigs() returned %d configs, want 4", len(configs))
	}

	// Verify expected queue names are present
	expectedNames := map[string]bool{
		"propvest.deposit.confirmed":     false,
		"propvest.withdrawal.requested":  false,
		"propvest.notification.dispatch": false,
		"propvest.receipt.generate":      false,
	}

	for _, cfg := range configs {
		if _, exists := expectedNames[cfg.Name]; exists {
			expectedNames[cfg.Name] = true
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("Queue config for %s not found in AllQueueConfigs()", name)
		}
	}
}

// TestGetQueueConfigByName verifies queue lookup by name
func TestGetQueueConfigByName(t *testing.T) {
	tests := []struct {
		name      string
		queueName string
		wantFound bool
	}{
		{
			name:      "DepositQueue",
			queueName: "propvest.deposit.confirmed",
			wantFound: true,
		},
		{
			name:      "WithdrawalQueue",
			queueName: "propvest.withdrawal.requested",
			wantFound: true,
		},
		{
			name:      "NotificationQueue",
			queueName: "propvest.notification.dispatch",
			wantFound: true,
		},
		{
			name:      "ReceiptQueue",
			queueName: "propvest.receipt.generate",
			wantFound: true,
		},
		{
			name:      "NonExistentQueue",
			queueName: "propvest.nonexistent.queue",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, found := GetQueueConfigByName(tt.queueName)
			if found != tt.wantFound {
				t.Errorf("GetQueueConfigByName(%s) found = %v, want %v", tt.queueName, found, tt.wantFound)
			}
			if tt.wantFound && config.Name != tt.queueName {
				t.Errorf("GetQueueConfigByName(%s) returned config with name %s", tt.queueName, config.Name)
			}
		})
	}
}

// TestDLQTTLMilliseconds verifies DLQ TTL constant
// Requirement 3.8: DLQ message expiry to 7 days
func TestDLQTTLMilliseconds(t *testing.T) {
	expectedTTL := int64(7 * 24 * 60 * 60 * 1000) // 7 days in milliseconds

	if DLQTTLMilliseconds != expectedTTL {
		t.Errorf("DLQTTLMilliseconds = %v, want %v", DLQTTLMilliseconds, expectedTTL)
	}
}

// TestMaxRetries verifies max retry constant
// Requirement 4.x: Retry policy with max 3 attempts
func TestMaxRetries(t *testing.T) {
	if MaxRetries != 3 {
		t.Errorf("MaxRetries = %v, want 3", MaxRetries)
	}
}

// TestRetryDelays verifies exponential backoff delays
// Requirement 4.1: Exponential backoff with delays: 5s, 15s, 45s
func TestRetryDelays(t *testing.T) {
	expectedDelays := []int{5, 15, 45}

	if len(RetryDelays) != len(expectedDelays) {
		t.Errorf("RetryDelays length = %v, want %v", len(RetryDelays), len(expectedDelays))
	}

	for i, expected := range expectedDelays {
		if RetryDelays[i] != expected {
			t.Errorf("RetryDelays[%d] = %v, want %v", i, RetryDelays[i], expected)
		}
	}
}

// TestGetRetryDelay verifies retry delay calculation
// Requirement 4.1: Exponential backoff with delays: 5s, 15s, 45s
func TestGetRetryDelay(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		wantDelay  int
	}{
		{
			name:       "NegativeRetryCount",
			retryCount: -1,
			wantDelay:  5, // Should return first delay
		},
		{
			name:       "FirstRetry",
			retryCount: 0,
			wantDelay:  5,
		},
		{
			name:       "SecondRetry",
			retryCount: 1,
			wantDelay:  15,
		},
		{
			name:       "ThirdRetry",
			retryCount: 2,
			wantDelay:  45,
		},
		{
			name:       "ExceedMaxRetries",
			retryCount: 10,
			wantDelay:  45, // Should return last delay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := GetRetryDelay(tt.retryCount)
			if delay != tt.wantDelay {
				t.Errorf("GetRetryDelay(%d) = %v, want %v", tt.retryCount, delay, tt.wantDelay)
			}
		})
	}
}

// TestQueuePriorities verifies priority ordering (withdrawal > deposit > notification/receipt)
// Requirement 12.4: Queue priority configuration
func TestQueuePriorities(t *testing.T) {
	// Withdrawal should have highest priority
	if WithdrawalQueueConfig.Priority <= DepositQueueConfig.Priority {
		t.Error("Withdrawal priority should be higher than deposit priority")
	}

	// Deposit should have higher priority than notification
	if DepositQueueConfig.Priority <= NotificationQueueConfig.Priority {
		t.Error("Deposit priority should be higher than notification priority")
	}

	// Deposit should have higher priority than receipt
	if DepositQueueConfig.Priority <= ReceiptQueueConfig.Priority {
		t.Error("Deposit priority should be higher than receipt priority")
	}
}
