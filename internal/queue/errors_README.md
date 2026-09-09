# Queue Error Classification

This package provides error types and classification helpers for the RabbitMQ queue system. Errors are classified as either **transient** (retry) or **permanent** (DLQ), enabling intelligent retry behavior in queue consumers.

## Error Types

### Transient Errors (Retry with Exponential Backoff)

Transient errors represent temporary failures that may succeed on retry. Consumers should **NACK with requeue=true** and apply exponential backoff.

| Error | Description | Common Causes | Retry Strategy |
|-------|-------------|---------------|----------------|
| `ErrDatabaseConnection` | Database connection unavailable | Network partition, connection pool exhausted, DB restart | 5s, 15s, 45s |
| `ErrDatabaseTimeout` | Database query timeout | Slow query, DB under load, lock contention | 5s, 15s, 45s |
| `ErrProviderTimeout` | Payment provider API timeout | Network latency, provider under load | 5s, 15s, 45s |
| `ErrProviderRateLimit` | Payment provider rate limit | Too many requests in time window | 5s, 15s, 45s |

### Permanent Errors (Route to DLQ Immediately)

Permanent errors represent failures that will **never** succeed on retry. Consumers should **ACK original message** and **publish to DLQ**.

| Error | Description | Common Causes | Resolution |
|-------|-------------|---------------|------------|
| `ErrInvalidEvent` | Event failed schema validation | Malformed JSON, missing fields, invalid types | Fix producer or update schema |
| `ErrInvalidAmount` | Amount validation failure | Negative amount, zero amount, exceeds limits | Investigate producer logic |
| `ErrUserNotFound` | User does not exist | User deleted after event published, invalid user_id | Check user deletion flow |
| `ErrInsufficientBalance` | Wallet has insufficient funds | Concurrent withdrawals, balance race condition | Implement SELECT FOR UPDATE |

## Helper Functions

### `IsTransient(err error) bool`

Returns `true` if the error should trigger a retry with exponential backoff.

**Usage:**
```go
if err != nil {
    if queue.IsTransient(err) {
        // NACK with requeue=true
        // Apply backoff: 5s, 15s, 45s
        return err
    }
}
```

**Behavior:**
- Returns `true` for queue-specific transient errors (e.g., `ErrDatabaseConnection`)
- Returns `true` for app-level infrastructure errors (e.g., `apperrors.ErrDatabase`)
- Returns `false` for `nil` errors
- Works with wrapped errors via `errors.Is()`

### `IsPermanent(err error) bool`

Returns `true` if the error should route to DLQ immediately without retry.

**Usage:**
```go
if err != nil {
    if queue.IsPermanent(err) {
        // ACK original message
        // Publish to DLQ with metadata
        routeToDLQ(msg, err)
        return nil
    }
}
```

**Behavior:**
- Returns `true` for queue-specific permanent errors (e.g., `ErrInvalidEvent`)
- Returns `true` for app-level business errors (e.g., `apperrors.ErrUserNotFound`)
- Returns `false` for `nil` errors
- Works with wrapped errors via `errors.Is()`

### `WrapTransient(err error, context string) error`

Wraps a transient error with additional context while preserving classification.

**Usage:**
```go
err := db.QueryRow(...)
if err != nil {
    return queue.WrapTransient(queue.ErrDatabaseConnection, "wallet credit failed")
}
```

### `WrapPermanent(err error, context string) error`

Wraps a permanent error with additional context while preserving classification.

**Usage:**
```go
user, err := repo.GetByID(userID)
if err != nil {
    return queue.WrapPermanent(queue.ErrUserNotFound, "deposit processing failed")
}
```

## Consumer Decision Flow

```
┌─────────────────────┐
│ Receive Message     │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ Process Message     │
└──────────┬──────────┘
           │
           ▼
      ┌────────┐
      │ Error? │──────No────▶ ACK message (success)
      └────┬───┘
           │Yes
           ▼
    ┌──────────────┐
    │ IsTransient? │──────Yes────▶ NACK with requeue=true
    └──────┬───────┘               Apply backoff: 5s, 15s, 45s
           │No                     If retry_count >= 3, route to DLQ
           ▼
    ┌──────────────┐
    │ IsPermanent? │──────Yes────▶ ACK message
    └──────┬───────┘               Publish to DLQ with metadata
           │No                     Update event_store: status=failed
           │
           ▼
    Unknown Error
    (Conservative: treat as transient)
```

## Integration with Application Errors

This package integrates with `internal/errors` package:

**Transient Mapping:**
- `ErrDatabase` → transient (retry)
- `ErrCache` → transient (retry)
- `ErrExternal` → transient (retry)
- `ErrPaymentProviderUnavailable` → transient (retry)

**Permanent Mapping:**
- `ErrUserNotFound` → permanent (DLQ)
- `ErrInsufficientFunds` → permanent (DLQ)
- `ErrInvalidAmount` → permanent (DLQ)
- `ErrWalletNotFound` → permanent (DLQ)
- `ErrValidation` → permanent (DLQ)
- `ErrInvalidJSON` → permanent (DLQ)
- `ErrInvalidUUID` → permanent (DLQ)

## Example: Consumer Implementation

```go
func (c *DepositConsumer) handleMessage(ctx context.Context, body []byte) error {
    // Parse event
    event, err := parseEvent(body)
    if err != nil {
        // Schema validation failed - permanent error
        if queue.IsPermanent(err) {
            c.routeToDLQ(body, err)
            return nil // ACK original
        }
    }

    // Check idempotency
    if c.eventStore.IsCompleted(event.ID) {
        return nil // Already processed
    }

    // Process in transaction
    tx := c.db.Begin()
    defer tx.Rollback()

    // Credit wallet
    err = c.walletRepo.Credit(tx, event.UserID, event.AmountKobo)
    if err != nil {
        // Classify error
        if queue.IsTransient(err) {
            // Database connection lost - retry
            return err // NACK with requeue
        }
        
        if queue.IsPermanent(err) {
            // User not found - permanent failure
            c.routeToDLQ(body, err)
            return nil // ACK original
        }
        
        // Unknown error - conservative retry
        return err
    }

    // Update event store
    c.eventStore.MarkCompleted(event.ID)
    
    tx.Commit()
    return nil // ACK message
}
```

## Testing

Run all error classification tests:

```bash
go test -v ./internal/queue
```

Run specific test suites:

```bash
# Transient error tests
go test -v ./internal/queue -run TestIsTransient

# Permanent error tests
go test -v ./internal/queue -run TestIsPermanent

# Error wrapping tests
go test -v ./internal/queue -run TestWrap

# Example tests
go test -v ./internal/queue -run Example
```

## Requirements Satisfied

This implementation satisfies the following requirements from the RabbitMQ Queue System spec:

- **Requirement 4.2**: Transient errors retry with exponential backoff (5s, 15s, 45s)
- **Requirement 4.3**: Permanent errors route to DLQ immediately without retry
- **Requirement 11.4**: Schema validation failures are permanent errors
- **Requirement 11.5**: Business rule violations are permanent errors

## Design Principles

1. **Conservative Classification**: Unknown errors default to neither transient nor permanent, allowing consumers to decide (typically treated as transient for safety)

2. **Error Wrapping Preservation**: `errors.Is()` works through multiple layers of wrapping, preserving classification

3. **Mutual Exclusivity**: No error can be both transient and permanent (enforced by tests)

4. **Integration with App Errors**: Reuses existing error types from `internal/errors` package, avoiding duplication

5. **Explicit over Implicit**: Error classification is explicit (not inferred from error string matching)

## Future Extensibility

To add new error types:

1. **Transient Error:**
   ```go
   var ErrNewTransient = errors.New("description")
   ```
   Add to `IsTransient()` function checks

2. **Permanent Error:**
   ```go
   var ErrNewPermanent = errors.New("description")
   ```
   Add to `IsPermanent()` function checks

3. **Add Tests:**
   - Add to `TestIsTransient_QueueErrors` or `TestIsPermanent_QueueErrors`
   - Add to `TestErrorClassification_MutuallyExclusive`

4. **Update Documentation:**
   - Add to error tables in this README
   - Update consumer decision flow if needed
