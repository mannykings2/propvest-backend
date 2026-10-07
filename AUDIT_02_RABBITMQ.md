# Audit Report 02: RabbitMQ Implementation

**Date:** 2026-10-06  
**Phase:** 1 - Audit & Analysis  
**Step:** 1.2 - RabbitMQ Implementation Audit

---

## Executive Summary

PropVest's RabbitMQ implementation is **functional but not production-ready**. The current system:

✅ **Has basic reliability** - Durable queues, persistent messages  
✅ **Has graceful degradation** - Works without RabbitMQ in disabled mode  
✅ **Has structured messages** - Type-safe message contracts  
⚠️ **Lacks reconnection** - Connection failures are permanent  
⚠️ **Lacks consumer recovery** - Lost consumers never restart  
⚠️ **Has infinite requeue** - Failed messages retry forever  
❌ **No DLQ implementation** - Poison messages block the queue  
❌ **No connection monitoring** - Failures detected too late  

**Risk Level:** 🔴 **HIGH** - Current implementation can **lose messages** and **fail silently**.

---

## Current Implementation

### File Structure

```
internal/queue/
├── queue.go           # Main client implementation
├── messages.go        # Message type definitions
├── config.go          # Queue configurations (advanced, not fully used)
├── errors.go          # Error types and handling
├── errors_test.go     # Error handling tests
└── config_test.go     # Configuration tests
```

---

## Queue Client (`internal/queue/queue.go`)

### Connection Architecture

```go
type Client struct {
    url      string
    mu       sync.RWMutex
    conn     *amqp.Connection
    channel  *amqp.Channel
    enabled  bool    // false => disabled/no-op mode
    closed   bool
}
```

**Current Behavior:**

1. **Startup:**
   ```go
   New(url) -> attempt connect -> success: enabled=true
                               -> failure: enabled=false, NO RETRY
   ```

2. **Runtime:**
   - No connection monitoring
   - No reconnection logic
   - No heartbeat handling
   - Connection loss = permanent disabled state

3. **Shutdown:**
   ```go
   Close() -> close channel -> close connection
   ```

---

### Queue Names (Constants)

```go
const (
    QueueEmailDispatch     = "propvest.email.dispatch"
    QueueSMSDispatch       = "propvest.sms.dispatch"
    QueueWithdrawalProcess = "propvest.withdrawal.process"
    QueueRealtimePush      = "propvest.realtime.push"
    QueueDepositReceipt    = "propvest.deposit.receipt"
)
```

**Analysis:**
- ✅ Constants prevent typos
- ✅ Clear naming convention
- ⚠️ Hardcoded in multiple places (no single source of truth)
- ⚠️ No DLQ queues defined

---

### Queue Declaration

**Current Code:**
```go
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

    // Declare every queue as durable
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
    
    return nil
}
```

**Analysis:**
- ✅ **Durable queues** - Survive broker restarts
- ✅ **Non-exclusive** - Can be consumed by multiple processes
- ✅ **Not auto-delete** - Queue persists when consumers disconnect
- ❌ **No DLX (dead-letter exchange)** configuration
- ❌ **No queue arguments** (TTL, max-length, priority)
- ❌ **No prefetch limit** - Could overwhelm workers
- ⚠️ **Single failure stops all** - One queue declaration failure aborts entire connection

---

### Publish Method

**Current Implementation:**
```go
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
        return nil  // ❌ MESSAGE LOST
    }

    pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    return ch.PublishWithContext(pctx,
        "",        // default exchange: route by queue name
        queueName, // routing key == queue name
        false,     // mandatory: false (don't return unroutable)
        false,     // immediate: false (deprecated)
        amqp.Publishing{
            ContentType:  "application/json",
            Body:         body,
            DeliveryMode: amqp.Persistent, // ✅ Survive broker restart
            Timestamp:    time.Now(),
        },
    )
}
```

**Analysis:**

✅ **Good:**
- Persistent messages (survive broker restarts)
- Context timeout prevents indefinite hangs
- JSON marshaling with error handling
- Thread-safe with RWMutex

❌ **Problems:**
- **Drops messages when disabled** - Returns nil instead of error
- **No publisher confirms** - Can't verify RabbitMQ received message
- **No mandatory flag** - Unroutable messages silently dropped
- **No retry on publish failure** - Single attempt only
- **No connection validation** - Channel might be closed without knowing

⚠️ **Risk:**
```
Scenario: RabbitMQ temporarily unavailable
Action: API calls Publish()
Result: enabled=false → message dropped → return nil (success!)
Impact: Withdrawal request lost, money stuck in locked_balance forever
```

---

### Consume Method

**Current Implementation:**
```go
func (c *Client) Consume(queueName string, handler func(ctx context.Context, body []byte) error) {
    c.mu.RLock()
    enabled, ch := c.enabled, c.channel
    c.mu.RUnlock()

    if !enabled || ch == nil {
        logger.Warn("queue disabled; not consuming", "queue", queueName)
        return  // ❌ CONSUMER NEVER STARTS
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
                _ = d.Nack(false, true)  // ❌ INFINITE REQUEUE
                continue
            }
            _ = d.Ack(false)
        }
        logger.Warn("consumer channel closed", "queue", queueName)
    }()
}
```

**Analysis:**

✅ **Good:**
- Manual ACK (autoAck=false) - Ensures message processed before removal
- Goroutine per consumer - Non-blocking
- Error logging

❌ **Critical Problems:**

1. **Infinite Requeue Loop:**
   ```go
   _ = d.Nack(false, true)  // requeue=true, ALWAYS
   ```
   - Poison message will retry forever
   - Blocks entire queue
   - No retry limit
   - No exponential backoff

2. **No Consumer Recovery:**
   ```go
   for d := range deliveries {  // loop ends when channel closes
       ...
   }
   logger.Warn("consumer channel closed", "queue", queueName)
   // ❌ CONSUMER NEVER RESTARTS
   ```

3. **Consumer Lost When RabbitMQ Unavailable:**
   ```go
   if !enabled || ch == nil {
       logger.Warn("queue disabled; not consuming", "queue", queueName)
       return  // Consumer registration lost
   }
   ```

4. **No Concurrency Control:**
   - Single-threaded consumer
   - No prefetch limit
   - No concurrent message handling

---

## Message Definitions (`internal/queue/messages.go`)

### Current Message Types

```go
type WithdrawalMessage struct {
    TransactionID string `json:"transaction_id"`
    UserID        string `json:"user_id"`
    AmountKobo    int64  `json:"amount_kobo"`
    Reference     string `json:"reference"`
}

type EmailMessage struct {
    To       string `json:"to"`
    Subject  string `json:"subject"`
    HTMLBody string `json:"html_body"`
    TextBody string `json:"text_body"`
}

type SMSMessage struct {
    To      string `json:"to"`
    Message string `json:"message"`
}

type DepositReceiptMessage struct {
    UserID     string `json:"user_id"`
    Email      string `json:"email"`
    AmountKobo int64  `json:"amount_kobo"`
    Reference  string `json:"reference"`
}

type RealtimeMessage struct {
    UserID  string `json:"user_id"`
    Event   string `json:"event"`
    Payload any    `json:"payload"`
}
```

**Analysis:**
- ✅ Type-safe message contracts
- ✅ Clear field names
- ✅ JSON serializable
- ⚠️ No message versioning
- ⚠️ No retry metadata (attempt count, first seen, etc.)
- ⚠️ No idempotency key in message

---

## Queue Configuration (`internal/queue/config.go`)

**Important Discovery:** This file contains **advanced queue configuration** but is **NOT CURRENTLY USED** by the main client.

### Defined Configurations

```go
var (
    DepositQueueConfig = QueueConfig{
        Name:          "propvest.deposit.confirmed",
        Durable:       true,
        MaxLength:     10000,
        TTL:           48 * 60 * 60 * 1000,  // 48 hours
        Priority:      5,
        DLXName:       "propvest.dlx",
        PrefetchCount: 10,
        Concurrency:   10,
    }

    WithdrawalQueueConfig = QueueConfig{
        Name:          "propvest.withdrawal.requested",
        Priority:      10,  // HIGHEST
        Concurrency:   5,
        // ... other settings
    }

    NotificationQueueConfig = QueueConfig{
        Name:          "propvest.notification.dispatch",
        Priority:      1,  // LOWEST
        Concurrency:   20,
        // ... other settings
    }
)
```

**⚠️ IMPORTANT:** These configurations exist but are **NOT APPLIED**. The current `queue.go` implementation does NOT use them.

**Gap:** Advanced features designed but not implemented.

---

## Failure Scenarios

### Scenario 1: RabbitMQ Unavailable at Startup

**Current Behavior:**
```
1. API starts
2. queue.New(url) called
3. Connection fails
4. Client created with enabled=false
5. API continues running
6. All Publish() calls drop messages
7. All Consume() calls are no-ops
```

**Result:** ❌ **System runs but silently loses all async operations**

**Expected Behavior:**
- Retry connection with backoff
- Eventually connect when RabbitMQ becomes available
- Start registered consumers automatically

---

### Scenario 2: RabbitMQ Crashes After Startup

**Current Behavior:**
```
1. System running normally
2. RabbitMQ process crashes
3. Connection lost
4. deliveries channel closes
5. Consumer goroutines exit
6. Publish() calls fail with error
7. enabled flag still true (no detection)
```

**Result:** ❌ **Messages start failing, consumers never recover**

**Expected Behavior:**
- Detect connection loss immediately
- Mark connection as unavailable
- Attempt reconnection with backoff
- Recreate channel
- Redeclare queues
- Restart consumers

---

### Scenario 3: Poison Message

**Current Behavior:**
```
1. Consumer receives malformed message
2. Handler returns error
3. Nack(requeue=true) called
4. Message goes back to queue
5. Consumer receives same message again
6. Handler returns error again
7. Infinite loop
```

**Result:** ❌ **Queue blocked by single bad message**

**Expected Behavior:**
- Track retry count (x-death header or external tracking)
- Retry with exponential backoff
- After max retries → route to DLQ
- Log poison message for investigation

---

### Scenario 4: High Message Volume

**Current Behavior:**
```
1. 1000 messages published rapidly
2. Single consumer thread
3. No prefetch limit
4. No rate limiting
5. Consumer processes one at a time
```

**Result:** ⚠️ **Slow processing, potential memory issues**

**Expected Behavior:**
- Prefetch limit (e.g., 10 messages)
- Concurrent message handling
- Backpressure when worker overloaded

---

## Connection Lifecycle Analysis

### Current Lifecycle

```
                  ┌──────────────┐
                  │  queue.New() │
                  └──────┬───────┘
                         │
                   Try Connect
                         │
           ┌─────────────┴─────────────┐
           │                           │
      ✅ Success                   ❌ Failure
           │                           │
    enabled=true                  enabled=false
           │                           │
    Declare Queues           Log Warning, Return
           │                           │
    Return Client            ❌ NO RETRY
           │
    ┌──────▼──────┐
    │   Running    │
    └──────────────┘
           │
    Connection Lost?
           │
     ❌ NO DETECTION
           │
    Consumers Die
    Publish Fails
           │
    ❌ NO RECOVERY
```

### Required Lifecycle

```
                  ┌──────────────┐
                  │  queue.New() │
                  └──────┬───────┘
                         │
                   Try Connect
                         │
           ┌─────────────┴─────────────┐
           │                           │
      ✅ Success                   ❌ Failure
           │                           │
    Setup Connection         Start Reconnect Loop
           │                    (with backoff)
    Declare Queues                     │
           │                     ┌─────┴─────┐
    Return Client               │  Waiting   │
           │                    └─────┬─────┘
    ┌──────▼──────┐                   │
    │   Running    │             Connected?
    └──────┬───────┘                   │
           │                      ✅ YES: Setup
           │                           │
    Connection Lost? ◄─────────────────┘
           │
    ✅ DETECTED (heartbeat/error channel)
           │
    Start Reconnect Loop
           │
    ┌──────▼──────┐
    │ Reconnecting│
    └──────┬──────┘
           │
    Exponential Backoff
           │
    Try Reconnect
           │
      ✅ Success: Restart Consumers
```

---

## Consumer Registration Pattern

### Current Pattern (Problematic)

```go
// In worker main.go
mqClient := queue.New(cfg.RabbitMQURL)

if mqClient.Enabled() {
    startWithdrawalProcessor(ctx, mqClient, ...)
    // Consumer registered only if enabled NOW
}
```

**Problem:** If RabbitMQ is down at startup, `Enabled() = false`, so consumers are **never registered**. Even if RabbitMQ comes online later, no consumers will start.

### Required Pattern

```go
// Always register consumers
mqClient.RegisterConsumer(queue.QueueWithdrawalProcess, withdrawalHandler)
mqClient.RegisterConsumer(queue.QueueEmailDispatch, emailHandler)

// If connected now: start consumers
// If not connected: start when connection established
// If connection lost: restart on reconnection
```

---

## Graceful Degradation Analysis

### Current Implementation

**API Without RabbitMQ:**
```
✅ API starts successfully
✅ HTTP endpoints work
✅ Database operations work
❌ Async operations silently dropped
❌ Withdrawals created but never processed
❌ Emails never sent
❌ Reconciliation becomes critical safety net
```

**Assessment:** Graceful degradation **exists but is dangerous** because:
1. No visibility into lost messages
2. No alerting on disabled queue
3. Users see "success" but operations never complete
4. Money can be locked indefinitely

### Improved Degradation

**Better Approach:**
```
✅ API starts successfully
✅ Create outbox events in database
✅ Outbox dispatcher retries publishing
✅ Eventual consistency when RabbitMQ recovers
✅ Metrics show queue disabled state
⚠️ Increased database load from outbox polling
```

---

## Publisher Confirms

**Current:** ❌ Not implemented

**What Are Publisher Confirms?**
- RabbitMQ feature to confirm message receipt
- Client waits for broker ACK before considering message sent
- Prevents silent message loss

**How to Implement:**
```go
ch.Confirm(false)  // Enable confirm mode
confirmChan := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

// Publish message
ch.PublishWithContext(...)

// Wait for confirm
select {
case confirm := <-confirmChan:
    if confirm.Ack {
        // ✅ Message confirmed by broker
    } else {
        // ❌ Message rejected by broker
    }
case <-time.After(5 * time.Second):
    // ❌ Timeout waiting for confirm
}
```

**When to Use:**
- ✅ Critical financial operations (withdrawals)
- ✅ Important notifications (withdrawal status)
- ⚠️ Adds latency (5-20ms per message)
- ⚠️ Not needed if using outbox pattern (DB is source of truth)

---

## Error Handling (`internal/queue/errors.go`)

The system has **sophisticated error types** defined but not fully utilized:

```go
type ErrQueueDisabled struct{}
type ErrPublishFailed struct{ Cause error }
type ErrConsumeFailed struct{ Cause error }
type ErrInvalidMessage struct{ Cause error }
```

**Gap:** Error types exist but `Publish()` currently returns `nil` when disabled instead of `ErrQueueDisabled`.

---

## Comparison: Current vs Required

| Feature | Current | Required | Priority |
|---------|---------|----------|----------|
| **Connection** |
| Initial connection | ✅ Yes | ✅ Yes | - |
| Retry on startup failure | ❌ No | ✅ Yes with backoff | 🔴 HIGH |
| Runtime reconnection | ❌ No | ✅ Yes with backoff | 🔴 HIGH |
| Connection monitoring | ❌ No | ✅ Heartbeat/error channel | 🔴 HIGH |
| **Queues** |
| Durable queues | ✅ Yes | ✅ Yes | - |
| DLX configuration | ❌ No | ✅ Yes | 🟡 MEDIUM |
| Queue arguments | ❌ No | ✅ TTL, max-length | 🟡 MEDIUM |
| **Publishing** |
| Persistent messages | ✅ Yes | ✅ Yes | - |
| Publisher confirms | ❌ No | ⚠️ Optional | 🟢 LOW |
| Retry on failure | ❌ No | ✅ Via outbox | 🔴 HIGH |
| Handles disabled state | ⚠️ Drops | ✅ Queue in DB | 🔴 HIGH |
| **Consuming** |
| Manual ACK | ✅ Yes | ✅ Yes | - |
| Consumer recovery | ❌ No | ✅ Auto restart | 🔴 HIGH |
| Retry limit | ❌ Infinite | ✅ Max 3 attempts | 🔴 HIGH |
| DLQ routing | ❌ No | ✅ After max retries | 🟡 MEDIUM |
| Prefetch limit | ❌ No | ✅ Yes (10-20) | 🟡 MEDIUM |
| Concurrent handlers | ❌ Single | ✅ Configurable | 🟡 MEDIUM |
| **Reliability** |
| Transactional outbox | ❌ No | ✅ Yes | 🔴 HIGH |
| Message deduplication | ❌ No | ✅ Via idempotency | 🔴 HIGH |
| Poison message handling | ❌ Infinite loop | ✅ DLQ after retries | 🔴 HIGH |

---

## Integration Points

### How Services Use Queue Client

**Wallet Service:**
```go
s.mq.Publish(ctx, queue.QueueWithdrawalProcess, queue.WithdrawalMessage{
    TransactionID: ledger.ID.String(),
    UserID:        userID.String(),
    AmountKobo:    req.Amount,
    Reference:     reference,
})
```

**Problem:** Publish happens **AFTER** database COMMIT. If publish fails, message lost.

**Worker Consumer:**
```go
mqClient.Consume(queue.QueueWithdrawalProcess, func(ctx context.Context, body []byte) error {
    // Process withdrawal
    // If error: message requeued FOREVER
})
```

---

## Recommendations Summary

### 🔴 Critical (Must Fix)

1. **Implement connection retry on startup**
   - Exponential backoff (1s, 2s, 4s, 8s, max 60s)
   - Don't block startup indefinitely
   - Log each attempt

2. **Implement runtime reconnection**
   - Monitor connection health
   - Detect disconnection immediately
   - Reconnect with backoff
   - Recreate channel and redeclare queues

3. **Implement consumer recovery**
   - Store registered consumers
   - Restart consumers on reconnection
   - Handle consumer goroutine panics

4. **Fix infinite requeue**
   - Track retry count (x-death header)
   - Limit to 3 attempts
   - Route to DLQ after max retries

5. **Implement transactional outbox**
   - Replace direct Publish() with outbox events
   - Dispatcher polls outbox and publishes
   - Atomic: business state + outbox event

### 🟡 Important (Should Fix)

6. **Implement DLQ infrastructure**
   - Create dead-letter exchange
   - Create DLQ for each primary queue
   - Configure DLX on primary queues
   - Add DLQ monitoring

7. **Add prefetch limits**
   - Limit messages fetched per consumer
   - Prevent memory overflow
   - Typical values: 10-20

8. **Add concurrent message handling**
   - Multiple goroutines per consumer
   - Configurable concurrency (5-20)
   - Respect resource limits

### 🟢 Nice to Have

9. **Implement publisher confirms**
   - For critical operations only
   - Adds latency but ensures delivery
   - May not be needed with outbox

10. **Add connection metrics**
    - Connection status (connected/disconnected)
    - Reconnection attempts
    - Messages published/consumed
    - Queue depths

---

## Testing Gaps

**Currently Missing:**
- ❌ Connection retry tests
- ❌ Reconnection tests
- ❌ Consumer recovery tests
- ❌ Poison message handling tests
- ❌ Connection loss simulation tests

**Existing Tests:**
- ✅ Error type tests (`errors_test.go`)
- ✅ Configuration tests (`config_test.go`)

---

## Conclusion

The PropVest RabbitMQ implementation has **good foundations** (durable queues, persistent messages, graceful degradation) but **critical gaps** that make it unsuitable for production financial operations:

### Key Findings

1. ❌ **Messages can be lost** - No reconnection, no outbox
2. ❌ **Consumers don't recover** - Connection loss = permanent failure
3. ❌ **Poison messages block queues** - Infinite requeue loop
4. ❌ **No visibility into failures** - Silent degradation
5. ⚠️ **Advanced features designed but not used** - Config exists but unused

### Risk Assessment

**Current State:** 🔴 **Not production-ready**

**Highest Risks:**
1. Withdrawal messages lost → money locked forever
2. Consumer death → no withdrawal processing
3. Poison message → entire queue blocked

### Path Forward

**Priority 1 (This Implementation):**
- ✅ Transactional outbox (eliminates message loss)
- ✅ Connection retry and reconnection
- ✅ Consumer recovery
- ✅ Retry limits and DLQ

**Priority 2 (Future Enhancement):**
- ⚠️ Publisher confirms (if needed)
- ⚠️ Advanced queue features (TTL, priorities)
- ⚠️ Monitoring and alerting

---

**Next Audit:** AUDIT_03_WALLET_WITHDRAWAL.md  
**Status:** ✅ Complete
