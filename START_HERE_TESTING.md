# 🚀 START HERE - Withdrawal Testing

## Quick Start (5 Minutes)

### 1. Start Everything (4 Terminals)

**Terminal 1: API**
```powershell
.\api.exe
```

**Terminal 2: Ngrok**
```powershell
ngrok http 8081
# Copy HTTPS URL: https://abc123.ngrok.io
```

**Terminal 3: Worker**
```powershell
.\worker.exe
```

**Terminal 4: Testing**
```powershell
# Set your token
$TOKEN = "YOUR_ACCESS_TOKEN"
```

### 2. Configure Paystack (One-Time)

1. **Add Webhook**: https://dashboard.paystack.com/settings/webhooks
   - URL: `https://YOUR_NGROK_URL/api/v1/webhooks/paystack/transfer`
   - Events: `transfer.success`, `transfer.failed`, `transfer.reversed`

2. **Enable Transfers**: https://dashboard.paystack.com/settings/transfers
   - Click "Enable Transfers"
   - Set Transfer PIN

### 3. Test Withdrawal

```powershell
# In Terminal 4

# Check balance
curl -X GET http://localhost:8081/api/v1/wallet -H "Authorization: Bearer $TOKEN"

# Initiate withdrawal
curl -X POST http://localhost:8081/api/v1/wallet/withdraw `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "amount_kobo": 5000000,
    "account_number": "0123456789",
    "account_name": "Test User",
    "bank_code": "058",
    "bank_name": "GTBank"
  }'

# Wait 1-5 minutes...

# Check final balance
curl -X GET http://localhost:8081/api/v1/wallet -H "Authorization: Bearer $TOKEN"
```

### 4. Watch the Logs

**Terminal 1 (API):** Webhook received ✓  
**Terminal 2 (Ngrok):** POST /webhooks/paystack/transfer 200 ✓  
**Terminal 3 (Worker):** Processing withdrawal ✓

---

## What You Should See

### Before Withdrawal
```json
{
  "main_balance": 10000000,    // ₦100,000
  "locked_balance": 0,
  "available": 10000000
}
```

### After Initiation (Immediate)
```json
{
  "main_balance": 10000000,    // ₦100,000 (unchanged)
  "locked_balance": 5000000,   // ₦50,000 (LOCKED) ⭐
  "available": 5000000         // ₦50,000 (can't spend locked)
}
```

### After Completion (1-5 min via webhook)
```json
{
  "main_balance": 5000000,     // ₦50,000 (DEBITED) ⭐
  "locked_balance": 0,         // Cleared ⭐
  "available": 5000000         // ₦50,000
}
```

---

## Detailed Guides

📖 **Full Testing Guide**: `WITHDRAWAL_TESTING_GUIDE.md`  
⚡ **Quick Reference**: `QUICK_TEST_REFERENCE.md`  
🏗️ **Architecture**: `WITHDRAWAL_ARCHITECTURE.md`  
✅ **Implementation Status**: `WITHDRAWAL_IMPLEMENTATION_STATUS.md`

---

## Common Issues

### "Webhook not received"
```powershell
# Manually trigger webhook
cd tools
go run trigger_transfer_webhook.go --reference WD-ABC123 --event success
```

### "Worker not processing"
- Check Terminal 3 - should show "withdrawal processor started"
- If RabbitMQ is down, reconciliation will process in 10 minutes
- Restart worker to force immediate reconciliation

### "Insufficient balance"
```powershell
# Credit test wallet
docker exec -it propvest-postgres psql -U propvest -d propvest
UPDATE wallets SET main_balance = 10000000 WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');
\q
```

---

## Need Help?

1. Check logs in each terminal
2. Verify services are running: `docker ps`
3. Check Paystack dashboard: https://dashboard.paystack.com/transfers
4. Read detailed guide: `WITHDRAWAL_TESTING_GUIDE.md`

---

## Testing Tools

### Trigger Test Webhook
```powershell
cd tools
go run trigger_transfer_webhook.go --reference WD-ABC123 --event success
```

### Check Database
```powershell
docker exec -it propvest-postgres psql -U propvest -d propvest
SELECT * FROM wallet_transactions WHERE type = 'withdrawal' ORDER BY created_at DESC LIMIT 5;
\q
```

---

**Ready to test? Start with Terminal 1! 🚀**
