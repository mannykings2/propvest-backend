package repositories

import (
	"context"

	"gorm.io/gorm"
)

// BaseRepository provides common database operations that all repositories can use.
// This eliminates code duplication across repository implementations.
type BaseRepository struct {
	db *gorm.DB
}

// NewBaseRepository creates a new base repository instance.
func NewBaseRepository(db *gorm.DB) *BaseRepository {
	return &BaseRepository{db: db}
}

// WithContext returns a new GORM DB instance with the given context.
// This allows request cancellation to propagate to database queries.
//
// Usage:
//   db := repo.WithContext(ctx)
//   db.Where("id = ?", id).First(&user)
func (r *BaseRepository) WithContext(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// Transaction executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, it is committed.
//
// This is CRITICAL for financial operations where multiple tables must be
// updated atomically (e.g., debit wallet + create investment + update property).
//
// Usage:
//   err := repo.Transaction(ctx, func(tx *gorm.DB) error {
//       // All operations here run in a transaction
//       if err := tx.Create(&wallet).Error; err != nil {
//           return err // Causes rollback
//       }
//       if err := tx.Create(&transaction).Error; err != nil {
//           return err // Causes rollback
//       }
//       return nil // Causes commit
//   })
func (r *BaseRepository) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// GetDB returns the underlying GORM DB instance.
// Use this sparingly — most operations should go through repository methods.
func (r *BaseRepository) GetDB() *gorm.DB {
	return r.db
}
