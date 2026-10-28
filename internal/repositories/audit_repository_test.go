package repositories

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// TestAuditRepository_Create tests creating audit log entries.
func TestAuditRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuditRepository(db)
	ctx := context.Background()

	actorID := uuid.New()
	targetID := uuid.New()

	t.Run("creates audit log successfully", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		auditLog := &models.AuditLog{
			ActorID:     &actorID,
			ActorType:   models.AuditActorTypeAdmin,
			ActorRole:   "admin",
			Action:      models.AuditActionUserSuspended,
			Description: "Admin suspended user account",
			TargetType:  "user",
			TargetID:    &targetID,
			Reason:      "Policy violation",
			IPAddress:   "192.168.1.100",
			UserAgent:   "Mozilla/5.0",
			RequestID:   "req-123",
		}

		err := repo.Create(ctx, tx, auditLog)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, auditLog.ID)

		tx.Commit()

		// Verify audit log was created
		var saved models.AuditLog
		err = db.First(&saved, "id = ?", auditLog.ID).Error
		require.NoError(t, err)
		assert.Equal(t, models.AuditActorTypeAdmin, saved.ActorType)
		assert.Equal(t, "admin", saved.ActorRole)
		assert.Equal(t, models.AuditActionUserSuspended, saved.Action)
		assert.Equal(t, "Policy violation", saved.Reason)
	})

	t.Run("creates audit log with old/new values", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		oldValues := map[string]interface{}{"status": "active", "is_active": true}
		newValues := map[string]interface{}{"status": "suspended", "is_active": false}

		oldJSON, _ := json.Marshal(oldValues)
		newJSON, _ := json.Marshal(newValues)

		auditLog := &models.AuditLog{
			ActorID:    &actorID,
			ActorType:  models.AuditActorTypeAdmin,
			ActorRole:  "admin",
			Action:     models.AuditActionUserSuspended,
			TargetType: "user",
			TargetID:   &targetID,
			OldValues:  datatypes.JSON(oldJSON),
			NewValues:  datatypes.JSON(newJSON),
		}

		err := repo.Create(ctx, tx, auditLog)
		require.NoError(t, err)

		tx.Commit()

		// Verify snapshots saved
		var saved models.AuditLog
		err = db.First(&saved, "id = ?", auditLog.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, saved.OldValues)
		assert.NotNil(t, saved.NewValues)

		// Verify can parse snapshots
		var oldParsed, newParsed map[string]interface{}
		err = json.Unmarshal(saved.OldValues, &oldParsed)
		require.NoError(t, err)
		err = json.Unmarshal(saved.NewValues, &newParsed)
		require.NoError(t, err)

		assert.Equal(t, "active", oldParsed["status"])
		assert.Equal(t, "suspended", newParsed["status"])
	})

	t.Run("creates audit log with idempotency key", func(t *testing.T) {
		tx := db.Begin()
		defer tx.Rollback()

		idempotencyKey := "role-change-" + uuid.New().String()

		auditLog := &models.AuditLog{
			ActorID:        &actorID,
			ActorType:      models.AuditActorTypeAdmin,
			Action:         models.AuditActionUserRoleChanged,
			TargetType:     "user",
			TargetID:       &targetID,
			IdempotencyKey: &idempotencyKey,
		}

		err := repo.Create(ctx, tx, auditLog)
		require.NoError(t, err)

		tx.Commit()

		// Verify idempotency key saved
		var saved models.AuditLog
		err = db.First(&saved, "id = ?", auditLog.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, saved.IdempotencyKey)
		assert.Equal(t, idempotencyKey, *saved.IdempotencyKey)
	})

	t.Run("rollback prevents audit log creation", func(t *testing.T) {
		tx := db.Begin()

		auditLog := &models.AuditLog{
			ActorID:    &actorID,
			ActorType:  models.AuditActorTypeAdmin,
			Action:     "test.rollback",
			TargetType: "test",
		}

		err := repo.Create(ctx, tx, auditLog)
		require.NoError(t, err)

		// Rollback transaction
		tx.Rollback()

		// Verify audit log NOT saved
		var count int64
		err = db.Model(&models.AuditLog{}).Where("action = ?", "test.rollback").Count(&count).Error
		require.NoError(t, err)
		assert.Equal(t, int64(0), count, "Audit log should not exist after rollback")
	})
}

// TestAuditRepository_FindByID tests retrieving audit log by ID.
func TestAuditRepository_FindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuditRepository(db)
	ctx := context.Background()

	// Create test audit log
	actorID := uuid.New()
	auditLog := &models.AuditLog{
		ActorID:    &actorID,
		ActorType:  models.AuditActorTypeAdmin,
		Action:     "test.action",
		TargetType: "test",
	}
	err := db.Create(auditLog).Error
	require.NoError(t, err)

	t.Run("finds existing audit log", func(t *testing.T) {
		found, err := repo.FindByID(ctx, auditLog.ID)
		require.NoError(t, err)
		assert.Equal(t, auditLog.ID, found.ID)
		assert.Equal(t, "test.action", found.Action)
	})

	t.Run("returns error for non-existent ID", func(t *testing.T) {
		nonExistentID := uuid.New()
		_, err := repo.FindByID(ctx, nonExistentID)
		assert.Error(t, err)
	})
}

// TestAuditRepository_List tests paginated audit log queries.
func TestAuditRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuditRepository(db)
	ctx := context.Background()

	// Create test data
	adminID := uuid.New()
	userID := uuid.New()
	targetID := uuid.New()

	auditLogs := []models.AuditLog{
		{
			ActorID:    &adminID,
			ActorType:  models.AuditActorTypeAdmin,
			ActorRole:  "admin",
			Action:     models.AuditActionUserSuspended,
			TargetType: "user",
			TargetID:   &targetID,
		},
		{
			ActorID:    &adminID,
			ActorType:  models.AuditActorTypeAdmin,
			ActorRole:  "admin",
			Action:     models.AuditActionPropertyApproved,
			TargetType: "property",
		},
		{
			ActorID:    &userID,
			ActorType:  models.AuditActorTypeUser,
			Action:     "user.login",
			TargetType: "user",
		},
	}

	for i := range auditLogs {
		err := db.Create(&auditLogs[i]).Error
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	t.Run("lists all audit logs with pagination", func(t *testing.T) {
		logs, total, err := repo.List(ctx, AuditLogFilter{}, 1, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, int(total), 3)
		assert.GreaterOrEqual(t, len(logs), 3)
	})

	t.Run("filters by actor ID", func(t *testing.T) {
		filter := AuditLogFilter{ActorID: &adminID}
		logs, total, err := repo.List(ctx, filter, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, logs, 2)
		for _, log := range logs {
			assert.Equal(t, adminID, *log.ActorID)
		}
	})

	t.Run("filters by actor type", func(t *testing.T) {
		actorType := models.AuditActorTypeAdmin
		filter := AuditLogFilter{ActorType: &actorType}
		logs, total, err := repo.List(ctx, filter, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, logs, 2)
		for _, log := range logs {
			assert.Equal(t, models.AuditActorTypeAdmin, log.ActorType)
		}
	})

	t.Run("filters by action", func(t *testing.T) {
		action := models.AuditActionUserSuspended
		filter := AuditLogFilter{Action: &action}
		logs, total, err := repo.List(ctx, filter, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, logs, 1)
		assert.Equal(t, models.AuditActionUserSuspended, logs[0].Action)
	})

	t.Run("filters by target type", func(t *testing.T) {
		targetType := "property"
		filter := AuditLogFilter{TargetType: &targetType}
		logs, total, err := repo.List(ctx, filter, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, logs, 1)
		assert.Equal(t, "property", logs[0].TargetType)
	})

	t.Run("filters by target ID", func(t *testing.T) {
		filter := AuditLogFilter{TargetID: &targetID}
		logs, total, err := repo.List(ctx, filter, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, logs, 1)
		assert.Equal(t, targetID, *logs[0].TargetID)
	})

	t.Run("filters by date range", func(t *testing.T) {
		now := time.Now()
		hourAgo := now.Add(-1 * time.Hour)
		filter := AuditLogFilter{CreatedFrom: &hourAgo}
		logs, total, err := repo.List(ctx, filter, 1, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, int(total), 3)
		for _, log := range logs {
			assert.True(t, log.CreatedAt.After(hourAgo) || log.CreatedAt.Equal(hourAgo))
		}
	})

	t.Run("stable sort by created_at DESC, id DESC", func(t *testing.T) {
		logs, _, err := repo.List(ctx, AuditLogFilter{}, 1, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 2)

		// Verify descending order
		for i := 1; i < len(logs); i++ {
			assert.True(t,
				logs[i-1].CreatedAt.After(logs[i].CreatedAt) ||
					(logs[i-1].CreatedAt.Equal(logs[i].CreatedAt) && logs[i-1].ID.String() > logs[i].ID.String()),
				"Logs should be sorted by created_at DESC, id DESC")
		}
	})

	t.Run("pagination works correctly", func(t *testing.T) {
		// Page 1
		page1Logs, total, err := repo.List(ctx, AuditLogFilter{}, 1, 2)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, int(total), 3)
		assert.Len(t, page1Logs, 2)

		// Page 2
		page2Logs, total2, err := repo.List(ctx, AuditLogFilter{}, 2, 2)
		require.NoError(t, err)
		assert.Equal(t, total, total2)
		assert.GreaterOrEqual(t, len(page2Logs), 1)

		// Verify no overlap
		assert.NotEqual(t, page1Logs[0].ID, page2Logs[0].ID)
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		nonExistentID := uuid.New()
		filter := AuditLogFilter{ActorID: &nonExistentID}
		logs, total, err := repo.List(ctx, filter, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Len(t, logs, 0)
	})
}

// TestAuditRepository_IdempotencyConstraint tests unique idempotency key constraint.
func TestAuditRepository_IdempotencyConstraint(t *testing.T) {
	db := setupTestDB(t)
	repo := NewAuditRepository(db)
	ctx := context.Background()

	actorID := uuid.New()
	idempotencyKey := "test-idempotency-" + uuid.New().String()

	t.Run("prevents duplicate idempotency key for same actor", func(t *testing.T) {
		// Create first audit log
		tx1 := db.Begin()
		auditLog1 := &models.AuditLog{
			ActorID:        &actorID,
			ActorType:      models.AuditActorTypeAdmin,
			Action:         "test.idempotent",
			TargetType:     "test",
			IdempotencyKey: &idempotencyKey,
		}
		err := repo.Create(ctx, tx1, auditLog1)
		require.NoError(t, err)
		tx1.Commit()

		// Try to create second audit log with same actor + idempotency key
		tx2 := db.Begin()
		defer tx2.Rollback()
		auditLog2 := &models.AuditLog{
			ActorID:        &actorID,
			ActorType:      models.AuditActorTypeAdmin,
			Action:         "test.idempotent",
			TargetType:     "test",
			IdempotencyKey: &idempotencyKey,
		}
		err = repo.Create(ctx, tx2, auditLog2)
		assert.Error(t, err, "Should fail due to unique constraint violation")
	})

	t.Run("allows same idempotency key for different actors", func(t *testing.T) {
		differentActorID := uuid.New()
		sharedKey := "shared-idempotency-" + uuid.New().String()

		// Actor 1
		tx1 := db.Begin()
		auditLog1 := &models.AuditLog{
			ActorID:        &actorID,
			ActorType:      models.AuditActorTypeAdmin,
			Action:         "test.shared_key",
			TargetType:     "test",
			IdempotencyKey: &sharedKey,
		}
		err := repo.Create(ctx, tx1, auditLog1)
		require.NoError(t, err)
		tx1.Commit()

		// Actor 2 with same key (should succeed)
		tx2 := db.Begin()
		defer tx2.Rollback()
		auditLog2 := &models.AuditLog{
			ActorID:        &differentActorID,
			ActorType:      models.AuditActorTypeAdmin,
			Action:         "test.shared_key",
			TargetType:     "test",
			IdempotencyKey: &sharedKey,
		}
		err = repo.Create(ctx, tx2, auditLog2)
		assert.NoError(t, err, "Different actors can use same idempotency key")
		tx2.Commit()
	})
}
