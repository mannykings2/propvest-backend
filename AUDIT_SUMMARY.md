# PropVest Backend — Outbox & Idempotency Audit Summary

**Date:** 2026-10-06  
**Phase:** 1 - Audit & Analysis Complete  
**Status:** ✅ All Audits Complete

---

## Executive Summary

PropVest has a **well-engineered foundation** with mature database practices, proper fund locking, and thoughtful business rules. However, the system has **one critical architectural gap** that undermines this solid foundation:

### 🔴 Critical Gap: Asynchronous Messaging Reliability

**The Problem:**
```
Database Transaction COMMITS ✅
        ↓
RabbitMQ Publish (OUTSIDE transaction) ❌
        ↓
If RabbitMQ unavailable or API crashes: MESSAGE LOST
```

**Impact:**
- Withdrawals created but never processed
- Money locked indefinitely
- No recovery mechanism except reconciliation (10+ minute delay)

---

## Audit Results Summary

| Component | Status | Risk Level | Key Findings |
|-----------|--------|------------|--------------|
| **Database** | ✅ Production-Ready | 🟢 LOW | Excellent foundation, migrations, constraints, immutable ledger |
| **RabbitMQ** | ⚠️ Functional | 🔴 HIGH | No reconnection, no consumer recovery, infinite requeue, messages dropped when disabled |
| **Wallet/Withdrawal** | ✅ Well-Designed | 🟡 MEDIUM | Proper fund locking, atomic operations, but publish outside transaction |
| **Paystack** | ✅ Functional | 🟡 LOW-MEDIUM | Good integration, implicit idempotency, needs explicit handling |
| **Reconciliation** | ✅ Well-Implemented | 🟢 LOW | Excellent safety net, currently overused, should be backup only |
| **Notifications** | ⚠️ Functional | 🟡 MEDIUM | Same gap as withdrawals, lower priority to fix |

---

## Detailed Findings

### 1. Database Layer ✅

**Strengths:**
- ✅ Production-grade migrations (golang-migrate)
- ✅ Immutable ledger pattern (wallet_transactions)
- ✅ Existing idempotency infrastructure (idempotency_key column)
- ✅ Proper constraints and indexes
- ✅ Fund locking mechanism (locked_balance)
- ✅ Row locking for concurrent safety
- ✅ Atomic transactions with GORM

**Gaps:**
- ❌ No outbox table
- ⚠️ Idempotency key exists but not enforced at API level
- ⚠️ No unique constraint on pending withdrawal per user

**Readiness:** ✅ **Ready for outbox implementation**

---

### 2. RabbitMQ Client ⚠️

**Strengths:**
- ✅ Durable queues
- ✅ Persistent messages
- ✅ Manual ACK (no auto-ack)
- ✅ Graceful degradation (disabled mode)
- ✅ Type-safe message contracts

**Critical Gaps:**
- ❌ **No startup retry** - If RabbitMQ down, client stays disabled forever
- ❌ **No runtime reconnection** - Connection loss = permanent failure
- ❌ **No consumer recovery** - Lost consumers never restart
- ❌ **Infinite requeue** - Poison messages block queue forever
- ❌ **No DLQ** - Failed messages have nowhere to go
- ❌ **Messages dropped when disabled** - Returns nil instead of error

**Current Behavior:**
```
RabbitMQ unavailable → enabled=false → ALL publishes silently dropped
```

**Risk:** 🔴 **HIGH** - Can lose critical financial messages

---

### 3. Withdrawal Flow ✅ (Design) / ❌ (Reliability)

**Strengths:**
- ✅ Proper fund locking (locked_balance)
- ✅ Atomic database operations
- ✅ Row locking prevents race conditions
- ✅ Idempotent finalization
- ✅ Business rule: one pending withdrawal per user
- ✅ Bank account verification before locking funds

**Critical Gap:**
```go
// Current code
err := db.Transaction(func(tx *gorm.DB) error {
    // Lock funds
    // Create pending transaction
    return nil
})  // ← COMMIT happens here

// ❌ Publish AFTER commit
mq.Publish(ctx, queue.QueueWithdrawalProcess, message)
```

**Failure Scenario:**
```
1. Database commits successfully
2. API crashes before publish
3. Result: Transaction in database, no message in queue
4. Impact: Money locked, withdrawal never processes
5. Recovery: Reconciliation after 10+ minutes
```

---

### 4. Worker Idempotency ⚠️

**Current Protection:**
```go
if txn.Status != "pending" {
    return nil  // Skip already processed
}
```

**Race Condition:**
```
Worker A: Read status=pending
Worker B: Read status=pending
Worker A: Call Paystack → Success
Worker B: Call Paystack → ❌ DUPLICATE TRANSFER
```

**Required:** Atomic status transition
```go
result := db.Where("id = ? AND status = ?", id, "pending").
    Update("status", "processing")
if result.RowsAffected == 0 {
    return ErrAlreadyProcessing  // Another worker claimed it
}
```

---

### 5. Paystack Integration ✅

**Strengths:**
- ✅ Stable reference generation (WD-XXX)
- ✅ Transfer verification available
- ✅ Webhook signature verification
- ✅ Status checking for reconciliation

**Gaps:**
- ⚠️ Implicit idempotency (relies on Paystack behavior)
- ⚠️ No explicit duplicate call handling
- ⚠️ No documented behavior for retry scenarios

**Mitigation:** Paystack enforces reference uniqueness, should return error or existing transfer on duplicate.

---

### 6. Reconciliation ✅

**Strengths:**
- ✅ Periodic polling (every 5 minutes)
- ✅ Grace period (10 minutes)
- ✅ Idempotent status verification
- ✅ Comprehensive logging
- ✅ Error handling

**Current Role:** **PRIMARY fallback** (wrong)  
**Should Be:** **RARE safety net** (right)

**With Outbox:**
- Outbox handles normal processing
- Reconciliation catches edge cases only
- Expected reconciliation rate: < 1%

---

### 7. Notification Services ⚠️

**Same Gap:** Publish outside transaction

**Risk Level:** 🟡 **MEDIUM** (UX impact, not financial)

**Priority:** Lower than financial operations

---

## Root Cause Analysis

### Why Messages Get Lost

```
┌─────────────────────────────────────────────────────────────┐
│                      FAILURE WINDOW                          │
│                                                              │
│  Database Transaction COMMITS ✅                             │
│           ↓                                                  │
│      [FAILURE WINDOW STARTS] ← API crash, RabbitMQ down     │
│           ↓                                                  │
│  RabbitMQ Publish Attempted ❌                               │
│           ↓                                                  │
│      [FAILURE WINDOW ENDS]                                   │
│                                                              │
│  Result: Business state persisted, event lost               │
└─────────────────────────────────────────────────────────────┘
```

### Why This Is Dangerous for Financial Operations

1. **Money Locked:** User's funds in `locked_balance`
2. **No Processing:** Worker never receives message
3. **No Timeout:** Lock persists indefinitely
4. **Delayed Recovery:** Reconciliation runs every 5 minutes, waits 10 minutes
5. **User Impact:** "Where's my money?" support tickets

---

## Solution: Transactional Outbox Pattern

### The Fix

```go
// NEW: Atomic business state + outbox event
err := db.Transaction(func(tx *gorm.DB) error {
    // 1. Lock funds
    // 2. Create pending transaction
    
    // 3. Create outbox event ← NEW
    outboxEvent := &models.OutboxEvent{
        EventType:     "withdrawal.process",
        AggregateType: "wallet_transaction",
        AggregateID:   transaction.ID,
        Payload:       json.Marshal(withdrawalMessage),
        Status:        "pending",
    }
    tx.Create(outboxEvent)
    
    return nil
})  // ← COMMIT: Both transaction AND outbox event

// Separate process: Outbox Dispatcher
// Polls outbox table → Publishes to RabbitMQ → Marks published
```

### Why This Works

```
Database = Source of Truth ✅
      ↓
Outbox Event Persisted ✅
      ↓
Dispatcher Polls & Retries ✅
      ↓
Eventually Publishes to RabbitMQ ✅
      ↓
No Message Loss ✅
```

---

## Priority Matrix

### 🔴 Critical (Must Fix - Phase 1)

| Component | Issue | Impact | Solution |
|-----------|-------|--------|----------|
| Withdrawals | Publish outside transaction | Money locked forever | Transactional outbox |
| RabbitMQ | No reconnection | Permanent failure | Connection retry logic |
| RabbitMQ | No consumer recovery | Workers stop forever | Consumer re-registration |
| Worker | No atomic state transition | Duplicate transfers | Atomic status update |
| RabbitMQ | Infinite requeue | Poison messages block queue | Retry limits + DLQ |

### 🟡 Important (Should Fix - Phase 2)

| Component | Issue | Impact | Solution |
|-----------|-------|--------|----------|
| Deposits | Same outbox gap | Less critical, webhook-driven | Extend outbox |
| API | No idempotency key enforcement | Duplicate requests | Idempotency middleware |
| Database | No unique constraint on pending | Race condition possible | Unique partial index |
| Paystack | No explicit retry handling | Unclear behavior | Document + handle |

### 🟢 Nice to Have (Future)

| Component | Issue | Impact | Solution |
|-----------|-------|--------|----------|
| Emails/SMS | No outbox | Occasional missed notification | Optional outbox |
| Reconciliation | No alerting | Can't detect problems | Metrics + alerts |
| RabbitMQ | No prefetch limit | Potential overload | Add prefetch |
| RabbitMQ | No concurrency | Slow processing | Concurrent handlers |

---

## Implementation Approach

### Phase 1: Core Reliability (This Implementation)

**Goal:** Make withdrawal flow production-ready

**Components:**

1. **Database Layer**
   - Create outbox_events table
   - Add indexes for claiming
   - Add idempotency_key unique constraint
   - Add partial unique index for pending withdrawals

2. **Outbox Implementation**
   - Create OutboxEvent model
   - Create OutboxRepository
   - Implement claim logic (FOR UPDATE SKIP LOCKED)
   - Implement mark published/failed

3. **Outbox Dispatcher**
   - Polling loop
   - Event claiming
   - RabbitMQ publishing
   - Retry logic with backoff
   - Stale event recovery

4. **RabbitMQ Improvements**
   - Startup connection retry
   - Runtime reconnection
   - Consumer recovery
   - Connection monitoring

5. **Idempotency**
   - Atomic withdrawal status transitions
   - API-level idempotency middleware
   - Worker-level deduplication

6. **Retry & DLQ**
   - Retry limits (max 3 attempts)
   - Exponential backoff
   - DLQ for failed messages

### Phase 2: Extended Coverage (Future)

7. Extend outbox to deposits
8. Extend outbox to emails/SMS (optional)
9. Add monitoring & metrics
10. Add alerting

---

## Expected Outcomes

### After Implementation

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Message Loss** | Possible | Impossible | ∞ |
| **Recovery Time** | 10+ minutes | < 1 minute | 10x |
| **RabbitMQ Downtime Impact** | Total failure | Graceful degradation | 100% |
| **Duplicate Transfers** | Possible | Prevented | 100% |
| **Poison Messages** | Block queue | Routed to DLQ | 100% |
| **Connection Loss Recovery** | Never | Automatic | ∞ |

### Reliability Guarantees

**Before:**
- ⚠️ At-most-once delivery (can lose messages)
- ❌ No recovery from RabbitMQ downtime
- ❌ No recovery from connection loss
- ⚠️ Manual intervention required for stuck withdrawals

**After:**
- ✅ At-least-once delivery (guaranteed)
- ✅ Automatic recovery from RabbitMQ downtime
- ✅ Automatic reconnection and consumer recovery
- ✅ Automatic retry with backoff
- ✅ Idempotent processing prevents duplicates
- ✅ DLQ for poison messages
- ✅ Self-healing system

---

## Risk Assessment

### Current System Risk: 🔴 HIGH

**Production Blockers:**
- Message loss possible
- RabbitMQ downtime = system failure
- No recovery from connection loss
- Poison messages block queues
- Duplicate transfers possible

### Post-Implementation Risk: 🟢 LOW

**Production Ready:**
- Message loss impossible (database source of truth)
- RabbitMQ downtime = graceful degradation
- Automatic recovery from all failure modes
- Idempotent processing at all layers
- Comprehensive error handling

---

## Testing Requirements

### Critical Test Scenarios

1. **RabbitMQ unavailable at startup**
   - System starts successfully
   - Outbox events created
   - Events published when RabbitMQ available

2. **RabbitMQ crashes during operation**
   - Detect connection loss
   - Automatic reconnection
   - Consumer recovery
   - Events published after reconnection

3. **API crashes after database commit**
   - Outbox event persisted
   - Dispatcher picks up event
   - Withdrawal processes normally

4. **Duplicate message delivery**
   - Worker receives same message twice
   - Atomic status transition prevents duplicate processing
   - Second attempt safely skipped

5. **Poison message**
   - Message fails 3 times
   - Routed to DLQ
   - Other messages continue processing

6. **Concurrent withdrawal requests**
   - Race condition prevented
   - Only one pending withdrawal created
   - Second request rejected

7. **Worker crashes after Paystack succeeds**
   - Message redelivered
   - Status check prevents duplicate transfer
   - Finalization completes

---

## Documentation Deliverables

1. ✅ **AUDIT_01_DATABASE.md** - Database layer analysis
2. ✅ **AUDIT_02_RABBITMQ.md** - RabbitMQ implementation analysis
3. ✅ **AUDIT_03_WALLET_WITHDRAWAL.md** - Withdrawal flow analysis
4. ✅ **AUDIT_04_PAYSTACK.md** - Payment provider integration
5. ✅ **AUDIT_05_RECONCILIATION.md** - Reconciliation analysis
6. ✅ **AUDIT_06_NOTIFICATIONS.md** - Notification services
7. ✅ **AUDIT_SUMMARY.md** - This document
8. ⏭️ **IMPLEMENTATION_PLAN.md** - Detailed implementation plan (NEXT)

---

## Conclusion

PropVest has a **strong engineering foundation** that is **90% production-ready**. The missing 10% is the **transactional outbox pattern** and **RabbitMQ reliability improvements**.

### Key Takeaway

> The system demonstrates mature practices (migrations, constraints, fund locking, idempotent finalization), but one architectural gap (publish outside transaction) creates a critical reliability window. Implementing the transactional outbox pattern closes this gap and makes the system truly production-ready for financial operations.

### Recommendation

✅ **Proceed with outbox implementation** as outlined in this audit

**Estimated Effort:**
- Database changes: 1-2 days
- Outbox implementation: 2-3 days
- RabbitMQ improvements: 2-3 days
- Idempotency layer: 1-2 days
- Testing: 2-3 days
- **Total: 8-13 days**

---

**Phase 1 Complete:** ✅ All audits finished  
**Next Phase:** Create detailed implementation plan  
**Status:** Ready for implementation planning
