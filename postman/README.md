# 📮 PropVest API - Postman Testing Guide

Complete API testing documentation for PropVest backend (Milestones 0-2 Complete).

---

## 📁 Files in This Folder

| File | Description | Status |
|------|-------------|--------|
| `API_ENDPOINTS_REFERENCE.md` | **📚 MAIN DOCUMENTATION** - Complete API reference | ✅ Use This! |
| `PropVest_Local.postman_environment.json` | Local environment variables | ✅ Import This |
| `PropVest_API_v2.postman_collection.json` | Minimal collection stub | ⚠️ Build Manually |
| `QUICK_START.md` | Quick setup guide | ✅ Read First |
| `README.md` | This file | 📖 Navigation |

---

## ⚠️ Important: Collection Setup

**The Postman collection JSON file is intentionally minimal.** Here's how to get started:

### ✅ **Fastest Method: Follow Step-by-Step Guide**
📄 **Open: `IMPORT_INSTRUCTIONS.md`**
- Copy-paste ready templates for all 12 endpoints
- Takes ~10 minutes to set up complete collection
- Includes test scripts for auto-saving tokens

### ✅ **Alternative: Use API Reference**
📄 **Open: `API_ENDPOINTS_REFERENCE.md`**
- Complete documentation with examples
- All request/response details
- Testing workflows
- Error handling

---

## 🚀 Quick Start (5 Minutes)

### Step 1: Import Environment
```
1. Open Postman
2. Click "Import" → Select "PropVest_Local.postman_environment.json"
3. Select "PropVest Local" from environment dropdown (top-right)
```

### Step 2: Start Backend
```bash
docker compose up -d
go run cmd/api/main.go
```

### Step 3: Test Health Check
```
1. Create new request in Postman
2. Method: GET
3. URL: http://localhost:8080/health
4. Click Send
5. Should get: { "status": "healthy" }
```

### Step 4: Build Your Collection
```
1. Open API_ENDPOINTS_REFERENCE.md
2. Copy endpoint details
3. Create requests in Postman
4. Test each endpoint
```

---

## 📋 Implemented Endpoints (12 Total)

### ✅ System (1)
- `GET /health` - Health check

### ✅ Authentication (5)
- `POST /auth/register` - Register new user
- `POST /auth/login` - Login with credentials
- `POST /auth/refresh` - Refresh access token (with rotation)
- `POST /auth/logout` - Logout current device
- `POST /auth/logout-all` - Logout all devices

### ✅ User Management (6)
- `GET /users/me` - Get profile
- `PATCH /users/me` - Update profile
- `PATCH /users/avatar` - Upload avatar (Cloudinary)
- `PATCH /users/password` - Change password
- `POST /users/phone/request` - Request phone change (OTP)
- `POST /users/phone/verify` - Verify phone change

**All endpoints documented in `API_ENDPOINTS_REFERENCE.md`!**

---

## 🧪 Testing Workflows

### Workflow 1: Basic Auth (5 min)
```
1. Health Check → Verify server running
2. Register User → Get tokens
3. Get Profile → Verify user data
4. Logout → End session
```

### Workflow 2: Token Rotation (3 min)
```
1. Login → Get tokens
2. Refresh Token → Get new tokens
3. Test old token → Should fail (401)
✅ Proves rotation works!
```

### Workflow 3: Complete User Journey (10 min)
```
1. Register
2. Get Profile
3. Update Profile
4. Upload Avatar
5. Request Phone Change → Check console for OTP
6. Verify Phone Change
7. Change Password → All tokens revoked
8. Login with new password
```

**Full workflows in `API_ENDPOINTS_REFERENCE.md`!**

---

## 🔧 Environment Variables

Auto-saved by test scripts:

| Variable | Description | Auto-Saved From |
|----------|-------------|-----------------|
| `base_url` | API base URL | Pre-configured |
| `access_token` | JWT access token (15 min) | Register, Login |
| `refresh_token` | Refresh token (30 days) | Register, Login, Refresh |
| `user_id` | User UUID | Register, Login |
| `old_refresh_token` | Previous token (testing) | Refresh |

---

## 📖 Documentation Guide

### 1. Start Here: `QUICK_START.md`
Quick 5-minute setup guide

### 2. Main Reference: `API_ENDPOINTS_REFERENCE.md`
**Most Important Document!**
- Complete endpoint definitions
- Request/response examples
- Headers and auth
- Testing workflows
- Error handling
- Tips and tricks

### 3. This README
Navigation and overview

---

## 🎯 Testing Each Feature

### Example: Test Authentication
```
POST {{base_url}}/auth/register
Headers: Content-Type: application/json
Body:
{
  "full_name": "Test User",
  "email": "test@example.com",
  "password": "SecurePass123!",
  "phone": "+2348012345678"
}

Expected: 201 Created + user + tokens
```

### Example: Test User Profile
```
GET {{base_url}}/users/me
Headers: Authorization: Bearer {{access_token}}

Expected: 200 OK + user profile
```

**Full examples in `API_ENDPOINTS_REFERENCE.md`!**

---

## ⚠️ Common Issues & Solutions

### Issue: Connection Refused
**Fix:**
```bash
# Check backend is running
go run cmd/api/main.go

# Check database
docker ps
```

### Issue: 401 Unauthorized
**Fix:**
```
1. Run Login/Register to get new tokens
2. Check access_token in environment
3. Tokens expire in 15 minutes
```

### Issue: Validation Error
**Fix:**
```
Check request format:
- Password: Min 12 chars, 1 upper, 1 lower, 1 digit, 1 special
- Phone: E.164 format (+234XXXXXXXXXX)
- Email: Valid format
```

### Issue: OTP Not Received
**Fix:**
```
In development, check console logs:
[SMS] Sending OTP to +2349087654321: 123456

Use the code from logs (not sent via SMS in dev mode)
```

### Issue: Avatar Upload Failed
**Fix:**
```
Requires Cloudinary configuration:

Add to .env:
CLOUDINARY_CLOUD_NAME=your_cloud_name
CLOUDINARY_API_KEY=your_key
CLOUDINARY_API_SECRET=your_secret
```

**More troubleshooting in `API_ENDPOINTS_REFERENCE.md`!**

---

## 🔍 Advanced Testing

### Test Token Rotation Security
```
1. Login → token_1
2. Refresh with token_1 → token_2 (token_1 revoked)
3. Try refresh with token_1 → Should FAIL (401)

✅ Proves rotation working!
```

### Test Logout All Devices
```
1. Login Device 1 → token_1
2. Login Device 2 → token_2
3. Logout-all with token_1
4. Try refresh with token_2 → Should FAIL (401)

✅ Proves all sessions revoked!
```

### Test Password Change Security
```
1. Login → Get tokens
2. Change Password
3. Try refresh → Should FAIL (401)

✅ Proves password change revokes all sessions!
```

**More advanced tests in `API_ENDPOINTS_REFERENCE.md`!**

---

## 📊 Test Coverage

**Milestones 0-2: 100% Complete**
- [x] Health check
- [x] User registration (+ automatic wallet creation)
- [x] User login
- [x] Token refresh with rotation
- [x] Logout (single + all devices)
- [x] User profile management
- [x] Avatar upload
- [x] Password change
- [x] Phone number verification (OTP)

**Coming in Milestone 3:**
- [ ] Get wallet
- [ ] Deposit funds
- [ ] Withdraw funds
- [ ] Transaction history

---

## 🎓 For Junior Developers

### Start Simple → Build Up
1. **Health Check** - Simplest endpoint
2. **Register** - Creates user + wallet
3. **Login** - Gets tokens
4. **Get Profile** - First authenticated endpoint
5. **Update Profile** - First POST with auth
6. **Other Features** - More complex flows

### Understanding Tokens
- **Access Token**: Short-lived (15 min), for API requests
- **Refresh Token**: Long-lived (30 days), gets new access tokens
- **Token Rotation**: Security feature that revokes old tokens

### Understanding OTP Flow
1. Request phone change → OTP generated
2. Check console logs → Get 6-digit code
3. Verify with code → Phone updated
4. OTP expires in 10 minutes, single-use

---

## 📚 Additional Resources

### Backend Docs
- `docs/03-API/3.2-API_SPECIFICATION.md` - Complete API spec
- `MILESTONE_1_COMPLETE.md` - Authentication details
- `MILESTONE_2_COMPLETE.md` - User management details
- `SCHEMA_FIXES_COMPLETE.md` - Database fixes

### Implementation
- `internal/handlers/auth.go` - Auth handlers
- `internal/handlers/user.go` - User handlers
- `internal/services/auth_service.go` - Auth logic
- `internal/services/user_service.go` - User logic

---

## 🚦 Next Steps

### 1. Import Environment ✅
```
Postman → Import → PropVest_Local.postman_environment.json
```

### 2. Read Documentation ✅
```
Open: API_ENDPOINTS_REFERENCE.md
Read: All 12 endpoint definitions
```

### 3. Start Testing ✅
```
Follow workflows in API reference
Test each endpoint
Verify responses
```

### 4. Build Collection ✅
```
Create requests manually
Add test scripts for auto-save
Organize into folders
```

---

## 📞 Support

Need help? Check:
1. `API_ENDPOINTS_REFERENCE.md` - Complete documentation
2. Backend console logs - Error details
3. Postman console - Request/response details
4. `SCHEMA_FIXES_COMPLETE.md` - Migration issues

---

**📚 START HERE → `API_ENDPOINTS_REFERENCE.md`**

**Last Updated:** 2026-08-05  
**Version:** Milestones 0-2 Complete  
**Total Endpoints:** 12  
**Database Version:** Migration 000006

