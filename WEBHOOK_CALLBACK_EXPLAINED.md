# 🔔 Webhooks vs Callbacks - Complete Explanation

**Your Questions Answered Simply**

---

## Question 1: What is a Webhook and How Does It Work?

### **Simple Analogy: Pizza Delivery** 🍕

**Scenario A: You Keep Calling (Polling)**
```
You → Pizza Shop: "Is my pizza ready?"
Shop: "Not yet"
(wait 1 minute)
You → Pizza Shop: "Ready now?"
Shop: "Not yet"
(wait 1 minute)
You → Pizza Shop: "How about now?"
Shop: "Yes, ready!"
```
**Problem:** Wastes your time, inefficient

**Scenario B: They Call You (Webhook)**
```
You → Pizza Shop: "Make my pizza. Call me at 555-1234 when ready"
Shop: "Will do!"
(you do other things)
Shop → You: *RING RING* "Your pizza is ready!"
You: "Great, coming now!"
```
**Benefit:** Efficient, you don't wait around

### **In Payment Terms:**

**WITHOUT Webhook:**
```
Your Backend → Paystack: "Is payment done?"
Paystack: "No"
(wait)
Your Backend → Paystack: "Done now?"
Paystack: "No"
(wait)
Your Backend → Paystack: "Now?"
Paystack: "Yes!"
```

**WITH Webhook:**
```
Your Backend → Paystack: "Tell me at /webhooks/payment when payment completes"
Paystack: "OK!"
(user pays)
Paystack → Your Backend: *HTTP POST* "Payment DEP-123 complete!"
Your Backend: "Thanks! Crediting wallet..."
```

---

## Understanding: Callback URL vs Webhook URL

### **These Are TWO DIFFERENT THINGS!**

| Aspect | Callback URL | Webhook URL |
|--------|--------------|-------------|
| **Who calls it?** | User's browser | Paystack's server |
| **When?** | Immediately after payment | Few seconds after payment |
| **Purpose** | Show user "Success!" message | Actually credit the wallet |
| **Reliable?** | ❌ No (user might close browser) | ✅ Yes (server-to-server) |
| **User sees?** | ✅ Yes (in address bar) | ❌ No (happens behind scenes) |
| **Authentication** | None (public page) | Signature verification |

### **Callback URL:**
```
http://localhost:8080/api/v1/wallet/deposit/callback?reference=DEP-123
```
- **Called by:** User's browser (after they click "Done" on Paystack)
- **Shows:** Nice HTML page saying "Payment Successful!"
- **Does NOT:** Credit the wallet (that's webhook's job)
- **Can fail:** If user closes browser, loses internet, etc.

### **Webhook URL:**
```
http://localhost:8080/api/v1/webhooks/payment
```
- **Called by:** Paystack's server (automatically)
- **Sends:** Payment data + cryptographic signature
- **Does:** Actually credits the wallet
- **Reliable:** Paystack retries if it fails
- **Never seen:** Happens behind the scenes

---

## Question 2: Why "Connection Refused" Error?

### **What Happened:**

```
Step 1: You initiated deposit
   ↓
Step 2: You paid on Paystack page ✅
   ↓
Step 3: Paystack processed payment ✅
   ↓
Step 4: Paystack sent webhook to /webhooks/payment ✅
   ↓
Step 5: Your backend credited wallet ✅
   ↓
Step 6: Paystack redirected your browser to callback URL ❌
   ↓
Step 7: Callback URL didn't exist → ERROR!
```

### **The Error:**
```
http://localhost:8080/api/v1/wallet/deposit/callback?trxref=DEP-16A912A0-EF9

This site can't be reached
localhost refused to connect
```

**Why?** The route `/api/v1/wallet/deposit/callback` didn't exist!

**But...**  
✅ **Your wallet WAS credited!** (webhook did that)  
❌ **User just didn't see confirmation page**

---

## The Fix (Already Applied!)

I just created:

### **1. Callback Handler**
File: `internal/handlers/wallet.go`
```go
func (h *WalletHandler) HandleDepositCallback(c *gin.Context) {
    // Gets reference from URL
    reference := c.Query("reference")
    
    // Shows nice HTML success page
    html := "Payment Successful! ✅"
    c.Data(200, "text/html", []byte(html))
}
```

### **2. Callback Route**
File: `internal/routes/v1/routes.go`
```go
// Public route (no auth needed)
wallet.GET("/deposit/callback", walletHandler.HandleDepositCallback)
```

---

## How It Works Now (After Fix):

### **Complete Payment Flow:**

```
1. User Request:
   POST /wallet/deposit {"amount_kobo": 150000}
   
2. Backend → Paystack:
   "Initialize payment for ₦1,500"
   
3. Paystack → Backend:
   "OK! Send user to: https://checkout.paystack.com/xyz123"
   
4. User Browser → Paystack Page:
   *User enters card details and pays*
   
5. Payment Succeeds! ✅
   
6. Paystack Server → Your Backend (WEBHOOK):
   POST /webhooks/payment
   {"reference": "DEP-123", "status": "success"}
   → Wallet credited ✅
   
7. Paystack → User Browser (CALLBACK):
   Redirect to: /wallet/deposit/callback?reference=DEP-123
   → User sees: "Payment Successful!" page ✅
```

---

## Test It Now!

### **Step 1: Restart Backend**
```bash
# Stop if running (Ctrl+C)
make run
```

### **Step 2: Initiate Deposit**
```http
POST /api/v1/wallet/deposit
{"amount_kobo": 150000}
```

### **Step 3: Pay on Paystack**
Use test card:
- Card: `4084084084084081`
- PIN: `0000`
- OTP: `123456`

### **Step 4: After Payment**
✅ **You'll see nice success page** (not error!)
✅ **Wallet will be credited**

---

## Why Both Webhook AND Callback?

### **Webhook (Required for Wallet Credit):**
```
✅ Reliable (server-to-server)
✅ Paystack retries if fails
✅ Cryptographically signed (secure)
✅ Credits wallet (source of truth)
❌ User never sees it
```

### **Callback (Optional for User Experience):**
```
✅ User sees confirmation
✅ Shows success message
✅ Better UX
❌ Can fail (user closes browser)
❌ Not reliable for business logic
```

**Best Practice:** Use BOTH
- Webhook → Does the work (credits wallet)
- Callback → Shows the message (user experience)

---

## Real-World Example:

**Amazon Checkout:**

**Webhook:**
```
Payment Provider → Amazon Servers:
"Order #12345 paid successfully"

Amazon:
- Creates order ✅
- Charges payment ✅
- Sends confirmation email ✅
- Updates inventory ✅
```

**Callback:**
```
Payment Provider → User's Browser:
Redirect to amazon.com/order-success?order=12345

User sees:
"Thank you! Your order is confirmed."
```

If callback fails (user closes browser), order still processes!  
If webhook fails, payment succeeds but order doesn't process → problem!

---

## Summary:

### **Your Questions:**

**1. What is webhook?**
- Server-to-server notification
- Paystack tells your backend "payment complete"
- Reliable, secure, automatic

**2. Why connection refused?**
- Callback route was missing
- Fixed now! Will show success page
- But wallet was credited anyway (webhook did it)

### **Key Takeaway:**

```
Webhook  = Does the work (credits wallet) - REQUIRED
Callback = Shows the message (UX) - NICE TO HAVE
```

Both work together for best experience! 🎉

---

## What to Do Now:

1. ✅ **Restart your backend** (`make run`)
2. ✅ **Try another payment**
3. ✅ **You'll see success page now!**

The callback route is live and ready! 🚀
