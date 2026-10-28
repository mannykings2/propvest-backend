# Admin API Quick Reference

**Version:** 1.0  
**Base URL:** `/api/v1/admin`  
**Auth:** All endpoints require `Authorization: Bearer <JWT_TOKEN>` + admin/super_admin role

---

## Dashboard

### Get Dashboard Metrics
```http
GET /api/v1/admin/dashboard
Authorization: Bearer <token>
```

**Response:**
```json
{
  "platform_overview": {
    "total_users": 1250,
    "active_properties": 15,
    "total_investments": 3420
  },
  "financial_metrics": {
    "platform_balance_ngn": 45000000.50,
    "total_deposits_ngn": 120000000.00,
    "total_withdrawals_ngn": 75000000.00
  },
  "activity_metrics": {
    "active_users_last_30_days": 856,
    "pending_kyc_count": 23,
    "suspended_users_count": 5
  }
}
```

---

## User Management

### List Users
```http
GET /api/v1/admin/users?page=1&page_size=20&search=john&role=investor&kyc_status=verified
Authorization: Bearer <token>
```

**Query Parameters:**
- `page` (default: 1)
- `page_size` (default: 20, max: 100)
- `search` - Search by name, email, user_code
- `role` - Filter by role
- `kyc_status` - Filter by KYC status
- `is_suspended` - Filter by suspension status

### Get User Details
```http
GET /api/v1/admin/users/:id
Authorization: Bearer <token>
```

### Suspend User
```http
PATCH /api/v1/admin/users/:id/suspend
Authorization: Bearer <token>
Content-Type: application/json

{
  "reason": "Violation of terms of service",
  "idempotency_key": "suspend-user-123"
}
```

### Activate User
```http
PATCH /api/v1/admin/users/:id/activate
Authorization: Bearer <token>
Content-Type: application/json

{
  "reason": "Appeal approved",
  "idempotency_key": "activate-user-123"
}
```

### KYC Decision
```http
PATCH /api/v1/admin/users/:id/kyc
Authorization: Bearer <token>
Content-Type: application/json

{
  "decision": "approved",
  "reason": "Documents verified",
  "idempotency_key": "kyc-decision-123"
}
```

### Change User Role (Super Admin Only)
```http
PATCH /api/v1/admin/users/:id/role
Authorization: Bearer <token>
Content-Type: application/json

{
  "new_role": "admin",
  "reason": "Promoted to admin",
  "idempotency_key": "role-change-123"
}
```

### Force Password Reset
```http
POST /api/v1/admin/users/:id/reset-password
Authorization: Bearer <token>
Content-Type: application/json

{
  "reason": "Security concern"
}
```

### Revoke All Sessions
```http
POST /api/v1/admin/users/:id/logout-all
Authorization: Bearer <token>
Content-Type: application/json

{
  "reason": "Security incident"
}
```

### Delete User (Super Admin Only)
```http
DELETE /api/v1/admin/users/:id
Authorization: Bearer <token>
```

---

## Financial Monitoring

### List Transactions
```http
GET /api/v1/admin/transactions?page=1&type=deposit&status=completed&date_from=2026-01-01T00:00:00Z
Authorization: Bearer <token>
```

**Query Parameters:**
- `page`, `page_size`
- `type` - deposit, withdrawal, investment, etc.
- `status` - completed, pending, failed
- `user_id` - Filter by user
- `date_from`, `date_to` - Date range
- `amount_min`, `amount_max` - Amount range

### Get Transaction Detail
```http
GET /api/v1/admin/transactions/:id
Authorization: Bearer <token>
```

### List Payments
```http
GET /api/v1/admin/payments?status=success&provider=paystack
Authorization: Bearer <token>
```

**Query Parameters:**
- `page`, `page_size`
- `status` - pending, success, failed
- `provider` - paystack, mock
- `channel` - deposit, withdrawal
- `user_id`, `date_from`, `date_to`

### Get Payment Detail
```http
GET /api/v1/admin/payments/:id
Authorization: Bearer <token>
```

---

## Audit Logs

### Search Audit Logs
```http
GET /api/v1/admin/audit-logs?action=user.suspended&date_from=2026-10-01T00:00:00Z
Authorization: Bearer <token>
```

**Query Parameters:**
- `page`, `page_size`
- `actor_id` - Filter by admin
- `actor_type` - admin, user, system
- `action` - Exact action name
- `action_like` - Action pattern (e.g., "user.")
- `target_type` - user, property, etc.
- `target_id` - Specific target
- `date_from`, `date_to` - Date range
- `request_id` - Correlation ID

### Get Audit Log Detail
```http
GET /api/v1/admin/audit-logs/:id
Authorization: Bearer <token>
```

### Export Audit Logs (CSV)
```http
GET /api/v1/admin/audit-logs/export?date_from=2026-10-01T00:00:00Z&date_to=2026-10-06T23:59:59Z
Authorization: Bearer <token>
```

**Requirements:**
- `date_from` and `date_to` REQUIRED
- Max 1 year date range
- Max 10,000 rows

**Response:** CSV file with formula injection protection

---

## Reports

### Generate Financial Report
```http
POST /api/v1/admin/reports/generate
Authorization: Bearer <token>
Content-Type: application/json

{
  "report_type": "financial_summary",
  "date_from": "2026-01-01T00:00:00Z",
  "date_to": "2026-12-31T23:59:59Z"
}
```

**Report Types:**
- `user_activity` - Registrations, KYC, suspensions
- `financial_summary` - Payments, withdrawals, amounts
- `investment_summary` - Investments, properties

---

## System Health

### Check System Health
```http
GET /api/v1/admin/health/system
Authorization: Bearer <token>
```

**Response:**
```json
{
  "status": "healthy",
  "checked_at": "2026-10-06T12:00:00Z",
  "components": {
    "postgresql": {
      "status": "healthy",
      "message": "database connection ok",
      "latency_ms": 15
    },
    "rabbitmq": {
      "status": "healthy",
      "message": "rabbitmq configured"
    },
    "paystack": {
      "status": "healthy",
      "message": "paystack configured"
    }
  }
}
```

### Check Queue Health
```http
GET /api/v1/admin/health/queues
Authorization: Bearer <token>
```

**Response:**
```json
{
  "status": "unsupported",
  "checked_at": "2026-10-06T12:00:00Z"
}
```

---

## Error Responses

### 401 Unauthorized
```json
{
  "error": "unauthorized",
  "message": "missing or invalid token"
}
```

### 403 Forbidden
```json
{
  "error": "forbidden",
  "message": "insufficient permissions"
}
```

### 404 Not Found
```json
{
  "error": "not_found",
  "message": "user not found"
}
```

### 400 Bad Request
```json
{
  "error": "bad_request",
  "message": "invalid request body: missing required field 'reason'"
}
```

---

## Rate Limits

- **Authenticated:** 300 requests per minute
- **Anonymous:** 100 requests per minute

Rate limit headers included in responses:
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 299
X-RateLimit-Reset: 1696600000
```

---

## Best Practices

1. **Always provide idempotency keys** for mutation operations
2. **Use specific date ranges** to improve query performance
3. **Implement exponential backoff** for rate limit handling
4. **Store audit logs** of your own admin actions for compliance
5. **Never log JWT tokens** in client-side code

---

## Security Notes

- All endpoints require authentication via JWT bearer token
- Admin routes require `admin` or `super_admin` role
- Some operations require `super_admin` role specifically (marked above)
- All mutations are audited automatically
- Session tokens expire after configured duration (default: 24h)
- Refresh tokens valid for 30 days

---

## Support

For technical support or to report security issues:
- Documentation: `/docs`
- Security: Report via secure channel (not public issues)
