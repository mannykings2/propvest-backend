package database

import (
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mannykings2/propvest-backend/internal/models"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the single shared database connection for the entire application.
// It is safe to use concurrently — GORM manages a connection pool internally.
var DB *gorm.DB

// Connect opens the database connection and stores it in the package-level
// DB variable. It is called once at startup.
func Connect(dsn string) {
	var err error

	DB, err = gorm.Open(pgdriver.Open(dsn), &gorm.Config{
		// In development, log every SQL query so you can see exactly
		// what GORM is sending to the database. In production you would
		// set this to logger.Silent or logger.Warn.
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Database connection established")

	// Get the underlying database/sql connection pool.
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}

	// Configure the connection pool.
	//
	// MaxOpenConns:
	//   Maximum number of open connections to the database.
	sqlDB.SetMaxOpenConns(25)

	// MaxIdleConns:
	//   Number of idle connections kept ready for future requests.
	sqlDB.SetMaxIdleConns(10)

	// ConnMaxLifetime:
	//   Recycle connections after one hour to avoid stale connections.
	sqlDB.SetConnMaxLifetime(time.Hour)

	// ConnMaxIdleTime:
	//   Close idle connections after 15 minutes.
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

	log.Println("Database connection pool configured")
}

// RunMigrations executes all pending database migrations using golang-migrate.
// This is the PRODUCTION approach — explicit, versioned SQL migrations with rollback support.
//
// Why golang-migrate instead of AutoMigrate?
//   1. Full control: You write exact SQL, no surprises
//   2. Rollback support: Every migration has a .down.sql for reverting
//   3. Version tracking: migrations_schema_version table tracks what's applied
//   4. Team-friendly: Migrations are code-reviewed like any other code
//   5. Production-safe: No magic behavior, explicit changes only
//
// Migration files live in internal/database/migrations/
// File naming: NNNNNN_description.up.sql and NNNNNN_description.down.sql
//
// How to create a new migration:
//   migrate create -ext sql -dir internal/database/migrations -seq add_users_table
//
// This creates:
//   000001_add_users_table.up.sql   ← your forward migration
//   000001_add_users_table.down.sql ← your rollback migration
func RunMigrations(databaseURL string) error {
	// Get the underlying *sql.DB from GORM
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Create a postgres driver instance for golang-migrate
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migrate driver: %w", err)
	}

	// Create the migration instance
	// "file://..." tells migrate where to find migration files
	m, err := migrate.NewWithDatabaseInstance(
		"file://internal/database/migrations",
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	// Run all pending migrations
	// migrate.Up() applies migrations in order: 000001, 000002, 000003...
	// It's idempotent — safe to run multiple times
	// Already-applied migrations are skipped automatically
	log.Println("Running database migrations...")
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Get current migration version for logging
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		log.Printf("WARNING: Database is in dirty state at version %d", version)
		log.Println("This means a previous migration failed partway through")
		log.Println("You may need to manually fix the database and force the version")
		return fmt.Errorf("database is in dirty state")
	}

	log.Printf("Database migrations completed successfully (current version: %d)", version)
	return nil
}

// AutoMigrate uses GORM's automatic migration feature.
// This is ONLY for rapid prototyping — NOT for production.
//
// Why AutoMigrate is risky:
//   1. No rollback: Can't undo what it did
//   2. No version control: Can't track what changed when
//   3. Only additive: Can add columns/tables but never removes them
//   4. Magic behavior: GORM decides the SQL, not you
//   5. Schema drift: Different developers might get different results
//
// We keep this function for DEVELOPMENT ONLY while building Milestone 0-1.
// After Milestone 1, all schema changes MUST use RunMigrations() instead.
//
// DEPRECATED: Use RunMigrations() for all new development
func AutoMigrate() {
	log.Println("WARNING: AutoMigrate is deprecated — use RunMigrations() instead")
	log.Println("AutoMigrate is for development only and will be removed before production")

	err := DB.AutoMigrate(
		&models.User{},
		&models.Wallet{},
		&models.WalletTransaction{},
		&models.Property{},
	)
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	log.Println("Database migration completed (via AutoMigrate)")
}