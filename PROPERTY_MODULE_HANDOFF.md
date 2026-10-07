# PropVest Property Module - Implementation Handoff Document

**Date:** 2026-10-06  
**Purpose:** Complete context for external AI to design production-grade Property Module  
**Project:** PropVest Backend (Real Estate Investment Platform)  
**Tech Stack:** Go + Gin + PostgreSQL + GORM + RabbitMQ + Docker

---

## 📋 Executive Summary

PropVest is a fractional real estate investment platform. Users can invest in properties with amounts as low as ₦10,000. The **Property Module** is the next critical component to implement.

**Current Status:** 
- ✅ Foundation, Auth, Users, Wallet modules complete
- ✅ Outbox pattern implemented for reliable messaging
- ❌ Property module needs implementation
- ❌ Investment engine blocked until properties exist

**What We Need:** A production-grade implementation plan/design for the Property Module that follows existing patterns and integrates seamlessly.

---

## 🏗️ Project Architecture Overview

### **Clean Architecture Pattern**
```
handlers (HTTP) → services (business logic) → repositories (data access) → database
           ↓
         DTOs (validation, mapping)
```

### **Key Principles**
- Repository pattern for all data access
- Service layer for business logic
- DTOs for request/response validation
- Dependency injection
- GORM for ORM
- Structured logging
- Centralized error handling
- Database migrations with golang-migrate
- Transactional operations where needed

### **Existing Infrastructure**
- ✅ PostgreSQL with GORM
- ✅ JWT authentication + refresh tokens
- ✅ RBAC middleware (user, admin roles)
- ✅ Cloudinary integration (image upload)
- ✅ RabbitMQ + Outbox pattern (reliable messaging)
- ✅ Structured logging
- ✅ Docker Compose setup
- ✅ Migration system (currently at version 15)

---

## 📁 Current Project Structure

```
propvest-backend/
├── cmd/
│   ├── api/main.go           # API server (property routes commented out)
│   └── worker/main.go        # Background worker + outbox dispatcher
├── internal/
│   ├── config/               # Configuration
│   ├── database/             # DB connection + migrations
│   ├── models/               # GORM models
│   ├── repositories/         # Data access layer
│   ├── services/             # Business logic layer
│   ├── handlers/             # HTTP handlers
│   ├── dto/                  # Request/response DTOs
│   ├── middleware/           # Auth, RBAC, logging, etc.
│   ├── routes/               # Route definitions
│   ├── errors/               # Error handling
│   ├── response/             # JSON response utilities
│   ├── logger/               # Structured logging
│   ├── utils/                # JWT, password, cloudinary, SMS
│   ├── payments/             # Paystack integration
│   ├── queue/                # RabbitMQ client
│   └── dispatcher/           # Outbox event dispatcher
├── docs/                     # Comprehensive documentation
├── migrations/               # SQL migrations (000001-000015)
└── docker-compose.yml        # Postgres, RabbitMQ, Redis
```

---

## ✅ What's Already Built (Milestones 0-3)

### **Milestone 0: Foundation** ✅

**Repository Pattern:**
- `BaseRepository` with common methods
- Example: `WalletRepository`, `UserRepository`
- Pattern: Interface + concrete implementation
- Transaction support via `*gorm.DB` parameter

**Service Layer:**
- Business logic separated from handlers
- Example: `AuthService`, `WalletService`, `UserService`
- Dependency injection pattern

**Middleware:**
- `Auth()` - JWT validation
- `RequireRole()` - RBAC (user, admin)
- `Recovery()` - Panic recovery
- `Logger()` - Request logging
- `CORS()` - Cross-origin
- Rate limiting available

**Utilities:**
- Structured logger (slog)
- Error handling with custom errors
- JSON response utilities
- Configuration management

---

### **Milestone 1: Authentication** ✅

**Features Implemented:**
- User registration with email verification
- Login with JWT access + refresh tokens
- Token refresh with rotation
- Password reset flow
- Email verification
- Session revocation
- RBAC (user/admin roles)

**Database Tables:**
- `users` (id, email, password, role, email_verified, etc.)
- `refresh_tokens` (token, user_id, expires_at, revoked)
- `verification_tokens` (email verification)
- `otp_verifications` (SMS/email OTP)

**Key Files:**
- `internal/models/user.go`
- `internal/repositories/user_repository.go`
- `internal/services/auth_service.go`
- `internal/handlers/auth.go`
- `internal/middleware/auth.go`
- `internal/middleware/rbac.go`

**APIs:**
```
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/logout
POST /api/v1/auth/refresh
POST /api/v1/auth/verify-email
POST /api/v1/auth/forgot-password
POST /api/v1/auth/reset-password
```

---

### **Milestone 2: User Management** ✅

**Features:**
- View/edit profile
- Avatar upload (Cloudinary)
- Change password
- User preferences

**APIs:**
```
GET   /api/v1/users/me
PATCH /api/v1/users/me
PATCH /api/v1/users/avatar
PATCH /api/v1/users/password
```

**Key Integrations:**
- Cloudinary for image storage
- SMS service (Termii)
- Email service (mock in dev)

---

### **Milestone 3: Wallet System** ✅ + Outbox Pattern

**Features:**
- Wallet retrieval (main balance, earnings, locked)
- Deposits (Paystack integration)
- **Withdrawals with transactional outbox pattern**
- Transaction history with filtering
- Fund locking/unlocking for withdrawals
- Idempotency (unique constraint on pending withdrawals)

**Database Tables:**
- `wallets` (user_id, main_balance, earnings_balance, locked_balance)
- `wallet_transactions` (type, status, amount, balance_before, balance_after)
- `payments` (Paystack payment tracking)
- `outbox_events` (transactional outbox for reliable messaging)

**Outbox Pattern:**
- Events created atomically with business state
- Dispatcher polls and publishes to RabbitMQ
- Exponential backoff retry (max 10 attempts)
- Stale event recovery
- FOR UPDATE SKIP LOCKED for concurrency

**Key Files:**
- `internal/models/wallet.go`
- `internal/repositories/wallet_repository.go`
- `internal/services/wallet_service.go`
- `internal/handlers/wallet_handler.go` (not created yet, logic in wallet.go)
- `internal/repositories/outbox_repository.go`
- `internal/dispatcher/outbox_dispatcher.go`

**APIs:**
```
GET  /api/v1/wallet
POST /api/v1/wallet/deposit
POST /api/v1/wallet/withdraw
GET  /api/v1/wallet/transactions
```

**Worker:**
- Processes withdrawal queue
- Calls Paystack for bank transfers
- Updates transaction status
- Releases locked funds

---

## 📊 Database Schema (Existing)

### **Users Table**
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    phone_number VARCHAR(20),
    role VARCHAR(20) DEFAULT 'user',
    email_verified BOOLEAN DEFAULT FALSE,
    kyc_verified BOOLEAN DEFAULT FALSE,
    avatar_url TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

### **Wallets Table**
```sql
CREATE TABLE wallets (
    id UUID PRIMARY KEY,
    user_id UUID UNIQUE REFERENCES users(id),
    main_balance BIGINT DEFAULT 0 CHECK (main_balance >= 0),
    earnings_balance BIGINT DEFAULT 0 CHECK (earnings_balance >= 0),
    locked_balance BIGINT DEFAULT 0 CHECK (locked_balance >= 0),
    currency VARCHAR(3) DEFAULT 'NGN',
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

### **Wallet Transactions Table**
```sql
CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY,
    wallet_id UUID REFERENCES wallets(id),
    user_id UUID REFERENCES users(id),
    type VARCHAR(20), -- deposit, withdrawal, investment, earning
    amount BIGINT NOT NULL,
    balance_before BIGINT,
    balance_after BIGINT,
    reference VARCHAR(100) UNIQUE,
    description TEXT,
    status VARCHAR(20), -- pending, processing, completed, failed
    metadata JSONB,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

### **Outbox Events Table**
```sql
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    attempts INT DEFAULT 0,
    available_at TIMESTAMP NOT NULL,
    claimed_at TIMESTAMP,
    claimed_by VARCHAR(100),
    published_at TIMESTAMP,
    last_error TEXT,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

---

## 🎯 Property Module Requirements (From Documentation)

### **Reference Documents in Project:**

1. **`docs/05-Modules/5.2-PROPERTY_MODULE.md`** - Property module specification
2. **`docs/02-Database/2.2-DATABASE_DESIGN.md`** - Database design
3. **`docs/01-Architecture/1.2-DOMAIN_MODEL.md`** - Domain model
4. **`docs/03-API/3.2-API_SPECIFICATION.md`** - API specification
5. **`docs/08-Roadmap/8.1-BACKEND_IMPLEMENTATION_ROADMAP.md`** - Implementation roadmap

### **Property Module Overview (Milestone 4)**

**Goal:** Enable complete property lifecycle management

**Core Features:**
1. Property CRUD operations
2. Image upload/management (multiple images per property)
3. Property listing with filters
4. Property status management (draft, active, funded, completed)
5. Property documents (legal docs, certificates)
6. Property financial tracking
7. Admin-only property creation/management
8. Public property viewing/search

**Business Rules:**
- Only admins can create/edit/delete properties
- Properties have lifecycle: draft → active → funded → completed
- Each property has target amount, raised amount, ROI
- Properties support multiple images and documents
- Properties have location, type, duration
- Properties track number of investors and units sold

**Expected Database Tables:**
- `properties` - Main property information
- `property_images` - Property photos/gallery
- `property_documents` - Legal documents, certificates

**Expected APIs:**
```
# Public APIs (all users)
GET    /api/v1/properties              - List properties with filters
GET    /api/v1/properties/:id          - Get property details

# Admin APIs (admin role required)
POST   /api/v1/admin/properties        - Create property
PATCH  /api/v1/admin/properties/:id    - Update property
DELETE /api/v1/admin/properties/:id    - Delete property
POST   /api/v1/admin/properties/:id/images    - Upload images
DELETE /api/v1/admin/properties/:id/images/:imageId - Delete image
POST   /api/v1/admin/properties/:id/documents - Upload documents
```

---

## 🔑 Key Existing Patterns to Follow

### **1. Repository Pattern**

**Example: WalletRepository**
```go
// Interface
type WalletRepository interface {
    Create(ctx context.Context, wallet *models.Wallet) error
    FindByID(ctx context.Context, id uuid.UUID) (*models.Wallet, error)
    FindByUserID(ctx context.Context, userID uuid.UUID) (*models.Wallet, error)
    Update(ctx context.Context, wallet *models.Wallet) error
    // ... more methods
}

// Implementation
type walletRepository struct {
    *BaseRepository
}

func NewWalletRepository(db *gorm.DB) WalletRepository {
    return &walletRepository{
        BaseRepository: NewBaseRepository(db),
    }
}
```

**Key Points:**
- Interface defines contract
- Concrete implementation embeds BaseRepository
- Context for cancellation
- Transaction support via `*gorm.DB` parameter for atomic operations

---

### **2. Service Layer Pattern**

**Example: WalletService**
```go
type WalletService interface {
    GetWallet(ctx context.Context, userID uuid.UUID) (*dto.WalletResponse, error)
    InitiateWithdrawal(ctx context.Context, userID uuid.UUID, req dto.WithdrawRequest) (*dto.TransactionResponse, error)
}

type walletService struct {
    walletRepo   repositories.WalletRepository
    userRepo     repositories.UserRepository
    outboxRepo   repositories.OutboxRepository
    // ... other dependencies
    db           *gorm.DB
}

func NewWalletService(
    walletRepo repositories.WalletRepository,
    // ... other repos
    db *gorm.DB,
) WalletService {
    return &walletService{
        walletRepo: walletRepo,
        // ...
        db: db,
    }
}
```

**Key Points:**
- Interface for testability
- Dependency injection
- Business logic lives here
- Uses repositories for data access
- Handles transactions

---

### **3. Handler Pattern**

**Example: AuthHandler**
```go
type AuthHandler struct {
    authService services.AuthService
}

func NewAuthHandler(authService services.AuthService) *AuthHandler {
    return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(c *gin.Context) {
    var req dto.RegisterRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.Error(c, http.StatusBadRequest, "Invalid request", err)
        return
    }

    user, err := h.authService.Register(c.Request.Context(), req)
    if err != nil {
        // ... error handling
        return
    }

    response.Success(c, http.StatusCreated, "Registration successful", user)
}
```

**Key Points:**
- Thin layer, delegates to service
- Uses DTOs for validation
- Centralized response utilities
- Error handling with proper status codes

---

### **4. DTO Pattern**

**Example: WalletDTO**
```go
type WithdrawRequest struct {
    Amount        int64  `json:"amount" binding:"required,gt=0"`
    BankCode      string `json:"bank_code" binding:"required"`
    AccountNumber string `json:"account_number" binding:"required,len=10"`
}

type WalletResponse struct {
    ID              uuid.UUID `json:"id"`
    MainBalance     int64     `json:"main_balance"`
    EarningsBalance int64     `json:"earnings_balance"`
    AvailableBalance int64    `json:"available_balance"`
    Currency        string    `json:"currency"`
}
```

**Key Points:**
- Input validation with binding tags
- Separate request/response DTOs
- JSON tags for serialization
- Mappers convert between models and DTOs

---

### **5. Migration Pattern**

**Example: Migration 000014**
```sql
-- 000014_create_outbox_events.up.sql
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(50) NOT NULL,
    -- ... columns
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_claim 
    ON outbox_events(status, available_at) 
    WHERE status IN ('pending', 'claimed');

COMMENT ON TABLE outbox_events IS 'Transactional outbox for reliable message delivery';
```

**Key Points:**
- Sequential numbering (000016_properties.up.sql)
- Includes indexes
- Includes comments
- Foreign keys with proper constraints
- Down migration for rollback

---

## 🔧 Development Environment

### **Running the Application**

```powershell
# 1. Start infrastructure
docker-compose up -d postgres rabbitmq redis

# 2. Run migrations
$env:DATABASE_URL = "postgres://propvest:password@localhost:5435/propvest?sslmode=disable"
migrate -path internal/database/migrations -database $env:DATABASE_URL up

# 3. Start API
go run cmd/api/main.go

# 4. Start Worker (separate terminal)
go run cmd/worker/main.go
```

### **Database Access**
```powershell
docker exec -it propvest_postgres psql -U propvest -d propvest
```

### **Current Migration Version**
- Latest: 000015 (idempotency constraints)
- Next: 000016 (properties - to be created)

---

## 📦 External Dependencies

**Go Modules:**
```
github.com/gin-gonic/gin                 # HTTP framework
github.com/google/uuid                   # UUID generation
gorm.io/gorm                             # ORM
gorm.io/driver/postgres                  # PostgreSQL driver
github.com/golang-jwt/jwt/v5             # JWT
github.com/rabbitmq/amqp091-go           # RabbitMQ
golang.org/x/crypto/bcrypt               # Password hashing
github.com/cloudinary/cloudinary-go/v2   # Image upload
github.com/go-playground/validator/v10   # Validation
```

---

## 🎨 Cloudinary Integration (Already Working)

**Service:** `internal/utils/cloudinary/cloudinary.go`

**Usage Example:**
```go
// Upload avatar (from UserService)
result, err := s.cloudinaryService.UploadImage(file, "avatars", userID)
// Returns URL, public_id, etc.

// Delete image
err := s.cloudinaryService.DeleteImage(publicID)
```

**Configuration:**
- `CLOUDINARY_CLOUD_NAME`
- `CLOUDINARY_API_KEY`
- `CLOUDINARY_API_SECRET`

---

## 🚀 What the Property Module Should Include

### **Minimum Requirements:**

1. **Database Schema**
   - Properties table with all necessary fields
   - Property images table (one-to-many)
   - Property documents table (one-to-many)
   - Proper foreign keys and indexes
   - Migrations (up + down)

2. **Models**
   - Property model with GORM tags
   - PropertyImage model
   - PropertyDocument model
   - Relationships defined

3. **Repository**
   - PropertyRepository interface
   - CRUD operations
   - Filtering/search methods
   - Image/document management
   - Pagination support

4. **Service**
   - PropertyService interface
   - Business logic
   - Validation
   - Image upload integration
   - Status management

5. **DTOs**
   - CreatePropertyRequest
   - UpdatePropertyRequest
   - PropertyResponse
   - PropertyListResponse
   - Proper validation tags

6. **Handler**
   - PropertyHandler
   - All API endpoints
   - Proper auth/RBAC
   - Error handling

7. **Routes**
   - Public routes (viewing)
   - Admin routes (CRUD)

8. **Testing Considerations**
   - Unit tests for repository
   - Integration tests for service
   - API tests for handlers

---

## 📝 Property Fields (Expected)

Based on domain model and business requirements:

**Core Fields:**
- ID (UUID)
- Title
- Description (rich text)
- Location (address, city, state, country)
- Property type (residential, commercial, land)
- Price/Target amount
- Raised amount (invested so far)
- ROI percentage
- Investment duration (months)
- Status (draft, active, funded, completed)
- Total units
- Units sold
- Minimum investment amount
- Feature flags (featured, verified, trending)

**Media:**
- Cover image URL
- Gallery images (via property_images table)
- Documents (via property_documents table)

**Metadata:**
- Number of investors
- Launch date
- Expected completion date
- Created by (admin ID)
- Created at, Updated at

---

## ⚠️ Important Constraints

1. **Security:**
   - Only admins can create/edit/delete properties
   - Public can only view active properties
   - Image upload size limits
   - File type validation

2. **Business Rules:**
   - Can't delete property with active investments
   - Raised amount can't exceed target amount
   - Units sold can't exceed total units
   - ROI must be positive
   - Duration must be positive

3. **Performance:**
   - Pagination for listings
   - Eager loading for images
   - Indexes on commonly queried fields
   - Caching considerations

4. **Consistency:**
   - Follow existing code patterns
   - Use same error handling
   - Use same response format
   - Follow same naming conventions

---

## 📄 Key Documentation Files to Reference

**Must Read:**
1. `docs/05-Modules/5.2-PROPERTY_MODULE.md` - Property specification
2. `docs/06-Engineering/6.1-CODING_STANDARDS.md` - Code standards
3. `docs/02-Database/2.2-DATABASE_DESIGN.md` - Database design
4. `docs/06-Engineering/6.2-ERROR_HANDLING_AND_LOGGING.md` - Error patterns

**Nice to Have:**
5. `docs/01-Architecture/1.3-BACKEND_FOLDER_STRUCTURE.md` - Project structure
6. `docs/06-Engineering/6.3-TESTING_STRATEGY.md` - Testing approach
7. `OUTBOX_IMPLEMENTATION_COMPLETE.md` - Example of well-documented implementation

---

## ✅ Success Criteria

The implementation plan should include:

1. **Complete database schema** with relationships, indexes, constraints
2. **Data models** following GORM patterns
3. **Repository interface** with all necessary methods
4. **Service interface** with business logic operations
5. **DTO definitions** with validation rules
6. **API endpoints** with authentication/authorization
7. **Error handling** strategy
8. **Testing approach** (unit, integration, API)
9. **Migration strategy** (up/down SQL)
10. **Integration points** (Cloudinary for images)
11. **Step-by-step implementation plan** similar to outbox implementation

---

## 🎯 Expected Deliverable

**A comprehensive implementation plan document** that includes:

### **Phase 1: Database Design**
- Complete SQL schema with all tables
- Relationships and foreign keys
- Indexes for performance
- Constraints and validations
- Migration files (up + down)

### **Phase 2: Models**
- Property model with GORM tags
- PropertyImage model
- PropertyDocument model
- Helper methods

### **Phase 3: Repository Layer**
- Interface definition
- Method signatures
- Implementation approach
- Transaction handling

### **Phase 4: Service Layer**
- Interface definition
- Business logic operations
- Validation rules
- Error scenarios

### **Phase 5: DTOs**
- Request DTOs with validation
- Response DTOs
- Mapping functions

### **Phase 6: API Layer**
- Handler structure
- Route definitions
- Middleware application
- Response formats

### **Phase 7: Testing**
- Unit test plan
- Integration test plan
- Test scenarios

### **Phase 8: Implementation Steps**
- Ordered task list
- Dependencies between tasks
- Estimated effort
- Risk mitigation

---

## 📞 Contact Points

**Questions to Address in Design:**

1. How should property images be ordered? (display order field?)
2. Should properties support versioning/audit trail?
3. What happens to property when it's fully funded? Auto-status change?
4. Should there be property categories/tags for filtering?
5. How to handle property updates when investments exist?
6. Should we support draft mode for properties?
7. What fields are required vs optional?
8. Should property creation use outbox pattern for notifications?

---

## 🔍 Example: How Wallet Module Works (For Reference)

**Flow: User Initiates Withdrawal**

1. **Handler:** Validates request DTO
2. **Service:** 
   - Validates business rules
   - Starts DB transaction
   - Locks funds in wallet
   - Creates pending transaction
   - **Creates outbox event (atomic)**
   - Commits transaction
3. **Dispatcher:** 
   - Polls outbox
   - Claims event
   - Publishes to RabbitMQ
4. **Worker:** 
   - Consumes message
   - Calls Paystack
   - Updates transaction status
   - Releases funds

**Key Insight:** Property module will need similar transaction-based operations for investments (future module).

---

## 📚 Additional Context

**Project Vision:**
PropVest enables fractional real estate investment in Nigeria. Users can:
- Browse properties
- Invest with as little as ₦10,000
- Earn ROI over investment period
- Withdraw earnings to bank account

**Property Module's Role:**
- Provides investment opportunities
- Displays property details and media
- Tracks funding progress
- Foundation for investment engine

**Tech Maturity:**
- Production-ready authentication
- Production-ready wallet with outbox pattern
- Clean architecture well-established
- Ready for property implementation

---

## 🎯 Final Request

**Please provide a comprehensive, production-grade implementation plan** that:

1. Follows all existing patterns
2. Addresses all requirements
3. Includes complete SQL schemas
4. Provides detailed step-by-step implementation
5. Considers edge cases and error scenarios
6. Includes testing strategy
7. Maintains code quality standards
8. Integrates seamlessly with existing code

**The plan should be detailed enough** that a developer (or AI) can implement it step-by-step without ambiguity, similar to how the outbox implementation was executed.

---

**Document Version:** 1.0  
**Last Updated:** 2026-10-06  
**Status:** Ready for external AI review and design
