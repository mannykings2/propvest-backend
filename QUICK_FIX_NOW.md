# 🚀 Quick Fix - Test Withdrawal Now!

## The Problem
Paystack test accounts **cannot make transfers/withdrawals**. You need a registered business account.

## The Solution
Use the **Mock Provider** instead (already built into your app).

---

## 3 Steps to Test Withdrawals Right Now

### Step 1: Stop Worker
```powershell
# In Terminal 2 (where worker is running)
# Press Ctrl+C to stop it
```

### Step 2: Clean Up Failed Withdrawal
```powershell
# Mark as failed
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallet_transactions SET status='failed' WHERE reference='WD-2CDA395F-C1F';"

# Unlock funds
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallets SET main_balance=main_balance+locked_balance, locked_balance=0 WHERE locked_balance>0;"
```

### Step 3: Restart Everything
```powershell
# Terminal 1: Restart API
# Press Ctrl+C, then:
go run cmd/api/main.go

# Should show:
[INFO] Payment provider: mock  ← Check this!

# Terminal 2: Restart Worker
go run cmd/worker/main.go
```

---

## ✅ Test Now!

**POST** `http://localhost:8081/api/v1/wallet/withdraw`

```json
{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "Test User"
}
```

**Expected Result:**
- ✅ API: "Withdrawal queued"
- ✅ Worker: "Transfer completed immediately"
- ✅ Status: `completed` (instant!)
- ✅ Balance: Reduced by 50000

---

## 🎉 It Should Work Now!

Check transaction history:
```
GET http://localhost:8081/api/v1/wallet/transactions
```

You should see status = `completed`!

---

**Read `PAYSTACK_ACCOUNT_LIMITATION.md` for full details.**
