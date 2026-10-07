# PropVest - Run Database Migrations
# ===================================
# This script runs migrations against the local PostgreSQL instance

Write-Host "🔄 Running PropVest Database Migrations..." -ForegroundColor Cyan

# Set local database URL (connects to Docker container on localhost:5435)
$env:DATABASE_URL = "postgres://propvest:password@localhost:5435/propvest?sslmode=disable"

Write-Host "📊 Database: $env:DATABASE_URL" -ForegroundColor Gray

# Run migrations
Write-Host "`n⬆️  Applying migrations..." -ForegroundColor Yellow

migrate -path internal/database/migrations -database $env:DATABASE_URL up

Write-Host "`n📋 Current schema version:" -ForegroundColor Cyan
migrate -path internal/database/migrations -database $env:DATABASE_URL version

Write-Host "`n💡 To verify in database:" -ForegroundColor Yellow
Write-Host "   docker exec -it propvest_postgres psql -U propvest -d propvest" -ForegroundColor Gray
Write-Host "   \d outbox_events" -ForegroundColor Gray
Write-Host "   \d+ outbox_events  # View with comments" -ForegroundColor Gray
Write-Host "   \di                # View indexes" -ForegroundColor Gray
