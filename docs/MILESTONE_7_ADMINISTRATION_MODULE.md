# Administration Module - Milestone 7

**Status:** ✅ Complete  
**Version:** 1.0  
**Last Updated:** 2026-10-06

---

## Overview

The Administration Module provides a comprehensive set of tools for platform administrators to manage users, monitor financial transactions, view system health, and maintain audit trails. Built following Clean Architecture principles with security, audit logging, and bounded query patterns.

---

## Architecture

### Design Principles

1. **Clean Architecture:** Separation of concerns across handlers → services → repositories
2. **Security First:** All endpoints require authentication + admin/super_admin role
3. **Audit Everything:** All mutations logged atomically with business changes
4. **Bounded Queries:** Pagination enforced (max 100 per page), timeouts on health checks
5. **Privacy by Design:** Sensitive data redacted, no secrets in responses

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                     Admin Handlers                           │
│  (HTTP layer - validation, auth middleware, response)        │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│                     Admin Services                           │
│  (Business logic, authorization hierarchy, audit recording)  │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────────┐
│                    Repositories                              │
│  (Data access, filtered queries, atomicity)                  │
└──────────────────────┬──────────────────────────────────────┘
                       │
                ┌──────▼──────┐
                │  PostgreSQL  │
                └─────────────┘
```

---

## Capabilities

### 1. Dashboard & Metrics (Phase 3)

**Endpoint:** `GET /api/v1/admin/dashboard`

**Metrics Provided:**
- Platform overview (total users, active properties, total investments)
- Financial metrics (platform balance, total deposits, total withdrawals)
- Activity metrics (active users last 30 days, pending KYC, suspended users)
- All queries bounded with explicit time windows

**Authorization:** Requires `admin` or `super_admin` role

---

### 2. User Management (Phase 4)

**Capabilities:**
- List users with filtering (search, role, KYC status, suspension status)
- View detailed user profile with wallet and investment history
- Suspend/activate users with reason tracking
- Approve/reject KYC submissions
- Change user roles (super_admin only)
- Force password reset
- Revoke all user sessions
- Soft delete users (super_admin only)

**Endpoints:**
- `GET /api/v1/admin/users` - List users (paginated)
- `GET /api/v1/admin/users/:id` - User details
- `PATCH /api/v1/admin/users/:id/suspend` - Suspend user
- `PATCH /api/v1/admin/users/:id/activate` - Activate user
- `PATCH /api/v1/admin/users/:id/kyc` - KYC decision
- `PATCH /api/v1/admin/users/:id/role` - Change role (super_admin)
- `POST /api/v1/admin/users/:id/reset-password` - Force reset
- `POST /api/v1/admin/users/:id/logout-all` - Revoke sessions
- `DELETE /api/v1/admin/users/:id` - Delete user (super_admin)

**Authorization Hierarchy:**
- `admin` can manage regular users
- `super_admin` can manage admins and change roles

**Audit Trail:** All mutations create audit logs with actor, target, reason, old/new state

---

### 3. Property Management (Phase 5)

**Status:** ✅ Verified - Property admin endpoints already exist from Milestone 4

**Capabilities:**
- Property CRUD operations
- Property approval/rejection workflow
- Image and document management
- Status management (draft → published → funded → completed)

**Endpoints:** See Property Module documentation (Milestone 4)

---

### 4. Investment Management (Phase 6)

**Status:** ✅ Verified - Investment admin endpoints already exist from Milestone 5

**Capabilities:**
- List all investments with filters
- View investment metrics (platform-wide)
- Monitor per-property investor lists

**Endpoints:** See Investment Module documentation (Milestone 5)

---

### 5. Financial Monitoring (Phase 7)

**Capabilities:**
- Monitor wallet transactions (deposits, withdrawals, investments)
- Monitor payment processing (success/failure rates)
- Filter by user, status, date range, amount range
- View detailed transaction and payment records

**Endpoints:**
- `GET /api/v1/admin/transactions` - List transactions
- `GET /api/v1/admin/transactions/:id` - Transaction detail
- `GET /api/v1/admin/payments` - List payments
- `GET /api/v1/admin/payments/:id` - Payment detail

**Security:** All queries bounded (max 100 per page), no sensitive provider data exposed

---

### 6. Audit Log Search & Export (Phase 8)

**Capabilities:**
- Search audit logs with comprehensive filters
- Export audit logs to CSV with security protections
- Generate financial reports (user activity, financial summary, investment summary)

**Endpoints:**
- `GET /api/v1/admin/audit-logs` - Search audit logs
- `GET /api/v1/admin/audit-logs/:id` - Audit log detail
- `GET /api/v1/admin/audit-logs/export` - CSV export (max 10,000 rows, max 1 year)
- `POST /api/v1/admin/reports/generate` - Generate reports

**Security Features:**
- CSV formula injection prevention (leading =,+,-,@ escaped)
- Bounded exports (10,000 row cap, 1 year date range max)
- Export actions audited
- Sensitive fields redacted from exports

**Report Types:**
1. **User Activity:** Registrations, KYC status, suspensions
2. **Financial Summary:** Payments, withdrawals, amounts
3. **Investment Summary:** Investment volume, property funding

---

### 7. System Health Monitoring (Phase 9)

**Capabilities:**
- Check PostgreSQL connectivity with latency tracking
- Verify RabbitMQ configuration
- Check third-party service configuration (Paystack, Cloudinary, SMS)
- Overall health status (healthy/degraded/unhealthy)

**Endpoints:**
- `GET /api/v1/admin/health/system` - System health check
- `GET /api/v1/admin/health/queues` - Queue health (returns "unsupported" currently)

**Security:** No secrets, DSNs, or internal topology exposed in responses

**Timeouts:** All checks bounded to 5 seconds maximum

---

## Security Model

### Authorization Hierarchy

```
super_admin
    ↓
  admin
    ↓
  staff
    ↓
  investor
```

### Permission Matrix

| Action | Admin | Super Admin |
|--------|-------|-------------|
| View dashboard | ✅ | ✅ |
| List users | ✅ | ✅ |
| Suspend/activate users | ✅ | ✅ |
| KYC decisions | ✅ | ✅ |
| Change roles | ❌ | ✅ |
| Delete users | ❌ | ✅ |
| View transactions | ✅ | ✅ |
| View audit logs | ✅ | ✅ |
| Export audit logs | ✅ | ✅ |
| System health | ✅ | ✅ |

### Middleware Stack

All admin routes protected by:
1. `middleware.Auth(cfg)` - JWT authentication
2. `middleware.RequireAdmin()` or `middleware.RequireSuperAdmin()` - Role authorization
3. `middleware.RateLimit()` - Rate limiting (300 req/min for authenticated)

### Data Protection

- ❌ No password hashes in responses
- ❌ No JWT secrets in responses
- ❌ No raw provider payloads in responses
- ❌ No internal error details exposed
- ✅ Audit log old/new values redacted (sensitive fields removed)
- ✅ CSV exports escape formula injection
- ✅ All queries parameterized (SQL injection prevention)

---

## Audit Logging

### Audit Coverage

**User Management:**
- user.suspended
- user.activated
- user.kyc_approved / user.kyc_rejected
- user.role_changed
- user.password_reset_forced
- user.sessions_revoked
- user.deleted

**Export Actions:**
- audit_logs.exported

### Audit Log Structure

```json
{
  "id": "uuid",
  "actor_id": "uuid",
  "actor_type": "admin",
  "actor_role": "super_admin",
  "action": "user.suspended",
  "target_type": "user",
  "target_id": "uuid",
  "old_values": {"is_active": true},
  "new_values": {"is_active": false, "suspended_at": "2026-10-06T12:00:00Z"},
  "reason": "Violation of terms of service",
  "idempotency_key": "suspend-user-123",
  "request_id": "req-abc-123",
  "ip_address": "192.168.1.1",
  "user_agent": "Mozilla/5.0...",
  "metadata": {},
  "created_at": "2026-10-06T12:00:00Z"
}
```

### Atomicity

All audit logs committed in same transaction as business changes:
```go
tx := db.Begin()
defer tx.Rollback()

// Business operation
err := userRepo.Update(ctx, tx, user)

// Audit log
err = auditService.Record(ctx, tx, event)

tx.Commit() // Both committed atomically
```

---

## Database Schema

### New Tables

**audit_logs** (extended in migration 000023):
- Added: `old_values`, `new_values`, `reason`, `idempotency_key`, `request_id`, `ip_address`, `user_agent`

**users** (extended in migration 000024):
- Added: `suspended_at`, `suspended_by`, `suspended_reason`
- Indexes on `suspended_at` and `suspended_by`

### Indexes

All admin queries use indexed columns:
- `users`: id, email, user_code, role, kyc_status, suspended_at
- `wallet_transactions`: id, user_id, type, status, created_at
- `payments`: id, user_id, status, provider, created_at
- `audit_logs`: id, actor_id, action, target_type, created_at

---

## Performance Characteristics

### Query Bounds

- **Pagination:** Max 100 results per page (enforced in all services)
- **Date Ranges:** Max 1 year for exports and reports
- **Export Caps:** Max 10,000 rows for CSV exports
- **Health Checks:** 5-second timeout per check

### Expected Latencies (Representative Data)

- Dashboard queries: < 2 seconds (target)
- User list: < 500ms (paginated)
- Transaction list: < 500ms (paginated)
- Audit log search: < 1 second (paginated)
- Health checks: < 5 seconds (with timeout)

---

## Deployment

### Prerequisites

1. PostgreSQL database with migrations 000023, 000024 applied
2. RabbitMQ configured (for background jobs)
3. Environment variables configured (see .env.example)

### Migration Commands

```bash
# Run migrations
migrate -database "${DATABASE_URL}" -path internal/database/migrations up

# Verify
psql "${DATABASE_URL}" -c "SELECT * FROM schema_migrations;"
```

### Environment Variables

```
# Required for admin module
DATABASE_URL=postgresql://...
JWT_SECRET=...
RABBITMQ_URL=amqp://...
```

---

## Deferred Features

The following features were identified but deferred to future milestones:

### Notification Delivery Tracking
- **Reason:** No `notification_delivery_events` table exists in Milestone 6
- **Recommendation:** Add delivery tracking infrastructure before implementing admin endpoints

### Withdrawal Approval Workflow
- **Reason:** No approval state machine in current wallet service
- **Recommendation:** Design withdrawal review workflow if business requires manual approval

### Refund Processing
- **Reason:** No safe provider-integrated refund workflow exists
- **Recommendation:** Implement provider refund API integration before admin endpoints

### Queue Metrics
- **Reason:** RabbitMQ management API not integrated
- **Current Status:** Returns "unsupported"
- **Recommendation:** Add management API access for detailed queue metrics

---

## Testing Strategy

### Test Coverage

**Unit Tests:** Repository, service business logic  
**Integration Tests:** Handler authorization, end-to-end workflows  
**Manual Tests:** Full admin workflows in staging environment

**Critical Test Cases:**
1. Admin cannot escalate own privileges
2. Admin cannot manage higher-privilege users
3. Suspended users cannot login
4. Role changes revoke existing sessions
5. CSV exports prevent formula injection
6. Audit logs atomic with business changes

See `docs/ADMIN_TESTING.md` for detailed test plan.

---

## Troubleshooting

### Common Issues

**Issue:** Admin routes return 403 Forbidden  
**Solution:** Verify user has `admin` or `super_admin` role in database

**Issue:** Dashboard queries timeout  
**Solution:** Check PostgreSQL indexes, review query performance with EXPLAIN ANALYZE

**Issue:** CSV export fails with "result set too large"  
**Solution:** Narrow date range (max 1 year, max 10,000 rows)

**Issue:** Health check shows "degraded" status  
**Solution:** Verify third-party service configuration (Paystack, Cloudinary, SMS)

---

## Maintenance

### Audit Log Retention

**Recommendation:** Implement audit log archival after 2 years:
```sql
-- Archive old audit logs
INSERT INTO audit_logs_archive SELECT * FROM audit_logs WHERE created_at < NOW() - INTERVAL '2 years';
DELETE FROM audit_logs WHERE created_at < NOW() - INTERVAL '2 years';
```

### Performance Monitoring

Monitor these metrics:
- Dashboard query latency (target: < 2s)
- Admin endpoint error rate
- Audit log write failures
- Health check failure rate

---

## Security Considerations

### Privilege Escalation Prevention

- Admins cannot change their own role
- Admins cannot manage higher-privilege users
- Super admin role assignment audited

### Session Management

- Role changes revoke all existing sessions
- Password resets revoke all sessions
- User deletion revokes all sessions

### Data Privacy

- User passwords never logged
- Provider secrets never exposed
- PII redacted in audit log exports
- Internal error details sanitized

---

## References

- **Milestone 7 Plan:** `MILESTONE_7_ADMINISTRATION_AI_CODING_AGENT_PLAN.md`
- **Implementation Blueprint:** `MILESTONE_7_ADMINISTRATION_IMPLEMENTATION_BLUEPRINT.md`
- **Task Tracking:** `MILESTONE_7_ADMINISTRATION_TASKS.md`
- **API Specification:** `docs/03-API/3.2-API_SPECIFICATION.md`
- **Security Architecture:** `docs/04-Security/4.1-SECURITY_ARCHITECTURE.md`
