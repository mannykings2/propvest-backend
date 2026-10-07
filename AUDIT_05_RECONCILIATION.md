# Audit Report 05: Reconciliation

**Date:** 2026-10-06  
**Phase:** 1 - Audit & Analysis  
**Step:** 1.5 - Reconciliation Audit

---

## Executive Summary

Reconciliation is **well-implemented and serves its purpose** as a safety net. It is currently the **only reliable mechanism** for processing withdrawals when webhooks fail or RabbitMQ is unavailable.

✅ **Periodic polling** - Every 5 minutes  
✅ **Grace period** - 10 minutes before reconciling  
✅ **Status verification** - Checks with Paystack  
✅ **Idempotent finalization** - Safe to run multiple times  
⚠️ **Currently critical** - Primary fallback when async fails  
⚠️ **Should be secondary** - After outbox, just a safety net  

---

## Current Implementation

### Reconciliation Worker

**File:** `cmd/worker/main.go`

```go
func startReconciliationWorker(ctx context.Context, db *gorm.DB, provider payments.Provider, walletService services.WalletService) func() {
    ticker := time.NewTicker(5 * time.Minute)
    done := make(chan struct{})
    
    go func() {
        // Run immediately on startup
        reconcilePendingWithdrawals(ctx, db, provider, walletService)
        
        // Then run every 5 minutes
        for {
            select {
            case <-ticker.C:
                reconcilePendingWithdrawals(ctx, db, provider, walletService)
            case <-done:
                ticker.Stop()
                return
            }
        }
    }()
    
    return func() { close(done) }
}
```

**Analysis:**
- ✅ Runs on worker startup (catches existing pending)
- ✅ Periodic execution (every 5 minutes)
- ✅ Graceful shutdown support
- ✅ Simple and reliable

---

### Reconciliation Logic

```go
func reconcilePendingWithdrawals(ctx context.Context, db *gorm.DB, provider payments.Provider, walletService services.WalletService) {
    log := logger.FromContext(ctx)
    log.Info("🔄 running reconciliation check...")
    
    // Find pending withdrawals older than 10 minutes
    cutoff := time.Now().Add(-10 * time.Minute)
    
    var txns []models.WalletTransaction
    err := db.Where("type = ? AND status = ? AND created_at < ?", 
        "withdrawal", "pending", cutoff).
        Find(&txns).Error
    
    if err != nil {
        log.Error("reconciliation query failed", "error", err)
        return
    }
    
    if len(txns) == 0 {
        log.Info("✓ no pending withdrawals to reconcile")
        return
    }
    
    log.Info("found pending withdrawals", "count", len(txns))
    
    for _, txn := range txns {
        // Skip if no transfer code (transfer not yet initiated)
        if txn.ExternalReference == nil || *txn.ExternalReference == "" {
            log.Warn("skipping transaction without transfer code",
                "transaction_id", txn.ID,
                "reference", txn.Reference)
            continue
        }
        
        log.Info("verifying withdrawal status",
            "transaction_id", txn.ID,
            "transfer_code", *txn.ExternalReference)
        
        // Verify current status with Paystack
        result, err := provider.VerifyTransfer(ctx, *txn.ExternalReference)
        if err != nil {
            log.Error("verification failed",
                "transaction_id", txn.ID,
                "error", err)
            continue
        }
        
        log.Info("transfer status verified",
            "transaction_id", txn.ID,
            "status", result.Status)
        
        // Finalize based on status
        switch result.Status {
        case "success":
            log.Info("reconciling successful transfer", "transaction_id", txn.ID)
            if err := walletService.FinalizeWithdrawal(ctx, txn.ID, true, result.TransferCode, ""); err != nil {
                log.Error("finalization failed", "transaction_id", txn.ID, "error", err)
            }
            
        case "failed", "reversed":
            reason := result.FailureReason
            if reason == "" {
                reason = fmt.Sprintf("Transfer %s", result.Status)
            }
            log.Warn("reconciling failed transfer",
                "transaction_id", txn.ID,
                "reason", reason)
            if err := walletService.FinalizeWithdrawal(ctx, txn.ID, false, result.TransferCode, reason); err != nil {
                log.Error("finalization failed", "transaction_id", txn.ID, "error", err)
            }
            
        case "pending":
            log.Info("transfer still pending", "transaction_id", txn.ID)
            // Still pending - will check again next cycle
            
        default:
            log.Warn("unknown transfer status",
                "transaction_id", txn.ID,
                "status", result.Status)
        }
    }
    
    log.Info("✓ reconciliation complete")
}
```

---

## Reconciliation Flow

```
Every 5 minutes
      ↓
Find pending withdrawals
  WHERE type = 'withdrawal'
    AND status = 'pending'
    AND created_at < (now - 10 minutes)
      ↓
For each withdrawal:
      ↓
Has external_reference?
      ├─ NO → Skip (not yet initiated)
      └─ YES → Continue
      ↓
Call Paystack VerifyTransfer
      ↓
Switch on result.Status:
      ├─ "success" → FinalizeWithdrawal(success=true)
      ├─ "failed" → FinalizeWithdrawal(success=false)
      ├─ "reversed" → FinalizeWithdrawal(success=false)
      └─ "pending" → Skip (check again later)
      ↓
Move to next withdrawal
      ↓
Log "reconciliation complete"
```

---

## Analysis

### ✅ Strengths

1. **Safety Net for Missed Webhooks**
   - Webhooks can be missed (network issues, downtime)
   - Reconciliation ensures eventual consistency
   - Critical for production reliability

2. **Grace Period**
   - 10-minute wait before reconciling
   - Gives webhook time to arrive
   - Avoids redundant Paystack API calls

3. **Idempotent**
   - Calls `FinalizeWithdrawal` (already idempotent)
   - Safe to run multiple times
   - Safe for multiple worker instances

4. **Skips Uninitiated Withdrawals**
   - Checks for `external_reference`
   - Prevents calling Paystack for withdrawals not yet sent
   - Clean separation of concerns

5. **Comprehensive Logging**
   - Logs each step
   - Easy to debug issues
   - Clear visibility into reconciliation

6. **Error Handling**
   - Continues processing other withdrawals if one fails
   - Logs errors but doesn't crash
   - Robust against Paystack API failures

### ⚠️ Limitations

1. **Currently Too Critical**
   - **Should be:** Safety net (rare)
   - **Actually is:** Primary fallback when RabbitMQ down
   - With outbox: Role returns to safety net

2. **No Limit on Reconciliation Attempts**
   - Perpetually retries pending withdrawals
   - No "give up after X days" logic
   - Could accumulate ancient pending transactions

3. **No Rate Limiting**
   - Makes 1 Paystack API call per pending withdrawal
   - No batch limit
   - Could hit rate limits if many pending

4. **10-Minute Delay**
   - Acceptable for webhook backup
   - But if worker failed, user waits 10+ minutes
   - With outbox: Faster processing (seconds)

5. **No Alerting**
   - Reconciliation finding many pending = problem
   - Should alert if reconciling > X withdrawals
   - No metrics tracked

---

## Reconciliation Scenarios

### Scenario 1: Webhook Missed

```
T+0:00  User requests withdrawal
T+0:00  API creates pending transaction
T+0:00  RabbitMQ message sent
T+0:01  Worker initiates Paystack transfer
T+0:01  Paystack returns "pending"
T+0:05  Paystack processes transfer (success)
T+0:05  Webhook sent... ❌ MISSED (network issue)
T+10:00 Reconciliation runs
T+10:00 Verifies with Paystack → "success"
T+10:00 Finalizes withdrawal → completed ✅
```

**Result:** ✅ Recovered after 10 minutes

---

### Scenario 2: Worker Crashed Before Transfer

```
T+0:00  User requests withdrawal
T+0:00  API creates pending transaction
T+0:00  RabbitMQ publish fails (RabbitMQ down)
T+5:00  Reconciliation runs
T+5:00  Finds pending withdrawal
T+5:00  No external_reference → skips ⏭️
T+10:00 Reconciliation runs again
T+10:00 Still no external_reference → skips again ⏭️
T+∞    ❌ STUCK FOREVER
```

**Result:** ❌ Reconciliation CAN'T help (no transfer initiated)

**This is the critical gap** that outbox solves.

---

### Scenario 3: Duplicate Reconciliation

```
Worker A (5:00): Reconciliation runs
Worker A (5:00): Finds pending withdrawal ABC
Worker A (5:00): Verifies status → "success"
Worker A (5:00): Calls FinalizeWithdrawal(ABC, true)
Worker A (5:00): Transaction updated to "completed"

Worker B (5:01): Reconciliation runs
Worker B (5:01): Finds no pending (ABC already completed)
Worker B (5:01): Logs "no pending withdrawals"
```

**Result:** ✅ Safe (idempotent finalization)

---

### Scenario 4: Paystack Still Pending

```
T+0:00  Transfer initiated
T+10:00 Reconciliation: Paystack status = "pending"
T+15:00 Reconciliation: Paystack status = "pending"
T+20:00 Reconciliation: Paystack status = "pending"
T+60:00 Reconciliation: Paystack status = "success"
T+60:00 Finalized ✅
```

**Result:** ✅ Eventually consistent (Paystack can take time)

---

## Role in Outbox Architecture

### Current Role (Without Outbox)

```
RabbitMQ Failed → Reconciliation is PRIMARY recovery
Webhook Missed → Reconciliation is PRIMARY recovery
Worker Crashed → Reconciliation is PRIMARY recovery (if transfer initiated)
```

**Criticality:** 🔴 **HIGH** (essential for system operation)

---

### Future Role (With Outbox)

```
Outbox Dispatcher → RabbitMQ → Worker → Paystack
      ↓               ↓         ↓         ↓
   Retries       Persistent  Idempotent  Status
      ↓               ↓         ↓         ↓
 99.9% Success   Reliable   Processing   Updates
                                           ↓
                                        Webhook
                                           ↓
                                     Finalization
                                           ↓
                                  Reconciliation (rare!)
                                     ↑
                              "Safety Net"
                          Catches edge cases
```

**Criticality:** 🟡 **MEDIUM** (important safety net, rarely used)

---

## Recommendations

### 🟡 Important (Post-Outbox)

1. **Add alerting for high reconciliation counts**
   ```go
   if len(txns) > 10 {
       alert.Send("High reconciliation count", map[string]interface{}{
           "count": len(txns),
           "message": "Many pending withdrawals found, investigate queue/outbox issues",
       })
   }
   ```

2. **Add age limit for pending withdrawals**
   ```go
   // After 7 days, mark as failed and alert
   if txn.CreatedAt.Before(time.Now().Add(-7 * 24 * time.Hour)) {
       log.Error("withdrawal too old, marking failed", "transaction_id", txn.ID)
       FinalizeWithdrawal(txn.ID, false, "", "Timeout: no status update after 7 days")
       alert.Send("Ancient withdrawal failed", ...)
   }
   ```

3. **Add metrics**
   ```go
   metrics.RecordGauge("reconciliation.pending_count", len(txns))
   metrics.RecordCounter("reconciliation.finalized_count", finalized)
   metrics.RecordCounter("reconciliation.still_pending_count", stillPending)
   ```

4. **Add rate limiting**
   ```go
   // Limit to 100 reconciliations per cycle
   if len(txns) > 100 {
       log.Warn("too many pending, limiting reconciliation to 100")
       txns = txns[:100]
   }
   ```

### 🟢 Nice to Have

5. **Add reconciliation history table**
   - Track each reconciliation attempt
   - Record Paystack response
   - Useful for debugging

6. **Add webhook vs reconciliation metrics**
   - Track: "Finalized by webhook: X, by reconciliation: Y"
   - High reconciliation % = webhook problem

---

## Conclusion

Reconciliation is **well-implemented** and currently serves as a **critical fallback**. With outbox implementation, its role will **correctly shift to being a safety net** rather than a primary mechanism.

### Key Findings

✅ **Solid implementation** - Idempotent, logged, robust  
✅ **Necessary safety net** - Catches missed webhooks  
✅ **Good design choices** - Grace period, skip uninitiated  
⚠️ **Currently overloaded** - Does too much due to lack of outbox  
⚠️ **No alerting** - Should alert on anomalies  

### With Outbox

**Expected reconciliation rate:** < 1% of withdrawals  
**Reasons reconciliation triggers:**
- Rare webhook miss (network blip)
- Outbox dispatcher restart during processing
- Edge case failures

**High reconciliation rate would indicate:** Outbox or webhook problems

---

**Status:** ✅ Complete  
**Next:** AUDIT_06_NOTIFICATIONS.md
