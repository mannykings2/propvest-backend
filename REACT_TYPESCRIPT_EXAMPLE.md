# React + TypeScript Integration Example

Complete TypeScript setup for PropVest backend integration.

---

## 📦 Installation

```bash
npm install axios
npm install --save-dev @types/node
```

---

## 🎯 TypeScript Type Definitions

### Create `src/types/api.ts`

```typescript
// ============================================================================
// API Response Types
// ============================================================================

export interface ApiResponse<T = any> {
  success: boolean;
  data?: T;
  message?: string;
  code?: string;
  errors?: Record<string, string[]>;
  request_id?: string;
}

export interface PaginatedResponse<T> {
  success: boolean;
  data: T[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    pages: number;
  };
  request_id?: string;
}

// ============================================================================
// Auth Types
// ============================================================================

export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegisterRequest {
  full_name: string;
  email: string;
  password: string;
  phone: string;
}

export interface AuthResponse {
  user: User;
  access_token: string;
  refresh_token: string;
  token_type: string;
}

export interface RefreshTokenRequest {
  refresh_token: string;
}

// ============================================================================
// User Types
// ============================================================================

export type UserRole = 'investor' | 'admin' | 'super_admin';
export type KYCStatus = 'not_started' | 'pending' | 'approved' | 'rejected';

export interface User {
  id: string;
  email: string;
  full_name: string;
  phone: string | null;
  avatar_url: string | null;
  role: UserRole;
  is_email_verified: boolean;
  is_phone_verified: boolean;
  kyc_status: KYCStatus;
  last_login_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface UpdateProfileRequest {
  full_name?: string;
  phone?: string;
}

export interface VerifyPhoneRequest {
  phone: string;
}

export interface ConfirmPhoneRequest {
  code: string;
}

// ============================================================================
// Wallet Types
// ============================================================================

export type TransactionType = 
  | 'deposit' 
  | 'withdrawal' 
  | 'investment' 
  | 'return' 
  | 'fee' 
  | 'refund';

export type TransactionStatus = 
  | 'pending' 
  | 'processing' 
  | 'completed' 
  | 'failed' 
  | 'cancelled';

export interface Wallet {
  id: string;
  user_id: string;
  balance: number;
  currency: string;
  total_deposits: number;
  total_withdrawals: number;
  total_invested: number;
  total_returns: number;
  created_at: string;
  updated_at: string;
}

export interface Transaction {
  id: string;
  wallet_id: string;
  type: TransactionType;
  amount: number;
  balance_before: number;
  balance_after: number;
  currency: string;
  status: TransactionStatus;
  description: string;
  reference: string;
  metadata: Record<string, any>;
  created_at: string;
  updated_at: string;
}

export interface DepositRequest {
  amount: number;
  payment_method: 'card' | 'bank_transfer';
}

export interface DepositResponse {
  reference: string;
  amount: number;
  authorization_url: string;
  access_code: string;
}

export interface WithdrawalRequest {
  amount: number;
  bank_code: string;
  account_number: string;
  account_name: string;
}

// ============================================================================
// Error Types
// ============================================================================

export interface ApiError {
  success: false;
  message: string;
  code: string;
  errors?: Record<string, string[]>;
  request_id?: string;
}

export class ApiException extends Error {
  constructor(
    public status: number,
    public data: ApiError
  ) {
    super(data.message);
    this.name = 'ApiException';
  }
}
```

---

## 🔧 API Client Setup

### Create `src/lib/api.ts`

```typescript
import axios, { AxiosInstance, AxiosError, InternalAxiosRequestConfig } from 'axios';
import { ApiResponse, ApiError, ApiException } from '../types/api';

// ============================================================================
// Configuration
// ============================================================================

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8081';
const API_VERSION = 'v1';

// ============================================================================
// Token Management
// ============================================================================

export const TokenStorage = {
  getAccessToken: (): string | null => {
    return localStorage.getItem('access_token');
  },

  setAccessToken: (token: string): void => {
    localStorage.setItem('access_token', token);
  },

  getRefreshToken: (): string | null => {
    return localStorage.getItem('refresh_token');
  },

  setRefreshToken: (token: string): void => {
    localStorage.setItem('refresh_token', token);
  },

  setTokens: (accessToken: string, refreshToken: string): void => {
    TokenStorage.setAccessToken(accessToken);
    TokenStorage.setRefreshToken(refreshToken);
  },

  clearTokens: (): void => {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
  },

  hasTokens: (): boolean => {
    return !!TokenStorage.getAccessToken();
  },
};

// ============================================================================
// Axios Instance
// ============================================================================

const apiClient: AxiosInstance = axios.create({
  baseURL: `${API_BASE_URL}/api/${API_VERSION}`,
  headers: {
    'Content-Type': 'application/json',
  },
  timeout: 30000, // 30 seconds
});

// ============================================================================
// Request Interceptor - Add Auth Token
// ============================================================================

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = TokenStorage.getAccessToken();
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// ============================================================================
// Response Interceptor - Handle Token Refresh
// ============================================================================

let isRefreshing = false;
let failedQueue: Array<{
  resolve: (value?: any) => void;
  reject: (reason?: any) => void;
}> = [];

const processQueue = (error: any = null): void => {
  failedQueue.forEach((prom) => {
    if (error) {
      prom.reject(error);
    } else {
      prom.resolve();
    }
  });
  failedQueue = [];
};

apiClient.interceptors.response.use(
  (response) => response,
  async (error: AxiosError<ApiError>) => {
    const originalRequest = error.config as InternalAxiosRequestConfig & {
      _retry?: boolean;
    };

    // If not a 401 or already retried, reject immediately
    if (error.response?.status !== 401 || originalRequest._retry) {
      return Promise.reject(
        new ApiException(
          error.response?.status || 500,
          error.response?.data || {
            success: false,
            message: 'An unexpected error occurred',
            code: 'unknown_error',
          }
        )
      );
    }

    if (isRefreshing) {
      // Queue this request
      return new Promise((resolve, reject) => {
        failedQueue.push({ resolve, reject });
      })
        .then(() => {
          return apiClient(originalRequest);
        })
        .catch((err) => {
          return Promise.reject(err);
        });
    }

    originalRequest._retry = true;
    isRefreshing = true;

    const refreshToken = TokenStorage.getRefreshToken();
    if (!refreshToken) {
      processQueue(new Error('No refresh token available'));
      TokenStorage.clearTokens();
      window.location.href = '/login';
      return Promise.reject(error);
    }

    try {
      const { data } = await axios.post<ApiResponse<{
        access_token: string;
        refresh_token: string;
      }>>(
        `${API_BASE_URL}/api/${API_VERSION}/auth/refresh`,
        { refresh_token: refreshToken }
      );

      if (data.success && data.data) {
        TokenStorage.setTokens(data.data.access_token, data.data.refresh_token);
        processQueue();

        // Retry original request with new token
        if (originalRequest.headers) {
          originalRequest.headers.Authorization = `Bearer ${data.data.access_token}`;
        }
        return apiClient(originalRequest);
      } else {
        throw new Error('Token refresh failed');
      }
    } catch (refreshError) {
      processQueue(refreshError);
      TokenStorage.clearTokens();
      window.location.href = '/login';
      return Promise.reject(refreshError);
    } finally {
      isRefreshing = false;
    }
  }
);

// ============================================================================
// Export
// ============================================================================

export default apiClient;
export { API_BASE_URL, API_VERSION };
```

---

## 🔐 Auth Service

### Create `src/services/authService.ts`

```typescript
import apiClient, { TokenStorage } from '../lib/api';
import {
  ApiResponse,
  LoginRequest,
  RegisterRequest,
  AuthResponse,
  User,
} from '../types/api';

export const authService = {
  /**
   * Register a new user
   */
  async register(data: RegisterRequest): Promise<AuthResponse> {
    const response = await apiClient.post<ApiResponse<AuthResponse>>(
      '/auth/register',
      data
    );

    if (response.data.success && response.data.data) {
      const authData = response.data.data;
      TokenStorage.setTokens(authData.access_token, authData.refresh_token);
      return authData;
    }

    throw new Error('Registration failed');
  },

  /**
   * Login user
   */
  async login(credentials: LoginRequest): Promise<AuthResponse> {
    const response = await apiClient.post<ApiResponse<AuthResponse>>(
      '/auth/login',
      credentials
    );

    if (response.data.success && response.data.data) {
      const authData = response.data.data;
      TokenStorage.setTokens(authData.access_token, authData.refresh_token);
      return authData;
    }

    throw new Error('Login failed');
  },

  /**
   * Logout user
   */
  async logout(): Promise<void> {
    const refreshToken = TokenStorage.getRefreshToken();
    try {
      if (refreshToken) {
        await apiClient.post('/auth/logout', { refresh_token: refreshToken });
      }
    } finally {
      TokenStorage.clearTokens();
    }
  },

  /**
   * Get current authenticated user
   */
  async getCurrentUser(): Promise<User> {
    const response = await apiClient.get<ApiResponse<User>>('/users/me');

    if (response.data.success && response.data.data) {
      return response.data.data;
    }

    throw new Error('Failed to fetch user');
  },

  /**
   * Check if user is authenticated
   */
  isAuthenticated(): boolean {
    return TokenStorage.hasTokens();
  },

  /**
   * Request password reset
   */
  async forgotPassword(email: string): Promise<void> {
    await apiClient.post('/auth/forgot-password', { email });
  },

  /**
   * Reset password with token
   */
  async resetPassword(token: string, password: string): Promise<void> {
    await apiClient.post('/auth/reset-password', { token, password });
  },

  /**
   * Verify email with token
   */
  async verifyEmail(token: string): Promise<void> {
    await apiClient.post('/auth/verify-email', { token });
  },

  /**
   * Resend verification email
   */
  async resendVerification(): Promise<void> {
    await apiClient.post('/auth/resend-verification');
  },
};
```

---

## 👤 User Service

### Create `src/services/userService.ts`

```typescript
import apiClient from '../lib/api';
import {
  ApiResponse,
  User,
  UpdateProfileRequest,
  VerifyPhoneRequest,
  ConfirmPhoneRequest,
} from '../types/api';

export const userService = {
  /**
   * Get current user profile
   */
  async getProfile(): Promise<User> {
    const response = await apiClient.get<ApiResponse<User>>('/users/me');
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error('Failed to fetch profile');
  },

  /**
   * Update user profile
   */
  async updateProfile(data: UpdateProfileRequest): Promise<User> {
    const response = await apiClient.put<ApiResponse<User>>('/users/me', data);
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error('Failed to update profile');
  },

  /**
   * Upload avatar
   */
  async uploadAvatar(file: File): Promise<User> {
    const formData = new FormData();
    formData.append('avatar', file);

    const response = await apiClient.post<ApiResponse<User>>(
      '/users/me/avatar',
      formData,
      {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      }
    );

    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error('Failed to upload avatar');
  },

  /**
   * Send phone verification OTP
   */
  async sendPhoneOTP(data: VerifyPhoneRequest): Promise<void> {
    await apiClient.post('/users/me/phone/verify', data);
  },

  /**
   * Confirm phone with OTP
   */
  async confirmPhone(data: ConfirmPhoneRequest): Promise<User> {
    const response = await apiClient.post<ApiResponse<User>>(
      '/users/me/phone/confirm',
      data
    );
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error('Failed to confirm phone');
  },
};
```

---

## 💰 Wallet Service

### Create `src/services/walletService.ts`

```typescript
import apiClient from '../lib/api';
import {
  ApiResponse,
  PaginatedResponse,
  Wallet,
  Transaction,
  DepositRequest,
  DepositResponse,
  WithdrawalRequest,
} from '../types/api';

export const walletService = {
  /**
   * Get user wallet
   */
  async getWallet(): Promise<Wallet> {
    const response = await apiClient.get<ApiResponse<Wallet>>('/wallets/me');
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error('Failed to fetch wallet');
  },

  /**
   * Get wallet transactions
   */
  async getTransactions(
    page: number = 1,
    limit: number = 20
  ): Promise<PaginatedResponse<Transaction>> {
    const response = await apiClient.get<PaginatedResponse<Transaction>>(
      '/wallets/me/transactions',
      {
        params: { page, limit },
      }
    );
    if (response.data.success) {
      return response.data;
    }
    throw new Error('Failed to fetch transactions');
  },

  /**
   * Initiate deposit
   */
  async deposit(data: DepositRequest): Promise<DepositResponse> {
    // Convert amount to kobo (multiply by 100)
    const requestData = {
      ...data,
      amount: data.amount * 100,
    };

    const response = await apiClient.post<ApiResponse<DepositResponse>>(
      '/wallets/deposit',
      requestData
    );

    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error('Failed to initiate deposit');
  },

  /**
   * Request withdrawal
   */
  async withdraw(data: WithdrawalRequest): Promise<void> {
    // Convert amount to kobo (multiply by 100)
    const requestData = {
      ...data,
      amount: data.amount * 100,
    };

    await apiClient.post('/wallets/withdraw', requestData);
  },
};
```

---

## 🎨 React Hook Examples

### Create `src/hooks/useAuth.ts`

```typescript
import { useState, useEffect } from 'react';
import { authService } from '../services/authService';
import { User, LoginRequest, RegisterRequest } from '../types/api';
import { ApiException } from '../types/api';

export const useAuth = () => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchUser = async () => {
      if (!authService.isAuthenticated()) {
        setLoading(false);
        return;
      }

      try {
        const userData = await authService.getCurrentUser();
        setUser(userData);
      } catch (err) {
        setUser(null);
      } finally {
        setLoading(false);
      }
    };

    fetchUser();
  }, []);

  const login = async (credentials: LoginRequest): Promise<void> => {
    setError(null);
    setLoading(true);
    try {
      const { user: userData } = await authService.login(credentials);
      setUser(userData);
    } catch (err) {
      const error = err as ApiException;
      setError(error.data?.message || 'Login failed');
      throw err;
    } finally {
      setLoading(false);
    }
  };

  const register = async (data: RegisterRequest): Promise<void> => {
    setError(null);
    setLoading(true);
    try {
      const { user: userData } = await authService.register(data);
      setUser(userData);
    } catch (err) {
      const error = err as ApiException;
      setError(error.data?.message || 'Registration failed');
      throw err;
    } finally {
      setLoading(false);
    }
  };

  const logout = async (): Promise<void> => {
    await authService.logout();
    setUser(null);
  };

  return {
    user,
    loading,
    error,
    login,
    register,
    logout,
    isAuthenticated: !!user,
  };
};
```

### Create `src/hooks/useWallet.ts`

```typescript
import { useState, useEffect } from 'react';
import { walletService } from '../services/walletService';
import { Wallet, Transaction, PaginatedResponse } from '../types/api';

export const useWallet = () => {
  const [wallet, setWallet] = useState<Wallet | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchWallet = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await walletService.getWallet();
      setWallet(data);
    } catch (err) {
      setError('Failed to fetch wallet');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchWallet();
  }, []);

  return {
    wallet,
    loading,
    error,
    refetch: fetchWallet,
  };
};

export const useTransactions = (page: number = 1, limit: number = 20) => {
  const [data, setData] = useState<PaginatedResponse<Transaction> | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchTransactions = async () => {
      setLoading(true);
      setError(null);
      try {
        const response = await walletService.getTransactions(page, limit);
        setData(response);
      } catch (err) {
        setError('Failed to fetch transactions');
      } finally {
        setLoading(false);
      }
    };

    fetchTransactions();
  }, [page, limit]);

  return { data, loading, error };
};
```

---

## 🧩 Component Examples

### Login Form Component

```typescript
import React, { useState } from 'react';
import { useAuth } from '../hooks/useAuth';
import { useNavigate } from 'react-router-dom';
import { ApiException } from '../types/api';

export const LoginForm: React.FC = () => {
  const { login } = useAuth();
  const navigate = useNavigate();
  
  const [formData, setFormData] = useState({
    email: '',
    password: '',
  });
  const [errors, setErrors] = useState<Record<string, string[]>>({});
  const [generalError, setGeneralError] = useState<string>('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrors({});
    setGeneralError('');
    setLoading(true);

    try {
      await login(formData);
      navigate('/dashboard');
    } catch (err) {
      const error = err as ApiException;
      if (error.data.errors) {
        setErrors(error.data.errors);
      } else {
        setGeneralError(error.data.message);
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      {generalError && (
        <div className="error-banner">{generalError}</div>
      )}

      <div className="form-group">
        <label htmlFor="email">Email</label>
        <input
          id="email"
          type="email"
          value={formData.email}
          onChange={(e) => setFormData({ ...formData, email: e.target.value })}
          required
        />
        {errors.email && (
          <div className="field-error">{errors.email[0]}</div>
        )}
      </div>

      <div className="form-group">
        <label htmlFor="password">Password</label>
        <input
          id="password"
          type="password"
          value={formData.password}
          onChange={(e) => setFormData({ ...formData, password: e.target.value })}
          required
        />
        {errors.password && (
          <div className="field-error">{errors.password[0]}</div>
        )}
      </div>

      <button type="submit" disabled={loading}>
        {loading ? 'Logging in...' : 'Login'}
      </button>
    </form>
  );
};
```

### Wallet Display Component

```typescript
import React from 'react';
import { useWallet } from '../hooks/useWallet';

export const WalletCard: React.FC = () => {
  const { wallet, loading, error } = useWallet();

  if (loading) return <div>Loading wallet...</div>;
  if (error) return <div className="error">{error}</div>;
  if (!wallet) return <div>No wallet found</div>;

  const formatCurrency = (amount: number): string => {
    return (amount / 100).toLocaleString('en-NG', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    });
  };

  return (
    <div className="wallet-card">
      <h2>Wallet Balance</h2>
      <p className="balance">
        ₦{formatCurrency(wallet.balance)}
      </p>
      <div className="wallet-stats">
        <div>
          <span>Total Deposits:</span>
          <span>₦{formatCurrency(wallet.total_deposits)}</span>
        </div>
        <div>
          <span>Total Invested:</span>
          <span>₦{formatCurrency(wallet.total_invested)}</span>
        </div>
        <div>
          <span>Total Returns:</span>
          <span>₦{formatCurrency(wallet.total_returns)}</span>
        </div>
      </div>
    </div>
  );
};
```

---

## 🛡️ Protected Route Component

```typescript
import React from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

export const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const { user, loading } = useAuth();

  if (loading) {
    return <div>Loading...</div>;
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
};
```

---

## 📝 Environment Variables

Create `.env.local`:

```env
VITE_API_URL=http://localhost:8081
```

---

**Complete TypeScript setup for PropVest backend integration!**
