package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WalletRepository defines the interface for wallet data access operations.
type WalletRepository interface {
	Create(ctx context.Context, wallet *models.Wallet) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Wallet, error)
	FindByUserID(ctx context.Context, userID uuid.UUID) (*models.Wallet, error)
	Update(ctx context.Context, wallet *models.Wallet) error
	UpdateBalance(ctx context.Context, walletID uuid.UUID, mainBalance, earningsBalance int64) error
	CreateTransaction(ctx context.Context, tx *models.WalletTransaction) error
	GetTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]models.WalletTransaction, error)
	GetTransactionByReference(ctx context.Context, reference string) (*models.WalletTransaction, error)

	// The following were previously implemented on the concrete type but were
	// unreachable because they weren't on the interface (handoff item P3). They
	// are exactly what the wallet/investment services need, so they are exposed
	// here now.

	// FindByUserIDForUpdate locks the wallet row (SELECT ... FOR UPDATE) inside a
	// transaction to serialize concurrent balance changes.
	FindByUserIDForUpdate(ctx context.Context, userID uuid.UUID, tx *gorm.DB) (*models.Wallet, error)
	// TransactionExists reports whether a ledger row with the reference exists
	// (idempotency check).
	TransactionExists(ctx context.Context, reference string) (bool, error)
	// ListTransactions returns filtered, paginated ledger rows for a user plus a
	// total count for pagination metadata.
	ListTransactions(ctx context.Context, userID uuid.UUID, txType, status string, limit, offset int) ([]models.WalletTransaction, int64, error)
}

type walletRepository struct {
	*BaseRepository
}

// NewWalletRepository creates a new wallet repository instance.
func NewWalletRepository(db *gorm.DB) WalletRepository {
	return &walletRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create inserts a new wallet into the database.
// Typically called during user registration within a transaction.
func (r *walletRepository) Create(ctx context.Context, wallet *models.Wallet) error {
	return r.WithContext(ctx).Create(wallet).Error
}

// FindByID retrieves a wallet by its UUID.
func (r *walletRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Wallet, error) {
	var wallet models.Wallet
	err := r.WithContext(ctx).Where("id = ?", id).First(&wallet).Error
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

// FindByUserID retrieves a user's wallet.
// Since the relationship is one-to-one, this should always return exactly one wallet
// for any valid user ID (assuming wallet creation happened during registration).
func (r *walletRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*models.Wallet, error) {
	var wallet models.Wallet
	err := r.WithContext(ctx).Where("user_id = ?", userID).First(&wallet).Error
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

// Update saves changes to an existing wallet.
// WARNING: For balance updates, prefer UpdateBalance() which uses optimistic locking.
func (r *walletRepository) Update(ctx context.Context, wallet *models.Wallet) error {
	return r.WithContext(ctx).Save(wallet).Error
}

// UpdateBalance updates wallet balances with row-level locking to prevent race conditions.
//
// This method uses SELECT FOR UPDATE to lock the wallet row during the transaction,
// preventing concurrent balance modifications that could lead to incorrect balances.
//
// CRITICAL: This must be called within a transaction.
//
// Example usage:
//   err := walletRepo.Transaction(ctx, func(tx *gorm.DB) error {
//       wallet, err := walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
//       if err != nil {
//           return err
//       }
//       newBalance := wallet.MainBalance - amount
//       if newBalance < 0 {
//           return ErrInsufficientFunds
//       }
//       return walletRepo.UpdateBalance(ctx, wallet.ID, newBalance, wallet.EarningsBalance)
//   })
func (r *walletRepository) UpdateBalance(ctx context.Context, walletID uuid.UUID, mainBalance, earningsBalance int64) error {
	result := r.WithContext(ctx).Model(&models.Wallet{}).
		Where("id = ?", walletID).
		Updates(map[string]interface{}{
			"main_balance":     mainBalance,
			"earnings_balance": earningsBalance,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("wallet not found: %s", walletID)
	}

	return nil
}

// FindByUserIDForUpdate retrieves a wallet with a row lock for atomic updates.
// This MUST be used within a transaction when you plan to modify the balance.
//
// The FOR UPDATE clause locks the row, preventing other transactions from reading
// or modifying it until this transaction commits or rolls back.
//
// Usage:
//   repo.Transaction(ctx, func(tx *gorm.DB) error {
//       wallet, err := repo.FindByUserIDForUpdate(ctx, userID, tx)
//       // Modify wallet
//       // Update wallet
//       return nil
//   })
func (r *walletRepository) FindByUserIDForUpdate(ctx context.Context, userID uuid.UUID, tx *gorm.DB) (*models.Wallet, error) {
	var wallet models.Wallet
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		First(&wallet).Error
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

// CreateTransaction inserts a new transaction record into the ledger.
// CRITICAL: Transaction records are IMMUTABLE — never update or delete them.
//
// Every balance change must create a corresponding transaction record.
// This is how we maintain an audit trail and can reconcile balances.
func (r *walletRepository) CreateTransaction(ctx context.Context, tx *models.WalletTransaction) error {
	return r.WithContext(ctx).Create(tx).Error
}

// GetTransactions retrieves transaction history for a wallet with pagination.
// Results are ordered by created_at DESC (most recent first).
func (r *walletRepository) GetTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]models.WalletTransaction, error) {
	var transactions []models.WalletTransaction
	err := r.WithContext(ctx).
		Where("wallet_id = ?", walletID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error
	if err != nil {
		return nil, err
	}
	return transactions, nil
}

// GetTransactionByReference retrieves a transaction by its unique reference.
// This is used for idempotency checks — if a transaction with this reference
// already exists, we know this payment has already been processed.
func (r *walletRepository) GetTransactionByReference(ctx context.Context, reference string) (*models.WalletTransaction, error) {
	var transaction models.WalletTransaction
	err := r.WithContext(ctx).Where("reference = ?", reference).First(&transaction).Error
	if err != nil {
		return nil, err
	}
	return &transaction, nil
}

// TransactionExists checks if a transaction with the given reference exists.
// Returns true if exists, false otherwise.
// This is more efficient than GetTransactionByReference when you only need existence.
func (r *walletRepository) TransactionExists(ctx context.Context, reference string) (bool, error) {
	var count int64
	err := r.WithContext(ctx).Model(&models.WalletTransaction{}).Where("reference = ?", reference).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListTransactions returns a filtered, paginated slice of a user's ledger rows
// (most recent first) plus the total matching count for pagination metadata.
// txType and status are optional filters (empty string = no filter).
func (r *walletRepository) ListTransactions(ctx context.Context, userID uuid.UUID, txType, status string, limit, offset int) ([]models.WalletTransaction, int64, error) {
	q := r.WithContext(ctx).Model(&models.WalletTransaction{}).Where("user_id = ?", userID)
	if txType != "" {
		q = q.Where("type = ?", txType)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var txns []models.WalletTransaction
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&txns).Error; err != nil {
		return nil, 0, err
	}
	return txns, total, nil
}
