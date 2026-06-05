package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/database"
	"github.com/mannykings2/propvest-backend/internal/handlers"
)

func main() {
	// ── 1. Load configuration ────────────────────────────────────────────
	// This is always the very first thing. If config fails,
	// nothing else should even try to start.
	cfg := config.Load()

	// ── 2. Connect to the database ───────────────────────────────────────
	// We pass the DSN from config — main.go is the only place that
	// knows about both config and database at the same time.
	database.Connect(cfg.DatabaseURL)


    // ── 3. Run migrations ────────────────────────────────────────────────
	// AutoMigrate runs after Connect because it needs an active DB connection.
	// Order matters — always connect first, migrate second.
	database.AutoMigrate()


	// ── 3. Set Gin mode ──────────────────────────────────────────────────
	// In development, Gin prints every route and request in colour.
	// In production, set GIN_MODE=release to silence that output.
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// ── 4. Create router ─────────────────────────────────────────────────
	r := gin.Default()
	// gin.Default() includes two built-in middlewares:
	//   - Logger: prints every request (method, path, status, latency)
	//   - Recovery: catches panics and returns 500 instead of crashing

	// ── 5. Register routes ───────────────────────────────────────────────
	// All routes live under /api/v1. Versioning the API means you can
	// introduce /api/v2 later without breaking existing clients.
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", handlers.HealthCheck)
	}

	// ── 6. Start server ──────────────────────────────────────────────────
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("PropVest API starting on %s (env: %s)", addr, cfg.AppEnv)

	if err := r.Run(addr); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}