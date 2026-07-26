# PropVest Backend — Complete Starter Guide
> Go + GORM + PostgreSQL (pgAdmin) + Docker · Based on your React frontend & architecture spec

---

## Prerequisites — Install These First

```bash
# 1. Go 1.22+
go version          # must print go1.22 or higher

# 2. Docker Desktop (ships with Docker Compose)
docker --version
docker compose version

# 3. pgAdmin 4 — download from https://www.pgadmin.org/download/
#    You will use this instead of the CLI to inspect your DB

# 4. Make sure Go binaries are on your PATH
export PATH="$PATH:$(go env GOPATH)/bin"
# Add this line to your ~/.bashrc or ~/.zshrc permanently
```

---

## Step 1 — Create the Project & Git Repo

```bash
mkdir propvest-backend
cd propvest-backend
git init
```

---

## Step 2 — Initialise the Go Module

```bash
go mod init github.com/your-username/propvest-backend
```

Replace `your-username` with your actual GitHub username or org name.
This string is used in every internal import across the project, e.g.:
`import "github.com/your-username/propvest-backend/internal/config"`

---

## Step 3 — Scaffold the Full Folder Structure

Run this entire block once from inside `propvest-backend/`:

```bash
# Entry points
mkdir -p cmd/api
mkdir -p cmd/worker

# API layer
mkdir -p internal/api/middleware
mkdir -p internal/api/handlers

# Database
mkdir -p internal/db/migrations
mkdir -p internal/db/models

# Services (one folder per domain)
mkdir -p internal/services/auth
mkdir -p internal/services/wallet
mkdir -p internal/services/kyc
mkdir -p internal/services/investment
mkdir -p internal/services/offplan
mkdir -p internal/services/voting
mkdir -p internal/services/storage
mkdir -p internal/services/notification
mkdir -p internal/services/document

# Background workers
mkdir -p internal/worker

# Config loader
mkdir -p internal/config

# Docker files
mkdir -p docker
```

---

## Step 4 — Create Root Skeleton Files

```bash
touch cmd/api/main.go
touch cmd/worker/main.go
touch internal/api/router.go
touch internal/config/config.go
touch docker-compose.yml
touch Makefile
touch .env
touch .env.example
touch README.md
```

Create `.gitignore` immediately — never commit secrets:

```bash
cat > .gitignore << 'EOF'
.env
*.pem
bin/
tmp/
EOF
```

---

## Step 5 — Install All Go Dependencies

```bash
# Core framework & DB
go get gorm.io/gorm
go get gorm.io/driver/postgres
go get github.com/go-chi/chi/v5

# Config & env loading
go get github.com/spf13/viper

# Auth
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto

# Validation
go get github.com/go-playground/validator/v10

# UUID
go get github.com/google/uuid

# Redis (for rate limiting, sessions, job queue later)
go get github.com/redis/go-redis/v9

# Background jobs (Asynq, add when ready for workers)
go get github.com/hibiken/asynq
```

After running these, `go mod tidy` to clean up:

```bash
go mod tidy
```

---

## Step 6 — Write `docker-compose.yml`

This spins up PostgreSQL and Redis locally. pgAdmin connects to the PostgreSQL container.

```yaml
version: "3.9"

services:
  postgres:
    image: postgres:16-alpine
    container_name: propvest_postgres
    environment:
      POSTGRES_USER: propvest
      POSTGRES_PASSWORD: password
      POSTGRES_DB: propvest
    ports:
      - "5432:5432"        # pgAdmin connects to localhost:5432
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    container_name: propvest_redis
    ports:
      - "6379:6379"

volumes:
  pgdata:
```

Start your local infrastructure:

```bash
docker compose up -d postgres redis
```

---

## Step 7 — Connect pgAdmin to the Database

1. Open pgAdmin 4
2. Right-click **Servers → Register → Server**
3. Fill in:
   - **Name:** PropVest Local
   - **Host:** `localhost`
   - **Port:** `5432`
   - **Username:** `propvest`
   - **Password:** `password`
4. Click **Save** — you should see the `propvest` database in the tree

> You will use pgAdmin to inspect tables, run ad-hoc queries, and verify migrations as you build.

---

## Step 8 — Write the Config Loader

`internal/config/config.go` — this is the **first real Go code you write**. Everything else depends on it.

```go
package config

import (
    "log"
    "github.com/spf13/viper"
)

type Config struct {
    AppEnv          string `mapstructure:"APP_ENV"`
    Port            string `mapstructure:"PORT"`
    DatabaseURL     string `mapstructure:"DATABASE_URL"`
    RedisURL        string `mapstructure:"REDIS_URL"`
    JWTSecret       string `mapstructure:"JWT_SECRET"`
    AccessTokenTTL  string `mapstructure:"ACCESS_TOKEN_TTL"`
    RefreshTokenTTL string `mapstructure:"REFRESH_TOKEN_TTL"`
    AllowedOrigins  string `mapstructure:"ALLOWED_ORIGINS"`
}

func Load() *Config {
    viper.SetConfigFile(".env")
    viper.AutomaticEnv()

    if err := viper.ReadInConfig(); err != nil {
        log.Printf("No .env file found, reading from environment: %v", err)
    }

    var cfg Config
    if err := viper.Unmarshal(&cfg); err != nil {
        log.Fatalf("Failed to unmarshal config: %v", err)
    }
    return &cfg
}
```

Create your `.env` file:

```env
APP_ENV=development
PORT=8080
DATABASE_URL=postgres://propvest:password@localhost:5432/propvest?sslmode=disable
REDIS_URL=redis://localhost:6379/0
JWT_SECRET=your-super-secret-key-change-in-production
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=720h
ALLOWED_ORIGINS=http://localhost:5173
```

---

## Step 9 — Write the GORM Models

Create one file per domain inside `internal/db/models/`.

**`internal/db/models/user.go`**
```go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type User struct {
    ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    UserCode     string         `gorm:"uniqueIndex;not null"`   // e.g. USR-001
    Email        string         `gorm:"uniqueIndex;not null"`
    Phone        string         `gorm:"uniqueIndex"`
    PasswordHash string         `gorm:"not null"`
    FullName     string         `gorm:"not null"`
    KYCStatus    string         `gorm:"default:'pending'"`      // pending|in_review|approved|rejected
    KYCScore     *int
    Role         string         `gorm:"default:'investor'"`     // investor|developer|staff|admin
    IsActive     bool           `gorm:"default:true"`
    CreatedAt    time.Time
    UpdatedAt    time.Time
    DeletedAt    gorm.DeletedAt `gorm:"index"`

    // Associations
    WalletAccount WalletAccount
}
```

**`internal/db/models/wallet.go`**
```go
package models

import (
    "time"
    "github.com/google/uuid"
)

type WalletAccount struct {
    ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    UserID           uuid.UUID `gorm:"type:uuid;uniqueIndex;not null"`
    MainBalance      int64     `gorm:"default:0"`      // in kobo
    EarningsBalance  int64     `gorm:"default:0"`      // rental + equity returns
    VirtualAcctNo    string
    VirtualBank      string
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

type WalletTransaction struct {
    ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    WalletID      uuid.UUID `gorm:"type:uuid;not null;index"`
    Type          string    `gorm:"not null"` // deposit|withdrawal|investment|rental_income|transfer|fee
    Amount        int64     `gorm:"not null"`
    BalanceBefore int64     `gorm:"not null"`
    BalanceAfter  int64     `gorm:"not null"`
    Reference     string    `gorm:"uniqueIndex;not null"`
    Description   string
    Status        string    `gorm:"default:'completed'"` // pending|completed|failed|reversed
    CreatedAt     time.Time
    // NOTE: Never UPDATE or DELETE rows in this table — it is an append-only ledger
}
```

**`internal/db/models/property.go`**
```go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type Property struct {
    ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
    PropCode        string         `gorm:"uniqueIndex;not null"` // e.g. PROP-001
    Name            string         `gorm:"not null"`
    Location        string         `gorm:"not null"`
    State           string         `gorm:"not null"`
    Type            string         `gorm:"not null"` // Residential|Apartment|Land|Commercial
    IncomeType      string         `gorm:"not null"` // rental|resale
    SPVName         string         `gorm:"not null"`
    SPVCACNo        string
    TotalSlots      int            `gorm:"not null"`
    SlotPrice       int64          `gorm:"not null"` // in kobo
    TotalValue      int64          `gorm:"not null"`
    PurchasePrice   int64          `gorm:"not null"`
    YieldPct        float64
    AnnualRent      *int64
    MonthlyRent     *int64
    FundedPct       int            `gorm:"default:0"`
    SlotsSold       int            `gorm:"default:0"`
    HoldYears       int            `gorm:"default:3"`
    Status          string         `gorm:"default:'draft'"` // draft|funding|live|sold|closed
    DeveloperID     *uuid.UUID     `gorm:"type:uuid"`
    Description     string
    Tag             string
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       gorm.DeletedAt `gorm:"index"`
}
```

> Add more model files (`investment.go`, `offplan.go`, `vote.go`, `kyc.go`) as you build each phase. Mirror the table schemas in your architecture spec.

---

## Step 10 — Write the Database Connection

**`internal/db/database.go`**

```go
package db

import (
    "log"
    "github.com/your-username/propvest-backend/internal/db/models"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func Connect(dsn string) *gorm.DB {
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info), // shows SQL in dev
    })
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }
    return db
}

// AutoMigrate runs GORM's auto-migration for all models.
// Safe for development. For production, write explicit SQL migrations instead.
func AutoMigrate(db *gorm.DB) {
    err := db.AutoMigrate(
        &models.User{},
        &models.WalletAccount{},
        &models.WalletTransaction{},
        &models.Property{},
        // add more models here as you build each phase
    )
    if err != nil {
        log.Fatalf("AutoMigrate failed: %v", err)
    }
}
```

> **Development:** `AutoMigrate` is fine here — it creates/updates tables automatically.
> **Production (later):** Switch to `golang-migrate` with explicit `.sql` files for full control.

---

## Step 11 — Write the API Entry Point

**`cmd/api/main.go`**

```go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/your-username/propvest-backend/internal/config"
    "github.com/your-username/propvest-backend/internal/db"
)

func main() {
    // 1. Load config
    cfg := config.Load()

    // 2. Connect to DB and run migrations
    database := db.Connect(cfg.DatabaseURL)
    db.AutoMigrate(database)

    // 3. Set up router
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    // Health check — first endpoint to verify everything works
    r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"status":"ok","service":"propvest-api"}`))
    })

    // 4. Start server
    addr := fmt.Sprintf(":%s", cfg.Port)
    log.Printf("PropVest API running on %s", addr)
    if err := http.ListenAndServe(addr, r); err != nil {
        log.Fatalf("Server failed: %v", err)
    }
}
```

---

## Step 12 — Write the Makefile

```makefile
.PHONY: run build up down tidy test

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

up:
	docker compose up -d

down:
	docker compose down

tidy:
	go mod tidy

test:
	go test ./... -race -cover
```

---

## Step 13 — Verify Everything Works

```bash
# 1. Start Docker services
make up

# 2. Run the API
make run
```

Open your browser or Postman and call:
```
GET http://localhost:8080/health
```

Expected response:
```json
{ "status": "ok", "service": "propvest-api" }
```

Then open **pgAdmin** and check the `propvest` database — you should see tables like `users`, `wallet_accounts`, `wallet_transactions`, `properties` already created by `AutoMigrate`.

---

## Your Final Starting Folder

```
propvest-backend/
├── cmd/
│   ├── api/
│   │   └── main.go                  ← server entry point ✅
│   └── worker/
│       └── main.go                  ← stub, fill in Phase 4
│
├── internal/
│   ├── api/
│   │   ├── router.go                ← stub, expand per phase
│   │   ├── middleware/              ← auth.go, rbac.go, ratelimit.go (Phase 1)
│   │   └── handlers/               ← auth.go, users.go, wallet.go … (per phase)
│   │
│   ├── config/
│   │   └── config.go                ← Viper env loader ✅
│   │
│   ├── db/
│   │   ├── database.go              ← GORM connect + AutoMigrate ✅
│   │   ├── migrations/              ← for production SQL migrations (later)
│   │   └── models/
│   │       ├── user.go              ✅
│   │       ├── wallet.go            ✅
│   │       ├── property.go          ✅
│   │       ├── investment.go        ← add in Phase 2
│   │       ├── offplan.go           ← add in Phase 4
│   │       ├── vote.go              ← add in Phase 3
│   │       └── kyc.go               ← add in Phase 4
│   │
│   ├── services/                    ← one package per domain, fill per phase
│   │   ├── auth/
│   │   ├── wallet/
│   │   ├── kyc/
│   │   ├── investment/
│   │   ├── offplan/
│   │   ├── voting/
│   │   ├── storage/
│   │   ├── notification/
│   │   └── document/
│   │
│   ├── worker/                      ← background jobs (Phase 4+)
│   └── config/
│       └── config.go                ✅
│
├── docker/
│   ├── Dockerfile.api               ← add before deploying
│   └── Dockerfile.worker
│
├── docker-compose.yml               ✅
├── Makefile                         ✅
├── .env                             ✅  (never commit)
├── .env.example                     ✅  (commit this)
├── .gitignore                       ✅
├── go.mod                           ✅
└── go.sum                           ✅
```

---

## What to Build Next (Phase Order)

Once `GET /health` returns 200 and pgAdmin shows your tables, follow this order:

### Phase 1 — Auth & Wallet Read (Week 1–2)
1. `POST /api/v1/auth/register` — create user + wallet account in one DB transaction
2. `POST /api/v1/auth/login` — verify password, return JWT access + refresh tokens
3. `POST /api/v1/auth/refresh` — issue new access token from refresh token
4. `GET  /api/v1/users/me` — return profile from JWT claims
5. `GET  /api/v1/wallet` — return `main_balance` + `earnings_balance`
6. **Wire to frontend:** add `VITE_API_BASE_URL=http://localhost:8080` to the React `.env` and create `src/lib/api.js` — this replaces the hardcoded `Chukwuemeka` data in `Dashboard`

### Phase 2 — Properties & Investments (Week 3–4)
7. `GET  /api/v1/properties` — replaces the `PROPS` constant in the frontend
8. `GET  /api/v1/properties/:id`
9. `POST /api/v1/investments` — slot purchase (wallet debit inside a DB transaction with `FOR UPDATE`)
10. Wire Paystack card top-up + webhook handler for bank transfer credit

### Phase 3 — Portfolio & Voting (Week 5–6)
11. `GET  /api/v1/portfolio/summary` — replaces `INVESTMENTS` constant
12. `GET  /api/v1/portfolio/history` — replaces `HISTORY` constant
13. Voting engine — create vote, cast vote, resolve vote
14. Withdrawal endpoint with OTP re-auth

### Phase 4 — KYC, Off-Plan, Workers (Week 7–8)
15. KYC submission + Smile Identity integration
16. Off-plan subscription + instalment schedule
17. Asynq background workers: rent distribution, instalment debit, reminders

### Phase 5 — Admin, Developer, Hardening (Week 9–10)
18. `/admin/*` and `/developer/*` endpoints (replaces `DEV_PROPS` constant)
19. Notification system (in-app + email via Resend + SMS via Termii)
20. Rate limiting, security audit, load testing

---

## One Rule to Keep Throughout

For wallet mutations (deposits, withdrawals, investments), **always wrap in a GORM transaction**:

```go
err := db.Transaction(func(tx *gorm.DB) error {
    // 1. Lock the wallet row
    var wallet models.WalletAccount
    if err := tx.Set("gorm:query_option", "FOR UPDATE").
        Where("user_id = ?", userID).First(&wallet).Error; err != nil {
        return err
    }

    // 2. Deduct balance
    wallet.MainBalance -= amountInKobo
    if err := tx.Save(&wallet).Error; err != nil {
        return err
    }

    // 3. Append ledger entry (never update/delete this table)
    txn := models.WalletTransaction{
        WalletID:      wallet.ID,
        Type:          "investment",
        Amount:        amountInKobo,
        BalanceBefore: wallet.MainBalance + amountInKobo,
        BalanceAfter:  wallet.MainBalance,
        Reference:     generateRef(),
        Status:        "completed",
    }
    return tx.Create(&txn).Error
})
```

This ensures atomicity — if anything fails mid-way, the whole operation rolls back cleanly.

---

*PropVest Backend Starter Guide · StaySmart PropVest Ltd © 2026*
