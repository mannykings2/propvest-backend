# 📮 Postman Quick Start - 5 Minutes

## Step 1: Import Collection (30 seconds)

1. **Open Postman**
2. **Click "Import"** button (top left)
3. **Drag and drop** this file: `postman/PropVest_Withdrawal_Collection.json`
4. **Click "Import"**

**Done!** You now have all requests ready to use.

---

## Step 2: Start Services (1 minute)

Open 3 terminals:

**Terminal 1:**
```powershell
.\api.exe
```

**Terminal 2:**
```powershell
ngrok http 8081
```

**Terminal 3:**
```powershell
.\worker.exe
```

---

## Step 3: Login & Test (3 minutes)

### 3.1 Login
1. **Open collection:** "PropVest Withdrawal Testing"
2. **Open folder:** "1. Authentication"
3. **Click:** "Login"
4. **Click:** "Send" button

**✅ Success:** Access token saved automatically!

### 3.2 Check Balance
1. **Open folder:** "2. Wallet Operations"
2. **Click:** "Get Wallet Balance"
3. **Click:** "Send"

**See:** Your current balance (₦100,000)

### 3.3 Withdraw Money
1. **Open folder:** "3. Withdrawal Flow"
2. **Click:** "Initiate Withdrawal"
3. **Click:** "Send"

**✅ Success:** Withdrawal reference saved!

### 3.4 Check Locked Balance
1. **Click:** "Get Wallet Balance" (repeat step 3.2)
2. **Look at response:**
   ```json
   {
     "main_balance": 10000000,    // ₦100,000 (unchanged)
     "locked_balance": 5000000,   // ₦50,000 (LOCKED!) ⭐
   }
   ```

**Perfect!** Funds are locked and can't be spent twice.

### 3.5 Wait for Completion
1. **Wait 1-5 minutes** (for webhook)
2. **Click:** "Check Withdrawal Status"
3. **See status change:** `pending` → `completed`

### 3.6 Verify Final Balance
1. **Click:** "Get Wallet Balance" again
2. **See:**
   ```json
   {
     "main_balance": 5000000,     // ₦50,000 (debited!) ⭐
     "locked_balance": 0,         // Cleared! ⭐
   }
   ```

**🎉 Success! Withdrawal completed!**

---

## Step 4: Test Errors (1 minute)

1. **Open folder:** "4. Error Scenarios"
2. **Try each request:**
   - Insufficient Balance → 422 error
   - Below Minimum → 422 error
   - Invalid Account → 400 error

**All should return proper error messages!**

---

## Pro Tips

### See Console Output
- **View → Show Postman Console** (Alt+Ctrl+C)
- See detailed logs for each request

### View Response Time
- Look at bottom right: "Time: 45ms"
- Monitor API performance

### Save Responses
- Click "Save Response"
- Compare before/after states

### Use Visualize Tab
- Some requests show pretty formatted data
- Click "Visualize" tab in response

---

## Common Issues

### "401 Unauthorized"
**Solution:** Run the Login request again. Token expired after 15 minutes.

### "No balance showing"
**Solution:** Credit wallet via database:
```sql
UPDATE wallets SET main_balance = 10000000 WHERE user_id = (SELECT id FROM users WHERE email = 'testuser@example.com');
```

### "Webhook not arriving"
**Solution:** Use manual webhook tool:
```powershell
cd tools
go run trigger_transfer_webhook.go --reference WD-ABC123 --event success
```

---

## Next Steps

- ✅ Read full guide: `POSTMAN_WITHDRAWAL_TESTING.md`
- ✅ Check API logs in Terminal 1
- ✅ Monitor worker in Terminal 3
- ✅ View Paystack dashboard for real transfers

---

**That's it! You're ready to test withdrawals in Postman! 🚀**
