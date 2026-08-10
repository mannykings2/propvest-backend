package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/dto"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/payments"
	"github.com/mannykings2/propvest-backend/internal/queue"
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// WalletService implements Milestone 3: wallet retrieval, deposits (via a
// payment provider + idempotent webhook credit), withdrawals (recorded then
// processed asynchronously by the worker), and transaction history.
//
// FINANCIAL SAFETY RULES enforced here (docs 2.3):
//   - Money is int64 kobo; balance can never go negative (guarded + DB CHECK).
//   - Every balance change writes exactly one ledger row (balance_before/after).
//   - Wallet credit happens ONLY after a payment is verified, inside a DB
//     transaction that locks the wallet row (SELECT ... FOR UPDATE).
//   - Deposits/webhooks are idempotent: a duplicate provider reference credits
//     the wallet at most once.
type WalletService interface {
	GetWallet(ctx context.Context, userID uuid.UUID) (*dto.WalletResponse, error)
	InitiateDeposit(ctx context.Context, userID uuid.UUID, amountKobo int64, idempotencyKey string) (*dto.DepositResponse, error)
	// CreditFromPaymentReference verifies + credits a deposit. Called by the
	// webhook handler (and callback). Idempotent by design.
	CreditFromPaymentReference(ctx context.Context, reference string, rawPayload []byte) error
	InitiateWithdrawal(ctx context.Context, userID uuid.UUID, req dto.WithdrawRequest) (*dto.TransactionResponse, error)
	GetTransactionHistory(ctx context.Context, userID uuid.UUID, txType, status string, page, limit int) ([]dto.TransactionResponse, int64, error)
	// VerifyWebhookSignature exposes the provider's signature check to the handler.
	VerifyWebhookSignature(signature string, body []byte) bool
}

type walletService struct {
	walletRepo   repositories.WalletRepository
	userRepo     repositories.UserRepository
	paymentRepo  repositories.PaymentRepository
	provider     payments.Provider
	notifier     NotificationService
	mq           *queue.Client
	cfg          *config.Config
	db           *gorm.DB
}

// NewWalletService constructs the wallet service with all its collaborators.
func NewWalletService(
	walletRepo repositories.WalletRepository,
	userRepo repositories.UserRepository,
	paymentRepo repositories.PaymentRepository,
	provider payments.Provider,
	notifier NotificationService,
	mq *queue.Client,
	cfg *config.Config,
	db *gorm.DB,
) WalletService {
	return &walletService{
		walletRepo:  walletRepo,
		userRepo:    userRepo,
		paymentRepo: paymentRepo,
		provider:    provider,
		notifier:    notifier,
		mq:          mq,
		cfg:         cfg,
		db:          db,
	}
}

// GetWallet returns the authenticated user's wallet.
func (s *walletService) GetWallet(ctx context.Context, userID uuid.UUID) (*dto.WalletResponse, error) {
	w, err := s.walletRepo.FindByUserID(ctx, userID)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return nil, apperrors.ErrWalletNotFound
		}
		return nil, apperrors.ErrInternalServer
	}
	return walletToResponse(w), nil
}

// InitiateDeposit creates a pending payment and asks the provider for a hosted
// payment URL. It does NOT touch the wallet balance — that only happens after
// the payment is verified (webhook/callback).
func (s *walletService) InitiateDeposit(ctx context.Context, userID uuid.UUID, amountKobo int64, idempotencyKey string) (*dto.DepositResponse, error) {
	if amountKobo < s.cfg.MinDepositAmount {
		return nil, apperrors.ErrMinimumDeposit
	}
	if amountKobo > s.cfg.MaxDepositAmount {
		return nil, apperrors.ErrMaximumDeposit
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, apperrors.ErrUserNotFound
	}

	// Our internal reference doubles as the idempotency anchor for the payment.
	reference := "DEP-" + strings.ToUpper(uuid.NewString()[:12])

	// Record a pending payment BEFORE calling the provider, so a webhook that
	// races back can always find the row to reconcile against.
	payment := &models.Payment{
		UserID:     userID,
		Provider:   s.provider.Name(),
		Reference:  reference,
		AmountKobo: amountKobo,
		Currency:   "NGN",
		Status:     models.PaymentStatusPending,
		Channel:    models.PaymentChannelDeposit,
	}
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		logger.FromContext(ctx).Error("failed to create pending payment", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	init, err := s.provider.InitializeDeposit(ctx, user.Email, amountKobo, reference)
	if err != nil {
		logger.FromContext(ctx).Error("provider initialize failed", "reference", reference, "error", err)
		return nil, apperrors.WrapError(apperrors.ErrExternal, "could not start payment", "payment_init_failed")
	}

	return &dto.DepositResponse{
		AuthorizationURL: init.AuthorizationURL,
		Reference:        reference,
	}, nil
}

// CreditFromPaymentReference is the idempotent credit path. It is safe to call
// multiple times for the same reference (duplicate webhook deliveries).
func (s *walletService) CreditFromPaymentReference(ctx context.Context, reference string, rawPayload []byte) error {
	log := logger.FromContext(ctx)

	// 1. Find the pending payment we created at initiate time.
	payment, err := s.paymentRepo.FindByReference(ctx, reference)
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			log.Warn("webhook for unknown payment reference", "reference", reference)
			return nil // ack: nothing we can do, don't requeue forever
		}
		return apperrors.ErrInternalServer
	}

	// 2. IDEMPOTENCY: if already successful, do nothing.
	if payment.Status == models.PaymentStatusSuccess {
		return nil
	}

	// 3. Server-side verify with the provider (never trust the webhook alone).
	verify, err := s.provider.VerifyTransaction(ctx, reference)
	if err != nil {
		log.Error("provider verify failed", "reference", reference, "error", err)
		return apperrors.ErrExternal // requeue: transient provider issue
	}
	if verify.Status != "success" {
		_ = s.paymentRepo.MarkFailed(ctx, reference, datatypes.JSON(rawPayload))
		return nil
	}

	// 4. Atomically credit the wallet + write the ledger row + mark payment done.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Idempotency backstop at the ledger level: unique reference.
		exists, terr := s.walletRepo.TransactionExists(ctx, reference)
		if terr != nil {
			return terr
		}
		if exists {
			return nil // already credited in a previous delivery
		}

		wallet, werr := s.walletRepo.FindByUserIDForUpdate(ctx, payment.UserID, tx)
		if werr != nil {
			return werr
		}

		before := wallet.MainBalance
		after := before + payment.AmountKobo

		if uerr := tx.Model(&models.Wallet{}).
			Where("id = ?", wallet.ID).
			Update("main_balance", after).Error; uerr != nil {
			return uerr
		}

		ledger := &models.WalletTransaction{
			WalletID:          wallet.ID,
			UserID:            payment.UserID,
			Type:              "deposit",
			Amount:            payment.AmountKobo,
			BalanceBefore:     before,
			BalanceAfter:      after,
			Reference:         reference,
			ExternalReference: &reference,
			Description:       "Wallet deposit",
			Status:            "completed",
			Metadata:          datatypes.JSON(rawPayload),
		}
		return tx.Create(ledger).Error
	})
	if err != nil {
		log.Error("failed to credit wallet in transaction", "reference", reference, "error", err)
		return apperrors.ErrInternalServer
	}

	_ = s.paymentRepo.MarkSuccess(ctx, reference, verify.Reference, datatypes.JSON(rawPayload))

	// 5. Side effects (non-critical): notify the user + queue a receipt email.
	if user, uerr := s.userRepo.FindByID(ctx, payment.UserID); uerr == nil {
		s.notifier.Notify(ctx, payment.UserID, models.NotificationDepositSuccess,
			"Deposit successful",
			fmt.Sprintf("Your wallet was credited with ₦%s.", formatKobo(payment.AmountKobo)),
			map[string]any{"reference": reference, "amount_kobo": payment.AmountKobo})

		if s.mq != nil {
			_ = s.mq.Publish(ctx, queue.QueueDepositReceipt, queue.DepositReceiptMessage{
				UserID:     payment.UserID.String(),
				Email:      user.Email,
				AmountKobo: payment.AmountKobo,
				Reference:  reference,
			})
		}
	}

	return nil
}

// InitiateWithdrawal validates + debits the wallet immediately (so the funds are
// reserved) and records a PENDING withdrawal ledger row, then enqueues the
// actual payout for the worker to process asynchronously.
func (s *walletService) InitiateWithdrawal(ctx context.Context, userID uuid.UUID, req dto.WithdrawRequest) (*dto.TransactionResponse, error) {
	if req.Amount <= 0 {
		return nil, apperrors.ErrInvalidAmount
	}

	reference := "WD-" + strings.ToUpper(uuid.NewString()[:12])
	var ledger *models.WalletTransaction

	err := s.db.Transaction(func(tx *gorm.DB) error {
		wallet, werr := s.walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
		if werr != nil {
			return werr
		}

		if wallet.MainBalance < req.Amount {
			return apperrors.ErrInsufficientFunds
		}

		before := wallet.MainBalance
		after := before - req.Amount

		if uerr := tx.Model(&models.Wallet{}).
			Where("id = ?", wallet.ID).
			Update("main_balance", after).Error; uerr != nil {
			return uerr
		}

		meta, _ := json.Marshal(map[string]any{
			"bank_code":      req.BankCode,
			"account_number": req.AccountNumber,
			"account_name":   req.AccountName,
		})
		ledger = &models.WalletTransaction{
			WalletID:      wallet.ID,
			UserID:        userID,
			Type:          "withdrawal",
			Amount:        req.Amount,
			BalanceBefore: before,
			BalanceAfter:  after,
			Reference:     reference,
			Description:   "Wallet withdrawal",
			Status:        "pending", // becomes completed/failed by the worker
			Metadata:      datatypes.JSON(meta),
		}
		return tx.Create(ledger).Error
	})
	if err != nil {
		if err == apperrors.ErrInsufficientFunds {
			return nil, apperrors.ErrInsufficientFunds
		}
		logger.FromContext(ctx).Error("withdrawal transaction failed", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	// Hand the payout to the worker (retryable). Disabled queue just logs.
	if s.mq != nil {
		_ = s.mq.Publish(ctx, queue.QueueWithdrawalProcess, queue.WithdrawalMessage{
			TransactionID: ledger.ID.String(),
			UserID:        userID.String(),
			AmountKobo:    req.Amount,
			Reference:     reference,
		})
	}

	s.notifier.Notify(ctx, userID, models.NotificationWithdrawalUpdate,
		"Withdrawal requested",
		fmt.Sprintf("Your withdrawal of ₦%s is being processed.", formatKobo(req.Amount)),
		map[string]any{"reference": reference})

	return txnToResponse(ledger), nil
}

// GetTransactionHistory returns a filtered, paginated ledger view.
func (s *walletService) GetTransactionHistory(ctx context.Context, userID uuid.UUID, txType, status string, page, limit int) ([]dto.TransactionResponse, int64, error) {
	offset := (page - 1) * limit
	rows, total, err := s.walletRepo.ListTransactions(ctx, userID, txType, status, limit, offset)
	if err != nil {
		return nil, 0, apperrors.ErrInternalServer
	}
	out := make([]dto.TransactionResponse, 0, len(rows))
	for i := range rows {
		out = append(out, *txnToResponse(&rows[i]))
	}
	return out, total, nil
}

// VerifyWebhookSignature delegates to the provider.
func (s *walletService) VerifyWebhookSignature(signature string, body []byte) bool {
	return s.provider.VerifyWebhookSignature(signature, body)
}

// ── mappers/helpers ─────────────────────────────────────────────────────────

func walletToResponse(w *models.Wallet) *dto.WalletResponse {
	return &dto.WalletResponse{
		ID:              w.ID,
		UserID:          w.UserID,
		MainBalance:     w.MainBalance,
		EarningsBalance: w.EarningsBalance,
		Currency:        w.Currency,
		VirtualAcctNo:   w.VirtualAcctNo,
		VirtualBank:     w.VirtualBank,
		CreatedAt:       w.CreatedAt,
	}
}

func txnToResponse(t *models.WalletTransaction) *dto.TransactionResponse {
	return &dto.TransactionResponse{
		ID:            t.ID,
		Type:          t.Type,
		Amount:        t.Amount,
		BalanceBefore: t.BalanceBefore,
		BalanceAfter:  t.BalanceAfter,
		Reference:     t.Reference,
		Description:   t.Description,
		Status:        t.Status,
		CreatedAt:     t.CreatedAt,
	}
}

// formatKobo renders kobo as a Naira amount string (e.g. 150000 -> "1,500.00").
// Presentation helper for notification copy only; the API returns raw kobo.
func formatKobo(kobo int64) string {
	naira := kobo / 100
	cents := kobo % 100
	// Simple thousands grouping.
	s := fmt.Sprintf("%d", naira)
	var grouped strings.Builder
	n := len(s)
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(c)
	}
	return fmt.Sprintf("%s.%02d", grouped.String(), cents)
}

var _ = time.Now
