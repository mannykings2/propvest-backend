# Testing Setup Guide

**Project:** PropVest Backend  
**Purpose:** Guide for running tests locally and in CI/CD  
**Last Updated:** October 6, 2026

---

## Overview

The PropVest backend has comprehensive test coverage across multiple layers:
- **Unit Tests:** 49+ tests (config, queue, services)
- **Repository Tests:** 23+ tests (require CGO/GCC)
- **Integration Tests:** Planned (Phase 3)
- **E2E Tests:** Planned (Phase 3)
- **Load Tests:** Planned (Phase 4)

---

## Running Tests Locally

### Quick Start

```powershell
# Run all tests (except repository tests on Windows without GCC)
go test ./...

# Run with coverage
go test ./... -coverprofile=coverage.out

# Generate HTML coverage report
go tool cover -html=coverage.out -o coverage.html

# Run with race detector
go test -race ./...

# Run specific package tests
go test ./internal/config -v
go test ./internal/queue -v
go test ./internal/services -v
```

### Test Results (Current Status)

| Package | Tests | Coverage | Status |
|---------|-------|----------|--------|
| `internal/config` | 13 | 51.1% | ✅ PASS |
| `internal/queue` | 32 | 33.6% | ✅ PASS |
| `internal/services` | 4 | 1.2% | ✅ PASS |
| `internal/repositories` | 23+ | Build fails (CGO) | ⚠️ CI/CD only |

---

## CGO Dependency Issue (Windows)

### Problem

Repository tests require **CGO** (C compiler) due to database driver dependencies:
- SQLite drivers in tests use C bindings
- Windows development environments often lack GCC in PATH
- Error: `cgo: C compiler "gcc" not found`

### Impact

- **Windows developers:** Repository tests cannot run locally without GCC
- **Linux/macOS:** Tests run fine (GCC typically installed)
- **CI/CD (GitHub Actions):** Tests run successfully (Linux environment with GCC)

### Solutions

#### Option 1: Run Tests in CI/CD (Recommended)

All tests run automatically in GitHub Actions on every push/PR:
- Location: `.github/workflows/test-investment-module.yml`
- Environment: Ubuntu with GCC
- PostgreSQL service container included
- **Advantage:** No local setup required, matches production environment

#### Option 2: Install GCC on Windows

Install TDM-GCC or MinGW:

**TDM-GCC (Recommended):**
1. Download: [https://jmeubank.github.io/tdm-gcc/](https://jmeubank.github.io/tdm-gcc/)
2. Install TDM-GCC 64-bit
3. Add to PATH: `C:\TDM-GCC-64\bin`
4. Verify: `gcc --version`
5. Run tests: `go test ./internal/repositories -v`

**MinGW:**
1. Download: [https://www.mingw-w64.org/](https://www.mingw-w64.org/)
2. Install MinGW-w64
3. Add to PATH: `C:\mingw64\bin`
4. Verify: `gcc --version`

#### Option 3: Use Docker for Testing

Run tests in containerized environment:

```powershell
# Build test container
docker-compose -f docker-compose.test.yml build

# Run tests in container
docker-compose -f docker-compose.test.yml run --rm api go test ./...

# Run with coverage
docker-compose -f docker-compose.test.yml run --rm api go test ./... -coverprofile=coverage.out
```

**Note:** `docker-compose.test.yml` needs to be created (see example below).

#### Option 4: Use Build Tags (Alternative)

Skip CGO-dependent tests locally:

```powershell
# Run tests without CGO
$env:CGO_ENABLED="0"; go test ./...
```

**Limitation:** Repository tests will be skipped.

---

## Test Database Setup

### Local PostgreSQL (For Integration Tests)

```powershell
# Start PostgreSQL via Docker Compose
docker-compose up -d postgres

# Wait for PostgreSQL to be ready
Start-Sleep -Seconds 5

# Run migrations
go run cmd/api/main.go migrate

# Run integration tests
go test ./tests/integration/... -v
```

### Test Database Configuration

Environment variables for tests:

```env
DATABASE_URL=postgres://propvest:password@localhost:5432/propvest_test?sslmode=disable
JWT_SECRET=test-secret-key-for-testing-only
APP_ENV=test
```

---

## CI/CD Testing

### GitHub Actions Workflows

**Current Workflows:**

1. **Investment Module Tests** (`.github/workflows/test-investment-module.yml`)
   - Triggers: Push to main/develop, PRs
   - Services: PostgreSQL 15
   - Go Version: 1.21 (will update to 1.25)
   - Jobs:
     - Repository tests with race detector
     - Service tests (when implemented)
     - Handler tests (when implemented)
     - Coverage reporting

**Planned Workflows (Phase 2):**

2. **Full Test Suite** (`.github/workflows/test.yml`)
   - All unit tests
   - Integration tests
   - E2E tests
   - Coverage ≥80%

3. **Security Scan** (`.github/workflows/security.yml`)
   - gosec (Go security checker)
   - trivy (vulnerability scanner)
   - Dependency scanning

4. **Lint** (`.github/workflows/lint.yml`)
   - gofmt
   - goimports
   - golangci-lint

### Running Tests Like CI/CD

Simulate CI/CD environment locally:

```powershell
# Set environment variables
$env:DATABASE_URL="postgres://propvest:password@localhost:5432/propvest_test?sslmode=disable"
$env:JWT_SECRET="test-secret-key"
$env:APP_ENV="test"

# Run tests with race detector (like CI/CD)
go test -race -coverprofile=coverage.out ./...

# Generate coverage report
go tool cover -func=coverage.out
```

---

## Coverage Targets

### Current Coverage

- **Overall:** ~3-10%
- **Config:** 51.1%
- **Queue:** 33.6%
- **Services:** 1.2%
- **Repositories:** ~85% (in CI/CD)

### Target Coverage (Milestone 8)

| Layer | Target | Critical? |
|-------|--------|-----------|
| Services | ≥90% | ✅ Critical |
| Repositories | ≥85% | ✅ Critical |
| Handlers | ≥80% | ✅ Critical |
| Middleware | ≥80% | Medium |
| Validators | ≥95% | ✅ Critical |
| DTOs | ≥95% | ✅ Critical |
| Utils | ≥85% | Medium |
| **Overall** | **≥80%** | **✅ Critical** |

---

## Test Structure

### Directory Layout

```
tests/
├── integration/          # Cross-module tests
├── e2e/                  # End-to-end user journeys
├── concurrency/          # Race condition tests
├── performance/          # Load tests (k6)
├── security/             # Security tests (OWASP)
├── fixtures/             # Test data (JSON)
├── testdata/             # Static files (images, docs)
└── helpers/              # Test utilities
    ├── setup.go          # DB setup/teardown
    ├── auth_helper.go    # JWT generation
    ├── api_client.go     # HTTP client wrapper
    └── assertions.go     # Custom assertions
```

### Unit Test Locations

```
internal/
├── handlers/
│   ├── auth.go
│   └── auth_test.go       # ← Handler tests
├── services/
│   ├── auth_service.go
│   └── auth_service_test.go  # ← Service tests
├── repositories/
│   ├── user_repository.go
│   └── user_repository_test.go  # ← Repository tests
└── utils/
    ├── jwt/
    │   ├── jwt.go
    │   └── jwt_test.go    # ← Utility tests
```

---

## Writing Tests

### Test Naming Convention

```go
// Format: Test<FunctionName>_<Scenario>_<ExpectedBehavior>
func TestCreateInvestment_InsufficientFunds_ReturnsError(t *testing.T) {
    // Test implementation
}
```

### AAA Pattern (Arrange, Act, Assert)

```go
func TestUserService_CreateUser_Success(t *testing.T) {
    // Arrange
    repo := &mockUserRepository{}
    service := NewUserService(repo)
    req := &dto.CreateUserRequest{
        Email: "test@example.com",
        Password: "SecurePass123",
    }

    // Act
    user, err := service.CreateUser(context.Background(), req)

    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, user)
    assert.Equal(t, req.Email, user.Email)
}
```

### Table-Driven Tests

```go
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        wantErr bool
    }{
        {"valid email", "user@example.com", false},
        {"missing @", "userexample.com", true},
        {"empty", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateEmail(tt.email)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

---

## Troubleshooting

### Test Failures

**Issue:** Tests fail with "database connection refused"

```powershell
# Solution: Start PostgreSQL
docker-compose up -d postgres

# Verify connection
psql -h localhost -U propvest -d propvest_test
```

**Issue:** Tests timeout

```powershell
# Solution: Increase timeout
go test -timeout 300s ./...
```

**Issue:** CGO error on Windows

```powershell
# Solution 1: Use CI/CD (recommended)
git push  # Tests run in GitHub Actions

# Solution 2: Install GCC (see "CGO Dependency Issue" section)

# Solution 3: Disable CGO (skip repository tests)
$env:CGO_ENABLED="0"; go test ./...
```

### Race Detector Issues

**Issue:** Race condition detected

```powershell
# Run with race detector to find issue
go test -race ./...

# Fix: Add proper synchronization (mutex, channels, atomic)
```

**Issue:** Race detector slows tests

```powershell
# Normal run (fast)
go test ./...

# Race detector run (slower, thorough)
go test -race ./...
```

---

## Performance Testing

### Benchmarks

```powershell
# Run benchmarks
go test -bench=. ./...

# Run benchmarks with memory profiling
go test -bench=. -benchmem ./...

# Run specific benchmark
go test -bench=BenchmarkCreateInvestment ./internal/services
```

### Load Testing (k6)

```powershell
# Install k6
choco install k6

# Run load test
k6 run tests/performance/load_test.js

# Run stress test
k6 run tests/performance/stress_test.js
```

---

## Best Practices

### DO ✅

- Write tests for new features
- Use table-driven tests for multiple scenarios
- Mock external dependencies (database, APIs)
- Use descriptive test names
- Test both success and failure cases
- Run tests before committing
- Achieve ≥80% coverage for critical code

### DON'T ❌

- Don't test private functions directly
- Don't use `time.Now()` or `uuid.New()` (use mocks)
- Don't share state between tests
- Don't commit commented-out tests
- Don't skip tests without a good reason
- Don't test implementation details

---

## Resources

### Internal Documentation

- `PROPVEST_COMPREHENSIVE_TESTING_PLAN.md` - Detailed testing strategy
- `docs/06-Engineering/6.3-TESTING_STRATEGY.md` - Testing philosophy
- `.github/workflows/test-investment-module.yml` - CI/CD example

### External Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify](https://github.com/stretchr/testify) - Assertion library
- [k6 Load Testing](https://k6.io/docs/)
- [TableDrivenTests](https://github.com/golang/go/wiki/TableDrivenTests)

---

## Quick Reference

```powershell
# Run all tests
go test ./...

# Run with coverage
go test ./... -coverprofile=coverage.out

# View coverage in browser
go tool cover -html=coverage.out

# Run with race detector
go test -race ./...

# Run specific package
go test ./internal/services -v

# Run specific test
go test ./internal/services -run TestCreateUser -v

# Run benchmarks
go test -bench=. ./...

# Clean test cache
go clean -testcache
```

---

**Status:** ✅ **Active**  
**Owner:** Engineering Team  
**Last Updated:** October 6, 2026  

---

*For issues or questions about testing, refer to this guide or check CI/CD workflows.*
