# Outbox Pattern Testing Guide

**Date:** 2026-10-06  
**Purpose:** Manual testing guide for outbox + idempotency implementation  
**Time Required:** 15-20 minutes for all tests

---

## Overview

This guide provides simple, practical tests to verify the outbox pattern works correctly. No complex test framework needed—just run the app and verify with database queries.

---

## Prerequisites (5 minutes)

### 1. Start Services

```powershell
# Start PostgreSQL and RabbitMQ
docker-compose up -d postgres rabbitmq

# Wait for services to be ready (check logs)
docker-compose logs -f postgres
# Wait for "database system is ready to accept connections"
```

### 2. Run Migrations

```powershell
# Set database URL (local port 5435)
$env:DATABASE_URL = "postgres://propvest:password@localhost:5435/propvest?sslmode=disable"

# Run migrations
migrate -path internal/database/migrations -database $env:DATABASE_URL up

# Verify migrations
migrate -path internal/database/migrations -database $env:DATABASE_URL version
# Expected: 15 (includes outbox migrations)
```

### 3. Start API and Worker

**Terminal 1 (API):**
```powershell
go run cmd/api/main.go
# Look for: "✓ outbox repository initialized"
```

**Terminal 2 (Worker):**
```powershell
go run cmd/worker/main.go
# Look for: "✓ outbox dispatcher started"
```

---

## Test 1: Normal Withdrawal (Happy Path)

**What it tests:** End-to-end flow works correctly

**Time:** 2 minutes

### Steps

**1. Create a withdrawal:**
```powershell
curl -X POST http://localhost:8080/api/v1/wallet/withdraw `
  -H "Authorization: Bearer YOUR_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount": 10000,
    "bank_code": "058",
    "account_number": "0123456789"
  }'
```

**Expected Response:**
```json
{
  "status": "success",
  "message": "Withdrawal initiated",
  "data": {
    "reference": "WD-xxx",
    "status": "pending"
  }
}
```

**2. Check outbox event created:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT id, event_type, status, created_at FROM outbox_events ORDER BY created_at DESC LIMIT 1;"
```

**Expected Output:**
```
                  id                  |    event_type      |  status   |         created_at
--------------------------------------+--------------------+-----------+----------------------------
 <uuid>                               | withdrawal.process | published | 2026-10-06 21:00:00
```

**3. Check worker logs:**

Look for these log messages in the worker terminal:
```
📤 processing outbox batch count=1
✓ event published event_id=<uuid> queue=propvest.withdrawal.process
processing withdrawal message
```

### ✅ Success Criteria

- [x] API returns 200 OK with withdrawal details
- [x] Outbox event exists in database
- [x] Event status changes from 'pending' → 'claimed' → 'published'
- [x] Worker processes withdrawal
- [x] No errors in logs

---

## Test 2: Outbox Survives RabbitMQ Down

**What it tests:** Messages never lost when RabbitMQ unavailable

**Time:** 3 minutes

### Steps

**1. Stop RabbitMQ:**
```powershell
docker-compose stop rabbitmq
```

**2. Create withdrawal (should still work!):**
```powershell
curl -X POST http://localhost:8080/api/v1/wallet/withdraw `
  -H "Authorization: Bearer YOUR_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount": 10000,
    "bank_code": "058",
    "account_number": "0123456789"
  }'
```

**Expected:** API still returns 200 OK (withdrawal created successfully)

**3. Verify outbox event created:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT id, status, attempts, available_at FROM outbox_events WHERE status='pending' ORDER BY created_at DESC LIMIT 1;"
```

**Expected Output:**
```
                  id                  |  status  | attempts |       available_at
--------------------------------------+----------+----------+----------------------------
 <uuid>                               | pending  |        0 | 2026-10-06 21:00:00
```

**4. Check worker logs:**

You should see dispatcher trying to publish:
```
failed to publish: connection refused
scheduling event retry, attempts=1, backoff=30s
```

**5. Restart RabbitMQ:**
```powershell
docker-compose start rabbitmq

# Wait for RabbitMQ to be ready
docker-compose logs -f rabbitmq
# Look for "Server startup complete"
```

**6. Watch dispatcher logs (Terminal 2):**

Within 30 seconds, you should see:
```
✓ event published event_id=<uuid>
```

**7. Verify event published:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT id, status, published_at FROM outbox_events WHERE id='<uuid-from-step-3>';"
```

**Expected Output:**
```
                  id                  |   status   |       published_at
--------------------------------------+------------+----------------------------
 <uuid>                               | published  | 2026-10-06 21:05:30
```

### ✅ Success Criteria

- [x] Withdrawal succeeded even with RabbitMQ down
- [x] Event persisted in database with status='pending'
- [x] Dispatcher retried with backoff
- [x] Event published automatically when RabbitMQ came back
- [x] No data loss

**🎯 Key Insight:** This proves the outbox pattern works—messages are NEVER lost!

---

## Test 3: Duplicate Prevention (Idempotency)

**What it tests:** Database constraint prevents duplicate pending withdrawals

**Time:** 2 minutes

### Steps

**1. Create first withdrawal:**
```powershell
curl -X POST http://localhost:8080/api/v1/wallet/withdraw `
  -H "Authorization: Bearer YOUR_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount": 10000,
    "bank_code": "058",
    "account_number": "0123456789"
  }'
```

**Expected:** 200 OK

**2. Immediately try to create another withdrawal (same user):**
```powershell
curl -X POST http://localhost:8080/api/v1/wallet/withdraw `
  -H "Authorization: Bearer YOUR_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount": 5000,
    "bank_code": "058",
    "account_number": "9876543210"
  }'
```

**Expected Response:**
```json
{
  "status": "error",
  "message": "You already have a pending withdrawal"
}
```

**3. Verify only 1 pending withdrawal:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT COUNT(*) as pending_count FROM wallet_transactions WHERE user_id='<YOUR_USER_ID>' AND status='pending' AND type='withdrawal';"
```

**Expected Output:**
```
 pending_count
---------------
             1
```

**4. Verify database constraint working:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "\d+ wallet_transactions" | Select-String "idx_one_pending"
```

**Expected:** Shows the unique partial index

### ✅ Success Criteria

- [x] First withdrawal succeeds (200 OK)
- [x] Second withdrawal rejected (400 Bad Request)
- [x] Only 1 pending withdrawal exists
- [x] Database constraint prevents race condition

**🎯 Key Insight:** Idempotency works at database level—no duplicate withdrawals possible!

---

## Test 4: Retry Logic with Exponential Backoff

**What it tests:** Failed events retry automatically with increasing delays

**Time:** 5 minutes (mostly waiting)

### Steps

**1. Stop RabbitMQ:**
```powershell
docker-compose stop rabbitmq
```

**2. Create withdrawal:**
```powershell
curl -X POST http://localhost:8080/api/v1/wallet/withdraw ...
```

**3. Restart worker to trigger immediate retry attempt:**
```powershell
# In Terminal 2, press Ctrl+C, then:
go run cmd/worker/main.go
```

**4. Watch dispatcher logs:**

You should see:
```
failed to publish: connection refused
scheduling event retry, attempts=1, backoff=30s, next_attempt=2026-10-06 21:05:30
```

**5. Check event status:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT id, status, attempts, available_at, last_error FROM outbox_events WHERE status='pending' ORDER BY created_at DESC LIMIT 1;"
```

**Expected Output:**
```
                  id                  | status  | attempts |       available_at      | last_error
--------------------------------------+---------+----------+-------------------------+---------------------------
 <uuid>                               | pending |        1 | 2026-10-06 21:05:30     | failed to publish: ...
```

**6. Wait 30 seconds and watch logs:**

Dispatcher will retry automatically:
```
processing outbox batch count=1
failed to publish: connection refused
scheduling event retry, attempts=2, backoff=60s
```

**7. Verify attempts incrementing:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT attempts, available_at FROM outbox_events WHERE id='<uuid>';"
```

**Expected:** attempts=2, available_at is 60 seconds in future

**8. Restart RabbitMQ and watch success:**
```powershell
docker-compose start rabbitmq
# Wait for next retry, should see: ✓ event published
```

### ✅ Success Criteria

- [x] Dispatcher retries failed publishes
- [x] Backoff increases: 30s → 60s → 120s
- [x] Event unclaimed between retries (status='pending')
- [x] Attempts counter increments
- [x] Eventually publishes when RabbitMQ available

**🎯 Key Insight:** Automatic retry with exponential backoff handles transient failures!

---

## Test 5: Stale Event Recovery (Optional)

**What it tests:** Crashed dispatcher doesn't lose events

**Time:** 5 minutes

### Steps

**1. Create withdrawal and claim it:**
```powershell
# Withdrawal should be created and claimed by dispatcher
curl -X POST http://localhost:8080/api/v1/wallet/withdraw ...
```

**2. Stop worker immediately (simulate crash):**
```powershell
# In Terminal 2, press Ctrl+C while "processing outbox batch" is in logs
```

**3. Check for claimed events:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT id, status, claimed_at, claimed_by FROM outbox_events WHERE status='claimed' ORDER BY created_at DESC LIMIT 1;"
```

**Expected Output:**
```
                  id                  |  status  |       claimed_at        | claimed_by
--------------------------------------+----------+-------------------------+--------------
 <uuid>                               | claimed  | 2026-10-06 21:10:00     | worker-...
```

**4. Manually age the claim (simulate 5+ minutes passing):**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "UPDATE outbox_events SET claimed_at = NOW() - INTERVAL '10 minutes' WHERE status='claimed';"
```

**5. Restart worker:**
```powershell
go run cmd/worker/main.go
```

**6. Watch for recovery logs:**

Within 1 minute, you should see:
```
⚠️ recovered stale events count=1
```

**7. Verify event recovered:**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT id, status, claimed_at FROM outbox_events WHERE id='<uuid>';"
```

**Expected:** status='pending' or 'published', claimed_at=NULL

### ✅ Success Criteria

- [x] Event left in 'claimed' state after crash
- [x] Recovery loop detected stale event
- [x] Event reset to 'pending'
- [x] Event eventually published
- [x] No manual intervention needed

**🎯 Key Insight:** Stale recovery ensures crashed dispatchers don't lose events!

---

## Quick Health Check Queries

Use these anytime to check system health:

### Pending Events (should be 0 or low)
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT COUNT(*) as pending FROM outbox_events WHERE status='pending' AND available_at <= NOW();"
```

### Failed Events (should be 0)
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT COUNT(*) as failed FROM outbox_events WHERE status='failed';"
```

### Recent Events
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT id, event_type, status, attempts, created_at FROM outbox_events ORDER BY created_at DESC LIMIT 10;"
```

### Events by Status
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "SELECT status, COUNT(*) as count FROM outbox_events GROUP BY status;"
```

**Expected healthy output:**
```
  status   | count
-----------+-------
 published |   127
 pending   |     2
```

---

## What Each Test Proves

| Test | Proves | Time | Critical |
|------|--------|------|----------|
| **Test 1** | Normal flow works end-to-end | 2 min | ✅ YES |
| **Test 2** | Messages never lost (outbox persists) | 3 min | ✅ YES |
| **Test 3** | No duplicate withdrawals (idempotency) | 2 min | ✅ YES |
| **Test 4** | Automatic retry with backoff | 5 min | ⚠️ NICE TO HAVE |
| **Test 5** | Stale recovery works | 5 min | ⚠️ NICE TO HAVE |

**Minimum for MVP:** Tests 1, 2, and 3 (7 minutes)

---

## Expected Results Summary

### ✅ All Tests Pass = Production Ready!

If **Test 1, 2, and 3** pass, you have verified:
- ✅ Outbox pattern works correctly
- ✅ Messages never lost
- ✅ Idempotency prevents duplicates
- ✅ System is production-ready

Tests 4 and 5 are **nice-to-have** confirmations but not strictly necessary for deployment.

---

## Troubleshooting

### Test Fails: What to Check

**1. Migrations didn't run:**
```powershell
migrate -path internal/database/migrations -database $env:DATABASE_URL version
# Should show: 15
```

**2. Services not running:**
```powershell
docker-compose ps
# All should be "Up"

# Check API logs
# Look for: "✓ outbox repository initialized"

# Check Worker logs
# Look for: "✓ outbox dispatcher started"
```

**3. Database connection issues:**
```powershell
# Test connection
docker exec -it propvest_postgres psql -U propvest -d propvest -c "SELECT 1;"
# Should return: 1
```

**4. Check for errors in logs:**
```powershell
# API logs (Terminal 1)
# Worker logs (Terminal 2)
# Docker logs
docker-compose logs postgres
docker-compose logs rabbitmq
```

---

## After Testing

### Clean Up Test Data (Optional)

```powershell
# Delete test events
docker exec -it propvest_postgres psql -U propvest -d propvest -c `
  "DELETE FROM outbox_events WHERE created_at < NOW() - INTERVAL '1 hour';"

# Or keep for production monitoring
```

### Monitor in Production

```powershell
# Set up a cron job to run health checks
# Example: Check every 5 minutes
# */5 * * * * /path/to/health-check-script.sh
```

---

## Summary

**Time Required:** 15-20 minutes total (7 minutes for critical tests)

**What You've Proven:**
1. ✅ Outbox pattern prevents message loss
2. ✅ Idempotency prevents duplicate withdrawals
3. ✅ Automatic retry handles transient failures
4. ✅ Stale recovery handles crashed dispatchers
5. ✅ System is production-ready

**Next Steps:**
1. Run migrations in production database
2. Deploy updated API
3. Deploy updated Worker
4. Monitor using health check queries
5. Celebrate! 🎉

---

**Document Version:** 1.0  
**Last Updated:** 2026-10-06  
**Status:** ✅ Ready for use
