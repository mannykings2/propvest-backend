# Backend ↔ Frontend Integration Checklist

Quick verification checklist to ensure your backend is ready for frontend integration.

---

## ✅ Backend Configuration - COMPLETE

### 1. ✅ CORS (Cross-Origin Resource Sharing)
- **Status:** ✅ CONFIGURED AND WORKING
- **File:** `internal/middleware/cors.go`
- **Config:** `.env` → `ALLOWED_ORIGINS`
- **Current Value:** 
  ```
  ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000,http://localhost:3001
  ```

**What it does:**
- Allows your React frontend (different port) to call the backend API
- Handles preflight OPTIONS requests automatically
- Supports credentials (cookies, auth headers)
- Configured for development (localhost) and can be updated for production

**To add more origins:**
Edit `.env` and restart backend:
```env
ALLOWED_ORIGINS=http://localhost:5173,https://yourapp.com,https://www.yourapp.com
```

---

### 2. ✅ Standard API Response Format
- **Status:** ✅ IMPLEMENTED
- **File:** `internal/response/response.go`
- **Format:** Consistent JSON envelope

**All responses follow this structure:**

**Success (200/201):**
```json
{
  "success": true,
  "data": { /* response data */ },
  "message": "Optional success message",
  "request_id": "uuid-for-tracing"
}
```

**Error (4xx/5xx):**
```json
{
  "success": false,
  "message": "Human-readable error",
  "code": "machine_readable_code",
  "errors": { /* field-level errors for validation */ },
  "request_id": "uuid-for-tracing"
}
```

**Benefits for frontend:**
- Consistent structure - easy to parse
- `success` field for quick status check
- `code` field for programmatic error handling
- `errors` object for form validation display
- `request_id` for debugging/support

---

### 3. ✅ JWT Authentication
- **Status:** ✅ FULLY IMPLEMENTED
- **Files:** 
  - `internal/middleware/auth.go` (authentication middleware)
  - `internal/services/auth_service.go` (token generation)
  - `internal/handlers/auth.go` (endpoints)

**Token Structure:**
```json
{
  "access_token": "eyJhbGc...",     // 15 minutes (default)
  "refresh_token": "eyJhbGc...",    // 30 days (default)
  "token_type": "Bearer"
}
```

**Configuration (in .env):**
```env
JWT_SECRET=your-super-secret-key-change-in-production
JWT_REFRESH_SECRET=your-super-secret-refresh-key-change-in-production
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=720h
```

**How frontend uses it:**
1. Login/Register → Receive tokens
2. Store in localStorage or httpOnly cookies
3. Include in requests: `Authorization: Bearer <access_token>`
4. When expired → Use refresh_token to get new tokens
5. Logout → Call logout endpoint + clear stored tokens

**Protected Endpoints:**
All endpoints under `/api/v1/users/*` and `/api/v1/wallets/*` require authentication.

---

## 📋 Available Endpoints

### Public Endpoints (No Auth Required)
```
POST /api/v1/auth/register         - Create new account
POST /api/v1/auth/login            - Get tokens
POST /api/v1/auth/refresh          - Refresh access token
POST /api/v1/auth/forgot-password  - Request password reset
POST /api/v1/auth/reset-password   - Reset password with token
POST /api/v1/auth/verify-email     - Verify email with token
```

### Protected Endpoints (Auth Required)
```
# User Management
GET    /api/v1/users/me                  - Get current user
PUT    /api/v1/users/me                  - Update profile
POST   /api/v1/users/me/avatar           - Upload avatar
POST   /api/v1/users/me/phone/verify     - Send phone OTP
POST   /api/v1/users/me/phone/confirm    - Confirm phone OTP
POST   /api/v1/auth/logout               - Logout
POST   /api/v1/auth/resend-verification  - Resend email verification

# Wallet Management
GET    /api/v1/wallets/me                - Get wallet
GET    /api/v1/wallets/me/transactions   - Get transactions (paginated)
POST   /api/v1/wallets/deposit           - Initiate deposit (Paystack)
POST   /api/v1/wallets/withdraw          - Request withdrawal
```

### Operational Endpoints
```
GET /health         - Health check (always returns 200)
GET /health/ready   - Readiness check (checks DB connection)
```

---

## 🔧 Backend Configuration Files

### `.env` - Main Configuration
**Current Port:** 8081  
**Database:** PostgreSQL on localhost:5435  
**Payment Provider:** Paystack (test mode)  
**Email Provider:** Brevo SMTP  
**SMS Provider:** Africa's Talking  
**Image Storage:** Cloudinary

### `docker-compose.yml` - Development Services
Includes:
- PostgreSQL (port 5435)
- Redis (port 6379)
- Mailpit (SMTP testing, port 1025, web UI port 8025)

---

## 🚀 Starting the Backend

### Option 1: Using Make (Recommended)
```bash
cd propvest-backend
make run
```

### Option 2: Direct Go Command
```bash
cd propvest-backend
go run cmd/api/main.go
```

### Option 3: Build and Run
```bash
cd propvest-backend
go build -o api.exe cmd/api/main.go
./api.exe
```

**Backend will start on:** http://localhost:8081

### Starting Dependencies (PostgreSQL, Redis)
```bash
cd propvest-backend
docker-compose up -d
```

---

## 🧪 Quick Test Commands

### Test Backend is Running
```bash
curl http://localhost:8081/health
```
Expected output:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Test CORS from Frontend
Open browser console on `http://localhost:5173`:
```javascript
fetch('http://localhost:8081/health')
  .then(r => r.json())
  .then(data => console.log('CORS working!', data))
  .catch(err => console.error('CORS error:', err));
```

### Test Authentication
```bash
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Test User",
    "email": "test@example.com",
    "password": "SecurePass123!",
    "phone": "+2348012345678"
  }'
```

### Test Protected Endpoint
```bash
# First login to get token
TOKEN=$(curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"SecurePass123!"}' \
  | jq -r '.data.access_token')

# Then use token
curl http://localhost:8081/api/v1/users/me \
  -H "Authorization: Bearer $TOKEN"
```

---

## 📚 Documentation Files

### For Frontend Developers
- **`FRONTEND_INTEGRATION_GUIDE.md`** - Complete integration guide with code examples
- **`FRONTEND_API_QUICKSTART.md`** - Quick copy-paste setup (5 minutes)
- **`postman/API_ENDPOINTS_REFERENCE.md`** - Detailed endpoint documentation
- **`postman/propvest-api.postman_collection.json`** - Postman collection for testing

### For Backend Developers
- **`docs/03-API/3.1-API_DESIGN.md`** - API design principles
- **`docs/04-Security/4.2-AUTHENTICATION_AND_AUTHORIZATION.md`** - Auth architecture
- **`docs/06-Engineering/6.2-ERROR_HANDLING_AND_LOGGING.md`** - Error handling
- **`handoff/10-BUILD-GUIDE.md`** - Build and deployment guide

---

## ⚙️ What Needs to Be Done for Production?

### High Priority

#### 1. 🔴 Update JWT Secrets
**Current (UNSAFE):**
```env
JWT_SECRET=your-super-secret-key-change-in-production
JWT_REFRESH_SECRET=your-super-secret-refresh-key-change-in-production
```

**Action Required:**
Generate strong secrets:
```bash
# Generate secure secrets
openssl rand -base64 64
```

Update `.env`:
```env
JWT_SECRET=<generated-secret-1>
JWT_REFRESH_SECRET=<generated-secret-2>
```

#### 2. 🔴 Configure Production CORS
**Current (Development):**
```env
ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000
```

**For Production:**
```env
ALLOWED_ORIGINS=https://propvest.com,https://www.propvest.com,https://app.propvest.com
```

**⚠️ NEVER use `*` in production!**

#### 3. 🔴 Add Rate Limiting
Protect against abuse:
- Login/Register endpoints: 10 requests/minute
- General API: 100 requests/minute (anonymous), 300 (authenticated)

**Implementation:**
```go
// In cmd/api/main.go
r.Use(middleware.RateLimiter(cfg))
```

#### 4. 🔴 Add Security Headers
Protect against common web vulnerabilities:

**Add middleware:**
```go
r.Use(middleware.SecurityHeaders())
```

Headers to add:
- `X-Frame-Options: DENY`
- `X-Content-Type-Options: nosniff`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security: max-age=31536000`
- `Content-Security-Policy: default-src 'self'`

#### 5. 🟡 Setup Monitoring
- Error tracking (Sentry, Rollbar)
- Performance monitoring (New Relic, DataDog)
- Logging aggregation (ELK, CloudWatch)
- Uptime monitoring (Pingdom, UptimeRobot)

#### 6. 🟡 Configure Production Database
Update connection pooling for load:
```env
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=20
DB_CONN_MAX_LIFETIME=1h
DB_CONN_MAX_IDLE_TIME=15m
```

#### 7. 🟡 Setup HTTPS
- Use TLS certificates (Let's Encrypt)
- Configure reverse proxy (Nginx, Caddy)
- Update BASE_URL to https://

#### 8. 🟡 Environment-specific Configuration
Use different `.env` files:
- `.env.development`
- `.env.staging`
- `.env.production`

Or use secret managers:
- AWS Secrets Manager
- HashiCorp Vault
- Azure Key Vault

---

## 🔒 Security Checklist

- [x] JWT tokens implemented
- [x] Password hashing (bcrypt)
- [x] CORS configured
- [x] Input validation
- [x] SQL injection prevention (parameterized queries)
- [ ] Rate limiting (needs to be enabled)
- [ ] Security headers (needs middleware)
- [ ] Strong JWT secrets (needs update for production)
- [ ] HTTPS/TLS (needs production setup)
- [ ] Request logging (implemented)
- [ ] Error handling (implemented)
- [ ] Audit logging (implemented)

---

## 📞 Troubleshooting

### Backend Won't Start
**Error:** `connection refused` or `address already in use`

**Solutions:**
1. Check if port 8081 is already in use:
   ```bash
   netstat -ano | findstr :8081
   ```
2. Check PostgreSQL is running:
   ```bash
   docker-compose ps
   ```
3. Check `.env` DATABASE_URL is correct

### CORS Errors
**Error:** `blocked by CORS policy`

**Solutions:**
1. Verify frontend URL is in `ALLOWED_ORIGINS`
2. Restart backend after changing `.env`
3. Check browser console for exact origin being blocked
4. Add that origin to `ALLOWED_ORIGINS`

### 401 Unauthorized
**Error:** API returns 401 even with token

**Solutions:**
1. Token format: `Authorization: Bearer <token>`
2. No extra spaces or newlines
3. Token not expired (check TTL)
4. Correct JWT_SECRET in backend
5. Try refreshing token

### Migration Errors
**Error:** `migration failed` or `table already exists`

**Solutions:**
1. Reset database:
   ```bash
   docker-compose down -v
   docker-compose up -d
   ```
2. Run migrations manually:
   ```bash
   make migrate-up
   ```

---

## ✅ Final Checklist

### Backend Ready for Frontend
- [x] CORS configured and tested
- [x] Standard API response format implemented
- [x] JWT authentication working
- [x] All auth endpoints functional
- [x] User endpoints protected
- [x] Wallet endpoints protected
- [x] Error handling consistent
- [x] Database migrations run
- [x] Docker services running
- [x] Health check endpoint working
- [x] Documentation complete

### Frontend Setup Required
- [ ] Install axios or fetch wrapper
- [ ] Create API client with interceptors
- [ ] Implement token storage (localStorage)
- [ ] Add Authorization header to requests
- [ ] Implement token refresh logic
- [ ] Handle error responses
- [ ] Display validation errors
- [ ] Create protected routes
- [ ] Add logout functionality
- [ ] Test all API endpoints

---

## 🎯 Next Steps

1. **Read:** `FRONTEND_API_QUICKSTART.md` for copy-paste setup
2. **Test:** Use Postman collection to test all endpoints
3. **Integrate:** Follow code examples in the guides
4. **Deploy:** Use `handoff/10-BUILD-GUIDE.md` for deployment

---

**Last Updated:** January 2024  
**Backend Version:** v1  
**Backend URL:** http://localhost:8081  
**API Base:** http://localhost:8081/api/v1
