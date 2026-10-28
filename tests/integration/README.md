# Integration Tests

This directory contains integration tests that verify full application workflows with real database connections and service interactions.

## Overview

Integration tests differ from unit tests:
- **Unit tests** mock dependencies and test individual components
- **Integration tests** use real dependencies (database, services) and test complete workflows

## Setup

### Prerequisites
- Docker and Docker Compose installed
- Go 1.21 or later

### Quick Start

```bash
# 1. Start test environment (PostgreSQL, Redis, RabbitMQ)
make test-integration-setup

# 2. Run integration tests
make test-integration

# 3. Stop test environment
make test-integration-teardown
```

### All-in-One Command

```bash
# Runs setup, tests, and teardown automatically
make test-integration-full
```

## Test Environment

Integration tests use a separate Docker environment:
- **Database:** PostgreSQL on port 5433 (not 5432)
- **Redis:** Redis on port 6380 (not 6379)
- **RabbitMQ:** RabbitMQ on port 5673 (not 5672)

Configuration is in:
- `docker-compose.test.yml` - Test infrastructure
- `.env.test` - Test environment variables

## Test Organization

### Current Tests

#### Auth Integration Tests (`auth_integration_test.go`)
- ✅ `TestRegistrationCreatesWalletAtomically` - Verifies user+wallet atomic creation
- ✅ `TestLoginWithValidCredentials` - Full registration → login flow
- ✅ `TestLoginWithInvalidPassword` - Login failure handling
- ⏭️ `TestRegistrationRollbackOnError` - Transaction rollback (TODO)

### Planned Tests

#### Wallet Integration Tests (TODO)
- Deposit flow with payment provider
- Withdrawal flow with balance locking
- Transaction history consistency

#### Investment Integration Tests (TODO)
- Property funding workflow
- Wallet debit + investment creation
- Property slot updates
- Transaction atomicity

#### End-to-End Scenarios (TODO)
- Complete user journey: Register → Deposit → Invest → Withdraw
- Error recovery scenarios
- Race condition handling

## Writing Integration Tests

### Test Structure

```go
func TestFeatureName(t *testing.T) {
    // Skip if running quick tests
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Setup
    testDB := SetupTestDB(t)
    defer testDB.Cleanup()  // Always clean up!

    // Create real repositories and services
    repo := repositories.NewRepository(testDB.DB)
    service := services.NewService(repo, testDB.Config)

    // Create handler and router
    handler := handlers.NewHandler(service)
    router := setupRouter(handler)

    // Execute HTTP request
    req := httptest.NewRequest("POST", "/endpoint", body)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    // Assert HTTP response
    assert.Equal(t, http.StatusOK, w.Code)

    // Assert database state
    result, err := repo.Find(ctx, id)
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Best Practices

1. **Always use `testDB.Cleanup()`** - Ensures database is cleaned between tests
2. **Use real dependencies** - No mocks in integration tests
3. **Test full workflows** - From HTTP request to database state
4. **Verify side effects** - Check database, queue messages, etc.
5. **Use descriptive names** - `TestRegistrationCreatesWalletAtomically` not `TestRegister`

### Test Data Management

```go
// Helper creates test user with all dependencies
testDB.CreateTestUser(t, "test@example.com", "+2348012345678", "Test User")
```

Cleanup happens automatically via `testDB.Cleanup()` which:
- Truncates all tables in correct order
- Handles foreign key constraints
- Resets sequences

## Running Tests

### Run All Integration Tests
```bash
make test-integration-full
```

### Run Specific Test
```bash
# Start test environment
make test-integration-setup

# Run specific test
go test ./tests/integration -v -run TestRegistrationCreatesWalletAtomically

# Teardown
make test-integration-teardown
```

### Run with Coverage
```bash
make test-integration-setup
go test ./tests/integration -v -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
make test-integration-teardown
```

### Skip Integration Tests in CI
```bash
# Run only unit tests (skips integration tests)
go test ./... -short
```

## Troubleshooting

### Database Connection Errors

```
Error: failed to connect to test database
```

**Solutions:**
1. Ensure Docker is running
2. Check test containers are up: `docker ps`
3. Wait for healthchecks: `docker compose -f docker-compose.test.yml ps`
4. Check port availability: `netstat -an | findstr 5433`

### Migration Failures

```
Error: failed to run migrations
```

**Solutions:**
1. Check migration files in `internal/database/migrations`
2. Ensure migrations are compatible with PostgreSQL 16
3. Check migration version: `migrate -path internal/database/migrations -database "postgres://propvest_test:test_password@localhost:5433/propvest_test?sslmode=disable" version`

### Slow Tests

Integration tests are slower than unit tests by design. Optimize:
1. Use `tmpfs` for test database (already configured)
2. Run tests in parallel: `go test -parallel 4`
3. Skip heavy tests locally: `if testing.Short() { t.Skip() }`

### Port Conflicts

```
Error: port 5433 already in use
```

**Solutions:**
1. Stop conflicting containers: `docker ps`
2. Use different port in `docker-compose.test.yml`
3. Update port in `.env.test`

## Test Performance

### Typical Run Times
- Setup: 3-5 seconds
- Single test: 0.5-2 seconds
- Full suite: 5-10 seconds
- Teardown: 1-2 seconds

### Optimization Tips
1. **Parallel execution** - Tests using different data can run in parallel
2. **Shared setup** - Use `TestMain` for once-per-package setup
3. **Minimal cleanup** - Only truncate changed tables

## Coverage Goals

Integration tests complement unit tests:
- **Unit tests:** 70-90% coverage, fast feedback
- **Integration tests:** Critical paths, full workflows

**Target:** 5-10 integration test scenarios covering:
- ✅ User registration + wallet creation
- ✅ Login flow
- ⏭️ Deposit flow
- ⏭️ Withdrawal flow
- ⏭️ Investment flow
- ⏭️ Error recovery
- ⏭️ Race conditions

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Setup Integration Tests
  run: make test-integration-setup

- name: Run Integration Tests
  run: make test-integration

- name: Teardown Integration Tests
  if: always()
  run: make test-integration-teardown
```

## Next Steps

1. **Complete auth tests** - Add token refresh, logout all devices
2. **Add wallet tests** - Deposit, withdrawal, transaction history
3. **Add investment tests** - Property funding, portfolio management
4. **Add E2E scenarios** - Complete user journeys
5. **Add performance tests** - Benchmark critical paths

## Related Documentation

- Unit tests: `internal/services/*_test.go`, `internal/handlers/*_test.go`
- Test strategy: `docs/06-Engineering/6.3-TESTING_STRATEGY.md`
- Database design: `docs/02-Database/2.2-DATABASE_DESIGN.md`
