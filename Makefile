.PHONY: run build up down tidy test migrate-up migrate-down migrate-create migrate-force migrate-version

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