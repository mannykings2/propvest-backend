# Investment Module - API Documentation

**Version:** 1.0  
**Last Updated:** 2026-10-06  
**Base URL:** `https://api.propvest.com/api/v1`

---

## Table of Contents

1. [Overview](#overview)
2. [Authentication](#authentication)
3. [User Endpoints](#user-endpoints)
4. [Admin Endpoints](#admin-endpoints)
5. [Data Models](#data-models)
6. [Error Handling](#error-handling)
7. [Rate Limiting](#rate-limiting)
8. [Examples](#examples)
9. [Postman Collection](#postman-collection)

---

## Overview

The Investment Module enables users to invest in properties by purchasing slots. All transactions are atomic, idempotent, and fully secure.

### Key Features:
- ✅ Atomic transactions (wallet debit + property funding + investment creation)
- ✅ Idempotency support (prevent duplicate investments on retry)
- ✅ Real-time balance updates
- ✅ Portfolio management
- ✅ Admin analytics and reporting

### Base URL:
```
Production:  https://api.propvest.com/api/v1
Staging:     https://staging-api.propvest.com/api/v1
Development: http://localhost:8080/api/v1
```

---

## Authentication

All endpoints require JWT authentication unless otherwise specified.

### Request Header:
```http
Authorization: Bearer {access_token}
```

### Obtaining Access Token:
```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePass123!"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Login successful",
  "data": {
    "user": { ... },
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "token_type": "Bearer"
  }
}
```

---

## User Endpoints

### 1. Create Investment

Purchase property slots.

**Endpoint:** `POST /api/v1/investments`  
**Authentication:** Required  
**Authorization:** User role

**Request Body:**
```json
{
  "property_id": "123e4567-e89b-12d3-a456-426614174000",
  "slots": 10,
  "idempotency_key": "unique-key-001" // Optional
}
```

**Field Descriptions:**
- `property_id` (required): UUID of the property to invest in
- `slots` (required): Number of slots to purchase (must be > 0)
- `idempotency_key` (optional): Unique string to prevent duplicate investments on retry

**Response (201 Created):**
```json
{
  "success": true,
  "message": "Investment created successfully",
  "data": {
    "id": "987fcdeb-51a2-43d2-b456-426614174111",
    "property_id": "123e4567-e89b-12d3-a456-426614174000",
    "property": {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "title": "Luxury Apartments Lagos",
      "slug": "luxury-apartments-lagos",
      "unit_price": 100000
    },
    "slots": 10,
    "amount_kobo": 1000000,
    "unit_price_kobo": 100000,
    "currency": "NGN",
    "status": "active",
    "reference": "INV-987fcdeb-51a2-43d2-b456-426614174111",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

**Error Responses:**

| Status | Code | Message |
|--------|------|---------|
| 400 | validation_error | "Slots must be greater than 0" |
| 404 | not_found | "Property not found" |
| 409 | conflict | "Investment with this idempotency key already exists" |
| 422 | resource_unavailable | "Property is not available for investment" |
| 422 | resource_unavailable | "Only 5 slots available" |
| 422 | validation_error | "Minimum investment is ₦5,000.00" |
| 422 | insufficient_funds | "Insufficient balance. Required: ₦10,000.00, Available: ₦5,000.00" |

**Business Rules:**
1. Amount calculated server-side: `amount = slots × property.unit_price`
2. Property must be in `active` status and not fully funded
3. User must have sufficient `main_balance` in wallet
4. Investment must meet property's minimum investment requirement
5. If idempotency_key provided and matches existing investment, returns existing investment (no new charge)

**Example cURL:**
```bash
curl -X POST https://api.propvest.com/api/v1/investments \
  -H "Authorization: Bearer {access_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "property_id": "123e4567-e89b-12d3-a456-426614174000",
    "slots": 10,
    "idempotency_key": "unique-key-001"
  }'
```

---

### 2. List My Investments

Retrieve all investments for the authenticated user.

**Endpoint:** `GET /api/v1/investments`  
**Authentication:** Required  
**Authorization:** User role

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `page` | integer | No | 1 | Page number (min: 1) |
| `page_size` | integer | No | 20 | Items per page (min: 1, max: 100) |

**Response (200 OK):**
```json
{
  "success": true,
  "message": "Investments retrieved successfully",
  "data": {
    "investments": [
      {
        "id": "987fcdeb-51a2-43d2-b456-426614174111",
        "property_id": "123e4567-e89b-12d3-a456-426614174000",
        "property": {
          "id": "123e4567-e89b-12d3-a456-426614174000",
          "title": "Luxury Apartments Lagos",
          "slug": "luxury-apartments-lagos"
        },
        "slots": 10,
        "amount_kobo": 1000000,
        "unit_price_kobo": 100000,
        "currency": "NGN",
        "status": "active",
        "reference": "INV-987fcdeb-51a2-43d2-b456-426614174111",
        "created_at": "2024-01-15T10:30:00Z",
        "updated_at": "2024-01-15T10:30:00Z"
      }
    ],
    "pagination": {
      "current_page": 1,
      "page_size": 20,
      "total_pages": 3,
      "total_records": 52,
      "has_next": true,
      "has_previous": false
    }
  }
}
```

**Example cURL:**
```bash
curl -X GET "https://api.propvest.com/api/v1/investments?page=1&page_size=20" \
  -H "Authorization: Bearer {access_token}"
```

---

### 3. Get Investment Details

Retrieve details of a specific investment.

**Endpoint:** `GET /api/v1/investments/:id`  
**Authentication:** Required  
**Authorization:** Owner or Admin

**Path Parameters:**
- `id` (required): Investment UUID

**Response (200 OK):**
```json
{
  "success": true,
  "message": "Investment retrieved successfully",
  "data": {
    "id": "987fcdeb-51a2-43d2-b456-426614174111",
    "property_id": "123e4567-e89b-12d3-a456-426614174000",
    "property": {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "title": "Luxury Apartments Lagos",
      "slug": "luxury-apartments-lagos",
      "unit_price": 100000,
      "status": "active"
    },
    "slots": 10,
    "amount_kobo": 1000000,
    "unit_price_kobo": 100000,
    "currency": "NGN",
    "status": "active",
    "reference": "INV-987fcdeb-51a2-43d2-b456-426614174111",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

**Error Responses:**
| Status | Code | Message |
|--------|------|---------|
| 403 | forbidden | "You don't have permission to view this investment" |
| 404 | not_found | "Investment not found" |

**Example cURL:**
```bash
curl -X GET https://api.propvest.com/api/v1/investments/987fcdeb-51a2-43d2-b456-426614174111 \
  -H "Authorization: Bearer {access_token}"
```

---

### 4. Get Portfolio Summary

Retrieve aggregate portfolio metrics for the authenticated user.

**Endpoint:** `GET /api/v1/portfolio/summary`  
**Authentication:** Required  
**Authorization:** User role

**Response (200 OK):**
```json
{
  "success": true,
  "message": "Portfolio summary retrieved successfully",
  "data": {
    "total_invested": 5000000,
    "active_count": 3,
    "wallet_balance": 2000000,
    "earnings_balance": 150000
  }
}
```

**Field Descriptions:**
- `total_invested`: Total amount invested (kobo) across all active investments
- `active_count`: Number of active investments
- `wallet_balance`: Current main wallet balance (kobo)
- `earnings_balance`: Current earnings balance (kobo)

**Example cURL:**
```bash
curl -X GET https://api.propvest.com/api/v1/portfolio/summary \
  -H "Authorization: Bearer {access_token}"
```

---

## Admin Endpoints

### 5. List All Investments (Admin)

Retrieve all investments across all users with optional filtering.

**Endpoint:** `GET /api/v1/admin/investments`  
**Authentication:** Required  
**Authorization:** Admin role

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `page` | integer | No | 1 | Page number |
| `page_size` | integer | No | 20 | Items per page |
| `status` | string | No | "" | Filter by status: `active`, `completed`, `cancelled`, `refunded` |

**Response (200 OK):**
```json
{
  "success": true,
  "message": "Investments retrieved successfully",
  "data": {
    "investments": [ ... ],
    "pagination": { ... }
  }
}
```

**Example cURL:**
```bash
curl -X GET "https://api.propvest.com/api/v1/admin/investments?status=active&page=1&page_size=50" \
  -H "Authorization: Bearer {admin_access_token}"
```

---

### 6. List Property Investors (Admin)

Retrieve all investors in a specific property.

**Endpoint:** `GET /api/v1/admin/properties/:id/investments`  
**Authentication:** Required  
**Authorization:** Admin role

**Path Parameters:**
- `id` (required): Property UUID

**Query Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `page` | integer | No | 1 | Page number |
| `page_size` | integer | No | 20 | Items per page |

**Response (200 OK):**
```json
{
  "success": true,
  "message": "Property investments retrieved successfully",
  "data": {
    "investments": [ ... ],
    "pagination": { ... }
  }
}
```

**Example cURL:**
```bash
curl -X GET "https://api.propvest.com/api/v1/admin/properties/123e4567-e89b-12d3-a456-426614174000/investments?page=1" \
  -H "Authorization: Bearer {admin_access_token}"
```

---

### 7. Get Investment Metrics (Admin)

Retrieve platform-wide investment statistics.

**Endpoint:** `GET /api/v1/admin/investments/metrics`  
**Authentication:** Required  
**Authorization:** Admin role

**Response (200 OK):**
```json
{
  "success": true,
  "message": "Investment metrics retrieved successfully",
  "data": {
    "total_investments": 1250,
    "total_amount_kobo": 125000000000,
    "total_amount_naira": 1250000000.00,
    "active_investments": 1000,
    "completed_investments": 200,
    "cancelled_investments": 30,
    "refunded_investments": 20,
    "unique_investors": 450,
    "average_investment_kobo": 100000000,
    "average_investment_naira": 1000000.00
  }
}
```

**Field Descriptions:**
- `total_investments`: Total number of all investments
- `total_amount_kobo`: Total invested amount in kobo
- `total_amount_naira`: Total invested amount in naira (convenience field)
- `active_investments`: Count of active investments
- `completed_investments`: Count of completed investments
- `cancelled_investments`: Count of cancelled investments
- `refunded_investments`: Count of refunded investments
- `unique_investors`: Number of unique users who have invested
- `average_investment_kobo`: Average investment amount in kobo
- `average_investment_naira`: Average investment amount in naira

**Example cURL:**
```bash
curl -X GET https://api.propvest.com/api/v1/admin/investments/metrics \
  -H "Authorization: Bearer {admin_access_token}"
```

---

## Data Models

### Investment Object

```typescript
interface Investment {
  id: string;                    // UUID
  property_id: string;           // UUID
  property?: Property;           // Property object (when preloaded)
  slots: number;                 // Number of slots purchased
  amount_kobo: number;           // Total amount paid in kobo
  unit_price_kobo: number;       // Price per slot at purchase time (snapshot)
  currency: string;              // Currency code (e.g., "NGN")
  status: InvestmentStatus;      // Investment status
  reference: string;             // Unique reference (e.g., "INV-...")
  cancelled_at?: string;         // ISO 8601 timestamp (nullable)
  completed_at?: string;         // ISO 8601 timestamp (nullable)
  refunded_at?: string;          // ISO 8601 timestamp (nullable)
  created_at: string;            // ISO 8601 timestamp
  updated_at: string;            // ISO 8601 timestamp
}

type InvestmentStatus = "active" | "completed" | "cancelled" | "refunded";
```

### Pagination Metadata

```typescript
interface PaginationMetadata {
  current_page: number;          // Current page number
  page_size: number;             // Items per page
  total_pages: number;           // Total number of pages
  total_records: number;         // Total number of items
  has_next: boolean;             // Whether there's a next page
  has_previous: boolean;         // Whether there's a previous page
}
```

### Portfolio Summary

```typescript
interface PortfolioSummary {
  total_invested: number;        // Total invested in kobo (active only)
  active_count: number;          // Number of active investments
  wallet_balance: number;        // Main wallet balance in kobo
  earnings_balance: number;      // Earnings balance in kobo
}
```

### Investment Metrics

```typescript
interface InvestmentMetrics {
  total_investments: number;           // Total count
  total_amount_kobo: number;           // Total amount in kobo
  total_amount_naira: number;          // Total amount in naira
  active_investments: number;          // Active count
  completed_investments: number;       // Completed count
  cancelled_investments: number;       // Cancelled count
  refunded_investments: number;        // Refunded count
  unique_investors: number;            // Unique user count
  average_investment_kobo: number;     // Average in kobo
  average_investment_naira: number;    // Average in naira
}
```

---

## Error Handling

All error responses follow the same format:

```json
{
  "success": false,
  "message": "Error description",
  "error": {
    "code": "error_code",
    "details": "Additional error details"
  }
}
```

### Common Error Codes:

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `validation_error` | 400 | Request validation failed |
| `unauthorized` | 401 | Authentication required or token invalid |
| `forbidden` | 403 | User lacks required permissions |
| `not_found` | 404 | Resource not found |
| `conflict` | 409 | Resource conflict (e.g., duplicate idempotency key) |
| `resource_unavailable` | 422 | Resource cannot accept action (e.g., property sold out) |
| `insufficient_funds` | 422 | User has insufficient wallet balance |
| `internal_server_error` | 500 | Server error |

---

## Rate Limiting

API requests are rate-limited to prevent abuse:

| User Type | Limit | Window |
|-----------|-------|--------|
| Anonymous | 100 requests | per minute |
| Authenticated | 300 requests | per minute |
| Admin | 500 requests | per minute |

**Rate Limit Headers:**
```http
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 295
X-RateLimit-Reset: 1640995200
```

**Rate Limit Exceeded Response (429):**
```json
{
  "success": false,
  "message": "Rate limit exceeded. Please try again later.",
  "error": {
    "code": "rate_limit_exceeded",
    "retry_after": 60
  }
}
```

---

## Examples

### Complete Investment Flow

**Step 1: Check wallet balance**
```bash
curl -X GET https://api.propvest.com/api/v1/wallet \
  -H "Authorization: Bearer {token}"
```

**Step 2: Browse properties**
```bash
curl -X GET "https://api.propvest.com/api/v1/properties?status=active"
```

**Step 3: Create investment**
```bash
curl -X POST https://api.propvest.com/api/v1/investments \
  -H "Authorization: Bearer {token}" \
  -H "Content-Type: application/json" \
  -d '{
    "property_id": "123e4567-e89b-12d3-a456-426614174000",
    "slots": 10,
    "idempotency_key": "unique-key-001"
  }'
```

**Step 4: View portfolio**
```bash
curl -X GET https://api.propvest.com/api/v1/portfolio/summary \
  -H "Authorization: Bearer {token}"
```

---

## Postman Collection

Import the Postman collection for easy testing:

**Collection URL:** `https://api.propvest.com/postman/investment-module.json`

**Variables to set:**
- `base_url`: API base URL
- `access_token`: Your JWT access token
- `property_id`: A valid property UUID

---

## Frontend Integration Guide

See: `INVESTMENT_MODULE_FRONTEND_INTEGRATION.md`

---

## Support

**Documentation:** https://docs.propvest.com  
**Support Email:** support@propvest.com  
**API Status:** https://status.propvest.com

---

*Last Updated: 2026-10-06*  
*Version: 1.0*
