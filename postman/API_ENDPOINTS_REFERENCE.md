# 📚 PropVest API Endpoints Reference

## Complete API Documentation for Postman Testing
**Version:** Milestones 0-3 Complete (2026-08-05)

---

## 🔧 Setup

### Base URL
```
http://localhost:8080/api/v1
```

### Environment Variables Needed
- `base_url` = `http://localhost:8080/api/v1`
- `access_token` = (auto-saved from login/register)
- `refresh_token` = (auto-saved from login/register)
- `user_id` = (auto-saved from login/register)

---

## 📋 Endpoint Summary

| Category | Endpoint | Method | Auth Required |
|----------|----------|--------|---------------|
| **System** |
| Health Check | `/health` | GET | No |
| **Authentication** |
| Register | `/auth/register` | POST | No |
| Login | `/auth/login` | POST | No |
| Refresh Token | `/auth/refresh` | POST | No |
| Logout | `/auth/logout` | POST | No |
| Logout All | `/auth/logout-all` | POST | Yes |
| **User Management** |
| Get Profile | `/users/me` | GET | Yes |
| Update Profile | `/users/me` | PATCH | Yes |
| Upload Avatar | `/users/avatar` | PATCH | Yes (multipart) |
| Change Password | `/users/password` | PATCH | Yes |
| Request Phone Change | `/users/phone/request` | POST | Yes |
| Verify Phone Change | `/users/phone/verify` | POST | Yes |
| **Wallet** |
| Get Wallet | `/wallet` | GET | Yes |
| Initiate Deposit | `/wallet/deposit` | POST | Yes |
| Request Withdrawal | `/wallet/withdraw` | POST | Yes |
| Get Transactions | `/wallet/transactions` | GET | Yes |
| **Webhooks** |
| Payment Webhook | `/webhooks/payment` | POST | No (signature-verified) |

---

## 📍 System Endpoints

### 1. Health Check

**Endpoint:** `GET {{base_url}}/health`

**Description:** Check if API server is running

**Headers:** None

**Response:**
```json
{
  "status": "healthy",
  "message": "PropVest API is running"
}
```

---

## 🔐 Authentication Endpoints

### 2. Register User

**Endpoint:** `POST {{base_url}}/auth/register`

**Headers:**
```
Content-Type: application/json
```

**Body:**
```json
{
  "full_name": "Chukwuemeka Obi",
  "email": "chukwuemeka@example.com",
  "password": "SecurePass123!",
  "phone": "+2348012345678"
}
```

**Password Requirements:**
- Min 12 characters
- 1 uppercase, 1 lowercase, 1 digit, 1 special char

**Phone Format:**
- E.164 format: `+234XXXXXXXXXX`
- Nigerian networks only (70x, 80x, 81x, 90x, 91x)

**Response (201):**
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid",
      "user_code": "USR-xxxxxxxx",
      "full_name": "Chukwuemeka Obi",
      "email": "chukwuemeka@example.com",
      "phone": "+2348012345678",
      "avatar_url": null,
      "email_verified": false,
      "kyc_status": "pending",
      "role": "investor",
      "is_active": true,
      "created_at": "2026-08-05T..."
    },
    "access_token": "eyJhbGc...",
    "refresh_token": "random-token-hash",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

**Auto-Actions:**
- Saves `access_token` to environment
- Saves `refresh_token` to environment
- Saves `user_id` to environment

---

### 3. Login

**Endpoint:** `POST {{base_url}}/auth/login`

**Headers:**
```
Content-Type: application/json
```

**Body:**
```json
{
  "email": "chukwuemeka@example.com",
  "password": "SecurePass123!"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid",
      "user_code": "USR-xxxxxxxx",
      "full_name": "Chukwuemeka Obi",
      "email": "chukwuemeka@example.com",
      "phone": "+2348012345678",
      "role": "investor"
    },
    "access_token": "eyJhbGc...",
    "refresh_token": "new-random-token",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

**Auto-Actions:**
- Saves new `access_token` to environment
- Saves new `refresh_token` to environment

---

###4. Refresh Access Token

**Endpoint:** `POST {{base_url}}/auth/refresh`

**Headers:**
```
Content-Type: application/json
```

**Body:**
```json
{
  "refresh_token": "{{refresh_token}}"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "access_token": "new-access-token",
    "refresh_token": "new-refresh-token",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

**Important - Token Rotation:**
- Old refresh token is immediately REVOKED
- New tokens are issued
- Must use NEW refresh token for next refresh
- Security feature to prevent token theft

**Auto-Actions:**
- Saves new `access_token`
- Saves new `refresh_token`
- Saves old token as `old_refresh_token` for testing

---

### 5. Logout (Current Device)

**Endpoint:** `POST {{base_url}}/auth/logout`

**Headers:**
```
Content-Type: application/json
```

**Body:**
```json
{
  "refresh_token": "{{refresh_token}}"
}
```

**Response (200):**
```json
{
  "success": true,
  "message": "Logout successful"
}
```

**What Happens:**
- Provided refresh token is revoked
- Can't use that token to refresh anymore
- Other devices remain logged in
- Access token remains valid until expiration

---

### 6. Logout All Devices

**Endpoint:** `POST {{base_url}}/auth/logout-all`

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Body:** None

**Response (200):**
```json
{
  "success": true,
  "message": "All sessions logged out"
}
```

**What Happens:**
- ALL user's refresh tokens are revoked
- Must login again on EVERY device
- Automatically called on password change

---

## 👤 User Management Endpoints

### 7. Get Profile

**Endpoint:** `GET {{base_url}}/users/me`

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "user_code": "USR-xxxxxxxx",
    "full_name": "Chukwuemeka Obi",
    "email": "chukwuemeka@example.com",
    "phone": "+2348012345678",
    "avatar_url": "https://res.cloudinary.com/.../avatar.jpg",
    "email_verified": false,
    "kyc_status": "pending",
    "role": "investor",
    "is_active": true,
    "created_at": "2026-08-05T..."
  }
}
```

---

### 8. Update Profile

**Endpoint:** `PATCH {{base_url}}/users/me`

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "full_name": "Chukwuemeka Emmanuel Obi"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "full_name": "Chukwuemeka Emmanuel Obi",
    ...
  }
}
```

**Validation:**
- `full_name`: 2-100 characters

---

### 9. Upload Avatar

**Endpoint:** `PATCH {{base_url}}/users/avatar`

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: multipart/form-data
```

**Body (form-data):**
```
avatar: (file) image.jpg
```

**File Requirements:**
- Format: JPG, JPEG, PNG
- Max size: 5MB
- Uploaded to Cloudinary
- Auto-cropped to 400x400px

**Response (200):**
```json
{
  "success": true,
  "data": {
    "avatar_url": "https://res.cloudinary.com/.../avatar.jpg"
  },
  "message": "Avatar uploaded successfully"
}
```

**Note:** Requires Cloudinary configuration in `.env`

---

### 10. Change Password

**Endpoint:** `PATCH {{base_url}}/users/password`

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "current_password": "SecurePass123!",
  "new_password": "NewSecurePass456!"
}
```

**Response (200):**
```json
{
  "success": true,
  "message": "Password changed successfully"
}
```

**Security Actions:**
- Validates current password
- Validates new password complexity
- Hashes new password with bcrypt
- **Revokes ALL refresh tokens** (logout everywhere)
- Forces re-login on all devices

---

### 11. Request Phone Change

**Endpoint:** `POST {{base_url}}/users/phone/request`

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "new_phone": "+2349087654321"
}
```

**Response (200):**
```json
{
  "success": true,
  "data": {
    "message": "OTP sent successfully",
    "expires_in": 600,
    "can_resend_after": 120
  }
}
```

**What Happens:**
1. Validates new phone format
2. Checks phone not already in use
3. Generates 6-digit OTP
4. Sends OTP via SMS (mock in development)
5. OTP expires in 10 minutes
6. Can resend after 2 minutes

**Development Mode:**
Check console logs for OTP code:
```
[SMS] Sending OTP to +2349087654321: 123456
```

**Rate Limits:**
- Max 5 requests per hour per user
- Max 3 verification attempts per OTP
- 2-minute cooldown between requests

---

### 12. Verify Phone Change

**Endpoint:** `POST {{base_url}}/users/phone/verify`

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "new_phone": "+2349087654321",
  "otp_code": "123456"
}
```

**Response (200):**
```json
{
  "success": true,
  "message": "Phone number updated successfully",
  "data": {
    "phone": "+2349087654321"
  }
}
```

**What Happens:**
1. Validates OTP code
2. Checks not expired (10 min)
3. Checks not already used
4. Updates user phone number
5. Marks OTP as used

**Errors:**
- `invalid_otp`: Wrong code
- `otp_expired`: More than 10 minutes old
- `otp_already_used`: Already verified
- `too_many_attempts`: 3 failed attempts
- `phone_already_exists`: Phone taken by another user

---

## 💰 Wallet Endpoints (Milestone 3)

### 13. Get Wallet

**Endpoint:** `GET {{base_url}}/wallet`

**Description:** Retrieve user's wallet with current balances

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Response (200):**
```json
{
  "success": true,
  "message": "Request successful",
  "data": {
    "id": "wallet-uuid",
    "main_balance": 150000,
    "main_balance_formatted": "₦1,500.00",
    "earnings_balance": 50000,
    "earnings_balance_formatted": "₦500.00",
    "currency": "NGN",
    "virtual_acct_no": null,
    "virtual_bank": null,
    "created_at": "2026-08-05T..."
  }
}
```

**Notes:**
- Balances are in kobo (₦1 = 100 kobo)
- main_balance: Available for withdrawal/investment
- earnings_balance: Profits from investments (auto-credited)
- Wallet created automatically during registration

---

### 14. Initiate Deposit

**Endpoint:** `POST {{base_url}}/wallet/deposit`

**Description:** Start deposit flow with payment provider

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "amount_kobo": 150000,
  "idempotency_key": "optional-unique-key"
}
```

**Parameters:**
- `amount_kobo` (required): Amount in kobo (₦1,500 = 150000)
- `idempotency_key` (optional): Prevents duplicate deposits

**Response (200):**
```json
{
  "success": true,
  "message": "Request successful",
  "data": {
    "authorization_url": "https://mock-provider.com/pay/DEP-abc123",
    "access_code": null,
    "reference": "DEP-abc123-uuid",
    "amount_kobo": 150000,
    "amount_formatted": "₦1,500.00"
  }
}
```

**Next Steps:**
1. Redirect user to `authorization_url`
2. User completes payment on provider's page
3. Provider sends webhook to backend
4. Wallet credited automatically

**Testing with Mock Provider:**
1. Call this endpoint → get reference
2. Manually call webhook endpoint (see #17)
3. Check wallet balance updated

**Validation:**
- Minimum amount: ₦100 (10000 kobo)
- Maximum amount: ₦5,000,000 (500000000 kobo)

---

### 15. Request Withdrawal

**Endpoint:** `POST {{base_url}}/wallet/withdraw`

**Description:** Withdraw funds to bank account

**Headers:**
```
Authorization: Bearer {{access_token}}
Content-Type: application/json
```

**Body:**
```json
{
  "amount_kobo": 50000,
  "account_number": "0123456789",
  "account_name": "John Doe",
  "bank_code": "058",
  "bank_name": "GTBank"
}
```

**Parameters:**
- `amount_kobo` (required): Amount to withdraw
- `account_number` (required): 10-digit Nigerian bank account
- `account_name` (required): Account holder name
- `bank_code` (required): 3-digit bank code (e.g., "058" for GTBank)
- `bank_name` (required): Bank name for display

**Common Bank Codes:**
- GTBank: 058
- Access Bank: 044
- Zenith: 057
- First Bank: 011
- UBA: 033

**Response (200):**
```json
{
  "success": true,
  "message": "Withdrawal initiated successfully",
  "data": {
    "id": "transaction-uuid",
    "reference": "WDR-abc123",
    "type": "withdrawal",
    "amount": 50000,
    "amount_formatted": "₦500.00",
    "status": "pending",
    "balance_before": 150000,
    "balance_after": 100000,
    "description": "Withdrawal to GTBank - 0123456789",
    "metadata": {
      "account_number": "0123456789",
      "account_name": "John Doe",
      "bank_code": "058",
      "bank_name": "GTBank"
    },
    "created_at": "2026-08-05T..."
  }
}
```

**What Happens:**
1. Balance debited immediately
2. Transaction created with status "pending"
3. Payout queued for worker processing
4. Worker processes payout to bank (async)
5. Status updated to "completed" or "failed"

**Validation:**
- Minimum withdrawal: ₦500 (50000 kobo)
- Maximum per transaction: ₦1,000,000
- Must have sufficient balance
- Account number must be 10 digits

**Errors:**
- `insufficient_balance`: Balance too low
- `amount_too_small`: Below minimum (₦500)
- `invalid_bank_account`: Invalid account format

---

### 16. Get Transaction History

**Endpoint:** `GET {{base_url}}/wallet/transactions`

**Description:** Retrieve paginated transaction history

**Headers:**
```
Authorization: Bearer {{access_token}}
```

**Query Parameters:**
- `type` (optional): Filter by type (`deposit`, `withdrawal`, `credit`, `debit`)
- `status` (optional): Filter by status (`pending`, `completed`, `failed`)
- `page` (optional): Page number (default: 1)
- `limit` (optional): Items per page (default: 20, max: 100)

**Example Request:**
```
GET {{base_url}}/wallet/transactions?type=deposit&status=completed&page=1&limit=10
```

**Response (200):**
```json
{
  "success": true,
  "message": "Request successful",
  "data": {
    "transactions": [
      {
        "id": "uuid",
        "reference": "DEP-abc123",
        "type": "deposit",
        "amount": 150000,
        "amount_formatted": "₦1,500.00",
        "status": "completed",
        "balance_before": 0,
        "balance_after": 150000,
        "description": "Deposit via Mock Provider",
        "metadata": {
          "provider": "mock",
          "provider_reference": "DEP-abc123"
        },
        "created_at": "2026-08-05T10:00:00Z"
      },
      {
        "id": "uuid",
        "reference": "WDR-xyz789",
        "type": "withdrawal",
        "amount": 50000,
        "amount_formatted": "₦500.00",
        "status": "pending",
        "balance_before": 150000,
        "balance_after": 100000,
        "description": "Withdrawal to GTBank - 0123456789",
        "metadata": {
          "account_number": "0123456789",
          "bank_code": "058"
        },
        "created_at": "2026-08-05T11:00:00Z"
      }
    ],
    "total": 2,
    "page": 1,
    "limit": 10,
    "pages": 1
  }
}
```

**Transaction Types:**
- `deposit`: Money added via payment provider
- `withdrawal`: Money sent to bank account
- `credit`: Manual credit (admin action, refund, earnings)
- `debit`: Manual debit (admin action, fee)

**Transaction Status:**
- `pending`: Not yet processed
- `completed`: Successfully processed
- `failed`: Processing failed

**Pagination:**
- Returns max 100 items per page
- Use `page` and `limit` for navigation
- `pages` shows total pages available
- `total` shows total matching records

---

## 🔔 Webhook Endpoints

### 17. Payment Webhook

**Endpoint:** `POST {{base_url}}/webhooks/payment`

**Description:** Called by payment provider when payment completes (PUBLIC endpoint)

**Security:** Uses signature verification instead of JWT

**Headers:**
```
X-Paystack-Signature: hmac_sha512_signature
Content-Type: application/json
```

**Body (Mock Provider):**
```json
{
  "event": "charge.success",
  "data": {
    "reference": "DEP-abc123-uuid",
    "amount": 150000,
    "currency": "NGN",
    "status": "success"
  }
}
```

**Response (200):**
```json
{
  "success": true,
  "message": "Webhook processed successfully"
}
```

**How to Test with Mock Provider:**

1. **Initiate Deposit** (get reference):
```bash
POST /wallet/deposit
{ "amount_kobo": 150000 }
→ Response: { "reference": "DEP-abc123" }
```

2. **Simulate Webhook** (mock provider signature):
```bash
POST /webhooks/payment
X-Mock-Signature: (auto-computed by mock)
{
  "data": {
    "reference": "DEP-abc123",
    "status": "success",
    "amount": 150000
  }
}
```

3. **Verify Wallet Credited**:
```bash
GET /wallet
→ Balance increased by ₦1,500
```

**Signature Verification:**
```javascript
// Mock provider uses HMAC-SHA512
const crypto = require('crypto');
const secret = 'mock-secret-key';
const body = JSON.stringify(payload);
const signature = crypto
  .createHmac('sha512', secret)
  .update(body)
  .digest('hex');
```

**Idempotency:**
- Safe to call multiple times with same reference
- Wallet credited only once
- Duplicate webhooks return 200 OK

**Errors:**
- `400 Bad Request`: Invalid signature or missing reference
- `500 Internal Server Error`: Verification failed (temporary)

---

**Errors:**
- `invalid_otp`: Wrong code
- `otp_expired`: More than 10 minutes old
- `otp_already_used`: Already verified
- `too_many_attempts`: 3 failed attempts
- `phone_already_exists`: Phone taken by another user

---

## 🧪 Testing Workflows

### Workflow 1: Complete Registration Flow
1. Register new user → Saves tokens
2. Get profile → Verify user data
3. Update profile → Change name
4. Get profile → Verify update

### Workflow 2: Authentication Flow
1. Login → Get tokens
2. Refresh token → Get new tokens
3. Test old token (should fail with 401)
4. Logout → Revoke current token

### Workflow 3: Phone Change Flow
1. Login → Get access token
2. Request phone change → Get OTP in console
3. Verify phone change → Phone updated
4. Get profile → Verify new phone

### Workflow 4: Password Change Flow
1. Login → Get tokens
2. Change password → All tokens revoked
3. Try to refresh (should fail with 401)
4. Login with new password → Get new tokens

### Workflow 5: Wallet Deposit Flow (Mock Provider)
1. Login → Get access token
2. Get wallet → Check initial balance (₦0)
3. Initiate deposit → Get authorization_url and reference
4. Simulate webhook → POST to /webhooks/payment with reference
5. Get wallet → Verify balance increased
6. Get transactions → Verify deposit recorded

### Workflow 6: Wallet Withdrawal Flow
1. Ensure wallet has balance (do deposit first)
2. Get wallet → Check current balance
3. Request withdrawal → Balance debited immediately
4. Get wallet → Verify balance reduced
5. Get transactions → Verify withdrawal with status "pending"
6. (Worker processes payout asynchronously)

### Workflow 7: Complete Wallet Journey
1. Register → User + wallet created (₦0 balance)
2. Get wallet → Verify empty wallet
3. Initiate deposit (₦1,500) → Get reference
4. Simulate webhook → Credit wallet
5. Get wallet → Balance = ₦1,500
6. Request withdrawal (₦500) → Balance = ₦1,000
7. Get transactions (all) → See deposit + withdrawal
8. Get transactions (deposits only) → Filter by type
9. Get transactions (completed only) → Filter by status

### Workflow 8: Complete User Journey
1. Register → User + wallet created, logged in
2. Get profile → View initial data
3. Upload avatar → Update profile picture
4. Update profile → Change name
5. Request phone change → Initiate verification
6. Verify phone → Complete verification
7. Change password → Security update
8. Login with new password → Re-authenticate
9. Get wallet → Check balance
10. Initiate deposit → Add funds
11. Logout all → Clean logout

---

## ❌ Common Error Responses

### Validation Error (400)
```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed",
    "details": [
      {
        "field": "email",
        "message": "Invalid email format"
      }
    ]
  }
}
```

### Unauthorized (401)
```json
{
  "success": false,
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Invalid or expired token"
  }
}
```

### Not Found (404)
```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "User not found"
  }
}
```

### Internal Error (500)
```json
{
  "success": false,
  "error": {
    "code": "INTERNAL_ERROR",
    "message": "An unexpected error occurred"
  },
  "request_id": "uuid"
}
```

---

## 🔍 Testing Tips

### 1. Check Console Logs
Postman console shows:
- Request details
- Response time
- Auto-saved variables
- OTP codes (in development)

### 2. Environment Variables
After register/login, check environment has:
- `access_token`
- `refresh_token`
- `user_id`

### 3. Token Expiration
- Access tokens expire in 15 minutes
- Refresh tokens expire in 30 days
- Test expiration by waiting or changing JWT expiry in config

### 4. SMS in Development
OTPs are logged to console, not sent via SMS:
```
[SMS] Sending OTP to +2349087654321: 123456
```

### 5. Avatar Upload Testing
Requires Cloudinary configuration. Without it:
- Returns error: `IMAGE_UPLOAD_FAILED`
- Other endpoints still work

---

## 📊 Test Coverage

### ✅ Implemented & Tested
- [x] Health check
- [x] User registration
- [x] User login
- [x] Token refresh with rotation
- [x] Logout (single device)
- [x] Logout all devices
- [x] Get user profile
- [x] Update user profile
- [x] Upload avatar
- [x] Change password
- [x] Request phone change
- [x] Verify phone change

### 🔜 Coming in Milestone 3
- [x] Get wallet
- [x] Deposit funds
- [x] Withdraw funds
- [x] Transaction history
- [x] Payment webhooks

### 🔜 Coming in Milestone 4+
- [ ] Property listings
- [ ] Property details
- [ ] Create investment
- [ ] Portfolio summary

---

## 🎯 Quick Start Commands

### Start Backend
```bash
docker compose up -d
go run cmd/api/main.go
```

### Check Health
```bash
curl http://localhost:8080/health
```

### Register User (curl)
```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Test User",
    "email": "test@example.com",
    "password": "SecurePass123!",
    "phone": "+2348012345678"
  }'
```

### Login (curl)
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123!"
  }'
```

---

## 📚 Additional Resources

- `MILESTONE_1_COMPLETE.md` - Authentication implementation details
- `MILESTONE_2_COMPLETE.md` - User management implementation details
- `MILESTONE_3_COMPLETE.md` - Wallet system implementation details
- `SCHEMA_FIXES_COMPLETE.md` - Database migration fixes
- API Specification: `docs/03-API/3.2-API_SPECIFICATION.md`

---

**Last Updated:** 2026-08-05  
**Backend Version:** Milestones 0-3 Complete  
**Database Version:** Migration 000006

