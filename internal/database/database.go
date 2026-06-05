package database

import (
	"log"

	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/driver/postgres"
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

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		// In development, log every SQL query so you can see exactly
		// what GORM is sending to the database. In production you would
		// set this to logger.Silent or logger.Warn.
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Database connection established")
}



// AutoMigrate compares the current model structs against the actual
// database schema and adds any missing tables or columns.
//
// CRITICAL RULES for AutoMigrate:
//   1. It ONLY adds — it never drops columns or tables.
//      Removing a field from the struct does NOT remove it from the DB.
//   2. It is safe to run on every startup in development.
//   3. In production, use explicit SQL migration files (golang-migrate)
//      instead — AutoMigrate gives you less control and no rollback.
//
// Add every new model here as you build each Phase.
func AutoMigrate() {
	err := DB.AutoMigrate(
		&models.User{},
		&models.Wallet{},
	)
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	log.Println("Database migration completed")
}