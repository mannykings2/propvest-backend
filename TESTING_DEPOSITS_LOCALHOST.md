# Testing Deposits on Localhost

## The Problem
When testing with real Paystack on localhost, Paystack **cannot send webhooks** to your local machine because it's not accessible from the internet.

## The Solution
Use the webhook trigger tool to manually simulate Paystack's webhook after completing a test payment.

## Step-by-Step Process

### 1. Initiate a Deposit
```bash
POST http://localhost:8081/api/v1/wallet/deposit
Headers:
  Authorization: Bearer <your_access_token>
  Content-Type: application/json
Body:
{
  "amount_kobo": 150000
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "authorization_url": "https://checkout.paystack.com/...",
    "reference": "DEP-XXXXX..."
  }
}
```

**Save the `reference` value!**

### 2. Complete Payment on Paystack
- Open the `authorization_url` in your browser
- Use Paystack test cards:
  - **Success:** `4084 0840 8408 4081` (any CVV, any future expiry)
  - **Insufficient Funds:** `5060 6666 6666 6666 66`
  - **Declined:** `5081 0814 0814 0814 08`
- Complete the payment

### 3. You'll See the Success Callback
After payment, you'll be redirected to:
```
http://localhost:8081/api/v1/wallet/deposit/callback?reference=DEP-XXXXX...
```

This shows a success message, but **your wallet is NOT credited yet** because the webhook hasn't been received.

### 4. Manually Trigger the Webhook
Run the webhook trigger tool with your payment reference:

```bash
go run tools/trigger_webhook.go DEP-XXXXX...
```

**Example:**
```bash
go run tools/trigger_webhook.go DEP-759B35C0-EB0
```

**Expected Output:**
```
✅ Webhook sent!
Status: 200
Response: {"success":true,"message":"Webhook processed successfully",...}

✅ Success! Your wallet should now be credited.
Check your wallet balance with: GET /api/v1/wallet
```

### 5. Verify Wallet Balance
```bash
GET http://localhost:8081/api/v1/wallet
Headers:
  Authorization: Bearer <your_access_token>
```

**Response:**
```json
{
  "success": true,
  "data": {
    "main_balance": 150000,
    "main_balance_formatted": "₦1,500.00",
    ...
  }
}
```

## Important Notes

### Why Two Steps?
- **Callback URL** = Shows success message to user (UX)
- **Webhook** = Actually credits the wallet (backend processing)

In production with ngrok or a real domain, Paystack sends the webhook automatically and you don't need step 4.

### Idempotency
The webhook is idempotent - you can trigger it multiple times with the same reference and the wallet will only be credited once.

### Testing Scenarios

#### Test Case 1: Normal Deposit
1. Initiate deposit → Get reference `DEP-ABC123`
2. Pay on Paystack → See callback page
3. Trigger webhook → `go run tools/trigger_webhook.go DEP-ABC123`
4. Check wallet → Balance increased ✅

#### Test Case 2: Duplicate Webhook
1. After Test Case 1, trigger webhook again
2. `go run tools/trigger_webhook.go DEP-ABC123`
3. Check wallet → Balance unchanged (already processed) ✅

#### Test Case 3: Invalid Reference
1. Trigger webhook with fake reference
2. `go run tools/trigger_webhook.go DEP-FAKE999`
3. Server logs warning: "webhook for unknown payment reference"
4. Wallet unchanged ✅

## Production Setup

For production or testing with real webhooks, use **ngrok**:

```bash
# Install ngrok
choco install ngrok
# or download from https://ngrok.com/download

# Start ngrok
ngrok http 8081

# Update .env
BASE_URL=https://abc123.ngrok.io

# Configure webhook in Paystack Dashboard
https://dashboard.paystack.com/#/settings/developer
Webhook URL: https://abc123.ngrok.io/api/v1/webhooks/payment

# Restart server
make run
```

Now Paystack will send real webhooks and you don't need the trigger tool!

## Troubleshooting

### "Invalid signature" Error
- Check that `PAYSTACK_WEBHOOK_SECRET` is empty or matches your dashboard
- The tool uses `PAYSTACK_SECRET_KEY` for signing when webhook secret is empty

### "Invalid JSON payload" Error
- Fixed! Make sure you've restarted the server after the recent code changes

### "Webhook for unknown payment reference"
- The payment doesn't exist in the database
- Make sure you used the correct reference from step 1

### Wallet Not Credited After Webhook
- Check server logs for errors
- Verify the payment exists: `SELECT * FROM payments WHERE reference = 'DEP-XXX';`
- Check if transaction was created: `SELECT * FROM wallet_transactions WHERE reference = 'DEP-XXX';`
