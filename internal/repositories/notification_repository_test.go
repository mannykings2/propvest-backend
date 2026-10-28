package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupNotificationTestDB creates an in-memory SQLite database for testing.
func setupNotificationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test database")

	// Run migrations
	err = db.AutoMigrate(
		&models.User{},
		&models.Notification{},
	)
	require.NoError(t, err, "failed to migrate test database")

	return db
}

// createTestNotificationUser creates a test user.
func createTestNotificationUser(db *gorm.DB, email string) *models.User {
	user := &models.User{
		UserCode:     "TEST001",
		FullName:     "Test User",
		Email:        email,
		Phone:        "+2348012345678",
		PasswordHash: "hashedpassword",
		Role:         "user",
	}
	db.Create(user)
	return user
}

// createTestNotification creates a test notification.
func createTestNotification(userID uuid.UUID, notifType, title, body string, read bool) *models.Notification {
	notif := &models.Notification{
		UserID: userID,
		Type:   notifType,
		Title:  title,
		Body:   body,
		Read:   read,
	}
	return notif
}

// TestNewNotificationRepository verifies repository initialization.
func TestNewNotificationRepository(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	assert.NotNil(t, repo)
}

// TestNotificationRepository_Create_Success verifies notification creation.
func TestNotificationRepository_Create_Success(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test@example.com")
	notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Deposit Successful", "Your deposit of ₦5,000 was successful", false)

	err := repo.Create(ctx, notif)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, notif.ID)
	assert.False(t, notif.Read)
}

// TestNotificationRepository_Create_WithMetadata verifies notification creation with metadata.
func TestNotificationRepository_Create_WithMetadata(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test2@example.com")
	
	metadata := datatypes.JSON([]byte(`{"property_id":"123","amount_kobo":500000}`))
	notif := &models.Notification{
		UserID:   user.ID,
		Type:     models.NotificationInvestmentCreated,
		Title:    "Investment Created",
		Body:     "You invested ₦5,000 in Property XYZ",
		Read:     false,
		Metadata: metadata,
	}

	err := repo.Create(ctx, notif)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, notif.ID)
	assert.NotNil(t, notif.Metadata)
}

// TestNotificationRepository_FindByID_Success verifies finding notification by ID.
func TestNotificationRepository_FindByID_Success(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test3@example.com")
	notif := createTestNotification(user.ID, models.NotificationWelcome, "Welcome!", "Welcome to PropVest", false)
	require.NoError(t, repo.Create(ctx, notif))

	found, err := repo.FindByID(ctx, notif.ID)
	require.NoError(t, err)
	assert.Equal(t, notif.ID, found.ID)
	assert.Equal(t, notif.UserID, found.UserID)
	assert.Equal(t, notif.Title, found.Title)
}

// TestNotificationRepository_FindByID_NotFound verifies error when notification doesn't exist.
func TestNotificationRepository_FindByID_NotFound(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	_, err := repo.FindByID(ctx, uuid.New())
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestNotificationRepository_FindByIDAndUserID_Success verifies ownership enforcement.
func TestNotificationRepository_FindByIDAndUserID_Success(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test4@example.com")
	notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
	require.NoError(t, repo.Create(ctx, notif))

	found, err := repo.FindByIDAndUserID(ctx, notif.ID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, notif.ID, found.ID)
	assert.Equal(t, user.ID, found.UserID)
}

// TestNotificationRepository_FindByIDAndUserID_WrongUser verifies ownership enforcement rejects wrong user.
func TestNotificationRepository_FindByIDAndUserID_WrongUser(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user1 := createTestNotificationUser(db, "user1@example.com")
	user2 := createTestNotificationUser(db, "user2@example.com")
	notif := createTestNotification(user1.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
	require.NoError(t, repo.Create(ctx, notif))

	// Try to fetch user1's notification as user2
	_, err := repo.FindByIDAndUserID(ctx, notif.ID, user2.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestNotificationRepository_ListByUser_Success verifies listing notifications.
func TestNotificationRepository_ListByUser_Success(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test5@example.com")

	// Create 5 notifications
	for i := 0; i < 5; i++ {
		notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
		require.NoError(t, repo.Create(ctx, notif))
	}

	// List all notifications (limit 10, offset 0)
	notifications, total, err := repo.ListByUser(ctx, user.ID, false, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, notifications, 5)
}

// TestNotificationRepository_ListByUser_Pagination verifies pagination.
func TestNotificationRepository_ListByUser_Pagination(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test6@example.com")

	// Create 10 notifications
	for i := 0; i < 10; i++ {
		notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
		require.NoError(t, repo.Create(ctx, notif))
	}

	// Page 1: limit 3, offset 0
	page1, total, err := repo.ListByUser(ctx, user.ID, false, 3, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(10), total)
	assert.Len(t, page1, 3)

	// Page 2: limit 3, offset 3
	page2, total, err := repo.ListByUser(ctx, user.ID, false, 3, 3)
	require.NoError(t, err)
	assert.Equal(t, int64(10), total)
	assert.Len(t, page2, 3)

	// Ensure different notifications
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}

// TestNotificationRepository_ListByUser_UnreadOnly verifies unread filter.
func TestNotificationRepository_ListByUser_UnreadOnly(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test7@example.com")

	// Create 3 unread and 2 read notifications
	for i := 0; i < 3; i++ {
		notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Unread", "Success", false)
		require.NoError(t, repo.Create(ctx, notif))
	}
	for i := 0; i < 2; i++ {
		notif := createTestNotification(user.ID, models.NotificationWithdrawalSuccess, "Read", "Success", true)
		require.NoError(t, repo.Create(ctx, notif))
	}

	// List unread only
	unreadNotifs, total, err := repo.ListByUser(ctx, user.ID, true, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, unreadNotifs, 3)
	for _, n := range unreadNotifs {
		assert.False(t, n.Read)
	}

	// List all
	allNotifs, total, err := repo.ListByUser(ctx, user.ID, false, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, allNotifs, 5)
}

// TestNotificationRepository_CountUnread verifies unread count.
func TestNotificationRepository_CountUnread(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test8@example.com")

	// Create 3 unread and 2 read notifications
	for i := 0; i < 3; i++ {
		notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Unread", "Success", false)
		require.NoError(t, repo.Create(ctx, notif))
	}
	for i := 0; i < 2; i++ {
		notif := createTestNotification(user.ID, models.NotificationWithdrawalSuccess, "Read", "Success", true)
		require.NoError(t, repo.Create(ctx, notif))
	}

	count, err := repo.CountUnread(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

// TestNotificationRepository_MarkRead_Success verifies marking notification as read.
func TestNotificationRepository_MarkRead_Success(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test9@example.com")
	notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
	require.NoError(t, repo.Create(ctx, notif))

	// Mark as read
	err := repo.MarkRead(ctx, notif.ID, user.ID)
	require.NoError(t, err)

	// Verify marked as read
	found, err := repo.FindByID(ctx, notif.ID)
	require.NoError(t, err)
	assert.True(t, found.Read)
	assert.NotNil(t, found.ReadAt)
}

// TestNotificationRepository_MarkRead_WrongUser verifies ownership enforcement.
func TestNotificationRepository_MarkRead_WrongUser(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user1 := createTestNotificationUser(db, "user1b@example.com")
	user2 := createTestNotificationUser(db, "user2b@example.com")
	notif := createTestNotification(user1.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
	require.NoError(t, repo.Create(ctx, notif))

	// Try to mark user1's notification as read by user2
	err := repo.MarkRead(ctx, notif.ID, user2.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Verify still unread
	found, err := repo.FindByID(ctx, notif.ID)
	require.NoError(t, err)
	assert.False(t, found.Read)
}

// TestNotificationRepository_MarkAllRead verifies batch mark as read.
func TestNotificationRepository_MarkAllRead(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test10@example.com")

	// Create 5 unread notifications
	for i := 0; i < 5; i++ {
		notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Unread", "Success", false)
		require.NoError(t, repo.Create(ctx, notif))
	}

	// Verify 5 unread
	count, err := repo.CountUnread(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	// Mark all as read
	err = repo.MarkAllRead(ctx, user.ID)
	require.NoError(t, err)

	// Verify 0 unread
	count, err = repo.CountUnread(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// TestNotificationRepository_Delete_Success verifies notification deletion.
func TestNotificationRepository_Delete_Success(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user := createTestNotificationUser(db, "test11@example.com")
	notif := createTestNotification(user.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
	require.NoError(t, repo.Create(ctx, notif))

	// Delete notification
	err := repo.Delete(ctx, notif.ID, user.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = repo.FindByID(ctx, notif.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestNotificationRepository_Delete_WrongUser verifies ownership enforcement.
func TestNotificationRepository_Delete_WrongUser(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user1 := createTestNotificationUser(db, "user1c@example.com")
	user2 := createTestNotificationUser(db, "user2c@example.com")
	notif := createTestNotification(user1.ID, models.NotificationDepositSuccess, "Deposit", "Success", false)
	require.NoError(t, repo.Create(ctx, notif))

	// Try to delete user1's notification as user2
	err := repo.Delete(ctx, notif.ID, user2.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Verify still exists
	found, err := repo.FindByID(ctx, notif.ID)
	require.NoError(t, err)
	assert.Equal(t, notif.ID, found.ID)
}

// TestNotificationRepository_MultiUser_Isolation verifies user isolation.
func TestNotificationRepository_MultiUser_Isolation(t *testing.T) {
	db := setupNotificationTestDB(t)
	repo := NewNotificationRepository(db)
	ctx := context.Background()

	user1 := createTestNotificationUser(db, "user1d@example.com")
	user2 := createTestNotificationUser(db, "user2d@example.com")

	// User 1 has 3 notifications
	for i := 0; i < 3; i++ {
		notif := createTestNotification(user1.ID, models.NotificationDepositSuccess, "User1", "Success", false)
		require.NoError(t, repo.Create(ctx, notif))
	}

	// User 2 has 2 notifications
	for i := 0; i < 2; i++ {
		notif := createTestNotification(user2.ID, models.NotificationWithdrawalSuccess, "User2", "Success", false)
		require.NoError(t, repo.Create(ctx, notif))
	}

	// User 1 sees only their 3 notifications
	user1Notifs, total, err := repo.ListByUser(ctx, user1.ID, false, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, user1Notifs, 3)
	for _, n := range user1Notifs {
		assert.Equal(t, user1.ID, n.UserID)
	}

	// User 2 sees only their 2 notifications
	user2Notifs, total, err := repo.ListByUser(ctx, user2.ID, false, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, user2Notifs, 2)
	for _, n := range user2Notifs {
		assert.Equal(t, user2.ID, n.UserID)
	}
}
