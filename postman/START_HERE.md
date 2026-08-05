# 🚀 START HERE - Postman Setup Guide

## Quick Links

📄 **[IMPORT_INSTRUCTIONS.md](IMPORT_INSTRUCTIONS.md)** ← **START HERE!**
- Step-by-step guide to create all 12 requests
- Copy-paste ready templates
- Takes ~10 minutes
- Best for beginners

📄 **[API_ENDPOINTS_REFERENCE.md](API_ENDPOINTS_REFERENCE.md)**
- Complete API documentation
- All request/response examples
- Testing workflows
- Best for reference

📄 **[README.md](README.md)**
- Overview and navigation
- Quick start summary

---

## ⚡ Quick Start (2 Steps)

### Step 1: Import Environment (30 seconds)
```
1. Open Postman
2. Click "Import"
3. Select: PropVest_Local.postman_environment.json
4. Select "PropVest Local" from dropdown (top-right)
```

### Step 2: Follow Instructions (10 minutes)
```
1. Open: IMPORT_INSTRUCTIONS.md
2. Copy-paste each request template
3. Save to new collection
4. Test endpoints
```

---

## 📋 What You'll Create

**12 Endpoints:**
1. Health Check
2. Register User
3. Login
4. Refresh Token
5. Logout
6. Logout All Devices
7. Get Profile
8. Update Profile
9. Upload Avatar
10. Change Password
11. Request Phone Change
12. Verify Phone Change

**With:**
- ✅ Auto-save tokens to environment
- ✅ Test scripts for validation
- ✅ Console logging
- ✅ Error handling

---

## 🎯 Why Not One JSON File?

**Short Answer:** The full collection JSON is 600+ lines and hard to maintain.

**Better Approach:** Copy-paste templates from `IMPORT_INSTRUCTIONS.md`
- Easier to customize
- Learn as you build
- See what each part does
- No JSON editing needed

---

## 📚 Files Explained

| File | Purpose | When to Use |
|------|---------|-------------|
| `IMPORT_INSTRUCTIONS.md` | **Step-by-step setup** | Setting up Postman |
| `API_ENDPOINTS_REFERENCE.md` | **Complete documentation** | During testing |
| `PropVest_Local.postman_environment.json` | Environment variables | Import first |
| `README.md` | Overview | Navigation |
| `START_HERE.md` | This file | Starting point |

---

## ✅ Success Path

```
1. Import environment (PropVest_Local.postman_environment.json)
   ↓
2. Open IMPORT_INSTRUCTIONS.md
   ↓
3. Create new collection in Postman
   ↓
4. Copy-paste 12 request templates
   ↓
5. Start backend (go run cmd/api/main.go)
   ↓
6. Test endpoints in order
   ↓
7. Use API_ENDPOINTS_REFERENCE.md for details
```

---

## 🆘 Need Help?

**Can't find instructions?**
→ Open `IMPORT_INSTRUCTIONS.md` in this folder

**Need endpoint details?**
→ Open `API_ENDPOINTS_REFERENCE.md` in this folder

**Backend not starting?**
→ Run: `docker compose up -d` then `go run cmd/api/main.go`

**Tokens not saving?**
→ Make sure "PropVest Local" environment is selected (top-right)

---

**👉 Next Step: Open [IMPORT_INSTRUCTIONS.md](IMPORT_INSTRUCTIONS.md)**

