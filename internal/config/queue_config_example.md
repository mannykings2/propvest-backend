# Queue Configuration Usage Example

This document demonstrates how to use the `QueueConfig` in your application.

## Loading Configuration

```go
package main

import (
    "log"
    "github.com/mannykings2/propvest-backend/internal/config"
)

func main() {
    // Load main application config first
    cfg := config.Load()
    
    // Validate main config
    if err := cfg.Validate(); err != nil {
        log.Fatalf("Invalid configuration: %v", err)
    }
    
    // Load queue-specific configuration
    queueCfg := config.LoadQueueConfig(cfg.AppEnv)
    
    // Validate queue configuration
    if err := queueCfg.Validate(); err != nil {
        log.Fatalf("Invalid queue configuration: %v", err)
    }
    
    // Check if queue system is enabled
    if !queueCfg.IsEnabled() {
        log.Println("RabbitMQ not configured; running without queue system")
        // Application continues, webhook handlers will use event store only
    } else {
        log.Printf("RabbitMQ enabled at: %s", queueCfg.URL)
        // Initialize RabbitMQ client with queueCfg
    }
}
```

## Environment-Specific Defaults

### Development Environment
```bash
APP_ENV=development
# These defaults are automatically applied:
# QUEUE_MAX_RETRIES=1
# QUEUE_CONSUMER_CONCURRENCY=2
# QUEUE_MAX_LENGTH=100
# QUEUE_PREFETCH_COUNT=5
```

### Production Environment
```bash
APP_ENV=production
# These defaults are automatically applied:
# QUEUE_MAX_RETRIES=3
# QUEUE_CONSUMER_CONCURRENCY=10
# QUEUE_MAX_LENGTH=10000
# QUEUE_PREFETCH_COUNT=10
```

## Using Configuration Values

```go
// Get retry delay for exponential backoff
delay := queueCfg.GetRetryDelay(retryCount)
time.Sleep(delay)

// Get queue-specific concurrency
depositConcurrency := queueCfg.GetConcurrency("deposit")
withdrawalConcurrency := queueCfg.GetConcurrency("withdrawal")

// Get message TTL in milliseconds for RabbitMQ
messageTTL := queueCfg.GetMessageTTL()
dlqTTL := queueCfg.GetDLQMessageTTL()
```

## Configuration Parameters

| Parameter | Development Default | Production Default | Description |
|-----------|--------------------|--------------------|-------------|
| `QUEUE_MAX_RETRIES` | 1 | 3 | Maximum retry attempts before DLQ |
| `QUEUE_CONSUMER_CONCURRENCY` | 2 | 10 | Concurrent handlers per consumer |
| `QUEUE_MAX_LENGTH` | 100 | 10000 | Maximum queue depth |
| `QUEUE_PREFETCH_COUNT` | 5 | 10 | Messages to fetch at once |
| `QUEUE_RETRY_DELAY_BASE` | 5s | 5s | Base delay for exponential backoff |
| `QUEUE_MESSAGE_TTL_HOURS` | 48 | 48 | Message TTL in hours |
| `QUEUE_DLQ_MESSAGE_TTL_DAYS` | 7 | 7 | DLQ message TTL in days |
| `QUEUE_RECONCILIATION_INTERVAL` | 5m | 5m | How often reconciliation runs |
| `QUEUE_RECONCILIATION_PENDING_AGE` | 10m | 10m | Age threshold for pending events |
| `QUEUE_RECONCILIATION_PROCESSING_AGE` | 30m | 30m | Age threshold for stuck events |
| `QUEUE_RECONCILIATION_BATCH_SIZE` | 100 | 100 | Batch size for reconciliation |
| `QUEUE_CIRCUIT_BREAKER_THRESHOLD` | 10 | 10 | Failures before opening circuit |
| `QUEUE_CIRCUIT_BREAKER_TIMEOUT` | 60s | 60s | Time before half-open attempt |

## Per-Queue Concurrency Overrides

You can override concurrency for specific queues:

```bash
# Override deposit queue concurrency (otherwise uses QUEUE_CONSUMER_CONCURRENCY)
QUEUE_DEPOSIT_CONCURRENCY=15

# Override withdrawal queue concurrency (lower due to bank API rate limits)
QUEUE_WITHDRAWAL_CONCURRENCY=5

# Override notification queue concurrency (higher for bulk notifications)
QUEUE_NOTIFICATION_CONCURRENCY=20
```

## Retry Delay Calculation

The configuration provides exponential backoff with 3x multiplier:

- Retry 1: 5s × 3^0 = 5s
- Retry 2: 5s × 3^1 = 15s
- Retry 3: 5s × 3^2 = 45s
- Maximum cap: 5 minutes

Example:
```go
for retryCount := 1; retryCount <= queueCfg.MaxRetries; retryCount++ {
    err := processMessage(msg)
    if err == nil {
        break
    }
    
    if !isTransientError(err) {
        // Permanent error, route to DLQ
        return err
    }
    
    // Transient error, wait and retry
    delay := queueCfg.GetRetryDelay(retryCount)
    time.Sleep(delay)
}
```

## Validation

The configuration validates all parameters on load with sensible ranges:

```go
queueCfg := config.LoadQueueConfig("production")

// This will return an error if any parameter is out of range
if err := queueCfg.Validate(); err != nil {
    log.Fatalf("Invalid configuration: %v", err)
}

// Validation checks:
// - MaxRetries: 0-10
// - Concurrency: 1-100
// - PrefetchCount: 1-1000
// - MaxLength: 10-1,000,000
// - MessageTTL: 1-168 hours
// - DLQMessageTTL: 1-30 days
// - ReconciliationInterval: >= 1 minute
// - CircuitBreakerThreshold: 1-100
// - CircuitBreakerTimeout: 10s-600s
```

## Graceful Degradation

When `RABBITMQ_URL` is not set, the queue system is disabled:

```go
if !queueCfg.IsEnabled() {
    // Webhook handlers still work, writing only to event_store
    // Reconciliation worker will process events when RabbitMQ is restored
    log.Println("Running without queue system; using event store only")
}
```

This enables:
- Development without RabbitMQ running
- Graceful operation during RabbitMQ outages
- Event store acts as durable buffer until queue is available
