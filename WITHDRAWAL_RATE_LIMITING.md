# Withdrawal Rate Limiting

## Overview

Withdrawal rate limiting restricts users to a maximum number of withdrawal requests per hour, preventing abuse and protecting the system.

---

## Configuration

### Default Limits
```
Maximum: 3 withdrawals per hour per user
Window: Sliding 60-minute window
Scope: Per authenticated user (user_id based)
```

### How It Works

**Sliding Window Algorithm:**
```
10:00 AM - Withdrawal #1 ✓ (1/3 used)
10:15 AM - Withdrawal #2 ✓ (2/3 used)
10:30 AM - Withdrawal #3 ✓ (3/3 used, limit reached)
10:45 AM - Withdrawal #4 ✗ (blocked, retry after 15 minutes)
11:01 AM - Withdrawal #5 ✓ (10:00 AM expired, 2/3 used)
```

The window slides continuously - not fixed hourly blocks.

---

## Why Rate Limiting?

### 1. Fraud Prevention
- Limits rapid-fire withdrawal attempts
- Flags suspicious account activity
- Prevents account takeover exploitation

### 2. System Protection
- Reduces load on payment provider APIs
- Prevents excessive queue congestion
- Ensures fair resource allocation

### 3. Cost Control
- Reduces payment provider API costs
- Minimizes transfer fees
- Optimizes worker utilization

### 4. User Protection
- Flags compromised accounts
- Prevents panic withdrawals
- Encourages intentional behavior

---

## Implementation

### Code Location
```
internal/middleware/withdrawal_rate_limit.go
```

### Middleware Application
```go
// File: internal/routes/v1/routes.go

authenticated.POST("/withdraw",
    middleware.WithdrawalRateLimit(3),  // ← Rate limiting
    walletHandler.RequestWithdrawal)
```

### Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    User Request                          │
│  POST /api/v1/wallet/withdraw                            │
└────────────────────┬─────────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────────┐
│              Auth Middleware                             │
│  Extract user_id from JWT                                │
└────────────────────┬─────────────────────────────────────┘
                     │
                     ▼
┌──────────────────────────────────────────────────────────┐
│         Withdrawal Rate Limit Middleware                 │
│                                                          │
│  1. Check attempts in last 60 minutes                   │
│  2. If < 3: Record attempt, allow request               │
│  3. If >= 3: Reject with 429                            │
│                                                          │
│  Storage: In-memory map (user_id → attempts[])          │
└────────────────────┬─────────────────────────────────────┘
                     │
        ┌────────────┴────────────┐
        │                         │
    Allowed                   Blocked
        │                         │
        ▼                         ▼
┌─────────────────┐    ┌─────────────────────┐
│  Withdrawal     │    │  HTTP 429           │
│  Handler        │    │  "Too many          │
│                 │    │   withdrawal        │
│  - Validate     │    │   attempts"         │
│  - Lock funds   │    │                     │
│  - Create txn   │    │  Headers:           │
│  - Queue job    │    │  - Retry-After      │
└─────────────────┘    │  - X-RateLimit-*    │
                       └─────────────────────┘
```

---

## Response Headers

All withdrawal requests include rate limit headers:

```http
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 1
X-RateLimit-Reset: 1694567890
```

### When Blocked:
```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1694567890
Retry-After: 900

{
  "success": false,
  "message": "Too many withdrawal attempts. Maximum 3 per hour. Try again at 14:30:00.",
  "code": "withdrawal_rate_limit",
  "request_id": "req-abc123"
}
```

### Header Meanings:
- **X-RateLimit-Limit** - Maximum withdrawals allowed per hour
- **X-RateLimit-Remaining** - Withdrawals remaining in current window
- **X-RateLimit-Reset** - Unix timestamp when limit resets
- **Retry-After** - Seconds until next withdrawal allowed

---

## Testing

### Test Rate Limit (bash)
```bash
# Get token
TOKEN=$(curl -s http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password"}' \
  | jq -r '.data.access_token')

# Try 4 withdrawals rapidly
for i in {1..4}; do
  echo "Attempt $i:"
  curl -i http://localhost:8080/api/v1/wallet/withdraw \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
      "amount_kobo": 50000,
      "account_number": "0123456789",
      "account_name": "Test User",
      "bank_code": "058"
    }'
  echo ""
  sleep 2
done
```

**Expected Output:**
```
Attempt 1:
HTTP/1.1 200 OK
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 2
✓ SUCCESS

Attempt 2:
HTTP/1.1 200 OK
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 1
✓ SUCCESS

Attempt 3:
HTTP/1.1 200 OK
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 0
✓ SUCCESS

Attempt 4:
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 3
X-RateLimit-Remaining: 0
Retry-After: 3540
✗ BLOCKED
```

---

### Check Rate Limit Status

You can check a user's current rate limit status:

```go
// In handler or middleware
status := middleware.GetWithdrawalRateLimitStatus(userID)
```

**Returns:**
```json
{
  "limit": 3,
  "used": 2,
  "remaining": 1,
  "resets_at": "2026-09-09T14:30:00Z",
  "can_withdraw": true
}
```

This can be exposed as an endpoint:
```go
// GET /api/v1/wallet/withdraw/status
func (h *WalletHandler) GetWithdrawalLimitStatus(c *gin.Context) {
    userID := c.GetString("user_id")
    status := middleware.GetWithdrawalRateLimitStatus(userID)
    response.Success(c, status)
}
```

---

## Customization

### Change Default Limit
```go
// Allow 5 withdrawals per hour
authenticated.POST("/withdraw",
    middleware.WithdrawalRateLimit(5),
    walletHandler.RequestWithdrawal)
```

### Different Window Duration
```go
// Allow 10 withdrawals per day
authenticated.POST("/withdraw",
    middleware.WithdrawalRateLimitConfigurable(10, 24*time.Hour),
    walletHandler.RequestWithdrawal)
```

### Per-User Custom Limits
```go
// In handler, apply different limits based on user tier
func (h *WalletHandler) RequestWithdrawal(c *gin.Context) {
    user := getCurrentUser(c)
    
    // Check custom rate limit based on user tier
    var limit int
    switch user.Tier {
    case "basic":
        limit = 3  // Basic: 3/hour
    case "premium":
        limit = 10 // Premium: 10/hour
    case "business":
        limit = 50 // Business: 50/hour
    }
    
    // Manually check rate limit
    allowed, _, _, nextTime := checkWithdrawalRateLimit(user.ID, limit)
    if !allowed {
        response.Error(c, 429, "Rate limit exceeded", "withdrawal_rate_limit")
        return
    }
    
    // Continue with withdrawal...
}
```

---

## Bypass Rate Limiting (Admin)

For admin/support operations, you may want to bypass rate limits:

```go
// Check if user is admin
if user.Role == "admin" || user.Role == "support" {
    // Skip rate limit check
} else {
    // Apply normal rate limit
    middleware.WithdrawalRateLimit(3)
}
```

Or create a dedicated admin endpoint:
```go
// POST /api/v1/admin/wallets/:user_id/withdraw
// No rate limiting for admin-initiated withdrawals
admin.POST("/wallets/:user_id/withdraw",
    middleware.RequireRole("admin"),
    // No WithdrawalRateLimit middleware
    adminHandler.AdminWithdrawal)
```

---

## Production Considerations

### Current Implementation: In-Memory

**Pros:**
- Simple, no external dependencies
- Fast (no network calls)
- Good for single-server deployments

**Cons:**
- Lost on server restart
- Doesn't work across multiple servers
- Not persistent

**Suitable for:**
- MVP / Early stage
- Single server deployment
- Development / Testing

---

### Production Upgrade: Redis

For production with multiple servers, migrate to Redis:

#### Setup Redis Client
```go
import "github.com/go-redis/redis/v8"

var redisClient *redis.Client

func InitRedis() {
    redisClient = redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
        DB:   0,
    })
}
```

#### Redis-Based Rate Limiter
```go
func WithdrawalRateLimitRedis(maxPerHour int) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx := c.Request.Context()
        userID := c.GetString("user_id")
        key := fmt.Sprintf("withdrawal_rate_limit:%s", userID)
        
        now := time.Now().Unix()
        hourAgo := now - 3600
        
        // Redis sorted set with timestamps as scores
        pipe := redisClient.Pipeline()
        
        // Add current attempt
        pipe.ZAdd(ctx, key, &redis.Z{
            Score:  float64(now),
            Member: now,
        })
        
        // Remove old attempts (older than 1 hour)
        pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", hourAgo))
        
        // Count attempts in window
        countCmd := pipe.ZCard(ctx, key)
        
        // Set expiry (cleanup)
        pipe.Expire(ctx, key, time.Hour)
        
        _, err := pipe.Exec(ctx)
        if err != nil {
            // Handle error
        }
        
        count := countCmd.Val()
        
        if count > int64(maxPerHour) {
            // Rate limit exceeded
            c.JSON(429, gin.H{
                "success": false,
                "message": fmt.Sprintf("Too many withdrawal attempts. Maximum %d per hour.", maxPerHour),
                "code": "withdrawal_rate_limit",
            })
            c.Abort()
            return
        }
        
        c.Next()
    }
}
```

#### Benefits:
- ✅ Works across multiple servers
- ✅ Persistent (survives restarts)
- ✅ Centralized rate limiting
- ✅ Can be monitored via Redis CLI

#### Commands for Monitoring:
```bash
# Check user's current attempts
redis-cli ZCARD withdrawal_rate_limit:user-123

# View all attempts with timestamps
redis-cli ZRANGE withdrawal_rate_limit:user-123 0 -1 WITHSCORES

# Manually reset for testing
redis-cli DEL withdrawal_rate_limit:user-123

# Check all rate-limited users
redis-cli KEYS "withdrawal_rate_limit:*"
```

---

## Monitoring & Alerts

### Metrics to Track

1. **Rate Limit Hit Rate**
   - % of requests blocked
   - High rate may indicate abuse or limits too strict

2. **Users Hitting Limits**
   - Track which users hit limits frequently
   - May indicate fraud or usability issues

3. **Average Withdrawals Per User**
   - Normal: 1-2 per day
   - Suspicious: 3+ per hour

### Alerting Rules

```yaml
# Alert if rate limit hit rate exceeds threshold
- alert: HighWithdrawalRateLimitRate
  expr: (rate_limit_blocked / rate_limit_total) > 0.1
  for: 5m
  annotations:
    summary: "10%+ of withdrawal requests are being rate limited"
    
# Alert if single user hits limit repeatedly
- alert: UserHittingWithdrawalLimit
  expr: rate_limit_blocked_per_user > 5
  for: 1h
  annotations:
    summary: "User {{ $labels.user_id }} has hit withdrawal rate limit 5+ times in 1 hour"
```

### Log Analysis

```bash
# Find users hitting rate limits
grep "withdrawal_rate_limit" logs/api.log | \
  jq -r '.user_id' | sort | uniq -c | sort -rn | head -10

# Rate limit hits per hour
grep "withdrawal_rate_limit" logs/api.log | \
  jq -r '.timestamp' | cut -d: -f1 | uniq -c
```

---

## Edge Cases

### 1. Clock Skew
If server clocks are out of sync in a multi-server setup, rate limits may behave unexpectedly.

**Solution:** Use Redis with single time source or NTP sync.

### 2. Failed Validations
Failed validations (insufficient balance, invalid account) currently **don't count** against the rate limit because middleware runs before validation.

**To count all attempts** (including failed validations), move middleware to beginning:
```go
authenticated.Use(middleware.WithdrawalRateLimit(3))
authenticated.POST("/withdraw", walletHandler.RequestWithdrawal)
```

### 3. Concurrent Requests
Two simultaneous withdrawal requests might both pass rate limit check.

**Solution:** Add pessimistic locking:
```go
entry.mu.Lock()
defer entry.mu.Unlock()
// Check and record atomically
```
(Already implemented in current code)

### 4. Memory Leak
If cleanup goroutine fails, old entries accumulate.

**Solution:** Cleanup runs every 15 minutes and removes expired entries:
```go
func (wrl *withdrawalRateLimiter) cleanup() {
    ticker := time.NewTicker(15 * time.Minute)
    // ... cleanup logic
}
```

---

## Security Considerations

### Cannot Be Bypassed
- **User-based:** Tied to user_id from JWT, not IP
- **Server-side:** Cannot be disabled by client
- **Signed tokens:** Cannot fake user_id

### Attack Vectors Prevented
1. **Rapid-fire withdrawals:** Limited to 3/hour
2. **Account takeover:** Flags suspicious activity
3. **API abuse:** Prevents excessive provider API calls
4. **Distributed attack:** Each user limited independently

### Not Protected Against
- **Multiple accounts:** Each account has own limit
- **Slow attacks:** 3 withdrawals/hour is allowed
- **Social engineering:** User explicitly requests withdrawals

---

## User Experience

### Good UX Practices

#### 1. Show Remaining Attempts
```javascript
// Fetch before showing withdrawal form
const status = await fetch('/api/v1/wallet/withdraw/status');
const { remaining } = await status.json();

if (remaining === 0) {
  showMessage('You have reached your withdrawal limit for this hour.');
  disableWithdrawButton();
}
```

#### 2. Display Reset Time
```javascript
const { resets_at } = await status.json();
const resetsIn = new Date(resets_at) - new Date();
const minutes = Math.ceil(resetsIn / 60000);

showMessage(`You can withdraw again in ${minutes} minutes`);
```

#### 3. Handle Rate Limit Errors Gracefully
```javascript
try {
  await fetch('/api/v1/wallet/withdraw', { ... });
} catch (error) {
  if (error.code === 'withdrawal_rate_limit') {
    showMessage('You have reached your hourly withdrawal limit. Please try again later.');
    showNextAvailableTime(error.resets_at);
  }
}
```

---

## FAQ

### Q: Why 3 withdrawals per hour?
**A:** Based on typical user behavior:
- Most users: 1-2 withdrawals per day
- Power users: 3-5 withdrawals per day
- 3/hour allows normal usage while preventing abuse

### Q: Can I change the limit?
**A:** Yes, modify the middleware parameter:
```go
middleware.WithdrawalRateLimit(5)  // 5/hour
```

### Q: Does it count failed withdrawals?
**A:** By default, NO. Only successful initiations count. Failed validations (insufficient balance, etc.) don't count.

To count all attempts, move middleware before validation.

### Q: What happens after server restart?
**A:** In-memory limits are reset. Users can withdraw again immediately.

For persistent limits, upgrade to Redis.

### Q: Can admins bypass limits?
**A:** Not automatically. You must implement admin bypass logic in the handler.

### Q: How to test rate limiting?
**A:** See [Testing](#testing) section for bash script to test 4 rapid withdrawals.

---

## Summary

✅ **Implemented:** 3 withdrawals per hour per user  
✅ **Sliding window:** Continuous, not fixed blocks  
✅ **User-based:** Cannot bypass via IP change  
✅ **Concurrent-safe:** Mutex-protected  
✅ **Memory-efficient:** Automatic cleanup  
✅ **Production-ready:** Redis migration path provided  

**Next steps:**
1. Test rate limiting with provided script
2. Monitor rate limit hit rates
3. Adjust limits based on usage patterns
4. Migrate to Redis for multi-server deployments
