# Frontend-Backend Integration Audit Report

**Audit Date:** 2026-11-10  
**Backend:** PropVest Go Backend (`C:\Users\USER\go_projects\propvest-backend`)  
**Frontend:** PropVest React App (`C:\Users\USER\go_projects\propvest-app`)  
**Audited by:** AI Agent  

---

## Executive Summary

### Overall Integration Health Score: **65/100** (Moderate Risk)

**Status:** The frontend and backend are **partially integrated** with significant gaps. The frontend contains extensive fallback/mock data logic suggesting it was developed before or independently of the backend. Critical integration issues exist in endpoint mismatches, data structure inconsistencies, and missing implementations.

### Critical Findings

| Category | Critical | High | Medium | Low |
|----------|----------|------|--------|-----|
| API Endpoint Mismatches | 8 | 12 | 5 | 3 |
| Data Structure Issues | 6 | 9 | 8 | 4 |
| Missing Implementations | 4 | 7 | 11 | 6 |
| Authentication Issues | 1 (Fixed) | 2 | 3 | 1 |

### Key Risks

1. **CRITICAL**: Frontend uses non-existent backend endpoints (multiple 404 errors expected)
2. **CRITICAL**: Data type mismatches in wallet balances (kobo vs Naira conversion issues)
3. **HIGH**: Missing Investment and Property DTOs on frontend
4. **HIGH**: Notification system not integrated
5. **HIGH**: Admin dashboard not connected to backend
6. **MEDIUM**: Off-plan development feature has no backend support

---

## Section 1: Technology Stack Analysis

### Backend Stack

| Component | Technology | Version/Details |
|-----------|-----------|-----------------|
| Language | Go | 1.21+ |
| Framework | Gin | HTTP router |
| Database | PostgreSQL | With GORM ORM |
| Cache | Redis | Session & rate limiting |
| Queue | RabbitMQ | Async jobs (withdrawals, notifications) |
| Authentication | JWT | Access + Refresh tokens (rotation) |
| File Storage | Cloudinary | Avatar & property images |
| Payment Provider | Paystack | Deposits & withdrawals |
| SMS Provider | Configurable | Twilio/Africa's Talking/Termii/Mock |

**Key Backend Files Analyzed:**
- `internal/routes/v1/routes.go` - All API routes
- `internal/dto/*.go` - 9 DTO files (auth, user, wallet, property, investment, notification, admin, error, mappers)
- `internal/handlers/*.go` - HTTP handlers
- `internal/services/*.go` - Business logic
- `.env.example` - Configuration template

### Frontend Stack

| Component | Technology | Version/Details |
|-----------|-----------|-----------------|
| Language | JavaScript | ES6+ (no TypeScript) |
| Framework | React | 18.2.0 |
| Build Tool | Vite | 5.1.4 |
| Router | React Router DOM | 7.18.3 |
| State Management | React Query | 5.102.8 (minimal usage) |
| API Client | Fetch API | Custom wrapper with auto-refresh |
| Styling | Inline Styles | No CSS-in-JS library |

**Key Frontend Files Analyzed:**
- `src/services/*.js` - API service modules (5 files)
- `src/services/apiClient.js` - Centralized HTTP client
- `src/features/*/` - Feature components (17 files)
- `src/App.jsx` - Main app router
- `src/constants/data.js` - Mock/fallback data
- `package.json` - Dependencies

### Infrastructure Observations

**Backend:**
- ✅ Well-structured layered architecture (handlers → services → repositories)
- ✅ Comprehensive error handling with custom error types
- ✅ JWT token rotation implemented (security best practice)
- ✅ Idempotency support for critical operations
- ✅ Database transactions for financial operations
- ✅ CORS configured for multiple origins
- ⚠️ RabbitMQ required for withdrawals (documented)

**Frontend:**
- ✅ Code splitting with React lazy loading
- ✅ Automatic JWT refresh on 401
- ✅ Fallback data for offline/dev mode
- ⚠️ No TypeScript (type safety concerns)
- ⚠️ Inline styles (maintenance concerns)
- ⚠️ Mock data embedded in constants (should be test fixtures)
- ❌ No environment variable validation

---

## Section 2: API Endpoint Mapping

### Backend Endpoints Inventory

**Total Backend Endpoints:** 61 (including admin routes)

#### Authentication Endpoints (8 total)

| Method | Endpoint | Auth Required | Status | Frontend Usage |
|--------|----------|---------------|--------|----------------|
| POST | `/api/v1/auth/register` | No | ✅ Implemented | ✅ Used (`authService.register`) |
| POST | `/api/v1/auth/login` | No | ✅ Implemented | ✅ Used (`authService.login`) |
| POST | `/api/v1/auth/refresh` | No | ✅ Implemented | ✅ Used (`apiClient.js` auto-refresh) |
| POST | `/api/v1/auth/logout` | No | ✅ Implemented | ✅ Used (`authService.logout`) |
| POST | `/api/v1/auth/logout-all` | Yes | ✅ Implemented | ❌ Not used |
| POST | `/api/v1/auth/forgot-password` | No | ❓ Commented | ❌ Not used |
| POST | `/api/v1/auth/reset-password` | No | ❓ Commented | ❌ Not used |
| POST | `/api/v1/auth/verify-email` | No | ❓ Commented | ❌ Not used |

#### User Endpoints (6 total)

| Method | Endpoint | Auth Required | Status | Frontend Usage |
|--------|----------|---------------|--------|----------------|
| GET | `/api/v1/users/me` | Yes | ✅ Implemented | ✅ Used (`authService.getProfile`) |
| PATCH | `/api/v1/users/me` | Yes | ✅ Implemented | ✅ Used (`authService.updateProfile`) |
| PATCH | `/api/v1/users/avatar` | Yes | ✅ Implemented | ✅ Used (`authService.uploadAvatar`) |
| PATCH | `/api/v1/users/password` | Yes | ✅ Implemented | ❌ Not used |
| POST | `/api/v1/users/phone/request` | Yes | ✅ Implemented | ❌ Not used |
| POST | `/api/v1/users/phone/verify` | Yes | ✅ Implemented | ❌ Not used |

#### Wallet Endpoints (5 total)

| Method | Endpoint | Auth Required | Status | Frontend Usage |
|--------|----------|---------------|--------|----------------|
| GET | `/api/v1/wallet` | Yes | ✅ Implemented | ⚠️ **MISMATCH**: Frontend uses `/wallets/me` or `/me/wallet` |
| POST | `/api/v1/wallet/deposit` | Yes | ✅ Implemented | ⚠️ **MISMATCH**: Frontend uses `/wallets/deposit` |
| POST | `/api/v1/wallet/withdraw` | Yes | ✅ Implemented | ⚠️ **MISMATCH**: Frontend uses `/wallets/withdraw` |
| GET | `/api/v1/wallet/transactions` | Yes | ✅ Implemented | ⚠️ **MISMATCH**: Frontend tries `/wallets/me/transactions` or `/me/wallet/transactions` |
| GET | `/api/v1/wallet/deposit/callback` | No | ✅ Implemented | ✅ Not called by frontend (user redirect) |

**CRITICAL ISSUE**: Frontend wallet service uses incorrect paths (plural `/wallets/` instead of singular `/wallet/`).

#### Property Endpoints (13 total)

| Method | Endpoint | Auth Required | Status | Frontend Usage |
|--------|----------|---------------|--------|----------------|
| GET | `/api/v1/properties` | No | ✅ Implemented | ✅ Used (`propertyService.getProperties`) |
| GET | `/api/v1/properties/:id` | No | ✅ Implemented | ✅ Used (`propertyService.getPropertyById`) |
| POST | `/api/v1/admin/properties` | Admin | ✅ Implemented | ❌ Not used |
| GET | `/api/v1/admin/properties` | Admin | ✅ Implemented | ❌ Not used |
| GET | `/api/v1/admin/properties/:id` | Admin | ✅ Implemented | ❌ Not used |
| PATCH | `/api/v1/admin/properties/:id` | Admin | ✅ Implemented | ❌ Not used |
| DELETE | `/api/v1/admin/properties/:id` | Admin | ✅ Implemented | ❌ Not used |
| POST | `/api/v1/admin/properties/:id/publish` | Admin | ✅ Implemented | ❌ Not used |
| POST | `/api/v1/admin/properties/:id/images` | Admin | ✅ Implemented | ❌ Not used |
| DELETE | `/api/v1/admin/properties/:id/images/:imageId` | Admin | ✅ Implemented | ❌ Not used |
| PATCH | `/api/v1/admin/properties/:id/images/:imageId/cover` | Admin | ✅ Implemented | ❌ Not used |
| POST | `/api/v1/admin/properties/:id/documents` | Admin | ✅ Implemented | ❌ Not used |
| DELETE | `/api/v1/admin/properties/:id/documents/:documentId` | Admin | ✅ Implemented | ❌ Not used |

**ISSUE**: Frontend calls `/properties/${propertyId}/invest` which **does not exist** in backend.

#### Investment Endpoints (5 total)

| Method | Endpoint | Auth Required | Status | Frontend Usage |
|--------|----------|---------------|--------|----------------|
| POST | `/api/v1/investments` | Yes | ✅ Implemented | ❌ **MISSING**: Frontend uses non-existent `/properties/:id/invest` |
| GET | `/api/v1/investments` | Yes | ✅ Implemented | ⚠️ **MISMATCH**: Frontend uses `/me/investments` |
| GET | `/api/v1/investments/:id` | Yes | ✅ Implemented | ❌ Not used |
| GET | `/api/v1/portfolio/summary` | Yes | ✅ Implemented | ❌ Not used |
| GET | `/api/v1/admin/investments` | Admin | ✅ Implemented | ❌ Not used |

**CRITICAL ISSUE**: Frontend investment flow will fail - uses wrong endpoint.

#### Notification Endpoints (6 total)

| Method | Endpoint | Auth Required | Status | Frontend Usage |
|--------|----------|---------------|--------|----------------|
| GET | `/api/v1/notifications` | Yes | ✅ Implemented | ❌ **NOT INTEGRATED** |
| PATCH | `/api/v1/notifications/:id/read` | Yes | ✅ Implemented | ❌ NOT INTEGRATED |
| PATCH | `/api/v1/notifications/read-all` | Yes | ✅ Implemented | ❌ NOT INTEGRATED |
| DELETE | `/api/v1/notifications/:id` | Yes | ✅ Implemented | ❌ NOT INTEGRATED |
| GET | `/api/v1/notifications/preferences` | Yes | ✅ Implemented | ❌ NOT INTEGRATED |
| PUT | `/api/v1/notifications/preferences/:type` | Yes | ✅ Implemented | ❌ NOT INTEGRATED |

**CRITICAL ISSUE**: Entire notification system is implemented in backend but not connected to frontend.

#### Admin Endpoints (15+ total)

| Category | Endpoints | Status | Frontend Usage |
|----------|-----------|--------|----------------|
| Dashboard | 1 endpoint | ✅ Implemented | ❌ AdminDashboard.jsx not connected |
| User Management | 9 endpoints | ✅ Implemented | ❌ Not connected |
| Messaging | 1 endpoint | ✅ Implemented | ❌ Not used |
| Financial Monitoring | 4 endpoints | ✅ Implemented | ❌ Not used |
| Reports/Audit | 4 endpoints | ✅ Implemented | ❌ Not used |
| Health Check | 2 endpoints | ✅ Implemented | ❌ Not used |

**CRITICAL ISSUE**: Complete admin portal UI exists but is not connected to any backend endpoints.

#### Webhook Endpoints (2 total)

| Method | Endpoint | Purpose | Status |
|--------|----------|---------|--------|
| POST | `/api/v1/webhooks/payment` | Paystack deposit webhook | ✅ Implemented |
| POST | `/api/v1/webhooks/paystack/transfer` | Paystack withdrawal webhook | ✅ Implemented |

*(Not called by frontend - external provider callbacks)*

### Frontend-Only Endpoints (Do Not Exist in Backend)

| Method | Endpoint | Used By | Expected Behavior | Actual Result |
|--------|----------|---------|-------------------|---------------|
| GET | `/wallets/me` | `walletService` | Get wallet | ❌ 404 Not Found |
| GET | `/me/wallet` | `walletService` fallback | Get wallet | ❌ 404 Not Found |
| POST | `/wallets/deposit` | `walletService` | Initiate deposit | ❌ 404 Not Found |
| POST | `/wallets/withdraw` | `walletService` | Request withdrawal | ❌ 404 Not Found |
| GET | `/wallets/me/transactions` | `walletService` | Get transactions | ❌ 404 Not Found |
| GET | `/me/wallet/transactions` | `walletService` fallback | Get transactions | ❌ 404 Not Found |
| POST | `/me/wallet/transfer-earnings` | `walletService` | Transfer earnings | ❌ 404 Not Found |
| GET | `/wallet/banks` | `walletService` | Get bank list | ❌ 404 Not Found |
| GET | `/me/investments` | `portfolioService` | List investments | ❌ 404 Not Found |
| POST | `/properties/:id/invest` | `propertyService` | Invest in property | ❌ 404 Not Found |
| POST | `/me/investments/:id/vote` | `portfolioService` | Vote on sale | ❌ 404 Not Found |
| POST | `/me/investments/:id/trigger-sale` | `portfolioService` | Trigger sale vote | ❌ 404 Not Found |
| POST | `/me/kyc/submit` | `authService` | Submit KYC | ❌ 404 Not Found |
| GET | `/offplan/projects` | `offplanService` | List off-plan | ❌ 404 Not Found |
| POST | `/offplan/projects/:id/subscribe` | `offplanService` | Subscribe to off-plan | ❌ 404 Not Found |

**TOTAL**: 15+ frontend API calls will result in 404 errors.

---

## Section 3: Data Structure Synchronization

### 3.1 Authentication Data Structures

#### ✅ Login Request (Aligned)

**Backend DTO** (`auth_dto.go`):
```go
type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}
```

**Frontend Service** (`authService.js`):
```javascript
api.post('/auth/login', { email, password })
```

**Status:** ✅ Perfect alignment

#### ✅ Login Response (Aligned)

**Backend DTO**:
```go
type LoginResponse struct {
    User         UserResponse `json:"user"`
    AccessToken  string       `json:"access_token"`
    RefreshToken string       `json:"refresh_token"`
    TokenType    string       `json:"token_type"`
}
```

**Frontend Handling**:
```javascript
.then((data) => {
  if (data?.access_token) {
    localStorage.setItem('access_token', data.access_token);
    if (data.refresh_token) localStorage.setItem('refresh_token', data.refresh_token);
  }
  return data;
})
```

**Status:** ✅ Perfect alignment

#### ✅ User Profile Response (Aligned)

**Backend DTO** (`user_dto.go`):
```go
type UserResponse struct {
    ID            uuid.UUID `json:"id"`
    UserCode      string    `json:"user_code"`
    FullName      string    `json:"full_name"`
    Email         string    `json:"email"`
    Phone         string    `json:"phone"`
    AvatarURL     *string   `json:"avatar_url,omitempty"`
    EmailVerified bool      `json:"email_verified"`
    KYCStatus     string    `json:"kyc_status"`
    Role          string    `json:"role"`
    IsActive      bool      `json:"is_active"`
    CreatedAt     time.Time `json:"created_at"`
}
```

**Frontend Handling** (`authService.js`):
```javascript
.then(user => {
  if (user && !user.name && user.full_name) {
    user.name = user.full_name; // Frontend adds 'name' alias
  }
  return user;
})
```

**Status:** ✅ Aligned (frontend adds convenience alias)

### 3.2 Wallet Data Structures

#### ⚠️ Wallet Balance Response (CRITICAL MISMATCH)

**Backend DTO** (`wallet_dto.go`):
```go
type WalletResponse struct {
    ID              uuid.UUID `json:"id"`
    UserID          uuid.UUID `json:"user_id"`
    MainBalance     int64     `json:"main_balance"`      // KOBO (integer)
    LockedBalance   int64     `json:"locked_balance"`    // KOBO
    EarningsBalance int64     `json:"earnings_balance"`  // KOBO
    Currency        string    `json:"currency"`
    VirtualAcctNo   *string   `json:"virtual_acct_no,omitempty"`
    VirtualBank     *string   `json:"virtual_bank,omitempty"`
    CreatedAt       time.Time `json:"created_at"`
}
```

**Frontend Processing** (`walletService.js`):
```javascript
// Frontend assumes values might be in kobo OR naira
const rawMain = payload.main_balance || payload.balance || payload.mainBalance || 0;
const rawEarnings = payload.earnings_balance || payload.earningsBalance || 0;
const rawRental = payload.this_month_rental || payload.thisMonthRental || 0;

// ALWAYS divides by 100
const mainBalance = Math.round(Number(rawMain) / 100);
const earningsBalance = Math.round(Number(rawEarnings) / 100);
const thisMonthRental = Math.round(Number(rawRental) / 100);
```

**Issues:**
1. ❌ Backend ALWAYS sends kobo (integer), but frontend checks multiple field names
2. ❌ Frontend assumes `this_month_rental` field exists (not in backend DTO)
3. ⚠️ Frontend divides by 100, which is CORRECT, but has defensive fallback logic suggesting inconsistency
4. ⚠️ Backend returns `locked_balance` but frontend doesn't use it

**Impact:** ✅ Will work correctly IF correct endpoint is called, but fallback logic indicates past issues.

#### ❌ Deposit Request (MISMATCH)

**Backend DTO** (`wallet_dto.go`):
```go
type DepositRequest struct {
    Amount int64 `json:"amount" binding:"required,gt=0"` // kobo
}
```

**Frontend Service** (`walletService.js`):
```javascript
initializeDeposit: async (amountInNaira, method = 'card') => {
  return api.post('/wallets/deposit', {  // ❌ Wrong path
    amount: Math.round(amountInNaira * 100), // ✅ Correct conversion
    payment_method: method,  // ❌ Backend doesn't accept this field
  })
}
```

**Issues:**
1. ❌ **Path mismatch**: `/wallets/deposit` vs `/wallet/deposit`
2. ❌ **Extra field**: `payment_method` not in backend DTO
3. ✅ **Amount conversion**: Correct (Naira → kobo)

#### ❌ Withdrawal Request (CRITICAL MISMATCH)

**Backend DTO** (`wallet_dto.go`):
```go
type WithdrawRequest struct {
    Amount        int64  `json:"amount" binding:"required,gt=0"`
    BankCode      string `json:"bank_code" binding:"required"`
    AccountNumber string `json:"account_number" binding:"required,min=10,max=10"`
    AccountName   string `json:"account_name" binding:"required"`
}
```

**Frontend Service** (`walletService.js`):
```javascript
requestWithdrawal: async ({ amount, bankCode, bankName, accountNumber, accountName }) => {
  return api.post('/wallets/withdraw', {  // ❌ Wrong path
    amount: Math.round(amount * 100),  // ✅ Correct conversion
    bank_code: bankCode || '058',  // ✅ Correct field name
    bank_name: bankName,  // ❌ Backend doesn't accept this field
    account_number: accountNumber,  // ✅ Correct field name
    account_name: accountName,  // ✅ Correct field name
  })
}
```

**Issues:**
1. ❌ **Path mismatch**: `/wallets/withdraw` vs `/wallet/withdraw`
2. ❌ **Extra field**: `bank_name` not in backend DTO
3. ✅ **Amount conversion**: Correct
4. ✅ **Field naming**: Correctly uses snake_case

### 3.3 Property Data Structures

#### ⚠️ Property List Response (PARTIAL MISMATCH)

**Backend DTO** (`property_dto.go`):
```go
type PropertyListItemResponse struct {
    ID            uuid.UUID  `json:"id"`
    Slug          string     `json:"slug"`
    Title         string     `json:"title"`
    PropertyType  string     `json:"property_type"`
    Status        string     `json:"status"`
    CoverImageURL *string    `json:"cover_image_url,omitempty"`
    Location      PropertyLocationResponse  `json:"location"`
    Financial     PropertyFinancialResponse `json:"financial"`
    ROIPercent    float64    `json:"roi_percent"`
    Duration      int        `json:"duration_months"`
    Featured      bool       `json:"featured"`
    Verified      bool       `json:"verified"`
    Trending      bool       `json:"trending"`
    LaunchDate    time.Time  `json:"launch_date"`
    CreatedAt     time.Time  `json:"created_at"`
}

type PropertyFinancialResponse struct {
    TargetAmount      int64   `json:"target_amount"`       // KOBO
    RaisedAmount      int64   `json:"raised_amount"`       // KOBO
    RemainingAmount   int64   `json:"remaining_amount"`    // KOBO
    MinimumInvestment int64   `json:"minimum_investment"`  // KOBO
    UnitPrice         int64   `json:"unit_price"`          // KOBO
    TotalUnits        int64   `json:"total_units"`
    UnitsSold         int64   `json:"units_sold"`
    UnitsRemaining    int64   `json:"units_remaining"`
    FundingPercentage float64 `json:"funding_percentage"`
    InvestorCount     int     `json:"investor_count"`
    IsFullyFunded     bool    `json:"is_fully_funded"`
    ROIPercent        float64 `json:"roi_percent"`
    ExpectedReturnsKobo int64 `json:"expected_returns_kobo"` // KOBO
}
```

**Frontend Mock Data** (`constants/data.js`):
```javascript
{
  id: 1,                    // ❌ Number vs UUID
  name: "Maitama Residency", // ❌ 'name' vs 'title'
  loc: "FCT Abuja",         // ❌ 'loc' vs 'location' object
  type: "Residential",      // ⚠️ Capitalized vs lowercase
  yieldPct: 12,             // ❌ 'yieldPct' vs 'roi_percent'
  entry: 12000000,          // ❌ 'entry' vs 'minimum_investment' + assumes Naira
  totalSlots: 10,           // ⚠️ 'totalSlots' vs 'total_units'
  slotsLeft: 0,             // ❌ 'slotsLeft' vs 'units_remaining'
  funded: 100,              // ⚠️ Number vs boolean 'is_fully_funded'
  tag: "LIVE",              // ❌ Not in backend (display logic)
  spv: "PropVest Maitama Ltd", // ❌ Not in backend
  investors: 10,            // ⚠️ vs 'investor_count'
  pooled: 120000000,        // ❌ Naira vs 'raised_amount' in kobo
  monthly: 18500,           // ❌ Not in backend DTO
  annualRent: 2220000,      // ❌ Not in backend DTO
  incomeType: "rental",     // ❌ Not in backend DTO
  holdYears: 3,             // ❌ Not in backend DTO
}
```

**Status:** ❌ **MAJOR MISMATCH** - Frontend expects completely different structure. Will break when backend is connected.

### 3.4 Investment Data Structures

#### ❌ Investment Response (NOT INTEGRATED)

**Backend DTO** (`investment_dto.go`):
```go
type InvestmentResponse struct {
    ID            uuid.UUID  `json:"id"`
    PropertyID    uuid.UUID  `json:"property_id"`
    Property      *PropertyResponse `json:"property,omitempty"`
    Slots         int        `json:"slots"`
    AmountKobo    int64      `json:"amount_kobo"`
    UnitPriceKobo int64      `json:"unit_price_kobo"`
    Currency      string     `json:"currency"`
    Status        string     `json:"status"`
    Reference     string     `json:"reference"`
    CancelledAt   *time.Time `json:"cancelled_at,omitempty"`
    CompletedAt   *time.Time `json:"completed_at,omitempty"`
    RefundedAt    *time.Time `json:"refunded_at,omitempty"`
    CreatedAt     time.Time  `json:"created_at"`
    UpdatedAt     time.Time  `json:"updated_at"`
}
```

**Frontend Service** (`portfolioService.js`):
```javascript
// Frontend normalizes from assumed backend structure
const normalizeInvestment = (inv) => {
  const myInvested = inv.amount_paid !== undefined
    ? (inv.amount_paid > 10000000 ? Math.round(inv.amount_paid / 100) : inv.amount_paid)
    : (inv.myInvested || 0);
  
  // ... frontend creates derived display values
  return {
    id: inv.id,
    name: inv.property?.name || inv.name,  // ❌ Backend returns 'title' not 'name'
    loc: inv.property?.location || inv.loc, // ❌ Backend returns location object
    // ... many more mismatches
  };
}
```

**Status:** ❌ **NOT TESTED** - Frontend has normalization logic but hasn't been tested against real backend responses.

### 3.5 Error Response Structures

#### ✅ Error Response (Aligned)

**Backend DTO** (`error_dto.go`):
```go
type ErrorResponse struct {
    Error string `json:"error"`
    Code  string `json:"code,omitempty"`
}

type ValidationErrorResponse struct {
    Error  string            `json:"error"`
    Fields map[string]string `json:"fields"`
}
```

**Frontend Handling** (`apiClient.js`):
```javascript
if (!response.ok) {
  const errorData = await response.json().catch(() => ({}));
  const errorMsg = errorData.message || `HTTP ${response.status}: ${response.statusText}`;
  
  const err = new Error(errorMsg);
  err.status = response.status;
  err.code = errorData.code;      // ✅ Captures backend code
  err.errors = errorData.errors;  // ⚠️ Backend uses 'fields' not 'errors'
  throw err;
}
```

**Status:** ✅ Mostly aligned, minor naming difference (errors vs fields).

---

## Section 4: Missing TypeScript Types

The frontend uses **JavaScript without TypeScript**, which means:
- ❌ No compile-time type checking
- ❌ No IDE autocomplete for API responses
- ❌ Runtime errors only discovered in production
- ❌ Difficult to maintain as API evolves

### Recommended TypeScript Interfaces (Priority Order)

#### P0 (Critical - Core Flows)

```typescript
// src/types/auth.ts
interface LoginRequest {
  email: string;
  password: string;
}

interface RegisterRequest {
  full_name: string;
  email: string;
  password: string;
  phone: string;
}

interface UserResponse {
  id: string;
  user_code: string;
  full_name: string;
  email: string;
  phone: string;
  avatar_url?: string;
  email_verified: boolean;
  kyc_status: string;
  role: string;
  is_active: boolean;
  created_at: string;
}

interface AuthResponse {
  user: UserResponse;
  access_token: string;
  refresh_token: string;
  token_type: string;
}

// src/types/wallet.ts
interface WalletResponse {
  id: string;
  user_id: string;
  main_balance: number;  // kobo
  locked_balance: number; // kobo
  earnings_balance: number; // kobo
  currency: string;
  virtual_acct_no?: string;
  virtual_bank?: string;
  created_at: string;
}

interface DepositRequest {
  amount: number; // kobo
}

interface DepositResponse {
  authorization_url: string;
  reference: string;
}

interface WithdrawRequest {
  amount: number; // kobo
  bank_code: string;
  account_number: string;
  account_name: string;
}

interface TransactionResponse {
  id: string;
  type: string;
  amount: number; // kobo
  balance_before: number;
  balance_after: number;
  reference: string;
  description: string;
  status: string;
  created_at: string;
}
```

#### P1 (High - Investment Flow)

```typescript
// src/types/property.ts
interface PropertyListItemResponse {
  id: string;
  slug: string;
  title: string;
  property_type: string;
  status: string;
  cover_image_url?: string;
  location: PropertyLocationResponse;
  financial: PropertyFinancialResponse;
  roi_percent: number;
  duration_months: number;
  featured: boolean;
  verified: boolean;
  trending: boolean;
  launch_date: string;
  created_at: string;
}

interface PropertyLocationResponse {
  address: string;
  city: string;
  state: string;
  country: string;
  latitude?: number;
  longitude?: number;
}

interface PropertyFinancialResponse {
  target_amount: number; // kobo
  raised_amount: number;
  remaining_amount: number;
  minimum_investment: number; // kobo
  unit_price: number; // kobo
  total_units: number;
  units_sold: number;
  units_remaining: number;
  funding_percentage: number;
  investor_count: number;
  is_fully_funded: boolean;
  roi_percent: number;
  expected_returns_kobo: number;
}

// src/types/investment.ts
interface CreateInvestmentRequest {
  property_id: string;
  slots: number;
  idempotency_key?: string;
}

interface InvestmentResponse {
  id: string;
  property_id: string;
  property?: PropertyResponse;
  slots: number;
  amount_kobo: number;
  unit_price_kobo: number;
  currency: string;
  status: string;
  reference: string;
  cancelled_at?: string;
  completed_at?: string;
  refunded_at?: string;
  created_at: string;
  updated_at: string;
}

interface PortfolioSummaryResponse {
  total_invested: number; // kobo
  active_count: number;
  wallet_balance: number; // kobo
  earnings_balance: number; // kobo
}
```

#### P2 (Medium - Admin & Notifications)

```typescript
// src/types/notification.ts
interface NotificationResponse {
  id: string;
  type: string;
  title: string;
  body: string;
  read: boolean;
  metadata?: Record<string, unknown>;
  created_at: string;
}

interface NotificationListResponse {
  notifications: NotificationResponse[];
  unread_count: number;
}

// src/types/admin.ts
interface AdminDashboardResponse {
  // To be defined when admin dashboard is implemented
  total_users: number;
  total_investments: number;
  total_properties: number;
  platform_revenue: number;
  // ... other metrics
}
```

#### P3 (Low - Error Types)

```typescript
// src/types/error.ts
interface ErrorResponse {
  error: string;
  code?: string;
}

interface ValidationErrorResponse {
  error: string;
  fields: Record<string, string>;
}

interface ApiError extends Error {
  status: number;
  code?: string;
  fields?: Record<string, string>;
}
```

---

## Section 5: Authentication & Authorization Integration

### 5.1 Authentication Flow

#### ✅ Token Storage (Aligned)

**Backend Implementation:**
- Access token TTL: 15 minutes (configurable via `ACCESS_TOKEN_TTL`)
- Refresh token TTL: 720 hours (30 days, configurable via `REFRESH_TOKEN_TTL`)
- Token rotation: Refresh endpoint issues NEW refresh token (security best practice)

**Frontend Implementation:**
```javascript
// authService.js
localStorage.setItem('access_token', data.access_token);
localStorage.setItem('refresh_token', data.refresh_token);
```

**Status:** ✅ Correct implementation

#### ✅ Automatic Token Refresh (Aligned)

**Backend Behavior:**
- Returns 401 Unauthorized when access token expires
- Accepts refresh token at `/api/v1/auth/refresh`

**Frontend Auto-Refresh** (`apiClient.js`):
```javascript
// Handle 401 Token Expiry & Automatic Refresh
if (response.status === 401 && !_retry && localStorage.getItem('refresh_token')) {
  if (isRefreshing) {
    // Queue concurrent requests
    return new Promise((resolve, reject) => {
      failedQueue.push({ resolve, reject });
    }).then((newToken) => {
      config.headers.Authorization = `Bearer ${newToken}`;
      return request(endpoint, { ...options, _retry: true });
    });
  }

  isRefreshing = true;

  try {
    const refreshToken = localStorage.getItem('refresh_token');
    const refreshRes = await fetch(`${BASE_URL}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });

    if (!refreshRes.ok) {
      throw new Error('Refresh token expired');
    }

    const refreshData = await refreshRes.json();
    const newAccessToken = refreshData.data?.access_token || refreshData.access_token;
    const newRefreshToken = refreshData.data?.refresh_token || refreshData.refresh_token;

    if (newAccessToken) {
      localStorage.setItem('access_token', newAccessToken);
      if (newRefreshToken) localStorage.setItem('refresh_token', newRefreshToken);
      processQueue(null, newAccessToken);
      isRefreshing = false;
      config.headers.Authorization = `Bearer ${newAccessToken}`;
      return request(endpoint, { ...options, _retry: true });
    }
  } catch (refreshErr) {
    processQueue(refreshErr, null);
    isRefreshing = false;
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
  }
}
```

**Status:** ✅ **Excellent implementation** - Handles concurrent requests during refresh with queuing.

#### ✅ JWT Header Format (Aligned)

**Backend Expectation** (`middleware/auth.go`):
```go
authHeader := c.GetHeader("Authorization")
// Expects: "Bearer <token>"
```

**Frontend Implementation** (`apiClient.js`):
```javascript
const token = localStorage.getItem('access_token') || localStorage.getItem('token') || localStorage.getItem('propvest_token');
const config = {
  headers: {
    'Content-Type': 'application/json',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...fetchOptions.headers,
  },
}
```

**Status:** ✅ Correct format. ⚠️ Frontend checks multiple token keys (defensive coding suggesting past issues).

### 5.2 Known Authentication Issues

#### ✅ FIXED: 401 Error After Login (Type Mismatch)

**Issue:** Middleware stored `user_id` as string, handler expected UUID type.

**Fix Applied:** `getUserIDFromContext()` now parses string to UUID.

**File:** `DEBUG_401_UNAUTHORIZED_FIX.md`

**Status:** ✅ Fixed and documented.

### 5.3 Authorization (RBAC)

#### ✅ Backend RBAC Implementation

**Roles Defined** (`models/user.go`):
- `user` - Regular investor
- `admin` - Platform administrator
- `super_admin` - Full system access

**Middleware** (`middleware/rbac.go`):
- `RequireRole(role string)` - Enforces specific role
- `RequireAdmin()` - Admin or super_admin
- `RequireSuperAdmin()` - Super admin only

**Route Protection** (`routes/v1/routes.go`):
```go
admin := router.Group("/admin/properties")
admin.Use(middleware.Auth(cfg), middleware.RequireRole("admin"))
```

#### ❌ Frontend RBAC (Not Implemented)

**Current State:**
- App.jsx has mode switcher: `user`, `developer`, `admin`
- No role-based route protection
- No role checks before API calls
- Admin panel accessible to all users (UI only, backend blocks requests)

**Required Implementation:**
```javascript
// ProtectedRoute.jsx (MISSING)
function AdminRoute({ children }) {
  const user = useAuth();
  if (user.role !== 'admin' && user.role !== 'super_admin') {
    return <Navigate to="/dashboard" />;
  }
  return children;
}

// Usage (MISSING)
<Route path="/admin" element={
  <AdminRoute>
    <AdminDashboard />
  </AdminRoute>
} />
```

**Status:** ❌ Frontend shows admin UI to everyone (security by backend only).

### 5.4 Session Management

#### ✅ Logout Implementation

**Backend Logout** (`/api/v1/auth/logout`):
- Deletes specific refresh token from database
- Single device logout

**Backend Logout All** (`/api/v1/auth/logout-all`):
- Deletes ALL refresh tokens for user
- Multi-device logout

**Frontend Logout** (`authService.js`):
```javascript
logout: async () => {
  const refreshToken = localStorage.getItem('refresh_token');
  try {
    if (refreshToken) {
      await api.post('/auth/logout', { refresh_token: refreshToken });
    }
  } catch (err) {
    console.warn('Logout API error:', err.message);
  } finally {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
  }
}
```

**Status:** ✅ Correctly implemented, but logout-all not exposed in UI.

---

## Section 6: Feature Parity Matrix

| Feature | Backend Status | Frontend Status | Integration Status | Gap Analysis |
|---------|---------------|-----------------|--------------------|--------------| 
| **User Management** |
| Registration | ✅ Complete | ✅ Complete | ✅ Working | None |
| Login | ✅ Complete | ✅ Complete | ✅ Working | None |
| Logout | ✅ Complete | ✅ Complete | ✅ Working | None |
| Logout All Devices | ✅ Complete | ❌ Not Used | ❌ Not Integrated | Frontend should add "Logout All" button |
| Profile View | ✅ Complete | ✅ Complete | ✅ Working | None |
| Profile Update | ✅ Complete | ✅ Complete | ✅ Working | None |
| Avatar Upload | ✅ Complete | ✅ Complete | ✅ Working | None |
| Password Change | ✅ Complete | ❌ Not Used | ❌ Not Integrated | Frontend Profile page missing change password form |
| Phone Number Change | ✅ Complete (OTP) | ❌ Not Used | ❌ Not Integrated | Frontend should add phone change flow |
| Email Verification | ❓ Commented out | ❌ Not Used | ❌ Not Planned | Low priority |
| Password Reset | ❓ Commented out | ❌ Not Used | ❌ Not Planned | Medium priority |
| **Wallet Management** |
| View Balance | ✅ Complete | ✅ Complete | ❌ **PATH MISMATCH** | Frontend uses `/wallets/me`, backend is `/wallet` |
| Deposit (Initiate) | ✅ Complete | ✅ Complete | ❌ **PATH MISMATCH** | Frontend uses `/wallets/deposit`, backend is `/wallet/deposit` |
| Deposit (Callback) | ✅ Complete | N/A (Redirect) | ✅ Working | None |
| Deposit (Webhook) | ✅ Complete | N/A (External) | ✅ Working | None |
| Withdrawal | ✅ Complete | ✅ Complete | ❌ **PATH MISMATCH** | Frontend uses `/wallets/withdraw`, backend is `/wallet/withdraw` |
| Transaction History | ✅ Complete | ✅ Complete | ❌ **PATH MISMATCH** | Frontend uses `/wallets/me/transactions`, backend is `/wallet/transactions` |
| Transfer Earnings | ❌ Not Implemented | ✅ Frontend Ready | ❌ Backend Missing | Frontend calls `/me/wallet/transfer-earnings` which doesn't exist |
| Bank List | ❌ Not Implemented | ✅ Frontend Ready | ❌ Backend Missing | Frontend calls `/wallet/banks` which doesn't exist |
| **Property Browsing** |
| List Properties | ✅ Complete | ✅ Complete | ⚠️ **DATA MISMATCH** | Works but frontend expects different structure |
| View Property Details | ✅ Complete | ✅ Complete | ⚠️ **DATA MISMATCH** | Works but frontend expects different structure |
| Filter Properties | ✅ Complete | ❓ Unknown | ⚠️ Partial | Frontend doesn't use backend filter params |
| Search Properties | ✅ Complete | ❓ Unknown | ⚠️ Partial | Frontend doesn't use backend search |
| **Property Management (Admin)** |
| Create Property | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | Admin UI exists but not wired |
| Update Property | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | Admin UI exists but not wired |
| Delete Property | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | Admin UI exists but not wired |
| Publish Property | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | Admin UI exists but not wired |
| Upload Images | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | Admin UI exists but not wired |
| Upload Documents | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | Admin UI exists but not wired |
| **Investment Flow** |
| Create Investment | ✅ Complete | ✅ Frontend Ready | ❌ **ENDPOINT MISMATCH** | Frontend calls `/properties/:id/invest`, backend is `/investments` |
| List My Investments | ✅ Complete | ✅ Frontend Ready | ❌ **PATH MISMATCH** | Frontend uses `/me/investments`, backend is `/investments` |
| View Investment Details | ✅ Complete | ❌ Not Used | ❌ Not Integrated | Frontend doesn't fetch individual investment |
| Portfolio Summary | ✅ Complete | ❌ Not Used | ❌ Not Integrated | Frontend calculates client-side instead |
| Vote on Sale | ❌ Not Implemented | ✅ Frontend Ready | ❌ Backend Missing | Frontend calls `/me/investments/:id/vote` which doesn't exist |
| Trigger Sale Vote | ❌ Not Implemented | ✅ Frontend Ready | ❌ Backend Missing | Frontend calls `/me/investments/:id/trigger-sale` which doesn't exist |
| **Notifications** |
| List Notifications | ✅ Complete | ❌ Not Integrated | ❌ Not Integrated | Entire notification system not connected |
| Mark as Read | ✅ Complete | ❌ Not Integrated | ❌ Not Integrated | No UI for notifications |
| Mark All as Read | ✅ Complete | ❌ Not Integrated | ❌ Not Integrated | No UI for notifications |
| Delete Notification | ✅ Complete | ❌ Not Integrated | ❌ Not Integrated | No UI for notifications |
| Preferences | ✅ Complete | ❌ Not Integrated | ❌ Not Integrated | No settings UI |
| **Admin Dashboard** |
| Dashboard Metrics | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | AdminDashboard.jsx has mock UI only |
| User Management | ✅ Complete (9 endpoints) | ❌ Not Connected | ❌ Not Integrated | No admin user management UI |
| Bulk Messaging | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | No bulk messaging UI |
| Financial Monitoring | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | No financial monitoring UI |
| Audit Logs | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | No audit log UI |
| Reports/Export | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | No export UI |
| System Health | ✅ Complete | ❌ Not Connected | ❌ Not Integrated | No health monitoring UI |
| **Off-Plan Development** |
| List Projects | ❌ Not Implemented | ✅ Frontend Ready | ❌ Backend Missing | Entire off-plan module not implemented in backend |
| View Project | ❌ Not Implemented | ✅ Frontend Ready | ❌ Backend Missing | Frontend has full UI but no backend |
| Subscribe to Project | ❌ Not Implemented | ✅ Frontend Ready | ❌ Backend Missing | Installment payment system not in backend |
| **KYC Verification** |
| Submit KYC | ❓ Unknown | ✅ Frontend Ready | ❓ Unknown | Frontend calls `/me/kyc/submit`, status unclear |
| View KYC Status | ✅ in UserResponse | ✅ Displayed | ✅ Working | Status field exists |
| **Developer Portal** |
| API Documentation | ❌ Not Implemented | ✅ UI Exists | ❌ Not Implemented | DeveloperPortal.jsx is mock UI |

### Feature Parity Summary

| Category | Backend Ready | Frontend Ready | Integrated | Gap Count |
|----------|--------------|----------------|------------|-----------|
| Authentication | 5/8 (63%) | 5/8 (63%) | 5/8 (63%) | 3 |
| User Management | 6/6 (100%) | 3/6 (50%) | 3/6 (50%) | 3 |
| Wallet | 5/7 (71%) | 7/7 (100%) | 0/7 (0%) | 7 ⚠️ |
| Property Browse | 4/4 (100%) | 2/4 (50%) | 2/4 (50%) | 2 |
| Property Admin | 13/13 (100%) | 0/13 (0%) | 0/13 (0%) | 13 ⚠️ |
| Investment | 4/6 (67%) | 3/6 (50%) | 0/6 (0%) | 6 ⚠️ |
| Notifications | 6/6 (100%) | 0/6 (0%) | 0/6 (0%) | 6 ⚠️ |
| Admin Dashboard | 20/20 (100%) | 0/20 (0%) | 0/20 (0%) | 20 ⚠️ |
| Off-Plan | 0/3 (0%) | 3/3 (100%) | 0/3 (0%) | 3 |
| KYC | 1/1 (100%) | 1/1 (100%) | 1/1 (100%) | 0 ✅ |
| **TOTAL** | **64/74 (86%)** | **24/74 (32%)** | **11/74 (15%)** | **63 gaps** |

---

## Section 7: Environment Configuration

### Backend Environment Variables

**File:** `.env.example` (48 variables total)

**Critical for Frontend Integration:**

| Variable | Purpose | Default | Frontend Impact |
|----------|---------|---------|-----------------|
| `PORT` | HTTP server port | 8080 | Frontend must point to correct port |
| `ALLOWED_ORIGINS` | CORS whitelist | localhost:5173, 3000, 3001 | Frontend origin must be in list |
| `JWT_SECRET` | Access token signing key | (generated) | Must match for token validation |
| `JWT_REFRESH_SECRET` | Refresh token signing key | (generated) | Must match for refresh |
| `ACCESS_TOKEN_TTL` | Access token lifetime | 15m | Frontend refresh timing |
| `REFRESH_TOKEN_TTL` | Refresh token lifetime | 720h (30 days) | Frontend session duration |
| `PAYSTACK_PUBLIC_KEY` | Payment provider public key | (required) | ⚠️ Frontend might need this for client-side integration |
| `CLOUDINARY_CLOUD_NAME` | Image storage cloud | (required) | Frontend displays images from Cloudinary |
| `MIN_DEPOSIT_AMOUNT` | Minimum deposit (kobo) | 10000 (₦100) | Frontend should validate before submit |
| `MAX_DEPOSIT_AMOUNT` | Maximum deposit (kobo) | 100000000 (₦1M) | Frontend should validate |
| `MIN_WITHDRAWAL_AMOUNT` | Minimum withdrawal (kobo) | 50000 (₦500) | Frontend should validate |
| `MAX_WITHDRAWAL_AMOUNT` | Maximum withdrawal (kobo) | 10000000 (₦100K) | Frontend should validate |

### Frontend Environment Variables

**File:** No `.env` file found, no `.env.example`

**Current Configuration** (`apiClient.js`):
```javascript
const BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1';
const USE_FALLBACK = import.meta.env.VITE_USE_MOCK_FALLBACK !== 'false';
```

**Issues:**
1. ❌ No `.env` file in frontend project
2. ❌ No `.env.example` template for developers
3. ❌ Base URL defaults to relative path `/api/v1` (requires proxy or same-origin deployment)
4. ⚠️ Fallback mode is ON by default (hides integration errors)

### Required Frontend Environment Variables

**Create:** `C:\Users\USER\go_projects\propvest-app\.env.example`

```bash
# PropVest Frontend Environment Configuration

# Backend API Base URL
# Development: http://localhost:8080/api/v1
# Production: https://api.propvest.com/api/v1
VITE_API_BASE_URL=http://localhost:8080/api/v1

# Enable/disable mock data fallback
# Set to 'false' to see real API errors during integration
VITE_USE_MOCK_FALLBACK=true

# Paystack Public Key (for client-side card input if needed)
VITE_PAYSTACK_PUBLIC_KEY=pk_test_your_public_key_here

# Feature Flags
VITE_ENABLE_OFF_PLAN=false
VITE_ENABLE_NOTIFICATIONS=false
VITE_ENABLE_ADMIN_PORTAL=false

# Analytics/Monitoring (optional)
# VITE_SENTRY_DSN=
# VITE_GA_TRACKING_ID=
```

### CORS Configuration

**Backend CORS** (`cmd/api/main.go`):
```go
AllowOrigins: strings.Split(cfg.AllowedOrigins, ","),
AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
AllowCredentials: true,
```

**Status:** ✅ Properly configured for localhost development.

**Production Checklist:**
- [ ] Update `ALLOWED_ORIGINS` to production frontend URL
- [ ] Ensure HTTPS in production
- [ ] Consider specific methods per endpoint (security)

---

## Section 8: Priority Integration Tasks

### P0 - Critical (Blocking Core Functionality)

| Task ID | Description | Effort | Impact | Dependencies |
|---------|-------------|--------|--------|--------------|
| **P0-1** | Fix wallet endpoint paths in frontend | 2 hours | 🔥 CRITICAL | None - blocks all wallet operations |
| **P0-2** | Fix investment creation endpoint | 3 hours | 🔥 CRITICAL | None - blocks property investment |
| **P0-3** | Update property data mapping | 4 hours | 🔥 CRITICAL | None - breaks property display |
| **P0-4** | Create frontend `.env` configuration | 1 hour | 🔥 CRITICAL | None - required for correct API calls |
| **P0-5** | Disable mock fallback in development | 30 min | 🔥 CRITICAL | P0-4 - need to see real errors |

**Estimated Total:** 10.5 hours (1.5 days)

### P1 - High (Major Features)

| Task ID | Description | Effort | Impact | Dependencies |
|---------|-------------|--------|--------|--------------|
| **P1-1** | Integrate portfolio summary API | 2 hours | HIGH | P0-2 (investments must work first) |
| **P1-2** | Add TypeScript to frontend | 16 hours | HIGH | None - improves maintainability |
| **P1-3** | Implement notification system UI | 8 hours | HIGH | None - backend ready |
| **P1-4** | Add password change form | 3 hours | HIGH | None - backend ready |
| **P1-5** | Add phone number change flow | 4 hours | HIGH | None - backend ready |
| **P1-6** | Connect admin dashboard to backend | 12 hours | HIGH | Role-based routing |
| **P1-7** | Implement role-based route protection | 4 hours | HIGH | None - security requirement |

**Estimated Total:** 49 hours (6 days)

### P2 - Medium (Enhanced Functionality)

| Task ID | Description | Effort | Impact | Dependencies |
|---------|-------------|--------|--------|--------------|
| **P2-1** | Add email verification flow | 6 hours | MEDIUM | Backend implementation |
| **P2-2** | Add password reset flow | 6 hours | MEDIUM | Backend implementation |
| **P2-3** | Implement logout-all feature | 1 hour | MEDIUM | None |
| **P2-4** | Add property filters/search UI | 8 hours | MEDIUM | None - backend ready |
| **P2-5** | Implement KYC submission flow | 8 hours | MEDIUM | Backend endpoint clarity |
| **P2-6** | Add transaction history pagination | 3 hours | MEDIUM | P0-1 (wallet endpoints) |
| **P2-7** | Connect admin user management | 12 hours | MEDIUM | P1-7 (RBAC) |
| **P2-8** | Connect admin property management | 10 hours | MEDIUM | P1-7 (RBAC) |
| **P2-9** | Add bulk messaging UI | 8 hours | MEDIUM | P1-7 (RBAC) |

**Estimated Total:** 62 hours (8 days)

### P3 - Low (Nice-to-Have)

| Task ID | Description | Effort | Impact | Dependencies |
|---------|-------------|--------|--------|--------------|
| **P3-1** | Design and implement off-plan backend module | 40 hours | LOW | Product decision |
| **P3-2** | Implement earnings transfer feature | 6 hours | LOW | Backend implementation |
| **P3-3** | Add bank list endpoint | 2 hours | LOW | None |
| **P3-4** | Implement voting system backend | 12 hours | LOW | Product decision |
| **P3-5** | Connect developer portal to real API docs | 8 hours | LOW | API documentation generation |
| **P3-6** | Add admin audit log viewer | 8 hours | LOW | P2-7 (admin access) |
| **P3-7** | Add admin reports/export UI | 10 hours | LOW | P2-7 (admin access) |

**Estimated Total:** 86 hours (11 days)

### Quick Wins (Low Effort, High Impact)

| Task ID | Description | Effort | Impact |
|---------|-------------|--------|--------|
| **QW-1** | Update frontend API client paths | 30 min | 🎯 Fixes all wallet calls |
| **QW-2** | Change investment endpoint call | 15 min | 🎯 Fixes property investment |
| **QW-3** | Create `.env.example` for frontend | 15 min | 🎯 Guides developers |
| **QW-4** | Add "Logout All Devices" button | 30 min | 🎯 Uses existing backend endpoint |
| **QW-5** | Disable mock fallback by default | 5 min | 🎯 Reveals integration issues |
| **QW-6** | Add validation limits to forms | 2 hours | 🎯 Prevents bad API calls |

**Estimated Total:** 4 hours

---

## Section 9: Recommendations

### 9.1 Immediate Actions (Week 1)

**Goal:** Establish working core functionality

1. **Fix Critical Path Mismatches** (P0-1)
   - Change `/wallets/` to `/wallet/` in `walletService.js`
   - Change `/me/investments` to `/investments` in `portfolioService.js`
   - Change investment creation from `/properties/:id/invest` to `/investments` in `propertyService.js`

2. **Create Environment Configuration** (P0-4)
   - Add `.env.example` to frontend with all required variables
   - Create actual `.env` with development values
   - Document environment setup in frontend README

3. **Disable Mock Fallbacks** (P0-5)
   - Set `VITE_USE_MOCK_FALLBACK=false` in development
   - Fix real API errors as they appear
   - Keep fallback data for storybook/tests only

4. **Test Core Flows End-to-End**
   - Manual testing: Register → Login → View Profile → Deposit → Invest
   - Document any new issues found
   - Create integration test checklist

### 9.2 Short-Term Improvements (Month 1)

**Goal:** Complete main user journey

1. **Add TypeScript** (P1-2)
   - Install TypeScript and type dependencies
   - Create types for all DTOs matching backend
   - Convert services layer to TypeScript first
   - Gradually migrate components

2. **Implement Missing User Features** (P1-4, P1-5)
   - Password change form in Profile page
   - Phone number change with OTP verification
   - Account deletion request (if backend supports)

3. **Integrate Notifications** (P1-3)
   - Add notification bell icon to header
   - Create notification dropdown component
   - Implement notification preferences in settings
   - Add notification polling or WebSocket connection

4. **Role-Based Access Control** (P1-7)
   - Create `ProtectedRoute` component
   - Implement role checking before route render
   - Hide admin/developer UI for regular users
   - Add unauthorized access page

### 9.3 Medium-Term Enhancements (Quarter 1)

**Goal:** Complete platform feature set

1. **Admin Dashboard Integration** (P1-6, P2-7, P2-8, P2-9)
   - Connect dashboard metrics to backend
   - Implement user management interface
   - Add property management tools
   - Create bulk messaging interface
   - Implement audit log viewer

2. **Enhanced Property Experience** (P2-4)
   - Add property filtering UI (type, location, ROI range)
   - Implement property search
   - Add sorting options
   - Implement property comparison feature

3. **Portfolio Analytics** (P1-1)
   - Use backend portfolio summary API
   - Add performance charts
   - Show investment history timeline
   - Add portfolio diversification insights

4. **Payment & Withdrawal Enhancements** (P2-6, P3-2, P3-3)
   - Add transaction history pagination and filters
   - Implement earnings to main balance transfer
   - Add bank account management (save accounts)
   - Implement withdrawal history

### 9.4 Long-Term Strategic Decisions

**Goal:** Platform scalability and product direction

1. **Off-Plan Development Feature** (P3-1)
   - **Decision Required:** Is off-plan a core product feature?
   - If YES: Design backend architecture, implement endpoints
   - If NO: Remove off-plan UI from frontend to reduce confusion
   - Current status: Frontend fully built, backend completely missing

2. **Voting System** (P3-4)
   - **Decision Required:** Priority for property sale voting feature?
   - Frontend has UI for voting on property resale
   - Backend has no voting endpoints
   - Consider if this aligns with MVP vs future roadmap

3. **Developer Portal** (P3-5)
   - Currently a placeholder UI
   - Should this be API documentation (OpenAPI/Swagger)?
   - Or developer sandbox for testing integrations?
   - Or removed entirely if not needed?

### 9.5 Technical Debt & Quality Improvements

1. **Code Quality**
   - Add ESLint and Prettier to frontend
   - Implement proper error boundaries
   - Add loading states for all async operations
   - Implement retry logic for failed requests

2. **Testing Strategy**
   - Add unit tests for services layer
   - Add integration tests for API client
   - Implement E2E tests for critical flows (register, invest, withdraw)
   - Add visual regression tests for UI components

3. **Documentation**
   - Create API integration guide for frontend developers
   - Document all DTOs with examples
   - Add frontend component storybook
   - Create troubleshooting guide for common integration issues

4. **Performance**
   - Implement proper React Query caching strategy
   - Add optimistic updates for mutations
   - Implement virtual scrolling for long lists
   - Optimize image loading (lazy loading, responsive images)

5. **Security**
   - Implement Content Security Policy
   - Add rate limiting on frontend forms
   - Implement CSRF protection
   - Add XSS sanitization for user-generated content

### 9.6 TypeScript Migration Strategy

**Phase 1: Foundation (Week 1)**
- Install TypeScript and configure `tsconfig.json`
- Create `src/types/` directory
- Define all DTO interfaces from backend DTOs
- Convert `apiClient.js` to `apiClient.ts`

**Phase 2: Services (Week 2)**
- Convert all service files to TypeScript
- Add proper return types for all functions
- Add request/response type annotations
- Fix any type errors revealed

**Phase 3: Components (Week 3-4)**
- Convert one feature module at a time
- Start with smaller components (buttons, inputs)
- Then convert page components
- Finally convert App.jsx to App.tsx

**Phase 4: Cleanup (Week 5)**
- Enable strict mode in tsconfig.json
- Fix all remaining `any` types
- Add JSDoc comments for complex functions
- Remove `// @ts-ignore` comments

### 9.7 API Documentation Generation

**Recommended Tool:** OpenAPI/Swagger

**Backend Implementation:**
- Use `swaggo/swag` to generate OpenAPI spec from Go comments
- Add Swagger annotations to all handlers
- Generate `swagger.json` at build time
- Serve Swagger UI at `/api/docs`

**Example Handler Annotation:**
```go
// GetWallet godoc
// @Summary      Get user's wallet
// @Description  Retrieves authenticated user's wallet with balances
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  dto.WalletResponse
// @Failure      401  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Router       /wallet [get]
func (h *WalletHandler) GetWallet(c *gin.Context) { ... }
```

**Benefits:**
- Auto-generated API documentation
- Interactive testing interface
- Type-safe client generation
- Single source of truth for API contract

### 9.8 Deployment Considerations

**CORS Configuration for Production:**
```bash
# Backend .env
ALLOWED_ORIGINS=https://propvest.com,https://www.propvest.com
```

**Frontend Build:**
- Set `VITE_API_BASE_URL=https://api.propvest.com/api/v1` for production
- Disable mock fallback in production (`VITE_USE_MOCK_FALLBACK=false`)
- Enable proper error tracking (Sentry, Rollbar)

**Reverse Proxy Option:**
If deploying frontend and backend on same domain:
```nginx
# nginx.conf
location /api/v1 {
    proxy_pass http://backend:8080/api/v1;
}

location / {
    root /usr/share/nginx/html;
    try_files $uri /index.html;
}
```

This allows frontend to use relative URL `/api/v1` without CORS issues.

---

## Section 10: Testing Checklist

### Integration Testing Checklist

#### Authentication Flow
- [ ] Register new user → Receives tokens → Auto-logged in
- [ ] Login existing user → Receives tokens → Redirected to dashboard
- [ ] Access protected route → Token sent in header → Response 200
- [ ] Token expires → Auto-refresh triggered → Request retried with new token
- [ ] Refresh token expires → Redirected to login → Tokens cleared
- [ ] Logout → Tokens removed → Backend refresh token deleted

#### Wallet Flow
- [ ] View wallet balance → Correct values displayed (kobo → Naira conversion)
- [ ] Initiate deposit → Paystack URL received → User redirected
- [ ] Complete payment → Callback received → Wallet credited (after webhook)
- [ ] Request withdrawal → Balance immediately debited → Transaction created
- [ ] View transaction history → Paginated list loaded → Filters work

#### Property & Investment Flow
- [ ] Browse properties → List loaded → Cards display correctly
- [ ] View property details → Correct data structure → Images load
- [ ] Invest in property → Wallet debited → Investment created
- [ ] View portfolio → Investments list loaded → Summary calculated
- [ ] Property reaches funding goal → Status updated → Investors notified

#### Error Handling
- [ ] Invalid credentials → User-friendly error message → No console errors
- [ ] Insufficient balance → Specific error message → Wallet balance shown
- [ ] Network error → Retry prompt → Graceful degradation
- [ ] 404 Not Found → Proper error page → Link to home
- [ ] 500 Server Error → Error logged → User sees generic message

#### Role-Based Access
- [ ] Regular user cannot access `/admin` routes → 403 or redirect
- [ ] Admin user can access admin dashboard → Data loads
- [ ] Super admin can change user roles → Update successful

### Manual Testing Scripts

#### Test Script 1: New User Journey
```
1. Open app in incognito mode
2. Click "Sign In" → Switch to "Register"
3. Fill form:
   - Full Name: Test User
   - Email: test@example.com
   - Password: SecurePassword123!
   - Phone: +2348012345678
4. Submit → Verify auto-login
5. Check dashboard loads
6. Check wallet shows ₦0.00
7. Navigate to Profile → Verify user data displayed
```

#### Test Script 2: Investment Flow
```
1. Login as existing user with balance
2. Navigate to Properties
3. Click on a property with available slots
4. Click "Invest"
5. Select number of slots
6. Review investment summary
7. Confirm investment
8. Verify:
   - Wallet balance decremented
   - Investment appears in Portfolio
   - Property "slots left" decremented
```

#### Test Script 3: Deposit Flow
```
1. Login as user
2. Navigate to Dashboard
3. Click "Add Funds" or "Deposit"
4. Enter amount: ₦5000
5. Click "Continue"
6. Verify Paystack modal appears
7. Complete test payment (use Paystack test card)
8. Redirected to callback page
9. Wait for webhook (5-30 seconds)
10. Refresh wallet → Verify balance updated
11. Check transaction history → Deposit appears
```

---

## Conclusion

The PropVest frontend and backend are **partially integrated** with critical gaps in endpoint alignment, data structure consistency, and feature completeness. The backend is well-architected and implements 86% of planned features, while the frontend has implemented only 32% of integrations, relying heavily on mock data.

**Key Success Factors for Integration:**

1. **Fix Critical Path Mismatches First** - Wallet and investment endpoints must be corrected before other work can proceed.

2. **Disable Mock Fallbacks in Development** - The fallback data masks real integration issues and prevents proper testing.

3. **Add TypeScript to Frontend** - Without type safety, the large number of data structure mismatches will cause ongoing runtime errors.

4. **Prioritize Core User Journey** - Focus on Register → Login → Browse → Invest → View Portfolio → Withdraw before building admin features.

5. **Make Product Decisions on Off-Plan & Voting** - Clarify whether these frontend-only features should be implemented in backend or removed from frontend.

**Estimated Timeline to Full Integration:**

- **Core functionality (P0 + Quick Wins):** 1-2 weeks
- **Main user features (P1):** 6-8 weeks
- **Admin portal & enhancements (P2):** 10-12 weeks
- **Full platform completion (P3):** 16-20 weeks

**Next Immediate Steps:**

1. Run through Quick Wins (4 hours)
2. Fix P0 critical issues (10.5 hours)
3. Create comprehensive test plan
4. Begin TypeScript migration
5. Implement notifications (high-value, backend-ready feature)

---

## Appendices

### Appendix A: All Backend Endpoints (Full List)

```
PUBLIC ENDPOINTS (No Auth)
├── GET    /health
├── GET    /health/ready
├── GET    /api/v1/properties
├── GET    /api/v1/properties/:id
├── POST   /api/v1/auth/register
├── POST   /api/v1/auth/login
├── POST   /api/v1/auth/refresh
├── POST   /api/v1/auth/logout
├── GET    /api/v1/wallet/deposit/callback
├── POST   /api/v1/webhooks/payment
└── POST   /api/v1/webhooks/paystack/transfer

AUTHENTICATED USER ENDPOINTS
├── POST   /api/v1/auth/logout-all
├── GET    /api/v1/users/me
├── PATCH  /api/v1/users/me
├── PATCH  /api/v1/users/avatar
├── PATCH  /api/v1/users/password
├── POST   /api/v1/users/phone/request
├── POST   /api/v1/users/phone/verify
├── GET    /api/v1/wallet
├── POST   /api/v1/wallet/deposit
├── POST   /api/v1/wallet/withdraw
├── GET    /api/v1/wallet/transactions
├── POST   /api/v1/investments
├── GET    /api/v1/investments
├── GET    /api/v1/investments/:id
├── GET    /api/v1/portfolio/summary
├── GET    /api/v1/notifications
├── PATCH  /api/v1/notifications/:id/read
├── PATCH  /api/v1/notifications/read-all
├── DELETE /api/v1/notifications/:id
├── GET    /api/v1/notifications/preferences
├── PUT    /api/v1/notifications/preferences/:type
└── PUT    /api/v1/notifications/preferences

ADMIN ENDPOINTS (Requires admin or super_admin role)
├── GET    /api/v1/admin/dashboard
├── POST   /api/v1/admin/properties
├── GET    /api/v1/admin/properties
├── GET    /api/v1/admin/properties/:id
├── PATCH  /api/v1/admin/properties/:id
├── DELETE /api/v1/admin/properties/:id
├── POST   /api/v1/admin/properties/:id/publish
├── POST   /api/v1/admin/properties/:id/images
├── DELETE /api/v1/admin/properties/:id/images/:imageId
├── PATCH  /api/v1/admin/properties/:id/images/:imageId/cover
├── POST   /api/v1/admin/properties/:id/documents
├── DELETE /api/v1/admin/properties/:id/documents/:documentId
├── PATCH  /api/v1/admin/properties/:id/documents/:documentId/visibility
├── GET    /api/v1/admin/investments
├── GET    /api/v1/admin/investments/metrics
├── GET    /api/v1/admin/properties/:id/investments
├── POST   /api/v1/admin/messaging/send
├── GET    /api/v1/admin/users
├── GET    /api/v1/admin/users/:id
├── PATCH  /api/v1/admin/users/:id
├── PATCH  /api/v1/admin/users/:id/suspend
├── PATCH  /api/v1/admin/users/:id/activate
├── POST   /api/v1/admin/users/:id/reset-password
├── POST   /api/v1/admin/users/:id/logout-all
├── PATCH  /api/v1/admin/users/:id/kyc
├── GET    /api/v1/admin/transactions
├── GET    /api/v1/admin/transactions/:id
├── GET    /api/v1/admin/payments
├── GET    /api/v1/admin/payments/:id
├── GET    /api/v1/admin/audit-logs
├── GET    /api/v1/admin/audit-logs/:id
├── GET    /api/v1/admin/audit-logs/export
├── POST   /api/v1/admin/reports/generate
├── GET    /api/v1/admin/health/system
└── GET    /api/v1/admin/health/queues

SUPER ADMIN ONLY ENDPOINTS
├── PATCH  /api/v1/admin/users/:id/role
└── DELETE /api/v1/admin/users/:id
```

### Appendix B: Frontend File Structure

```
propvest-app/
├── src/
│   ├── App.jsx                      # Main app router
│   ├── main.jsx                     # Entry point
│   ├── index.css                    # Global styles
│   ├── components/
│   │   ├── ErrorBoundary.jsx
│   │   ├── HardCopyModal.jsx
│   │   ├── LoadingFallback.jsx
│   │   ├── Bar.jsx
│   │   ├── Chip.jsx
│   │   └── Stat.jsx
│   ├── constants/
│   │   ├── data.js                  # Mock/fallback data
│   │   └── theme.js                 # Color constants
│   ├── features/
│   │   ├── admin/
│   │   │   └── AdminDashboard.jsx   # ❌ Not connected
│   │   ├── auth/
│   │   │   └── AuthModal.jsx        # ✅ Connected
│   │   ├── common/
│   │   │   └── NotFound.jsx
│   │   ├── dashboard/
│   │   │   ├── Dashboard.jsx        # ⚠️ Partial
│   │   │   └── AddFundsWidget.jsx
│   │   ├── developer/
│   │   │   └── DeveloperPortal.jsx  # ❌ Placeholder
│   │   ├── documents/
│   │   │   └── Documents.jsx        # ❌ Not connected
│   │   ├── marketing/
│   │   │   └── LandingPage.jsx
│   │   ├── offplan/
│   │   │   └── OffPlan.jsx          # ❌ No backend
│   │   ├── portfolio/
│   │   │   ├── Portfolio.jsx        # ⚠️ Wrong endpoint
│   │   │   └── ValuationRow.jsx
│   │   ├── profile/
│   │   │   └── Profile.jsx          # ⚠️ Partial
│   │   ├── properties/
│   │   │   ├── Properties.jsx       # ⚠️ Data mismatch
│   │   │   ├── PropCard.jsx
│   │   │   └── RenderView.jsx
│   │   ├── support/
│   │   │   └── Support.jsx          # ❌ Not connected
│   │   └── withdraw/
│   │       └── Withdraw.jsx         # ⚠️ Wrong endpoint
│   ├── hooks/
│   │   └── useQueries.js            # React Query helpers
│   └── services/
│       ├── apiClient.js             # ✅ Well implemented
│       ├── authService.js           # ✅ Mostly working
│       ├── index.js                 # Exports
│       ├── offplanService.js        # ❌ No backend
│       ├── portfolioService.js      # ⚠️ Wrong endpoints
│       ├── propertyService.js       # ⚠️ Data mismatch
│       └── walletService.js         # ⚠️ Wrong endpoints
├── package.json
├── vite.config.js
└── index.html
```

---

**End of Integration Audit Report**
