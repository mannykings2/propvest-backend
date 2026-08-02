# 🚀 PropVest API - Postman Collection Setup Guide

## 📦 **What's Included**

This folder contains everything you need to test the PropVest authentication API in Postman:

1. **`PropVest_API.postman_collection.json`** - Complete API collection with 7 requests
2. **`PropVest_Local.postman_environment.json`** - Environment variables for local testing
3. **`README.md`** - This setup guide

---

## 🎯 **Features**

✅ **7 Pre-configured Requests:**
- Health Check
- Register User
- Login
- Refresh Token (with rotation)
- Logout (Current Device)
- Logout All Devices
- Test Old Refresh Token (Verify Rotation)

✅ **Automatic Token Management:**
- Tokens automatically saved to environment variables
- No manual copy/paste needed
- Old tokens saved for testing rotation

✅ **Test Scripts:**
- Validates response structure
- Checks status codes
- Logs important information
- Verifies token rotation works

✅ **Complete Documentation:**
- Each request has detailed description
- Explains what happens behind the scenes
- Shows expected responses
- Security notes included

---

## 📥 **Step-by-Step Setup in Postman**

### **Step 1: Open Postman**

If you don't have Postman installed:
- Download from: https://www.postman.com/downloads/
- Or use Postman Web: https://web.postman.com/

### **Step 2: Import the Collection**

1. Click **Import** button (top left)
2. Click **Upload Files** or drag and drop
3. Select `PropVest_API.postman_collection.json`
4. Click **Import**

You should see "PropVest API - Authentication" collection appear in the left sidebar.

### **Step 3: Import the Environment**

1. Click **Import** button again
2. Select `PropVest_Local.postman_environment.json`
3. Click **Import**

### **Step 4: Activate the Environment**

1. Look at the top-right corner of Postman
2. Click the **environment dropdown** (says "No Environment")
3. Select **"PropVest Local"**

You should now see "PropVest Local" selected in the top-right.

### **Step 5: Verify Setup**

1. Expand the "PropVest API - Authentication" collection
2. You should see 7 requests
3. Click on any request
4. Check the URL - should show `{{base_url}}/...` 
5. Hover over `{{base_url}}` - should show `http://localhost:8080/api/v1`

✅ **Setup Complete!**

---

## 🧪 **How to Test (Complete Flow)**

### **Prerequisites:**

Make sure your API server is running:

```bash
# Terminal 1: Start database
docker-compose up -d

# Terminal 2: Start API server
go run cmd/api/main.go
```

You should see:
```
✓ PropVest API successfully initialized
✓ Server starting on :8080
```

---

### **Test Flow (Run in This Order):**

#### **1️⃣ Health Check**

**Purpose:** Verify API server is running

1. Click on **"Health Check"** request
2. Click **Send**
3. Should get **200 OK**

**Expected Response:**
```json
{
  "status": "healthy",
  "message": "PropVest API is running"
}
```

✅ **If this works, your server is running!**

---

#### **2️⃣ Register User**

**Purpose:** Create a new account

1. Click on **"Register User"** request
2. **IMPORTANT:** Change the email in the request body to something unique:
   ```json
   {
     "full_name": "Your Name",
     "email": "yourname@example.com",  ← Change this!
     "password": "SecurePass123!",
     "phone": "+2348012345678"
   }
   ```
3. Click **Send**
4. Should get **201 Created**

**Expected Response:**
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid-here",
      "user_code": "USR-xxxxxxxx",
      "full_name": "Your Name",
      "email": "yourname@example.com",
      "phone": "+2348012345678",
      "kyc_status": "pending",
      "role": "investor",
      "created_at": "2026-08-01T10:30:00Z"
    },
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc...",
    "token_type": "Bearer"
  }
}
```

**What Happened:**
- ✅ User account created
- ✅ Wallet created (₦0.00 balance)
- ✅ Tokens generated and **automatically saved** to environment
- ✅ User is immediately logged in

**Check Environment Variables:**
1. Click the **eye icon** (👁️) in top-right
2. Look under "PropVest Local" environment
3. You should see `access_token` and `refresh_token` are now filled!

---

#### **3️⃣ Login**

**Purpose:** Login with existing credentials

1. Click on **"Login"** request
2. Make sure email/password match what you registered with
3. Click **Send**
4. Should get **200 OK**

**Expected Response:**
Same structure as registration - user + tokens

**What Happened:**
- ✅ Password verified
- ✅ New tokens generated
- ✅ Tokens **automatically saved** (replacing old ones)

---

#### **4️⃣ Refresh Token**

**Purpose:** Get new access token without logging in again

1. Click on **"Refresh Token"** request
2. Notice the body uses `{{refresh_token}}` variable
3. Click **Send**
4. Should get **200 OK**

**Expected Response:**
```json
{
  "success": true,
  "message": "Token refreshed successfully",
  "data": {
    "access_token": "new_access_token...",
    "refresh_token": "new_refresh_token...",
    "token_type": "Bearer"
  }
}
```

**What Happened (Token Rotation):**
- ✅ Old refresh token **REVOKED** (can't be used again)
- ✅ New access token generated
- ✅ New refresh token generated
- ✅ New tokens **automatically saved**
- ✅ Old token saved as `old_refresh_token` for testing

**Check the Console Tab:**
You should see logs explaining what happened.

---

#### **5️⃣ Test Old Refresh Token (Should Fail)**

**Purpose:** Verify token rotation security works

1. Click on **"Test Old Refresh Token"** request
2. Notice it uses `{{old_refresh_token}}`
3. Click **Send**
4. Should get **401 Unauthorized** ✅ This is CORRECT!

**Expected Response:**
```json
{
  "success": false,
  "error": "Invalid or expired token",
  "code": "invalid_token"
}
```

**Why This is Good:**
- ✅ Old token correctly rejected
- ✅ Token rotation is working
- ✅ Security feature active
- ✅ Prevents replay attacks

**If you got 200 instead:** ❌ Token rotation is broken (contact developer)

---

#### **6️⃣ Logout (Current Device)**

**Purpose:** Logout from this device only

1. Click on **"Logout (Current Device)"** request
2. Click **Send**
3. Should get **200 OK**

**Expected Response:**
```json
{
  "success": true,
  "message": "Logout successful"
}
```

**What Happened:**
- ✅ Refresh token revoked
- ✅ Can't refresh anymore
- ✅ Other devices still logged in (if any)

**Important:** Access token is still valid for ~15 minutes (can't be revoked - it's stateless)

---

#### **7️⃣ Logout All Devices**

**Purpose:** Logout from ALL devices simultaneously

1. **First**, run **Login** again to get fresh tokens
2. Then click on **"Logout All Devices"** request
3. Notice it uses **Bearer Token** auth (check the Auth tab)
4. Click **Send**
5. Should get **200 OK**

**Expected Response:**
```json
{
  "success": true,
  "message": "Logged out from all devices"
}
```

**What Happened:**
- ✅ ALL user's refresh tokens revoked
- ✅ Must login again on EVERY device
- ✅ Forces fresh authentication everywhere

**Use Cases:**
- User changes password (automatic)
- User clicks "Logout everywhere"
- Admin suspends account
- Security breach detected

---

## 🎨 **Understanding the Test Scripts**

Each request has a **Tests** tab with JavaScript that runs after the response:

```javascript
// Example from Register request
pm.test("Status code is 201", function () {
    pm.response.to.have.status(201);
});

// Save tokens to environment
var jsonData = pm.response.json();
pm.environment.set("access_token", jsonData.data.access_token);
pm.environment.set("refresh_token", jsonData.data.refresh_token);
```

**What These Do:**
- ✅ Validate response structure
- ✅ Check status codes
- ✅ Save tokens automatically
- ✅ Log useful information

**To View Test Results:**
Click the **Test Results** tab after sending a request.

---

## 🔍 **Understanding Token Flow**

### **Access Token (Short-lived: 15 minutes)**
- Used for API requests
- Sent in `Authorization: Bearer <token>` header
- Contains user_id and role
- Cannot be revoked (stateless)
- Expires automatically

### **Refresh Token (Long-lived: 30 days)**
- Used to get new access tokens
- Stored in database (hashed)
- Can be revoked manually
- Rotated on every use (security)
- Expires after 30 days

### **Token Rotation Security:**

```
User refreshes → Old token revoked → New tokens issued
                         ↓
              If stolen token used:
              - Attacker can only use ONCE
              - User's next refresh fails
              - User knows something is wrong
```

---

## 🐛 **Troubleshooting**

### **Problem: "Error: connect ECONNREFUSED"**

**Cause:** API server not running

**Fix:**
```bash
go run cmd/api/main.go
```

---

### **Problem: "Email already exists"**

**Cause:** Trying to register same email twice

**Fix:** Change the email in the Register request body

---

### **Problem: "Invalid email or password"**

**Cause:** Wrong credentials

**Fix:** Make sure email/password match what you registered

---

### **Problem: "{{base_url}} not resolving"**

**Cause:** Environment not selected

**Fix:** 
1. Top-right dropdown
2. Select "PropVest Local"

---

### **Problem: "Authorization header required"**

**Cause:** Access token not in request

**Fix:**
1. Check if token is in environment (eye icon 👁️)
2. Run Register or Login to get tokens
3. Make sure environment is selected

---

## 📊 **Expected Test Results Summary**

| Request | Status | Response Time | Tokens Saved |
|---------|--------|---------------|--------------|
| Health Check | 200 OK | < 50ms | No |
| Register | 201 Created | < 200ms | Yes ✅ |
| Login | 200 OK | < 150ms | Yes ✅ |
| Refresh | 200 OK | < 100ms | Yes ✅ |
| Test Old Token | 401 Unauthorized | < 50ms | No |
| Logout | 200 OK | < 100ms | No |
| Logout All | 200 OK | < 100ms | No |

---

## 🎯 **Next Steps After Testing**

Once all tests pass:

1. **✅ Milestone 1 Complete** - Authentication working!
2. **📱 Frontend Integration** - Implement login/register UI
3. **🔄 Milestone 2** - User profile management
4. **💰 Milestone 3** - Wallet system
5. **🏢 Milestone 4** - Property listings
6. **💼 Milestone 5** - Investment engine

---

## 📞 **Need Help?**

If something isn't working:

1. Check server logs in terminal
2. Check Postman console (bottom panel, click "Console")
3. Verify database is running: `docker ps`
4. Check migrations ran: `make migrate-status`

---

## 🎉 **Happy Testing!**

You now have a complete testing environment for the PropVest authentication API!

**Pro Tips:**
- Use the **Collection Runner** to run all requests in sequence
- Export environment after testing to save your tokens
- Duplicate requests to test error cases
- Add more test scripts as you learn Postman

---

**Last Updated:** August 2, 2026  
**API Version:** v1  
**Milestone:** 1 (Authentication Complete)
