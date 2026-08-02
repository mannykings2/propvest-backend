# ⚡ Quick Start Guide - 2 Minutes to Testing

## 📥 **1. Import into Postman (30 seconds)**

### Method A: Drag and Drop (Easiest)
1. Open Postman
2. Drag both JSON files into Postman window:
   - `PropVest_API.postman_collection.json`
   - `PropVest_Local.postman_environment.json`
3. Done!

### Method B: Import Button
1. Click **Import** (top left)
2. Select both JSON files
3. Click **Import**

---

## 🎯 **2. Select Environment (10 seconds)**

1. Top-right corner: Click dropdown that says "No Environment"
2. Select **"PropVest Local"**
3. Done!

---

## 🚀 **3. Start Your Server (30 seconds)**

```bash
# Start database
docker-compose up -d

# Start API
go run cmd/api/main.go
```

Wait for: `✓ Server starting on :8080`

---

## ✅ **4. Test! (1 minute)**

### Quick Test:
1. Click **"Health Check"** → Send → Should get 200 ✅
2. Click **"Register User"** → Change email → Send → Should get 201 ✅
3. Click **"Login"** → Send → Should get 200 ✅

### Full Test (run in order):
1. Health Check
2. Register User (change email!)
3. Login
4. Refresh Token
5. Test Old Refresh Token (should get 401 ✅)
6. Logout
7. Login again → Logout All Devices

---

## 🎊 **Done!**

If all tests passed, your authentication system is working perfectly!

**Next:** Read `README.md` for detailed explanations.

---

## 🐛 Common Issues:

**"Connect ECONNREFUSED"**
→ Server not running. Run: `go run cmd/api/main.go`

**"Email already exists"**
→ Change the email in Register request body

**"{{base_url}} not resolving"**
→ Select "PropVest Local" environment (top-right)

---

**Need Help?** Check `README.md` for detailed troubleshooting.
