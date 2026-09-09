# Frontend Integration Guide

This guide helps you integrate your React frontend with the PropVest backend API.

---

## ✅ Backend Preparation Checklist - COMPLETED

### 1. ✅ CORS Configuration
**Status:** ✅ Already Configured and Working

The backend is configured with proper CORS middleware that:
- Accepts requests from configured frontend origins
- Allows credentials (cookies, authorization headers)
- Permits all standard HTTP methods (GET, POST, PUT, PATCH, DELETE)
- Allows required headers: `Content-Type`, `Authorization`, `X-Request-ID`, `X-Idempotency-Key`

**Current Configuration:**
```env
# In .env file
ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000,http://localhost:3001
```

**CORS Middleware Features:**
- ✅ Automatic preflight (OPTIONS) handling
- ✅ Credential support enabled
- ✅ Configurable allowed origins (environment-based)
- ✅ Proper header exposure for client-side access

**Location:** `internal/middleware/cors.go`

**To Add More Origins:**
Edit `.env` and add comma-separated origins:
```env
ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000,https://propvest.com
```

---

### 2. ✅ Standard API Response Format
**Status:** ✅ Already Implemented

All API endpoints return consistent JSON structures using the `StandardResponse` envelope.

**Success Response Structure:**
```json
{
  "success": true,
  "message": "Operation successful",
  "data": {
    // Your response data here
  },
  "request_id": "uuid-v4-request-id"
}
```

**Error Response Structure:**
```json
{
  "success": false,
  "message": "Human-readable error message",
  "code": "machine_readable_error_code",
  "errors": {
    "field_name": ["Error message 1", "Error message 2"]
  },
  "request_id": "uuid-v4-request-id"
}
```

**Validation Error (422) Structure:**
```json
{
  "success": false,
  "message": "Validation failed",
  "code": "validation_error",
  "errors": {
    "email": ["Must be a valid email address"],
    "password": ["Must be at least 8 characters"]
  },
  "request_id": "uuid-v4-request-id"
}
```

**Paginated Response Structure:**
```json
{
  "success": true,
  "data": [...],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 100,
    "pages": 5
  },
  "request_id": "uuid-v4-request-id"
}
```

**Location:** `internal/response/response.go`

---

### 3. ✅ JWT Authentication
**Status:** ✅ Fully Implemented

The backend uses JWT tokens for authentication with access + refresh token pattern.

**Token Response (Login/Register):**
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid",
      "email": "user@example.com",
      "full_name": "John Doe",
      "role": "investor",
      "is_email_verified": false,
      "kyc_status": "not_started"
    },
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "token_type": "Bearer"
  }
}
```

**Token Configuration:**
- Access Token TTL: 15 minutes (configurable via `ACCESS_TOKEN_TTL`)
- Refresh Token TTL: 30 days (configurable via `REFRESH_TOKEN_TTL`)
- Token Type: Bearer
- Algorithm: HS256

**Authentication Flow:**
1. User logs in → Receive access_token + refresh_token
2. Store tokens securely (localStorage or httpOnly cookies)
3. Include access_token in all API requests: `Authorization: Bearer <token>`
4. When access_token expires → Use refresh_token to get new tokens
5. When refresh_token expires → User must log in again

---

## 🔧 Frontend Setup Instructions

### Step 1: Install Axios or Fetch API Wrapper

```bash
npm install axios
# or use native fetch
```

### Step 2: Create API Client

Create `src/services/api.js` (or `api.ts` for TypeScript):

```javascript
import axios from 'axios';

// Base API URL
const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8081';

// Create axios instance
const apiClient = axios.create({
  baseURL: `${API_BASE_URL}/api/v1`,
  headers: {
    'Content-Type': 'application/json',
  },
  withCredentials: true, // Enable cookies if using httpOnly
});

// Request interceptor: Add auth token
apiClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('access_token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error)
);

// Response interceptor: Handle token refresh
apiClient.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config;

    // If 401 and not already retrying
    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true;

      try {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
          throw new Error('No refresh token');
        }

        // Call refresh endpoint
        const { data } = await axios.post(
          `${API_BASE_URL}/api/v1/auth/refresh`,
          { refresh_token: refreshToken }
        );

        // Update tokens
        localStorage.setItem('access_token', data.data.access_token);
        localStorage.setItem('refresh_token', data.data.refresh_token);

        // Retry original request
        originalRequest.headers.Authorization = `Bearer ${data.data.access_token}`;
        return apiClient(originalRequest);
      } catch (refreshError) {
        // Refresh failed - redirect to login
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        window.location.href = '/login';
        return Promise.reject(refreshError);
      }
    }

    return Promise.reject(error);
  }
);

export default apiClient;
```

### Step 3: Create Auth Service

Create `src/services/authService.js`:

```javascript
import apiClient from './api';

const authService = {
  // Register new user
  async register(userData) {
    const response = await apiClient.post('/auth/register', userData);
    const { access_token, refresh_token } = response.data.data;
    
    // Store tokens
    localStorage.setItem('access_token', access_token);
    localStorage.setItem('refresh_token', refresh_token);
    
    return response.data;
  },

  // Login user
  async login(credentials) {
    const response = await apiClient.post('/auth/login', credentials);
    const { access_token, refresh_token } = response.data.data;
    
    // Store tokens
    localStorage.setItem('access_token', access_token);
    localStorage.setItem('refresh_token', refresh_token);
    
    return response.data;
  },

  // Logout user
  async logout() {
    const refreshToken = localStorage.getItem('refresh_token');
    try {
      await apiClient.post('/auth/logout', { refresh_token: refreshToken });
    } finally {
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
    }
  },

  // Refresh access token
  async refreshToken() {
    const refreshToken = localStorage.getItem('refresh_token');
    const response = await apiClient.post('/auth/refresh', {
      refresh_token: refreshToken,
    });
    
    const { access_token, refresh_token: newRefreshToken } = response.data.data;
    localStorage.setItem('access_token', access_token);
    localStorage.setItem('refresh_token', newRefreshToken);
    
    return response.data;
  },

  // Get current user
  async getCurrentUser() {
    const response = await apiClient.get('/users/me');
    return response.data;
  },

  // Check if user is authenticated
  isAuthenticated() {
    return !!localStorage.getItem('access_token');
  },
};

export default authService;
```

### Step 4: Create Environment Variables

Create `.env.local` in your React project:

```env
VITE_API_URL=http://localhost:8081
```

### Step 5: Example Usage in Components

```javascript
import { useState } from 'react';
import authService from './services/authService';

function LoginForm() {
  const [credentials, setCredentials] = useState({
    email: '',
    password: '',
  });
  const [error, setError] = useState(null);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);

    try {
      const response = await authService.login(credentials);
      console.log('Login successful:', response.data.user);
      // Redirect to dashboard
      window.location.href = '/dashboard';
    } catch (err) {
      // Handle error
      if (err.response?.data) {
        setError(err.response.data.message);
      } else {
        setError('Network error. Please try again.');
      }
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      {error && <div className="error">{error}</div>}
      
      <input
        type="email"
        value={credentials.email}
        onChange={(e) => setCredentials({ ...credentials, email: e.target.value })}
        placeholder="Email"
        required
      />
      
      <input
        type="password"
        value={credentials.password}
        onChange={(e) => setCredentials({ ...credentials, password: e.target.value })}
        placeholder="Password"
        required
      />
      
      <button type="submit">Login</button>
    </form>
  );
}
```

---

## 🔐 Security Best Practices

### Token Storage Options

**1. localStorage (Simpler, recommended for MVP)**
- ✅ Simple to implement
- ✅ Survives page reloads
- ❌ Vulnerable to XSS attacks
- ✅ Good for development/MVP

**2. httpOnly Cookies (More Secure)**
- ✅ Protected from XSS
- ✅ Automatic token sending
- ❌ Requires CSRF protection
- ✅ Better for production

**3. sessionStorage (Session-based)**
- ✅ Protected from XSS in different tabs
- ❌ Lost on page reload
- ✅ Good for sensitive temporary data

**Recommendation:** Start with localStorage for development, migrate to httpOnly cookies for production.

### CORS Security

**Development:**
```env
ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000
```

**Production:**
```env
ALLOWED_ORIGINS=https://propvest.com,https://www.propvest.com,https://app.propvest.com
```

**Never use `*` (wildcard) in production!**

---

## 📡 API Endpoints Reference

### Authentication Endpoints

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| POST | `/api/v1/auth/register` | Register new user | No |
| POST | `/api/v1/auth/login` | Login user | No |
| POST | `/api/v1/auth/logout` | Logout user | Yes |
| POST | `/api/v1/auth/refresh` | Refresh access token | No |
| POST | `/api/v1/auth/verify-email` | Verify email with token | No |
| POST | `/api/v1/auth/resend-verification` | Resend verification email | Yes |
| POST | `/api/v1/auth/forgot-password` | Request password reset | No |
| POST | `/api/v1/auth/reset-password` | Reset password with token | No |

### User Endpoints

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/v1/users/me` | Get current user profile | Yes |
| PUT | `/api/v1/users/me` | Update user profile | Yes |
| POST | `/api/v1/users/me/avatar` | Upload avatar image | Yes |
| POST | `/api/v1/users/me/phone/verify` | Send OTP to phone | Yes |
| POST | `/api/v1/users/me/phone/confirm` | Confirm OTP code | Yes |

### Wallet Endpoints

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/api/v1/wallets/me` | Get user wallet | Yes |
| GET | `/api/v1/wallets/me/transactions` | Get wallet transactions | Yes |
| POST | `/api/v1/wallets/deposit` | Initiate deposit | Yes |
| POST | `/api/v1/wallets/withdraw` | Request withdrawal | Yes |

For complete API documentation, see: `postman/API_ENDPOINTS_REFERENCE.md`

---

## 🧪 Testing the Integration

### 1. Start the Backend
```bash
cd propvest-backend
make run
# Backend runs on http://localhost:8081
```

### 2. Start the Frontend
```bash
cd your-react-app
npm run dev
# Frontend runs on http://localhost:5173
```

### 3. Test CORS
Open browser console and try:
```javascript
fetch('http://localhost:8081/health')
  .then(r => r.json())
  .then(console.log);
// Should return: { status: "healthy", timestamp: "..." }
```

### 4. Test Authentication
```javascript
fetch('http://localhost:8081/api/v1/auth/register', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    full_name: 'Test User',
    email: 'test@example.com',
    password: 'SecurePass123!',
    phone: '+2348012345678'
  })
})
  .then(r => r.json())
  .then(console.log);
// Should return tokens in response.data
```

---

## 🚀 What Else Needs to Be Done?

### Backend Enhancements for Production

#### 1. ✅ Rate Limiting (Recommended)
**Status:** Middleware exists but not enabled
**Priority:** HIGH for production

Add rate limiting middleware to prevent abuse:
```go
// In cmd/api/main.go
r.Use(middleware.RateLimiter(cfg))
```

**Configuration needed:**
```env
RATE_LIMIT_ANONYMOUS=100      # requests per minute
RATE_LIMIT_AUTHENTICATED=300
RATE_LIMIT_AUTH_ENDPOINTS=10  # for login/register
```

#### 2. ⚠️ Request Validation & Sanitization
**Status:** Basic validation exists
**Priority:** MEDIUM

- Input sanitization for XSS prevention
- SQL injection prevention (using parameterized queries - ✅ already done)
- File upload validation (size, type, malware scanning)

#### 3. ⚠️ API Versioning Strategy
**Status:** v1 implemented
**Priority:** LOW (plan for future)

Currently using `/api/v1` prefix. Plan for:
- Version deprecation policy
- Breaking change handling
- Migration guides

#### 4. 🔴 Logging & Monitoring
**Status:** Basic logging exists
**Priority:** HIGH for production

Add:
- Request/response logging (✅ already done)
- Error tracking (consider Sentry)
- Performance monitoring (APM)
- Audit logs for sensitive operations

#### 5. ⚠️ API Documentation
**Status:** Postman collection exists
**Priority:** MEDIUM

Consider adding:
- OpenAPI/Swagger documentation
- Interactive API explorer
- SDK generation for frontend

#### 6. 🔴 Health Checks
**Status:** ✅ Basic health check exists
**Priority:** MEDIUM

Enhance:
```go
// Add dependency checks
GET /health        // Basic liveness
GET /health/ready  // Readiness (DB, Redis, etc.)
```

#### 7. ⚠️ Graceful Shutdown
**Status:** ✅ Already implemented
**Priority:** HIGH for production

Already handles:
- ✅ Connection draining
- ✅ Cleanup on shutdown
- ✅ Signal handling

#### 8. ⚠️ Database Connection Pooling
**Status:** ✅ Configured with defaults
**Priority:** MEDIUM

Tune for production load:
```env
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME=1h
```

#### 9. 🔴 Security Headers
**Status:** Basic CORS headers
**Priority:** HIGH for production

Add middleware for:
- `X-Frame-Options: DENY`
- `X-Content-Type-Options: nosniff`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security` (HSTS)
- `Content-Security-Policy`

#### 10. ⚠️ File Upload Handling
**Status:** Cloudinary integration exists
**Priority:** MEDIUM

Add validation for:
- File size limits
- Allowed MIME types
- Virus scanning (ClamAV)
- Rate limiting on uploads

---

## 📚 Additional Resources

### Backend Documentation
- **Architecture:** `docs/01-Architecture/1.1-SYSTEM_ARCHITECTURE.md`
- **API Design:** `docs/03-API/3.1-API_DESIGN.md`
- **Security:** `docs/04-Security/4.2-AUTHENTICATION_AND_AUTHORIZATION.md`
- **Error Handling:** `docs/06-Engineering/6.2-ERROR_HANDLING_AND_LOGGING.md`

### Testing & Development
- **Postman Collection:** `postman/propvest-api.postman_collection.json`
- **API Reference:** `postman/API_ENDPOINTS_REFERENCE.md`
- **Build Guide:** `handoff/10-BUILD-GUIDE.md`

### Deployment
- **Docker Compose:** `docker-compose.yml` (includes PostgreSQL, Redis, Mailpit)
- **Ngrok Setup:** `SETUP_NGROK.md` (for webhook testing)
- **Paystack Integration:** `PAYSTACK_INTEGRATION_GUIDE.md`

---

## 🐛 Troubleshooting

### CORS Errors
**Error:** `Access to fetch at 'http://localhost:8081' from origin 'http://localhost:5173' has been blocked by CORS`

**Solution:**
1. Check `ALLOWED_ORIGINS` in `.env` includes your frontend URL
2. Restart backend after changing `.env`
3. Verify frontend is using the correct API URL

### 401 Unauthorized
**Error:** API returns 401 even with token

**Solution:**
1. Check token is being sent: `Authorization: Bearer <token>`
2. Verify token hasn't expired (15min default)
3. Check token format (no extra spaces/newlines)
4. Try refreshing token

### Connection Refused
**Error:** `Failed to fetch` or `net::ERR_CONNECTION_REFUSED`

**Solution:**
1. Verify backend is running: `curl http://localhost:8081/health`
2. Check port matches (default: 8081)
3. Ensure no firewall blocking

### Validation Errors Not Showing
**Error:** Getting 400 but no field errors

**Solution:**
1. Check response structure: `response.data.errors`
2. Validation errors are in 422 responses
3. 400 is for malformed requests (invalid JSON)

---

## 📞 Support

For issues or questions:
1. Check the documentation in `docs/`
2. Review the handoff guide in `handoff/`
3. Test with Postman collection in `postman/`
4. Check error logs: Backend prints structured JSON logs

---

**Last Updated:** 2024
**Backend Version:** v1
**Minimum Frontend Requirements:**
- Node.js 16+
- React 18+ (or any modern framework)
- Axios or Fetch API support
