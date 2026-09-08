# Frontend API Quick Start Guide

Quick reference for integrating React frontend with PropVest backend API.

---

## 🚀 Quick Setup (5 Minutes)

### Step 1: Install Dependencies
```bash
npm install axios
```

### Step 2: Create `.env.local`
```env
VITE_API_URL=http://localhost:8081
```

### Step 3: Copy API Client Code

Create `src/lib/api.js`:

```javascript
import axios from 'axios';

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL + '/api/v1',
  headers: { 'Content-Type': 'application/json' },
});

// Add token to requests
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

// Handle token refresh on 401
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config;
    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true;
      try {
        const { data } = await axios.post(
          `${import.meta.env.VITE_API_URL}/api/v1/auth/refresh`,
          { refresh_token: localStorage.getItem('refresh_token') }
        );
        localStorage.setItem('access_token', data.data.access_token);
        localStorage.setItem('refresh_token', data.data.refresh_token);
        originalRequest.headers.Authorization = `Bearer ${data.data.access_token}`;
        return api(originalRequest);
      } catch {
        localStorage.clear();
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

export default api;
```

### Step 4: Start Using the API

```javascript
import api from './lib/api';

// Register
const register = async (userData) => {
  const { data } = await api.post('/auth/register', userData);
  localStorage.setItem('access_token', data.data.access_token);
  localStorage.setItem('refresh_token', data.data.refresh_token);
  return data.data.user;
};

// Login
const login = async (email, password) => {
  const { data } = await api.post('/auth/login', { email, password });
  localStorage.setItem('access_token', data.data.access_token);
  localStorage.setItem('refresh_token', data.data.refresh_token);
  return data.data.user;
};

// Get current user
const getCurrentUser = async () => {
  const { data } = await api.get('/users/me');
  return data.data;
};

// Get wallet
const getWallet = async () => {
  const { data } = await api.get('/wallets/me');
  return data.data;
};

// Logout
const logout = async () => {
  try {
    await api.post('/auth/logout', {
      refresh_token: localStorage.getItem('refresh_token')
    });
  } finally {
    localStorage.clear();
    window.location.href = '/login';
  }
};
```

---

## 📋 API Response Format

All responses follow this structure:

**Success:**
```json
{
  "success": true,
  "data": { /* your data */ },
  "request_id": "uuid"
}
```

**Error:**
```json
{
  "success": false,
  "message": "Error description",
  "code": "error_code",
  "request_id": "uuid"
}
```

**Validation Error (422):**
```json
{
  "success": false,
  "message": "Validation failed",
  "code": "validation_error",
  "errors": {
    "email": ["Must be a valid email"],
    "password": ["Must be at least 8 characters"]
  },
  "request_id": "uuid"
}
```

---

## 🔑 Authentication Flow

```javascript
// 1. Register/Login - Get tokens
const { data } = await api.post('/auth/login', { email, password });
const { access_token, refresh_token } = data.data;

// 2. Store tokens
localStorage.setItem('access_token', access_token);
localStorage.setItem('refresh_token', refresh_token);

// 3. Make authenticated requests (automatic via interceptor)
const user = await api.get('/users/me');

// 4. Token refresh (automatic on 401 via interceptor)
// Or manual:
const refreshResponse = await api.post('/auth/refresh', { refresh_token });

// 5. Logout
await api.post('/auth/logout', { refresh_token });
localStorage.clear();
```

---

## 📡 Common API Endpoints

### Auth
```javascript
// Register
POST /auth/register
Body: { full_name, email, password, phone }

// Login
POST /auth/login
Body: { email, password }

// Logout
POST /auth/logout
Body: { refresh_token }

// Refresh token
POST /auth/refresh
Body: { refresh_token }

// Verify email
POST /auth/verify-email
Body: { token }

// Forgot password
POST /auth/forgot-password
Body: { email }

// Reset password
POST /auth/reset-password
Body: { token, password }
```

### User
```javascript
// Get current user
GET /users/me

// Update profile
PUT /users/me
Body: { full_name, phone }

// Upload avatar
POST /users/me/avatar
Body: FormData with 'avatar' field

// Send phone OTP
POST /users/me/phone/verify
Body: { phone }

// Confirm phone OTP
POST /users/me/phone/confirm
Body: { code }
```

### Wallet
```javascript
// Get wallet
GET /wallets/me

// Get transactions
GET /wallets/me/transactions?page=1&limit=20

// Deposit
POST /wallets/deposit
Body: { amount, payment_method: "card" }

// Withdraw
POST /wallets/withdraw
Body: { amount, bank_code, account_number, account_name }
```

---

## 🎨 React Component Examples

### Login Form
```jsx
import { useState } from 'react';
import api from './lib/api';

export default function LoginForm() {
  const [form, setForm] = useState({ email: '', password: '' });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const { data } = await api.post('/auth/login', form);
      localStorage.setItem('access_token', data.data.access_token);
      localStorage.setItem('refresh_token', data.data.refresh_token);
      window.location.href = '/dashboard';
    } catch (err) {
      setError(err.response?.data?.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      {error && <div className="error">{error}</div>}
      
      <input
        type="email"
        placeholder="Email"
        value={form.email}
        onChange={(e) => setForm({ ...form, email: e.target.value })}
        required
      />
      
      <input
        type="password"
        placeholder="Password"
        value={form.password}
        onChange={(e) => setForm({ ...form, password: e.target.value })}
        required
      />
      
      <button type="submit" disabled={loading}>
        {loading ? 'Loading...' : 'Login'}
      </button>
    </form>
  );
}
```

### Protected Route
```jsx
import { useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import api from './lib/api';

export default function ProtectedRoute({ children }) {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const { data } = await api.get('/users/me');
        setUser(data.data);
      } catch {
        setUser(null);
      } finally {
        setLoading(false);
      }
    };

    fetchUser();
  }, []);

  if (loading) return <div>Loading...</div>;
  if (!user) return <Navigate to="/login" />;

  return children;
}
```

### Wallet Display
```jsx
import { useEffect, useState } from 'react';
import api from './lib/api';

export default function WalletCard() {
  const [wallet, setWallet] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchWallet = async () => {
      try {
        const { data } = await api.get('/wallets/me');
        setWallet(data.data);
      } catch (err) {
        console.error('Failed to fetch wallet:', err);
      } finally {
        setLoading(false);
      }
    };

    fetchWallet();
  }, []);

  if (loading) return <div>Loading wallet...</div>;
  if (!wallet) return <div>No wallet found</div>;

  return (
    <div className="wallet-card">
      <h3>Wallet Balance</h3>
      <p className="balance">
        ₦{(wallet.balance / 100).toLocaleString('en-NG', {
          minimumFractionDigits: 2,
          maximumFractionDigits: 2,
        })}
      </p>
      <p className="currency">{wallet.currency}</p>
    </div>
  );
}
```

### Deposit Form
```jsx
import { useState } from 'react';
import api from './lib/api';

export default function DepositForm() {
  const [amount, setAmount] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleDeposit = async (e) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      const { data } = await api.post('/wallets/deposit', {
        amount: parseFloat(amount) * 100, // Convert to kobo
        payment_method: 'card',
      });

      // Redirect to Paystack payment page
      window.location.href = data.data.authorization_url;
    } catch (err) {
      setError(err.response?.data?.message || 'Deposit failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <form onSubmit={handleDeposit}>
      {error && <div className="error">{error}</div>}
      
      <label>
        Amount (₦)
        <input
          type="number"
          min="100"
          step="0.01"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          placeholder="Enter amount"
          required
        />
      </label>
      
      <button type="submit" disabled={loading}>
        {loading ? 'Processing...' : 'Deposit'}
      </button>
    </form>
  );
}
```

---

## 🧪 Testing

### Manual Test with cURL

```bash
# Health check
curl http://localhost:8081/health

# Register
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Test User",
    "email": "test@example.com",
    "password": "SecurePass123!",
    "phone": "+2348012345678"
  }'

# Login
curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePass123!"
  }'

# Get user (with token)
curl http://localhost:8081/api/v1/users/me \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

### Browser Console Test

```javascript
// Test CORS
fetch('http://localhost:8081/health')
  .then(r => r.json())
  .then(console.log);

// Test Login
fetch('http://localhost:8081/api/v1/auth/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    email: 'test@example.com',
    password: 'SecurePass123!'
  })
})
  .then(r => r.json())
  .then(console.log);
```

---

## ⚠️ Common Issues

### CORS Error
```
Access to fetch blocked by CORS policy
```
**Fix:** Add your frontend URL to `ALLOWED_ORIGINS` in backend `.env`

### 401 Unauthorized
```json
{ "success": false, "code": "unauthorized" }
```
**Fix:** Check token is included: `Authorization: Bearer <token>`

### 422 Validation Error
```json
{ "success": false, "code": "validation_error", "errors": {...} }
```
**Fix:** Check required fields and format in request body

### Connection Refused
```
net::ERR_CONNECTION_REFUSED
```
**Fix:** Ensure backend is running on port 8081

---

## 📚 Full Documentation

For complete details, see:
- **Full Integration Guide:** `FRONTEND_INTEGRATION_GUIDE.md`
- **API Reference:** `postman/API_ENDPOINTS_REFERENCE.md`
- **Postman Collection:** `postman/propvest-api.postman_collection.json`

---

## 🎯 Checklist

- [ ] Install axios: `npm install axios`
- [ ] Create `.env.local` with `VITE_API_URL`
- [ ] Copy API client code to `src/lib/api.js`
- [ ] Test health endpoint: `curl http://localhost:8081/health`
- [ ] Test CORS from browser console
- [ ] Implement login form
- [ ] Store tokens in localStorage
- [ ] Add Authorization header to requests
- [ ] Implement token refresh logic
- [ ] Add logout functionality
- [ ] Create protected routes
- [ ] Handle error responses
- [ ] Display validation errors

---

**Backend URL:** http://localhost:8081  
**API Base:** http://localhost:8081/api/v1  
**Health Check:** http://localhost:8081/health
