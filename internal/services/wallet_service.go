package services

import (
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// WalletService handles all wallet-related business logic.
// This will be fully implemented in Milestone 3.
type WalletService interface {
	// GetWallet retrieves a user's wallet
	// TODO: Implement in Milestone 3
	// GetWallet(ctx context.Context, userID uuid.UUID) (*dto.WalletResponse, error)

	// InitiateDeposit starts the deposit flow with payment provider
	// TODO: Implement in Milestone 3
	// InitiateDeposit(ctx context.Context, userID uuid.UUID, amount int64) (*dto.DepositResponse, error)

	// ProcessDepositWebhook handles payment provider callbacks
	// TODO: Implement in Milestone 3
	// ProcessDepositWebhook(ctx context.Context, payload dto.WebhookPayload) error

	// InitiateWithdrawal processes withdrawal requests
	// TODO: Implement in Milestone 3
	// InitiateWithdrawal(ctx context.Context, userID uuid.UUID, amount int64) error

	// GetTransactionHistory retrieves paginated transaction history
	// TODO: Implement in Milestone 3
	// GetTransactionHistory(ctx context.Context, userID uuid.UUID, page, limit int) (*dto.TransactionHistoryResponse, error)
}

type walletService struct {
	walletRepo repositories.WalletRepository
	userRepo   repositories.UserRepository
}

// NewWalletService creates a new wallet service instance.
func NewWalletService(
	walletRepo repositories.WalletRepository,
	userRepo repositories.UserRepository,
) WalletService {
	return &walletService{
		walletRepo: walletRepo,
		userRepo:   userRepo,
	}
}

// Milestone 3 will implement:
//   - GetWallet: retrieve wallet by user ID
//   - InitiateDeposit: integrate Paystack, generate payment link
//   - ProcessDepositWebhook: verify signature, credit wallet, create transaction
//   - InitiateWithdrawal: validate balance, process withdrawal
//   - GetTransactionHistory: retrieve and paginate transactions
//
// Financial Operations Pattern:
//   1. Start database transaction
//   2. Lock wallet row (SELECT FOR UPDATE)
//   3. Validate business rules (e.g., sufficient balance)
//   4. Update wallet balance
//   5. Create transaction record
//   6. Commit transaction
//   7. Publish event (optional)
