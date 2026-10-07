# Audit Report 06: Notification Services

**Date:** 2026-10-06  
**Phase:** 1 - Audit & Analysis  
**Step:** 1.6 - Notification Services Audit

---

## Executive Summary

Notification services have **same async messaging gap** as withdrawals - messages published outside database transactions.

✅ **Multiple channels** - In-app, email, SMS, realtime  
✅ **Structured notification model** - Database-backed  
⚠️ **Non-critical operations** - User experience impact, not financial  
❌ **Same RabbitMQ gap** - Publish outside transaction  
⚠️ **Lower priority** - Less critical than withdrawal reliability  

---

## Current Queues

```go
const (
    QueueEmailDispatch  = "propvest.email.dispatch"
    QueueSMSDispatch    = "propvest.sms.dispatch"
    QueueRealtimePush   = "propvest.realtime.push"
    QueueDepositReceipt = "propvest.deposit.receipt"
)
```

---

## Notification Flow

### In-App Notifications

```go
// Create notification in database
notification := &models.Notification{
    UserID:  userID,
    Type:    notificationType,
    Title:   title,
    Message: message,
    Data:    metadata,
    Read:    false,
}
db.Create(notification)

// ❌ Publish realtime notification (OUTSIDE transaction)
if mq != nil && mq.Enabled() {
    mq.Publish(ctx, queue.QueueRealtimePush, queue.RealtimeMessage{
        UserID:  userID.String(),
        Event:   "notification.created",
        Payload: notification,
    })
}
```

**Gap:** Same as withdrawal - publish outside transaction.

---

### Email Notifications

```go
// ❌ Publish email message directly (no database record)
if mq != nil && mq.Enabled() {
    mq.Publish(ctx, queue.QueueEmailDispatch, queue.EmailMessage{
        To:       email,
        Subject:  subject,
        HTMLBody: htmlBody,
        TextBody: textBody,
    })
}
```

**Gaps:**
- No database record of email intent
- If RabbitMQ down, email lost
- No retry mechanism
- No audit trail

---

### SMS Notifications

```go
// ❌ Publish SMS message directly (no database record)
if mq != nil && mq.Enabled() {
    mq.Publish(ctx, queue.QueueSMSDispatch, queue.SMSMessage{
        To:      phone,
        Message: message,
    })
}
```

**Gaps:** Same as email

---

## Risk Assessment

### Financial Impact: 🟢 LOW

Missed notifications don't cause:
- Money loss
- Locked funds
- Data corruption

### User Experience Impact: 🟡 MEDIUM

Missed notifications cause:
- User confusion ("Where's my confirmation email?")
- Support load
- Trust issues

### Compliance Impact: 🟡 MEDIUM

Missed notifications could cause:
- Regulatory issues (transaction confirmations required)
- Audit problems (no proof email sent)

---

## Outbox Priority

| Operation | Financial Risk | UX Impact | Outbox Priority |
|-----------|----------------|-----------|-----------------|
| Withdrawals | 🔴 HIGH | 🔴 HIGH | 🔴 **CRITICAL** |
| Deposits | 🟡 MEDIUM | 🔴 HIGH | 🟡 **HIGH** |
| Email | 🟢 LOW | 🟡 MEDIUM | 🟢 **MEDIUM** |
| SMS | 🟢 LOW | 🟡 MEDIUM | 🟢 **MEDIUM** |
| Realtime | 🟢 LOW | 🟡 MEDIUM | 🟢 **LOW** |

---

## Recommendations

### Phase 1: Financial Operations (Immediate)

1. ✅ **Withdrawals** - Outbox pattern
2. ✅ **Deposit receipts** - Outbox pattern

### Phase 2: User Communications (Later)

3. ⚠️ **Emails** - Consider outbox OR accept occasional loss
4. ⚠️ **SMS** - Consider outbox OR accept occasional loss

### Phase 3: Realtime (Optional)

5. ⏭️ **Realtime push** - Accept loss (not critical)

---

## Email/SMS Outbox Consideration

### Option A: Include in Outbox

**Pros:**
- Reliable delivery
- Audit trail
- Retry capability

**Cons:**
- More outbox events
- More database writes
- Complexity

### Option B: Accept Occasional Loss

**Pros:**
- Simpler implementation
- Lower database load
- Faster response time

**Cons:**
- Occasional missed emails
- Support tickets

**Recommendation:** Start with Option B, add outbox later if needed.

---

## Notification Database Model

```go
type Notification struct {
    ID        uuid.UUID
    UserID    uuid.UUID
    Type      string  // withdrawal_update, deposit_confirmed, etc.
    Title     string
    Message   string
    Data      datatypes.JSON
    Read      bool
    ReadAt    *time.Time
    CreatedAt time.Time
}
```

**Analysis:**
- ✅ Persisted to database
- ✅ Queryable by user
- ✅ Read status tracking
- ✅ Structured data field
- ⚠️ No delivery status (was email sent?)
- ⚠️ No retry tracking

---

## Conclusion

Notification services have the **same architectural gap** as withdrawals (publish outside transaction) but with **lower risk impact**. Outbox implementation should **prioritize financial operations first**.

### Implementation Strategy

**Phase 1 (This Implementation):**
- ✅ Outbox for withdrawals
- ✅ Outbox for deposit confirmations
- ⏭️ Skip email/SMS for now

**Phase 2 (Future Enhancement):**
- Add outbox for email/SMS if needed
- Add delivery status tracking
- Add retry management

---

**Status:** ✅ Complete  
**Next:** AUDIT_07_SUMMARY.md
