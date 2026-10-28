# Milestone 7: Administration Module - Design Prompt

**Purpose:** Create a comprehensive implementation plan for the PropVest Backend Administration Module (Milestone 7)

**Target:** Superior AI for architectural design and implementation planning

**Output Required:** Detailed implementation plan with architecture, database schema, API specifications, and phased breakdown

---

## Project Context

### Project Overview
**Name:** PropVest Backend  
**Type:** Real Estate Investment Platform  
**Stack:** Go (Golang), Gin Framework, PostgreSQL, GORM, RabbitMQ  
**Architecture:** Clean Architecture (Repository → Service → Handler pattern)  
**Current Status:** 6 out of 8 milestones complete

### Completed Milestones (1-6)
1. ✅ **Milestone 0:** Foundation (Repository pattern, Service layer, Clean architecture)
2. ✅ **Milestone 1:** Authentication (JWT, Refresh tokens, Email verification, Password reset)
3. ✅ **Milestone 2:** User Management (Profile, Avatar upload, Password change, Phone verification)
4. ✅ **Milestone 3:** Wallet System (Deposits, Withdrawals, Transactions, Paystack integration)
5. ✅ **Milestone 4:** Property Module (CRUD, Images, Documents, Approval workflow)
6. ✅ **Milestone 5:** Investment Engine (Purchase slots, Portfolio, ROI calculations)
7. ✅ **Milestone 6:** Notification System (In-app, Email/SMS via RabbitMQ, Preferences, Admin bulk messaging)

### Current Milestone (7)
**Target:** Administration Module - Operational tools for platform administrators

---

## Existing System Architecture

### Technology Stack
- **Language:** Go 1.21+
- **Web Framework:** Gin
- **Database:** PostgreSQL 15+
- **ORM:** GORM
- **Message Queue:** RabbitMQ
- **Cache:** Redis (available, not heavily used yet)
- **File Storage:** Cloudinary
- **Payment:** Paystack
- **SMS:** Twilio / Africa's Talking
- **Email:** SMTP (Gmail/SendGrid)

### Project Structure
```
propvest-backend/
├── cmd/
│   ├── api/main.go          # API server entry point
│   └── worker/main.go       # Background worker
├── internal/
│   ├── config/              # Configuration management
│   ├── database/            # Database connection & migrations
│   │   └── migrations/      # SQL migration files (000001-000022)
│   ├── models/              # GORM models
│   ├── repositories/        # Data access layer
│   ├── services/            # Business logic layer
│   ├── handlers/            # HTTP handlers (controllers)
│   ├── dto/                 # Data Transfer Objects
│   ├── middleware/          # HTTP middleware (auth, RBAC, rate limiting)
│   ├── routes/              # Route definitions
│   ├── errors/              # Custom error types
│   ├── response/            # Standard response formatting
│   ├── logger/              # Structured logging
│   ├── utils/               # Utilities (JWT, password, token)
│   ├── queue/               # RabbitMQ client and messages
│   ├── email/               # Email service
│   ├── payments/            # Payment provider integration
│   └── realtime/            # WebSocket hub
├── docs/                    # Architecture documentation
└── tests/                   # Test files
```

### Existing Database Tables (Migration 000022)
1. **users** - User accounts (id, email, name, phone, role, kyc_status, avatar_url, status, etc.)
2. **refresh_tokens** - JWT refresh tokens with rotation
3. **wallets** - User wallets (main_balance, earnings_balance, currency)
4. **wallet_transactions** - Append-only ledger
5. **payments** - Payment gateway transactions
6. **properties** - Investment properties
7. **property_images** - Property photos
8. **property_documents** - Property legal documents
9. **investments** - User investments in properties
10. **notifications** - In-app notifications
11. **notification_preferences** - User notification settings
12. **otp_verifications** - Phone number OTP codes
13. **verification_tokens** - Email verification tokens
14. **outbox** - Transactional outbox pattern for eventual consistency

### Existing Models (Key Fields)

**User Model:**
```go
type User struct {
    ID              uuid.UUID
    Email           string (unique, indexed)
    Password        string (hashed)
    FirstName       string
    LastName        string
    PhoneNumber     string (unique, indexed)
    Role            string (default: "user", values: "user", "admin", "super_admin")
    KYCStatus       string (pending, approved, rejected)
    EmailVerified   bool
    PhoneVerified   bool
    AvatarURL       string
    Status          string (active, suspended, deleted)
    LastLoginAt     *time.Time
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       gorm.DeletedAt (soft delete)
}
```

**Property Model:**
```go
type Property struct {
    ID                  uuid.UUID
    Title               string
    Description         string
    Location            string
    PricePerSlot        int64 (in kobo/cents)
    TotalSlots          int
    AvailableSlots      int
    MinimumInvestment   int64
    ExpectedROI         float64
    InvestmentDuration  int (months)
    Status              string (draft, published, funded, closed)
    Images              []PropertyImage
    Documents           []PropertyDocument
    CreatedBy           uuid.UUID (admin user)
    PublishedAt         *time.Time
    CreatedAt           time.Time
    UpdatedAt           time.Time
    DeletedAt           gorm.DeletedAt
}
```

**Investment Model:**
```go
type Investment struct {
    ID                uuid.UUID
    UserID            uuid.UUID
    PropertyID        uuid.UUID
    Slots             int
    AmountInvested    int64 (in kobo)
    CurrentValue      int64
    Status            string (pending, active, completed, cancelled)
    PurchasedAt       time.Time
    MaturityDate      time.Time
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

**Wallet Model:**
```go
type Wallet struct {
    ID              uuid.UUID
    UserID          uuid.UUID (unique)
    MainBalance     int64 (in kobo, default: 0)
    EarningsBalance int64 (in kobo, default: 0)
    Currency        string (default: NGN)
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

### Existing Services & Repositories

**All services follow this pattern:**
```go
// Repository interface (data access)
type UserRepository interface {
    Create(ctx context.Context, user *User) error
    FindByID(ctx context.Context, id uuid.UUID) (*User, error)
    FindByEmail(ctx context.Context, email string) (*User, error)
    Update(ctx context.Context, user *User) error
    List(ctx context.Context, filters UserFilters, limit, offset int) ([]*User, int64, error)
    // ... more methods
}

// Service interface (business logic)
type UserService interface {
    GetProfile(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error)
    UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) error
    // ... more methods
}
```

### Existing Middleware

1. **Auth Middleware** (`middleware.Auth(cfg)`)
   - Validates JWT token
   - Extracts user_id and stores in context
   - Returns 401 if invalid/missing

2. **RBAC Middleware** (`middleware.RequireRole("admin")`)
   - Checks user role from JWT claims
   - Returns 403 if insufficient permissions
   - Supports: "user", "admin", "super_admin"

3. **Rate Limiting** (`middleware.RateLimit(config)`)
   - Per-IP and per-user rate limiting
   - Configurable limits

### Existing Patterns & Conventions

**Handler Pattern:**
```go
type SomeHandler struct {
    someService services.SomeService
}

func NewSomeHandler(service services.SomeService) *SomeHandler {
    return &SomeHandler{someService: service}
}

func (h *SomeHandler) SomeEndpoint(c *gin.Context) {
    // 1. Extract user ID from context
    userID, err := getUserIDFromContext(c)
    
    // 2. Bind and validate request
    var req dto.SomeRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.ValidationError(c, err)
        return
    }
    
    // 3. Call service
    result, err := h.someService.DoSomething(c.Request.Context(), userID, req)
    if err != nil {
        response.Error(c, err)
        return
    }
    
    // 4. Return response
    response.Success(c, http.StatusOK, result)
}
```

**Error Handling:**
- Custom error types in `internal/errors/errors.go`
- `response.Error()` maps service errors to HTTP status codes
- Standard error codes: ErrNotFound, ErrUnauthorized, ErrValidationFailed, etc.

**Database Transactions:**
```go
err := s.db.Transaction(func(tx *gorm.DB) error {
    // All operations within transaction
    return nil
})
```

**Pagination:**
```go
// Query parameters: page (default: 1), page_size (default: 20, max: 100)
// Response includes pagination metadata
```

---

## Milestone 7: Administration Module - Requirements

### Goal
Provide comprehensive operational tools for platform administrators to manage users, monitor activity, approve content, and maintain platform health.

### Priority: High
### Complexity: High
### Estimated Time: 20-30 hours

### Core Features Required

#### 1. Admin Dashboard
**Purpose:** High-level overview of platform health and activity

**Metrics to Display:**
- **User Statistics:**
  - Total registered users
  - Active users (last 30 days)
  - New users (this month vs last month)
  - Users by KYC status (pending, approved, rejected)
  - Users by status (active, suspended)

- **Financial Statistics:**
  - Total platform value (sum of all wallet balances)
  - Total deposits (all-time)
  - Total withdrawals (all-time)
  - Total investments (all-time)
  - Pending withdrawals count and amount

- **Property Statistics:**
  - Total properties
  - Properties by status (draft, published, funded, closed)
  - Total available slots across all properties
  - Funding completion percentage

- **Investment Statistics:**
  - Total active investments
  - Total investment value
  - Average investment size
  - Investment growth trend (last 6 months)

- **Recent Activity:**
  - Latest user registrations (last 10)
  - Recent investments (last 10)
  - Recent transactions (last 10)
  - Recent notifications sent

**API Endpoint:**
```
GET /api/v1/admin/dashboard
```

#### 2. User Management
**Purpose:** View, search, and manage user accounts

**Features:**
- **List Users** (with filters)
  - Filter by: status, role, KYC status, registration date
  - Search by: email, name, phone
  - Sort by: created_at, last_login, total_invested
  - Pagination support

- **View User Details**
  - Complete user profile
  - Wallet balance (main + earnings)
  - Investment portfolio
  - Transaction history
  - Login history
  - Notifications history
  - KYC documents (if uploaded)

- **Update User**
  - Edit profile fields (name, phone, email)
  - Update role (user → admin, etc.)
  - Update KYC status (approve/reject)
  - Add admin notes

- **User Actions**
  - Suspend user account
  - Activate suspended account
  - Force password reset
  - Invalidate all sessions (logout everywhere)
  - Delete user (with safety checks)

**API Endpoints:**
```
GET    /api/v1/admin/users                    # List users with filters
GET    /api/v1/admin/users/:id                # Get user details
PATCH  /api/v1/admin/users/:id                # Update user
PATCH  /api/v1/admin/users/:id/suspend        # Suspend user
PATCH  /api/v1/admin/users/:id/activate       # Activate user
POST   /api/v1/admin/users/:id/reset-password # Force password reset
POST   /api/v1/admin/users/:id/logout-all     # Invalidate all sessions
DELETE /api/v1/admin/users/:id                # Delete user (with checks)
GET    /api/v1/admin/users/:id/investments    # User's investments
GET    /api/v1/admin/users/:id/transactions   # User's transactions
GET    /api/v1/admin/users/:id/audit-log      # User's audit trail
```

#### 3. Property Management (Enhanced)
**Purpose:** Extended property management for admins

**Features:**
- **List All Properties** (enhanced filters)
  - Filter by: status, created_by, funding_level, date_range
  - Search by: title, location
  - Sort by: created_at, total_investment, available_slots

- **Property Approval Workflow**
  - Approve pending properties
  - Reject with reason
  - Request changes

- **Property Analytics**
  - Funding progress
  - Investor count
  - Revenue generated
  - Performance metrics

- **Bulk Operations**
  - Bulk approve/reject
  - Bulk status change
  - Export property data

**API Endpoints:**
```
GET    /api/v1/admin/properties               # Enhanced list with admin filters
GET    /api/v1/admin/properties/:id/analytics # Property performance metrics
PATCH  /api/v1/admin/properties/:id/approve   # Approve property
PATCH  /api/v1/admin/properties/:id/reject    # Reject property with reason
GET    /api/v1/admin/properties/:id/investors # List all investors
```

#### 4. Investment Management
**Purpose:** Monitor and manage platform investments

**Features:**
- **List All Investments**
  - Filter by: status, property, user, date_range
  - Search by: user, property
  - Analytics view

- **Investment Details**
  - View complete investment details
  - See related transactions
  - Calculate actual ROI

- **Investment Actions**
  - Cancel investment (refund scenario)
  - Adjust investment (admin correction)
  - Mark as matured
  - Distribute returns

**API Endpoints:**
```
GET    /api/v1/admin/investments              # List all investments
GET    /api/v1/admin/investments/:id          # Get investment details
PATCH  /api/v1/admin/investments/:id/cancel   # Cancel investment
POST   /api/v1/admin/investments/:id/distribute-returns # Distribute ROI
GET    /api/v1/admin/investments/metrics      # Platform investment metrics
```

#### 5. Financial Management
**Purpose:** Monitor platform financials and transactions

**Features:**
- **Transaction Monitoring**
  - List all wallet transactions
  - Filter by: type, status, amount_range, date_range
  - Export transactions

- **Payment Management**
  - View all payment attempts
  - Retry failed payments
  - Refund processing

- **Withdrawal Management**
  - Approve pending withdrawals
  - Reject with reason
  - Process bulk withdrawals

- **Financial Reports**
  - Revenue reports
  - Transaction volume
  - Payment success rates

**API Endpoints:**
```
GET    /api/v1/admin/transactions             # All wallet transactions
GET    /api/v1/admin/payments                 # All payment attempts
GET    /api/v1/admin/withdrawals              # Pending withdrawals
PATCH  /api/v1/admin/withdrawals/:id/approve  # Approve withdrawal
PATCH  /api/v1/admin/withdrawals/:id/reject   # Reject withdrawal
GET    /api/v1/admin/financial-reports        # Generate reports
```

#### 6. Audit Logging System
**Purpose:** Track all admin actions and critical user activities for compliance

**Features:**
- **Audit Log Recording**
  - Automatically log admin actions
  - Log critical user actions (large withdrawals, status changes)
  - Store before/after state
  - Capture IP address, user agent, timestamp

- **Audit Log Viewing**
  - Filter by: admin, action type, target, date_range
  - Search audit logs
  - Export audit logs

- **Audit Report Generation**
  - Admin activity reports
  - User activity reports
  - Compliance reports

**API Endpoints:**
```
GET    /api/v1/admin/audit-logs               # List audit logs
GET    /api/v1/admin/audit-logs/:id           # Get specific audit entry
GET    /api/v1/admin/audit-logs/export        # Export audit logs (CSV)
POST   /api/v1/admin/audit-logs/report        # Generate audit report
```

**Audit Log Actions to Track:**
- User: account.created, account.suspended, account.activated, account.deleted
- User: kyc.approved, kyc.rejected, role.changed, password.reset
- Property: property.approved, property.rejected, property.published
- Investment: investment.created, investment.cancelled
- Wallet: large_withdrawal, manual_adjustment
- Admin: bulk_message.sent, report.generated

#### 7. Notification Management (Extended from Milestone 6)
**Purpose:** Admin tools for managing notifications

**Features:**
- **View All Notifications**
  - List all notifications sent
  - Filter by: type, user, status, date
  - Resend failed notifications

- **Notification Analytics**
  - Delivery rates (email/SMS)
  - Open rates (email)
  - Notification volume by type

**API Endpoints:**
```
GET    /api/v1/admin/notifications/sent       # All sent notifications
GET    /api/v1/admin/notifications/analytics  # Delivery analytics
POST   /api/v1/admin/notifications/:id/resend # Resend notification
```

#### 8. System Health & Monitoring
**Purpose:** Monitor platform health

**Features:**
- **Health Checks**
  - Database connection
  - RabbitMQ connection
  - Redis connection
  - External services (Paystack, Cloudinary, SMS)

- **Queue Monitoring**
  - RabbitMQ queue depths
  - Failed message counts
  - Consumer status

- **Performance Metrics**
  - API response times
  - Database query performance
  - Error rates

**API Endpoints:**
```
GET    /api/v1/admin/health/system            # System health status
GET    /api/v1/admin/health/queues            # RabbitMQ queue status
GET    /api/v1/admin/health/metrics           # Performance metrics
```

---

## Database Schema Requirements

### New Table: audit_logs

**Purpose:** Store all audit-worthy actions for compliance and debugging

**Schema:**
```sql
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Actor (who performed the action)
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_type VARCHAR(50) NOT NULL, -- 'admin', 'user', 'system'
    
    -- Action details
    action VARCHAR(100) NOT NULL, -- 'user.suspended', 'property.approved', etc.
    description TEXT,
    
    -- Target (what was acted upon)
    target_type VARCHAR(50) NOT NULL, -- 'user', 'property', 'investment', etc.
    target_id UUID NOT NULL,
    
    -- Change tracking
    old_values JSONB, -- State before action
    new_values JSONB, -- State after action
    
    -- Metadata
    ip_address VARCHAR(45),
    user_agent TEXT,
    request_id VARCHAR(50),
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_audit_logs_actor_id ON audit_logs(actor_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_target_type_id ON audit_logs(target_type, target_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX idx_audit_logs_actor_type ON audit_logs(actor_type);
```

### Optional: admin_settings

**Purpose:** Store admin-configurable platform settings

**Schema:**
```sql
CREATE TABLE admin_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(100) UNIQUE NOT NULL,
    value JSONB NOT NULL,
    description TEXT,
    updated_by UUID REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### Enhancements to Existing Tables

**users table:**
- Add `admin_notes TEXT` - Admin notes about the user
- Add `suspended_at TIMESTAMP` - When user was suspended
- Add `suspended_by UUID` - Which admin suspended the user
- Add `suspended_reason TEXT` - Why user was suspended

**properties table:**
- Add `approved_by UUID` - Which admin approved the property
- Add `approved_at TIMESTAMP` - When property was approved
- Add `rejection_reason TEXT` - Why property was rejected

---

## Technical Requirements

### Security Requirements
1. **Authorization:**
   - All admin endpoints require authentication
   - All admin endpoints require "admin" or "super_admin" role
   - Super admins can manage other admins
   - Regular admins cannot modify super admin accounts

2. **Audit Logging:**
   - Every admin action must be logged
   - Audit logs are append-only (no updates/deletes)
   - Sensitive data (passwords) never logged

3. **Rate Limiting:**
   - Stricter rate limits for sensitive operations
   - Separate rate limits for admin endpoints

4. **Data Protection:**
   - Never expose full password hashes
   - Mask sensitive PII in logs
   - Require additional authentication for destructive operations

### Performance Requirements
1. **Dashboard:**
   - Load in < 2 seconds
   - Cache expensive queries (Redis)
   - Use database views for complex aggregations

2. **User List:**
   - Support pagination (100 users max per page)
   - Optimize filters with proper indexes
   - Consider search service (Elasticsearch) for large datasets

3. **Audit Logs:**
   - Paginated results
   - Async write (use queue if needed)
   - Partition by date for old logs

### Data Consistency
1. **Transactions:**
   - User suspension must invalidate all sessions
   - Audit log creation happens in same transaction
   - Property approval updates related records atomically

2. **Idempotency:**
   - All admin actions should be idempotent where possible
   - Use idempotency keys for financial operations

---

## Integration Points

### Existing Services to Integrate With

1. **Notification Service:**
   - Send notification when admin performs action on user
   - Notify user of account suspension/activation
   - Email admins on critical events

2. **Wallet Service:**
   - Query wallet balances for dashboard
   - Process admin-initiated refunds
   - View transaction details

3. **Authentication Service:**
   - Invalidate user sessions
   - Force password reset
   - Update user roles

4. **Property Service:**
   - Approve/reject properties
   - Update property status
   - View property analytics

5. **Investment Service:**
   - View all investments
   - Process returns distribution
   - Handle investment cancellations

---

## Expected Deliverables

### Implementation Plan Document Should Include:

1. **Architecture Design:**
   - Component diagram
   - Data flow diagrams
   - Service interactions
   - Error handling strategy

2. **Database Design:**
   - Complete schema for audit_logs table
   - Migration file structure
   - Indexes strategy
   - Data retention policy

3. **API Specification:**
   - All endpoint definitions
   - Request/response schemas
   - Error codes
   - Authentication/authorization requirements

4. **Service Layer Design:**
   - AdminService interface and methods
   - AuditService interface and methods
   - DashboardService interface
   - Repository interfaces

5. **Implementation Phases:**
   - Phase 1: Foundation (audit logging, base admin service)
   - Phase 2: Dashboard & metrics
   - Phase 3: User management
   - Phase 4: Financial management
   - Phase 5: Reports & analytics
   - Phase 6: Testing & documentation
   - Estimated hours per phase
   - Dependencies between phases

6. **Testing Strategy:**
   - Unit tests approach
   - Integration tests
   - Security tests
   - Performance tests

7. **Documentation Requirements:**
   - API documentation format
   - Admin user guide
   - Audit log interpretation guide

8. **Security Considerations:**
   - Authorization matrix
   - Audit logging strategy
   - Data privacy compliance
   - Rate limiting strategy

9. **Monitoring & Observability:**
   - Metrics to track
   - Alerting rules
   - Dashboard requirements

10. **Migration Strategy:**
    - Database migration steps
    - Data seeding requirements
    - Rollback plan

---

## Success Criteria

The implementation plan should result in:

1. ✅ Complete admin dashboard with real-time metrics
2. ✅ Comprehensive user management capabilities
3. ✅ Full audit trail of admin actions
4. ✅ Secure, role-based access control
5. ✅ Performance-optimized queries
6. ✅ Clear API documentation
7. ✅ Phased implementation approach
8. ✅ Testing strategy for all components
9. ✅ Compliance with data protection requirements
10. ✅ Production-ready code quality

---

## Constraints & Considerations

1. **Maintain Existing Patterns:**
   - Follow established repository/service/handler pattern
   - Use existing error handling
   - Maintain consistency with existing APIs

2. **Backward Compatibility:**
   - Don't break existing admin endpoints (property approval already exists)
   - Enhance, don't replace

3. **Performance:**
   - Dashboard queries must be optimized
   - Consider caching for expensive operations
   - Use pagination for all list endpoints

4. **Security First:**
   - Every endpoint must be secured
   - Audit logging is mandatory
   - Sensitive data must be protected

5. **Scalability:**
   - Design for millions of audit log entries
   - Consider partitioning strategies
   - Plan for distributed deployment

---

## Reference Examples from Existing Codebase

### Example: Notification Module Implementation (Milestone 6)
- Successfully implemented with 13 phases
- Created comprehensive documentation
- Followed clean architecture
- Includes audit trail (admin bulk messaging)
- Reference: `NOTIFICATION_MODULE_TASKS.md`

### Example: Investment Module (Milestone 5)
- Complex business logic with transactions
- Financial operations with audit trail
- Portfolio calculations
- Reference: `INVESTMENT_MODULE_TASKS.md`

### Example: Property Module (Milestone 4)
- Admin approval workflow already exists
- Image/document management
- Status transitions
- Reference: `internal/services/property_service.go`

---

## Questions to Address in Implementation Plan

1. Should we use Redis caching for dashboard metrics? If so, what's the cache invalidation strategy?
2. Should audit logs be written synchronously or asynchronously (via queue)?
3. Should we implement soft deletes for users or hard deletes with data retention?
4. Should financial reports be pre-generated and cached, or generated on-demand?
5. Should we implement role-based permissions beyond "admin" vs "user" (e.g., "moderator", "finance_admin")?
6. Should audit logs have a retention period (e.g., auto-delete after 2 years)?
7. Should we implement export functionality (CSV/PDF) for admin reports?
8. Should there be a separate admin authentication flow or use same auth?
9. Should we implement admin notifications (email admin on critical events)?
10. Should there be an admin approval workflow for certain actions (e.g., large refunds)?

---

## Output Format Required

Please provide the implementation plan in Markdown format with:

1. **Executive Summary** (1 page)
2. **Architecture Overview** (with diagrams in Mermaid format)
3. **Database Schema** (complete SQL)
4. **API Specification** (all endpoints with request/response)
5. **Service Interfaces** (Go interface definitions)
6. **Implementation Phases** (detailed breakdown with hours)
7. **Testing Strategy** (what to test and how)
8. **Security & Compliance** (authorization matrix, audit strategy)
9. **Deployment Guide** (migration steps, rollback plan)
10. **Documentation Plan** (what docs to create)

---

**End of Design Prompt**

---

## Instructions for AI

Please design a comprehensive, production-ready implementation plan for Milestone 7 (Administration Module) that:

1. Follows the existing PropVest architecture patterns
2. Addresses all requirements listed above
3. Provides detailed technical specifications
4. Breaks down into manageable implementation phases
5. Includes security and compliance considerations
6. Provides clear testing strategy
7. Answers the key questions listed
8. Results in a plan that can be executed phase-by-phase

The plan should be detailed enough that a developer can implement it without additional clarification, but flexible enough to adapt to discoveries during implementation.

**Target audience:** Senior backend engineer who will implement this module
**Success metric:** Complete, tested, documented administration module ready for production deployment
