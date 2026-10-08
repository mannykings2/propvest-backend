package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupInvestmentTestDB creates an in-memory SQLite database for testing.
func setupInvestmentTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test database")

	// Run migrations
	err = db.AutoMigrate(
		&models.User{},
		&models.Property{},
		&models.Investment{},
	)
	require.NoError(t, err, "failed to migrate test database")

	return db
}

// createTestUser creates a test user.
func createTestInvestmentUser(db *gorm.DB) *models.User {
	user := &models.User{
		UserCode:     "TEST001",
		FullName:     "Test User",
		Email:        "test@example.com",
		Phone:        "+2348012345678",
		PasswordHash: "hashedpassword",
		Role:         "user",
	}
	db.Create(user)
	return user
}

// createTestInvestmentProperty creates a test property.
func createTestInvestmentProperty(db *gorm.DB) *models.Property {
	launchDate := time.Now().AddDate(0, 1, 0)
	completionDate := time.Now().AddDate(1, 0, 0)
	
	property := &models.Property{
		Title:                  "Test Property",
		Slug:                   "test-property",
		Description:            "Test description",
		PropertyType:           "residential",
		Status:                 "active",
		TargetAmount:           10000000, // 100,000 NGN
		MinimumInvestment:      500000,   // 5,000 NGN
		UnitPrice:              100000,   // 1,000 NGN per unit
		TotalUnits:             100,
		ROIPercent:             15.5,
		DurationMonths:         12,
		LaunchDate:             &launchDate,
		ExpectedCompletionDate: &completionDate,
		City:                   "Lagos",
		State:                  "Lagos",
		Country:                "Nigeria",
		Address:                "123 Test Street",
	}
	db.Create(property)
	return property
}

// createTestInvestment creates a test investment.
func createTestInvestment(userID, propertyID uuid.UUID) *models.Investment {
	return &models.Investment{
		UserID:        userID,
		PropertyID:    propertyID,
		Slots:         10,
		AmountKobo:    1000000,
		UnitPriceKobo: 100000,
		Currency:      "NGN",
		Status:        models.InvestmentStatusActive,
		Reference:     "INV-" + uuid.New().String(),
	}
}

// TestNewInvestmentRepository verifies repository initialization.
func TestNewInvestmentRepository(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	assert.NotNil(t, repo)
}

// TestInvestmentRepository_Create_Success verifies investment creation.
func TestInvestmentRepository_Create_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)
	investment := createTestInvestment(user.ID, property.ID)

	err := repo.Create(ctx, investment, nil)

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, investment.ID)
	assert.Equal(t, models.InvestmentStatusActive, investment.Status)
	assert.NotZero(t, investment.CreatedAt)
}

// TestInvestmentRepository_Create_WithIdempotencyKey verifies idempotency key handling.
func TestInvestmentRepository_Create_WithIdempotencyKey(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)
	idempotencyKey := "test-key-001"

	investment := createTestInvestment(user.ID, property.ID)
	investment.IdempotencyKey = &idempotencyKey

	err := repo.Create(ctx, investment, nil)

	require.NoError(t, err)
	assert.Equal(t, idempotencyKey, *investment.IdempotencyKey)
}

// TestInvestmentRepository_FindByID_Success verifies finding investment by ID.
func TestInvestmentRepository_FindByID_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)
	investment := createTestInvestment(user.ID, property.ID)
	err := repo.Create(ctx, investment, nil)
	require.NoError(t, err)

	found, err := repo.FindByID(ctx, investment.ID)

	require.NoError(t, err)
	assert.Equal(t, investment.ID, found.ID)
	assert.Equal(t, investment.Reference, found.Reference)
	assert.Equal(t, investment.AmountKobo, found.AmountKobo)
}

// TestInvestmentRepository_FindByID_NotFound verifies not found error.
func TestInvestmentRepository_FindByID_NotFound(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, uuid.New())

	assert.Error(t, err)
	assert.True(t, IsErrRecordNotFound(err))
}

// TestInvestmentRepository_FindByIDForUpdate_Success verifies row locking.
func TestInvestmentRepository_FindByIDForUpdate_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)
	investment := createTestInvestment(user.ID, property.ID)
	err := repo.Create(ctx, investment, nil)
	require.NoError(t, err)

	// Test with transaction
	err = db.Transaction(func(tx *gorm.DB) error {
		found, err := repo.FindByIDForUpdate(ctx, investment.ID, tx)
		require.NoError(t, err)
		assert.Equal(t, investment.ID, found.ID)
		return nil
	})

	require.NoError(t, err)
}

// TestInvestmentRepository_FindByIDForUpdate_RequiresTransaction verifies transaction requirement.
func TestInvestmentRepository_FindByIDForUpdate_RequiresTransaction(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)
	investment := createTestInvestment(user.ID, property.ID)
	err := repo.Create(ctx, investment, nil)
	require.NoError(t, err)

	// Should fail without transaction
	_, err = repo.FindByIDForUpdate(ctx, investment.ID, nil)

	assert.Error(t, err)
	assert.Equal(t, gorm.ErrInvalidTransaction, err)
}

// TestInvestmentRepository_FindByUserAndIdempotencyKey_Success verifies idempotency check.
func TestInvestmentRepository_FindByUserAndIdempotencyKey_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)
	idempotencyKey := "test-key-001"

	investment := createTestInvestment(user.ID, property.ID)
	investment.IdempotencyKey = &idempotencyKey
	err := repo.Create(ctx, investment, nil)
	require.NoError(t, err)

	found, err := repo.FindByUserAndIdempotencyKey(ctx, user.ID, idempotencyKey, nil)

	require.NoError(t, err)
	assert.Equal(t, investment.ID, found.ID)
	assert.Equal(t, idempotencyKey, *found.IdempotencyKey)
}

// TestInvestmentRepository_FindByUserAndIdempotencyKey_NotFound verifies not found case.
func TestInvestmentRepository_FindByUserAndIdempotencyKey_NotFound(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	_, err := repo.FindByUserAndIdempotencyKey(ctx, uuid.New(), "nonexistent", nil)

	assert.Error(t, err)
	assert.True(t, IsErrRecordNotFound(err))
}

// TestInvestmentRepository_HasActiveInvestmentByUserAndProperty verifies investor check.
func TestInvestmentRepository_HasActiveInvestmentByUserAndProperty(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Initially no investment
	hasInvested, err := repo.HasActiveInvestmentByUserAndProperty(ctx, nil, user.ID, property.ID)
	require.NoError(t, err)
	assert.False(t, hasInvested, "should not have investment initially")

	// Create investment
	investment := createTestInvestment(user.ID, property.ID)
	err = repo.Create(ctx, investment, nil)
	require.NoError(t, err)

	// Now should have investment
	hasInvested, err = repo.HasActiveInvestmentByUserAndProperty(ctx, nil, user.ID, property.ID)
	require.NoError(t, err)
	assert.True(t, hasInvested, "should have active investment")
}

// TestInvestmentRepository_ListByUser_Success verifies user investment listing.
func TestInvestmentRepository_ListByUser_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create 3 investments
	for i := 0; i < 3; i++ {
		investment := createTestInvestment(user.ID, property.ID)
		err := repo.Create(ctx, investment, nil)
		require.NoError(t, err)
	}

	investments, total, err := repo.ListByUser(ctx, user.ID, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, investments, 3)
}

// TestInvestmentRepository_ListByUser_Pagination verifies pagination.
func TestInvestmentRepository_ListByUser_Pagination(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create 5 investments
	for i := 0; i < 5; i++ {
		investment := createTestInvestment(user.ID, property.ID)
		err := repo.Create(ctx, investment, nil)
		require.NoError(t, err)
	}

	// Get first page (limit 2)
	investments, total, err := repo.ListByUser(ctx, user.ID, 2, 0)

	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, investments, 2)

	// Get second page (limit 2, offset 2)
	investments, total, err = repo.ListByUser(ctx, user.ID, 2, 2)

	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, investments, 2)
}

// TestInvestmentRepository_ListByProperty_Success verifies property investment listing.
func TestInvestmentRepository_ListByProperty_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	property := createTestInvestmentProperty(db)

	// Create investments from 3 different users
	for i := 0; i < 3; i++ {
		user := createTestInvestmentUser(db)
		investment := createTestInvestment(user.ID, property.ID)
		err := repo.Create(ctx, investment, nil)
		require.NoError(t, err)
	}

	investments, total, err := repo.ListByProperty(ctx, property.ID, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, investments, 3)
}

// TestInvestmentRepository_ListAll_Success verifies listing all investments.
func TestInvestmentRepository_ListAll_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create investments with different statuses
	statuses := []string{
		models.InvestmentStatusActive,
		models.InvestmentStatusActive,
		models.InvestmentStatusCompleted,
	}

	for _, status := range statuses {
		investment := createTestInvestment(user.ID, property.ID)
		investment.Status = status
		err := repo.Create(ctx, investment, nil)
		require.NoError(t, err)
	}

	// List all
	investments, total, err := repo.ListAll(ctx, "", 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, investments, 3)
}

// TestInvestmentRepository_ListAll_WithStatusFilter verifies status filtering.
func TestInvestmentRepository_ListAll_WithStatusFilter(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create investments with different statuses
	activeInv := createTestInvestment(user.ID, property.ID)
	activeInv.Status = models.InvestmentStatusActive
	repo.Create(ctx, activeInv, nil)

	completedInv := createTestInvestment(user.ID, property.ID)
	completedInv.Status = models.InvestmentStatusCompleted
	repo.Create(ctx, completedInv, nil)

	// Filter by active status
	investments, total, err := repo.ListAll(ctx, models.InvestmentStatusActive, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, investments, 1)
	assert.Equal(t, models.InvestmentStatusActive, investments[0].Status)
}

// TestInvestmentRepository_PortfolioSummary_Success verifies portfolio aggregation.
func TestInvestmentRepository_PortfolioSummary_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create 2 active investments
	for i := 0; i < 2; i++ {
		investment := createTestInvestment(user.ID, property.ID)
		investment.AmountKobo = 1000000 // 10,000 NGN each
		investment.Status = models.InvestmentStatusActive
		err := repo.Create(ctx, investment, nil)
		require.NoError(t, err)
	}

	// Create 1 completed investment (should not be counted)
	completedInv := createTestInvestment(user.ID, property.ID)
	completedInv.AmountKobo = 500000
	completedInv.Status = models.InvestmentStatusCompleted
	repo.Create(ctx, completedInv, nil)

	totalInvested, count, err := repo.PortfolioSummary(ctx, user.ID)

	require.NoError(t, err)
	assert.Equal(t, int64(2000000), totalInvested, "should sum only active investments")
	assert.Equal(t, int64(2), count, "should count only active investments")
}

// TestInvestmentRepository_Metrics_Success verifies metrics calculation.
func TestInvestmentRepository_Metrics_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user1 := createTestInvestmentUser(db)
	user2 := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create investments with different statuses
	activeInv := createTestInvestment(user1.ID, property.ID)
	activeInv.AmountKobo = 1000000
	activeInv.Status = models.InvestmentStatusActive
	repo.Create(ctx, activeInv, nil)

	completedInv := createTestInvestment(user2.ID, property.ID)
	completedInv.AmountKobo = 2000000
	completedInv.Status = models.InvestmentStatusCompleted
	repo.Create(ctx, completedInv, nil)

	metrics, err := repo.Metrics(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(2), metrics.TotalInvestments)
	assert.Equal(t, int64(3000000), metrics.TotalAmountKobo)
	assert.Equal(t, int64(1), metrics.ActiveInvestments)
	assert.Equal(t, int64(1), metrics.CompletedInvestments)
	assert.Equal(t, int64(2), metrics.UniqueInvestors)
	assert.Equal(t, int64(1500000), metrics.AverageInvestmentKobo)
}

// TestInvestmentRepository_CountAll_Success verifies total count.
func TestInvestmentRepository_CountAll_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create 3 investments
	for i := 0; i < 3; i++ {
		investment := createTestInvestment(user.ID, property.ID)
		err := repo.Create(ctx, investment, nil)
		require.NoError(t, err)
	}

	count, err := repo.CountAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

// TestInvestmentRepository_SumAll_Success verifies total sum.
func TestInvestmentRepository_SumAll_Success(t *testing.T) {
	db := setupInvestmentTestDB(t)
	repo := NewInvestmentRepository(db)
	ctx := context.Background()

	user := createTestInvestmentUser(db)
	property := createTestInvestmentProperty(db)

	// Create investments with known amounts
	amounts := []int64{1000000, 2000000, 3000000}
	for _, amount := range amounts {
		investment := createTestInvestment(user.ID, property.ID)
		investment.AmountKobo = amount
		err := repo.Create(ctx, investment, nil)
		require.NoError(t, err)
	}

	sum, err := repo.SumAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(6000000), sum)
}

