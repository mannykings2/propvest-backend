package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestDB holds the test database connection and cleanup functions
type TestDB struct {
	DB      *gorm.DB
	Config  *config.Config
	Cleanup func()
}

// SetupTestDB initializes a test database connection
// It loads .env.test, connects to the test database, runs migrations,
// and returns a TestDB with a cleanup function
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	// Load test environment variables
	err := godotenv.Load("../../.env.test")
	if err != nil {
		t.Logf("Warning: Could not load .env.test file: %v", err)
	}

	// Load config from environment
	cfg := config.Load()

	// Connect to test database (sets global database.DB)
	database.Connect(cfg)
	require.NotNil(t, database.DB, "Failed to connect to test database")

	// Run migrations
	err = database.RunMigrations(cfg.DatabaseURL)
	require.NoError(t, err, "Failed to run migrations")

	// Create cleanup function
	cleanup := func() {
		// Clean all tables in reverse dependency order
		tables := []string{
			"settlement_items",
			"settlement_batches",
			"str_reports",
			"ctr_reports",
			"screening_results",
			"risk_alerts",
			// risk_rules deliberately NOT truncated: it is seeded by migration
			// 000032 (golang-migrate only runs once), so clearing it would leave
			// every later test with zero rules.
			"support_replies",
			"support_tickets",
			"refresh_tokens",
			"verification_tokens",
			"otp_verifications",
			"audit_logs",
			"outbox_events",
			"investments",
			"property_images",
			"property_documents",
			"properties",
			"kyc_submissions",
			"transactions",
			"payments",
			"wallets",
			"users",
		}

		for _, table := range tables {
			database.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		}

		// Close connection
		sqlDB, _ := database.DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}

	return &TestDB{
		DB:      database.DB,
		Config:  cfg,
		Cleanup: cleanup,
	}
}

// CreateTestUser creates a test user with a wallet
// Returns user ID and wallet ID
func (tdb *TestDB) CreateTestUser(t *testing.T, email, phone, fullName string) (userID, walletID string) {
	t.Helper()

	// This will be implemented with actual repository calls
	// For now, placeholder
	require.NotEmpty(t, email)
	require.NotEmpty(t, phone)

	return "", ""
}

// WaitForDatabase waits for the test database to be ready
// Used in test setup to ensure database is available
func WaitForDatabase(cfg *config.Config, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for database")
		case <-ticker.C:
			// Try to connect (sets database.DB)
			database.Connect(cfg)
			if database.DB != nil {
				sqlDB, _ := database.DB.DB()
				if sqlDB != nil {
					if err := sqlDB.Ping(); err == nil {
						sqlDB.Close()
						return nil
					}
					sqlDB.Close()
				}
			}
		}
	}
}

// GetTestEnv returns an environment variable with a fallback to default
func GetTestEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
