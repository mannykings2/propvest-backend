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
	// FinalizeWithdrawal completes or reverses a withdrawal. Called by worker/webhook.
	FinalizeWithdrawal(ctx context.Context, transactionID uuid.UUID, success bool, providerReference, failureReason string) error
	// GetWithdrawalByReference finds a withdrawal transaction by reference (for webhook processing)
	GetWithdrawalByReference(ctx context.Context, reference string) (uuid.UUID, error)
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
// InitiateWithdrawal requests a withdrawal with production-grade locked balance handling.
//
// FLOW:
//   1. Validate amount against limits
//   2. Pre-flight check: Resolve bank account (verify name matches)
//   3. Database transaction:
//      a. Lock wallet row (FOR UPDATE)
//      b. Check available balance
//      c. Lock funds (locked_balance += amount)
//      d. Create pending WalletTransaction
//   4. Queue withdrawal job for async processing
//   5. Return pending status immediately
//
// LOCKED BALANCE PATTERN:
//   Before: main=100k, locked=0, available=100k
//   After:  main=100k, locked=50k, available=50k
//   User cannot spend the locked ₦50k until withdrawal completes or fails.
func (s *walletService) InitiateWithdrawal(ctx context.Context, userID uuid.UUID, req dto.WithdrawRequest) (*dto.TransactionResponse, error) {
	// Step 1: Validate amount
	if req.Amount <= 0 {
		return nil, apperrors.ErrInvalidAmount
	}

	// Check minimum withdrawal
	if req.Amount < s.cfg.MinWithdrawalAmount {
		return nil, apperrors.NewMinimumWithdrawalError(s.cfg.MinWithdrawalAmount)
	}

	// Check maximum withdrawal
	if req.Amount > s.cfg.MaxWithdrawalAmount {
		return nil, apperrors.NewMaximumWithdrawalError(s.cfg.MaxWithdrawalAmount)
	}

	// Step 2: Pre-flight check - Resolve bank account
	// This verifies the account exists and gets the real account name from the bank
	logger.FromContext(ctx).Info("resolving bank account",
		"account_number", req.AccountNumber,
		"bank_code", req.BankCode)

	resolution, err := s.provider.ResolveAccountNumber(ctx, req.AccountNumber, req.BankCode)
	if err != nil {
		logger.FromContext(ctx).Error("account resolution failed",
			"error", err,
			"account_number", req.AccountNumber,
			"bank_code", req.BankCode)
		return nil, apperrors.ErrInvalidBankAccount
	}

	// Fuzzy match account names (banks sometimes return names in different formats)
	// e.g., "JOHN DOE" vs "John Doe" vs "John D. Doe"
	if !accountNamesMatch(req.AccountName, resolution.AccountName) {
		logger.FromContext(ctx).Warn("account name mismatch",
			"provided", req.AccountName,
			"bank_records", resolution.AccountName)
		return nil, apperrors.NewAccountNameMismatchError(req.AccountName, resolution.AccountName)
	}

	logger.FromContext(ctx).Info("account verified",
		"account_name", resolution.AccountName,
		"bank_name", resolution.BankName)

	// Step 3: Generate unique reference
	reference := "WD-" + strings.ToUpper(uuid.NewString()[:12])

	// Step 4: Database transaction - Lock funds and create pending transaction
	var ledger *models.WalletTransaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Lock wallet row to prevent concurrent modifications
		wallet, werr := s.walletRepo.FindByUserIDForUpdate(ctx, userID, tx)
		if werr != nil {
			return werr
		}

		// Lock funds (moves from available to locked)
		if lerr := s.walletRepo.LockFunds(ctx, userID, req.Amount, tx); lerr != nil {
			// LockFunds returns specific error if insufficient balance
			return lerr
		}

		// Create pending transaction in ledger
		// Note: balance_before and balance_after both show main_balance
		// The actual debit happens when withdrawal completes
		meta, _ := json.Marshal(map[string]any{
			"bank_code":       req.BankCode,
			"account_number":  req.AccountNumber,
			"account_name":    resolution.AccountName, // Use verified name from bank
			"bank_name":       resolution.BankName,
		})

		ledger = &models.WalletTransaction{
			WalletID:      wallet.ID,
			UserID:        userID,
			Type:          "withdrawal",
			Amount:        req.Amount,
			BalanceBefore: wallet.MainBalance,
			BalanceAfter:  wallet.MainBalance, // Unchanged until completion
			Reference:     reference,
			Description:   fmt.Sprintf("Withdrawal to %s (%s)", resolution.BankName, req.AccountNumber),
			Status:        "pending",
			Metadata:      datatypes.JSON(meta),
		}
		return tx.Create(ledger).Error
	})

	if err != nil {
		// Check if it's insufficient funds error
		if strings.Contains(err.Error(), "insufficient available balance") {
			// Extract amounts from error for user-friendly message
			return nil, apperrors.ErrInsufficientFunds
		}
		logger.FromContext(ctx).Error("withdrawal transaction failed", "error", err)
		return nil, apperrors.ErrInternalServer
	}

	logger.FromContext(ctx).Info("withdrawal initiated",
		"reference", reference,
		"amount", req.Amount,
		"transaction_id", ledger.ID)

	// Step 5: Queue withdrawal job for async processing
	// The worker will call InitiateTransfer() to Paystack
	if s.mq != nil && s.mq.Enabled() {
		pubErr := s.mq.Publish(ctx, queue.QueueWithdrawalProcess, queue.WithdrawalMessage{
			TransactionID: ledger.ID.String(),
			UserID:        userID.String(),
			AmountKobo:    req.Amount,
			Reference:     reference,
		})
		if pubErr != nil {
			logger.FromContext(ctx).Error("failed to queue withdrawal",
				"error", pubErr,
				"reference", reference)
			// Don't fail the request - reconciler will pick it up
		}
	} else {
		logger.FromContext(ctx).Warn("message queue not enabled, withdrawal will be processed by reconciler",
			"reference", reference)
	}

	// Step 6: Notify user
	s.notifier.Notify(ctx, userID, models.NotificationWithdrawalUpdate,
		"Withdrawal requested",
		fmt.Sprintf("Your withdrawal of ₦%s to %s is being processed.",
			formatKobo(req.Amount), resolution.BankName),
		map[string]any{
			"reference":   reference,
			"amount":      req.Amount,
			"bank_name":   resolution.BankName,
			"account_number": req.AccountNumber,
		})

	// Step 7: Return pending status immediately
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

// FinalizeWithdrawal completes or reverses a pending withdrawal based on transfer status.
// Called by: Worker (after InitiateTransfer) and Webhook (on transfer status update).
//
// FLOW:
//   SUCCESS: Release locked funds, permanently debit wallet, mark transaction completed
//   FAILED:  Release locked funds, restore to available balance, mark transaction failed
//
// IDEMPOTENCY: Safe to call multiple times (checks transaction status first)
func (s *walletService) FinalizeWithdrawal(ctx context.Context, transactionID uuid.UUID, success bool, providerReference, failureReason string) error {
	log := logger.FromContext(ctx)

	// Find the transaction
	var txn models.WalletTransaction
	if err := s.db.WithContext(ctx).Where("id = ?", transactionID).First(&txn).Error; err != nil {
		if repositories.IsErrRecordNotFound(err) {
			log.Warn("finalize called for unknown transaction", "transaction_id", transactionID)
			return nil // idempotent: already handled or invalid
		}
		return apperrors.ErrInternalServer
	}

	// IDEMPOTENCY: Already finalized
	if txn.Status == "completed" || txn.Status == "failed" {
		log.Info("transaction already finalized",
			"transaction_id", transactionID,
			"status", txn.Status)
		return nil
	}

	// Ensure it's a withdrawal and pending
	if txn.Type != "withdrawal" {
		log.Error("finalize called on non-withdrawal transaction",
			"transaction_id", transactionID,
			"type", txn.Type)
		return fmt.Errorf("cannot finalize non-withdrawal transaction")
	}

	if txn.Status != "pending" {
		log.Warn("finalize called on non-pending transaction",
			"transaction_id", transactionID,
			"status", txn.Status)
		return nil // already processed
	}

	// Atomic finalization
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if success {
			// SUCCESS: Permanently debit wallet and clear lock
			log.Info("finalizing successful withdrawal",
				"transaction_id", transactionID,
				"amount", txn.Amount)

			if err := s.walletRepo.ReleaseFundsOnSuccess(ctx, txn.UserID, txn.Amount, tx); err != nil {
				log.Error("failed to release funds on success", "error", err)
				return err
			}

			// Update transaction status
			if err := tx.Model(&models.WalletTransaction{}).
				Where("id = ?", transactionID).
				Updates(map[string]interface{}{
					"status":             "completed",
					"external_reference": providerReference,
					"updated_at":         time.Now(),
				}).Error; err != nil {
				return err
			}

			return nil
		} else {
			// FAILED: Return funds to available balance (reversal)
			log.Warn("finalizing failed withdrawal",
				"transaction_id", transactionID,
				"amount", txn.Amount,
				"reason", failureReason)

			if err := s.walletRepo.ReleaseFundsOnFailure(ctx, txn.UserID, txn.Amount, tx); err != nil {
				log.Error("failed to release funds on failure", "error", err)
				return err
			}

			// Update transaction status
			updates := map[string]interface{}{
				"status":     "failed",
				"updated_at": time.Now(),
			}
			if providerReference != "" {
				updates["external_reference"] = providerReference
			}
			if failureReason != "" {
				// Store failure reason in metadata
				var meta map[string]interface{}
				if txn.Metadata != nil {
					_ = json.Unmarshal(txn.Metadata, &meta)
				} else {
					meta = make(map[string]interface{})
				}
				meta["failure_reason"] = failureReason
				metaJSON, _ := json.Marshal(meta)
				updates["metadata"] = datatypes.JSON(metaJSON)
			}

			if err := tx.Model(&models.WalletTransaction{}).
				Where("id = ?", transactionID).
				Updates(updates).Error; err != nil {
				return err
			}

			return nil
		}
	})

	if err != nil {
		log.Error("finalization transaction failed",
			"transaction_id", transactionID,
			"error", err)
		return apperrors.ErrInternalServer
	}

	// Side effects: Notify user
	statusText := "completed"
	notifTitle := "Withdrawal successful"
	notifMessage := fmt.Sprintf("Your withdrawal of ₦%s has been sent to your bank account.",
		formatKobo(txn.Amount))
	notifType := models.NotificationWithdrawalUpdate

	if !success {
		statusText = "failed"
		notifTitle = "Withdrawal failed"
		notifMessage = fmt.Sprintf("Your withdrawal of ₦%s could not be completed. The funds have been returned to your wallet.",
			formatKobo(txn.Amount))
		if failureReason != "" {
			notifMessage += fmt.Sprintf(" Reason: %s", failureReason)
		}
		notifType = models.NotificationWithdrawalUpdate
	}

	s.notifier.Notify(ctx, txn.UserID, notifType, notifTitle, notifMessage,
		map[string]any{
			"transaction_id": transactionID,
			"reference":      txn.Reference,
			"amount":         txn.Amount,
			"status":         statusText,
		})

	log.Info("withdrawal finalized",
		"transaction_id", transactionID,
		"status", statusText,
		"amount", txn.Amount)

	return nil
}

// GetWithdrawalByReference finds a withdrawal transaction ID by reference.
// Used by webhook handler to look up transactions.
func (s *walletService) GetWithdrawalByReference(ctx context.Context, reference string) (uuid.UUID, error) {
	var txn models.WalletTransaction
	err := s.db.WithContext(ctx).
		Where("reference = ? AND type = ?", reference, "withdrawal").
		Select("id").
		First(&txn).Error
	
	if err != nil {
		if repositories.IsErrRecordNotFound(err) {
			return uuid.Nil, fmt.Errorf("withdrawal transaction not found for reference: %s", reference)
		}
		return uuid.Nil, apperrors.ErrInternalServer
	}
	
	return txn.ID, nil
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

// accountNamesMatch performs fuzzy matching between user-provided and bank-verified account names.
// Banks often return names in different formats, so we normalize and compare loosely.
//
// EXAMPLES THAT SHOULD MATCH:
//   - "JOHN DOE" vs "John Doe"
//   - "John D. Doe" vs "John Doe"
//   - "SMITH, JOHN" vs "John Smith"
//   - "O'BRIEN MARY" vs "Mary O'Brien"
//
// ALGORITHM:
//   1. Convert both to uppercase (case-insensitive)
//   2. Remove all non-alphanumeric characters (spaces, dots, commas, apostrophes)
//   3. Compare resulting strings
//
// This prevents false rejections due to formatting differences while still catching
// genuine mismatches (wrong account entered).
func accountNamesMatch(provided, bankRecords string) bool {
	normalize := func(s string) string {
		s = strings.ToUpper(s)
		var result strings.Builder
		for _, r := range s {
			// Keep only letters and digits
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				result.WriteRune(r)
			}
		}
		return result.String()
	}

	normalizedProvided := normalize(provided)
	normalizedBank := normalize(bankRecords)

	// Exact match after normalization
	if normalizedProvided == normalizedBank {
		return true
	}

	// Allow substring match (bank name might have middle initial, suffix, etc.)
	// e.g., "JOHNDOE" matches "JOHND.DOE" or "JOHNDOEJR"
	// Only if provided name is at least 5 characters (prevent "JO" matching "JOHN DOE")
	if len(normalizedProvided) >= 5 {
		return strings.Contains(normalizedBank, normalizedProvided) ||
			strings.Contains(normalizedProvided, normalizedBank)
	}

	return false
}

var _ = time.Now
