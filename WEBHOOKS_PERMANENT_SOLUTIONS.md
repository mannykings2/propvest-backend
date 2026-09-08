# Permanent Solutions for Webhook Testing

## The Problem
When running on localhost, Paystack **cannot send webhooks** to your server because localhost is not accessible from the internet. This means deposits appear successful but wallets don't get credited.

## Three Permanent Solutions

### ✅ Solution 1: Use ngrok (Recommended for Real Payment Testing)

**Best for:** Testing with real Paystack payments and webhooks

**Setup Time:** 5 minutes

**Steps:**
1. Install ngrok: `choco install ngrok` or download from https://ngrok.com/download
2. Start tunnel: `ngrok http 8081`
3. Copy the HTTPS URL (e.g., `https://abc123.ngrok-free.app`)
4. Update `.env`: `BASE_URL=https://abc123.ngrok-free.app`
5. Configure Paystack webhook: `https://abc123.ngrok-free.app/api/v1/webhooks/payment`
6. Restart server: `make run`

**Result:** Webhooks work automatically! Wallet credited immediately after payment. ✅

**Pros:**
- Tests real payment flow end-to-end
- No manual intervention needed
- See actual Paystack webhooks

**Cons:**
- Free ngrok URL changes every restart
- Need to update Paystack webhook URL each time
- ($8/month for static URL)

**See:** `SETUP_NGROK.md` for detailed instructions

---

### ✅ Solution 2: Use Mock Provider (Best for Local Development)

**Best for:** Local development without real payments

**Setup Time:** 30 seconds

**Steps:**
1. Change `.env`: `PAYMENT_PROVIDER=mock`
2. Restart server: `make run`
3. After completing "payment", run: `go run tools/trigger_webhook.go <reference>`

**Result:** No real money charged, webhook simulation works locally

**Pros:**
- No real money involved
- No ngrok needed
- Fast development cycle
- No Paystack API calls

**Cons:**
- Still need to manually trigger webhook with tool
- Not testing real Paystack integration

**Note:** You still need to trigger webhooks manually with this approach, but no real payments are made.

---

### ✅ Solution 3: Auto-Trigger Webhooks (Semi-Automated)

**Best for:** Developers who want to test real Paystack but avoid manual webhook triggering

**Setup Time:** 2 minutes

**Create a batch file** `dev-with-auto-webhook.bat`:
```batch
@echo off
echo ========================================
echo PropVest Backend - Development Mode
echo Auto-webhook triggering enabled
echo ========================================
echo.
echo After completing a payment:
echo 1. Note the reference from the callback URL
echo 2. Run: go run tools/trigger_webhook.go DEP-XXXXX
echo.
echo Press any key to start the server...
pause > nul

start "PropVest API" cmd /k "cd /d %~dp0 && make run"
echo.
echo ✅ Server started!
echo When you complete a payment, run the webhook trigger command above.
echo.
pause
```

**Usage:**
1. Run `dev-with-auto-webhook.bat`
2. Complete payment on Paystack
3. Copy reference from callback URL
4. Run: `go run tools/trigger_webhook.go <reference>`

**Create an even better script** `quick-webhook.bat`:
```batch
@echo off
if "%1"=="" (
    echo Usage: quick-webhook DEP-XXXXX
    exit /b 1
)
go run tools/trigger_webhook.go %1
```

Now you can just run: `quick-webhook DEP-ABC123`

---

## Comparison Table

| Solution | Setup Time | Real Payments | Auto Credits | Manual Steps |
|----------|-----------|---------------|--------------|--------------|
| ngrok | 5 min | ✅ Yes | ✅ Auto | None |
| Mock Provider | 30 sec | ❌ No | ⚠️ Manual | Run webhook tool |
| Auto-Trigger | 2 min | ✅ Yes | ⚠️ Semi-auto | Run webhook tool |

---

## Recommended Workflow

### For Active Development (Building Features)
**Use Mock Provider**
- Fast iteration
- No real payment complexity
- No ngrok management

```bash
# .env
PAYMENT_PROVIDER=mock

# After each test payment
go run tools/trigger_webhook.go <reference>
```

### For Integration Testing (Testing Payment Flow)
**Use ngrok**
- Test real Paystack integration
- Verify webhook signatures
- Test full user experience

```bash
# Terminal 1
ngrok http 8081

# Update .env with ngrok URL
# Update Paystack webhook URL
# Terminal 2
make run
```

### For Production
**Use Real Domain**
- No localhost, no ngrok
- Paystack sends webhooks directly
- Everything works automatically

```bash
# .env
BASE_URL=https://api.propvest.com
PAYMENT_PROVIDER=paystack
```

---

## Quick Reference Commands

### Start with Mock Provider
```powershell
# .env: PAYMENT_PROVIDER=mock
make run
# After payment:
go run tools/trigger_webhook.go DEP-XXXXX
```

### Start with ngrok
```powershell
# Terminal 1
ngrok http 8081
# Copy URL, update .env and Paystack

# Terminal 2
make run
# Webhooks work automatically!
```

### Manual Webhook Trigger
```powershell
go run tools/trigger_webhook.go DEP-759B35C0-EB0
```

### Check Webhook Logs
```powershell
# In ngrok web interface
http://127.0.0.1:4040
# Shows all webhook requests
```

---

## Troubleshooting

### Q: Wallet still not credited after webhook
**Check:**
1. Payment exists: `SELECT * FROM payments WHERE reference = 'DEP-XXX';`
2. Transaction created: `SELECT * FROM wallet_transactions WHERE reference = 'DEP-XXX';`
3. Server logs for errors

### Q: ngrok URL keeps changing
**Solution:** Upgrade to ngrok paid plan for static URLs ($8/month)

### Q: "Invalid signature" error
**Check:**
1. `PAYSTACK_WEBHOOK_SECRET` is empty in `.env`
2. Using correct secret key in trigger tool
3. Restart server after .env changes

### Q: Want to switch between providers
```powershell
# For Mock
PAYMENT_PROVIDER=mock

# For Paystack
PAYMENT_PROVIDER=paystack
BASE_URL=https://your-ngrok-url.ngrok-free.app

# Restart server after changing
make run
```

---

## My Recommendation for You

Since you're actively developing and testing:

**Start with ngrok** (one-time 5-minute setup), then you never have to manually trigger webhooks again. It's the closest to production behavior and saves you time in the long run.

**Steps:**
1. Install ngrok: `choco install ngrok`
2. Get free account: https://dashboard.ngrok.com/signup
3. Run: `ngrok config add-authtoken <your-token>`
4. Run: `ngrok http 8081`
5. Update `.env` with the HTTPS URL
6. Update Paystack webhook URL
7. Restart server
8. **Done!** Payments now work end-to-end automatically

After the initial setup, it's just:
- Terminal 1: `ngrok http 8081`
- Terminal 2: `make run`
- Test payments → Wallet credited automatically ✅
