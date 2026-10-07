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

// setupPropertyTestDB creates an in-memory SQLite database for testing.
func setupPropertyTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test database")

	// Run migrations
	err = db.AutoMigrate(
		&models.Property{},
		&models.PropertyImage{},
		&models.PropertyDocument{},
		&models.PropertyStatusHistory{},
	)
	require.NoError(t, err, "failed to migrate test database")

	return db
}

// createTestProperty is a helper to create a test property.
func createTestProperty(status string) *models.Property {
	launchDate := time.Now().AddDate(0, 1, 0)  // 1 month from now
	completionDate := time.Now().AddDate(1, 0, 0) // 1 year from now
	
	return &models.Property{
		Title:                  "Test Property",
		Slug:                   "test-property",
		Description:            "Test description",
		PropertyType:           "residential",
		Status:                 status,
		TargetAmount:           10000000, // 100,000 NGN in kobo
		MinimumInvestment:      500000,   // 5,000 NGN in kobo
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
}

// TestNewPropertyRepository verifies repository initialization.
func TestNewPropertyRepository(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	assert.NotNil(t, repo)
}

// TestPropertyRepository_Create_Success verifies property creation.
func TestPropertyRepository_Create_Success(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	property := createTestProperty("draft")

	err := repo.Create(ctx, property, nil)

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, property.ID, "property ID should be set")
	assert.Equal(t, "draft", property.Status)
	assert.NotZero(t, property.CreatedAt)
}

// TestPropertyRepository_FindByID_Success verifies finding property by ID.
func TestPropertyRepository_FindByID_Success(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create property
	property := createTestProperty("active")
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Find property
	found, err := repo.FindByID(ctx, property.ID)

	require.NoError(t, err)
	assert.Equal(t, property.ID, found.ID)
	assert.Equal(t, property.Title, found.Title)
	assert.Equal(t, "active", found.Status)
}

// TestPropertyRepository_FindByID_NotFound verifies error when property doesn't exist.
func TestPropertyRepository_FindByID_NotFound(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, uuid.New())

	assert.Error(t, err)
	assert.True(t, IsErrRecordNotFound(err))
}

// TestPropertyRepository_FindPublicByID_ExcludesDrafts verifies draft exclusion.
func TestPropertyRepository_FindPublicByID_ExcludesDrafts(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create draft property
	property := createTestProperty("draft")
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Try to find as public (should fail)
	_, err = repo.FindPublicByID(ctx, property.ID)

	assert.Error(t, err)
	assert.True(t, IsErrRecordNotFound(err))
}

// TestPropertyRepository_FindPublicByID_AllowsActive verifies active properties visible.
func TestPropertyRepository_FindPublicByID_AllowsActive(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create active property
	property := createTestProperty("active")
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Find as public (should succeed)
	found, err := repo.FindPublicByID(ctx, property.ID)

	require.NoError(t, err)
	assert.Equal(t, property.ID, found.ID)
	assert.Equal(t, "active", found.Status)
}

// TestPropertyRepository_FindBySlug_Success verifies finding by slug.
func TestPropertyRepository_FindBySlug_Success(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	property := createTestProperty("active")
	property.Slug = "unique-slug-123"
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	found, err := repo.FindBySlug(ctx, "unique-slug-123")

	require.NoError(t, err)
	assert.Equal(t, property.ID, found.ID)
}

// TestPropertyRepository_Update_Success verifies property update.
func TestPropertyRepository_Update_Success(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create property
	property := createTestProperty("draft")
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Update title
	property.Title = "Updated Title"
	err = repo.Update(ctx, property, nil)

	require.NoError(t, err)

	// Verify update
	found, err := repo.FindByID(ctx, property.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", found.Title)
}

// TestPropertyRepository_SoftDelete_Success verifies soft deletion.
func TestPropertyRepository_SoftDelete_Success(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create property
	property := createTestProperty("draft")
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Soft delete
	err = repo.SoftDelete(ctx, property.ID, nil)
	require.NoError(t, err)

	// Verify not found in normal query
	_, err = repo.FindByID(ctx, property.ID)
	assert.Error(t, err)
	assert.True(t, IsErrRecordNotFound(err))

	// Verify still exists with Unscoped
	var deleted models.Property
	err = db.Unscoped().First(&deleted, property.ID).Error
	require.NoError(t, err)
	assert.NotNil(t, deleted.DeletedAt)
}

// TestPropertyRepository_List_Pagination verifies pagination.
func TestPropertyRepository_List_Pagination(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create 5 properties
	for i := 0; i < 5; i++ {
		property := createTestProperty("active")
		property.Title = "Property " + string(rune('A'+i))
		err := repo.Create(ctx, property, nil)
		require.NoError(t, err)
	}

	// List with pagination
	filter := PropertyFilter{
		Page:     1,
		PageSize: 2,
	}
	properties, total, err := repo.List(ctx, filter)

	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, properties, 2)
}

// TestPropertyRepository_List_FilterByCity verifies city filtering.
func TestPropertyRepository_List_FilterByCity(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create properties in different cities
	lagos := createTestProperty("active")
	lagos.City = "Lagos"
	lagos.Title = "Lagos Property"
	err := repo.Create(ctx, lagos, nil)
	require.NoError(t, err)

	abuja := createTestProperty("active")
	abuja.City = "Abuja"
	abuja.Title = "Abuja Property"
	err = repo.Create(ctx, abuja, nil)
	require.NoError(t, err)

	// Filter by Lagos
	filter := PropertyFilter{
		City:     "Lagos",
		Page:     1,
		PageSize: 10,
	}
	properties, total, err := repo.List(ctx, filter)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Lagos Property", properties[0].Title)
}

// TestPropertyRepository_List_FilterByStatus verifies status filtering.
func TestPropertyRepository_List_FilterByStatus(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create draft and active properties
	draft := createTestProperty("draft")
	err := repo.Create(ctx, draft, nil)
	require.NoError(t, err)

	active := createTestProperty("active")
	err = repo.Create(ctx, active, nil)
	require.NoError(t, err)

	// Filter by active only
	filter := PropertyFilter{
		Status:   "active",
		Page:     1,
		PageSize: 10,
	}
	properties, total, err := repo.List(ctx, filter)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "active", properties[0].Status)
}

// TestPropertyRepository_List_Search verifies text search.
func TestPropertyRepository_List_Search(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create properties with different names
	luxury := createTestProperty("active")
	luxury.Title = "Luxury Apartment"
	err := repo.Create(ctx, luxury, nil)
	require.NoError(t, err)

	budget := createTestProperty("active")
	budget.Title = "Budget House"
	err = repo.Create(ctx, budget, nil)
	require.NoError(t, err)

	// Search for "Luxury"
	filter := PropertyFilter{
		Search:   "Luxury",
		Page:     1,
		PageSize: 10,
	}
	properties, total, err := repo.List(ctx, filter)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "Luxury Apartment", properties[0].Title)
}

// TestPropertyRepository_UpdateStatus_Success verifies status update.
func TestPropertyRepository_UpdateStatus_Success(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create draft property
	property := createTestProperty("draft")
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Update status to active
	err = repo.UpdateStatus(ctx, property.ID, "draft", "active", nil)
	require.NoError(t, err)

	// Verify status changed
	found, err := repo.FindByID(ctx, property.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", found.Status)
}

// TestPropertyRepository_UpdateStatus_WrongFromStatus verifies status validation.
func TestPropertyRepository_UpdateStatus_WrongFromStatus(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create draft property
	property := createTestProperty("draft")
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Try to update with wrong fromStatus
	err = repo.UpdateStatus(ctx, property.ID, "active", "funded", nil)

	assert.Error(t, err)
}

// TestPropertyRepository_IncrementFunding_Success verifies funding increment.
func TestPropertyRepository_IncrementFunding_Success(t *testing.T) {
	db := setupPropertyTestDB(t)
	repo := NewPropertyRepository(db)
	ctx := context.Background()

	// Create property
	property := createTestProperty("active")
	property.RaisedAmount = 0
	property.UnitsSold = 0
	property.InvestorCount = 0
	err := repo.Create(ctx, property, nil)
	require.NoError(t, err)

	// Increment funding
	err = repo.IncrementFunding(ctx, property.ID, 1000000, 2, 1, nil)
	require.NoError(t, err)

	// Verify increment
	found, err := repo.FindByID(ctx, property.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1000000), found.RaisedAmount)
	assert.Equal(t, int64(2), found.UnitsSold)
	assert.Equal(t, 1, found.InvestorCount)
}
