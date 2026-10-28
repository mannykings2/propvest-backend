.PHONY: run build up down tidy test test-unit test-integration test-integration-setup test-integration-teardown migrate-up migrate-down migrate-create migrate-force migrate-version db-backup db-restore-drill

run:
	@if exist .env.local (copy /Y .env.local .env >nul)
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

up:
	docker compose up -d

down:
	docker compose down

tidy:
	go mod tidy

# Run all tests
test:
	go test ./... -race -cover

# Run only unit tests (fast)
test-unit:
	go test ./internal/... -short -race -cover

# Setup integration test environment
test-integration-setup:
	docker compose -f docker-compose.test.yml up -d
	@echo "Waiting for test database to be ready..."
	@timeout /t 5 > nul 2>&1
	@echo "Test environment ready!"

# Teardown integration test environment
test-integration-teardown:
	docker compose -f docker-compose.test.yml down -v

# Run integration tests (requires test-integration-setup first)
test-integration:
	go test ./tests/integration/... -v -cover

# Run integration tests with setup and teardown
test-integration-full: test-integration-setup
	@echo "Running integration tests..."
	go test ./tests/integration/... -v -cover || (docker compose -f docker-compose.test.yml down -v && exit 1)
	docker compose -f docker-compose.test.yml down -v
	@echo "Integration tests complete!"

# ───────────────────────────────────────────────────────────────────────────
# DATABASE BACKUP & RESTORE DRILL (Phase 3.5 — see OPERATIONS_RUNBOOK.md)
# ───────────────────────────────────────────────────────────────────────────

# Logical backup -> backups/propvest_<timestamp>.dump (14 kept, rotated)
db-backup:
	bash scripts/backup_db.sh

# Restore the newest backup into scratch DB propvest_restore_drill and verify
# table list + row counts against the source. THIS is the go/no-go signal.
db-restore-drill:
	bash scripts/restore_drill.sh

# ───────────────────────────────────────────────────────────────────────────
# DATABASE MIGRATION COMMANDS
# ───────────────────────────────────────────────────────────────────────────
# These commands use golang-migrate to manage database schema changes.
# Migrations are versioned, reversible SQL files in internal/database/migrations/

# Apply all pending migrations
migrate-up:
	migrate -path internal/database/migrations -database "postgres://propvest:password@localhost:5435/propvest?sslmode=disable" up

# Rollback the last migration
migrate-down:
	migrate -path internal/database/migrations -database "postgres://propvest:password@localhost:5435/propvest?sslmode=disable" down 1

# Create a new migration file pair (up and down)
# Usage: make migrate-create name=add_users_table
migrate-create:
	@if [ -z "$(name)" ]; then \
		echo "Error: name parameter is required"; \
		echo "Usage: make migrate-create name=your_migration_name"; \
		exit 1; \
	fi
	migrate create -ext sql -dir internal/database/migrations -seq $(name)

# Force set migration version (use carefully!)
# Usage: make migrate-force version=1
migrate-force:
	@if [ -z "$(version)" ]; then \
		echo "Error: version parameter is required"; \
		echo "Usage: make migrate-force version=1"; \
		exit 1; \
	fi
	migrate -path internal/database/migrations -database "postgres://propvest:password@localhost:5435/propvest?sslmode=disable" force $(version)

# Show current migration version
migrate-version:
	migrate -path internal/database/migrations -database "postgres://propvest:password@localhost:5435/propvest?sslmode=disable" version