# Quick Test Commands Reference

Fast copy-paste commands for testing withdrawals.

## 🚀 Quick Start (3 Commands)

```powershell
# 1. Start services
docker-compose up -d

# 2. Start API (watch logs here!)
go run cmd/api/main.go

# 3. In new terminal - Get user ID
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, email FROM users LIMIT 1;"
```

## 💰 Credit Wallet Fast

```powershell
# Edit scripts/credit_wallet.sql first, then:
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -f scripts/credit_wallet.sql
```

## 🔍 Debug Commands

```powershell
# Check wallet balance
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT email, main_balance/100.0 as balance_ngn, locked_balance/100.0 as locked_ngn FROM wallets w JOIN users u ON u.id = w.user_id;"

# View recent transactions
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT type, amount/100.0 as amount_ngn, status, reference, created_at FROM wallet_transactions ORDER BY created_at DESC LIMIT 5;"

# Check pending withdrawals
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "SELECT id, reference, amount/100.0, status, created_at FROM wallet_transactions WHERE type='withdrawal' AND status='pending';"

# Test Paystack API access
curl -H "Authorization: Bearer sk_test_YOUR_TEST_SECRET_KEY" https://api.paystack.co/bank
```

## 🧹 Quick Fixes

```powershell
# Cancel all pending withdrawals
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallet_transactions SET status='failed' WHERE type='withdrawal' AND status='pending';"

# Unlock all funds
$env:PGPASSWORD='password'; psql -h localhost -p 5435 -U propvest -d propvest -c "UPDATE wallets SET main_balance=main_balance+locked_balance, locked_balance=0 WHERE locked_balance>0;"

# Clear rate limit
docker exec -it propvest_redis redis-cli FLUSHDB
```

## 📬 Postman Quick Test

```
POST http://localhost:8081/api/v1/wallet/withdraw
Headers:
  Authorization: Bearer YOUR_TOKEN
  Content-Type: application/json

Body:
{
  "amount": 50000,
  "bank_code": "058",
  "account_number": "0123456789",
  "account_name": "Test User"
}
```

## 🎯 Watch Logs

Your API terminal should show:
```
[INFO] Withdrawal request received
[INFO] Resolving bank account
[INFO] Account verified
[INFO] Withdrawal initiated
```

If you see `[ERROR]` - that's your problem! Read the error message.

