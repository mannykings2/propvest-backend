package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/database"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/payments"
	"github.com/mannykings2/propvest-backend/internal/queue"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// PROPVEST BACKGROUND WORKER
// ═══════════════════════════════════════════════════════════════════════════
//
// PURPOSE:
// This worker processes async jobs that must NOT block HTTP requests:
//   1. Withdrawal processing: Initiates bank transfers via Paystack
//   2. Reconciliation: Polls pending withdrawals (safety net for missed webhooks)
//   3. Email dispatch: Sends transactional emails (deposit receipts, etc.)
//   4. SMS dispatch: Sends SMS notifications
//
// WHY A SEPARATE PROCESS?
// -----------------------
// Bank transfers can take 1-5 minutes to complete. If we did this in the HTTP
// handler, requests would timeout and users would see errors even though the
// withdrawal succeeded. By using a queue + worker, we can:
//   - Return immediately ("Withdrawal initiated")
//   - Process the transfer in the background
//   - Notify user when complete (via webhook or polling)
//
// ARCHITECTURE:
// ------------
// API Process:
//   1. User requests withdrawal
//   2. Lock funds (locked_balance += amount)
//   3. Create pending transaction
//   4. Queue withdrawal message
//   5. Return "pending" status
//
// Worker Process (THIS FILE):
//   1. Consume withdrawal message from queue
//   2. Call Paystack InitiateTransfer API
//   3. Save transfer_code for tracking
//   4. Webhook or polling finalizes the withdrawal
//
// RECONCILIATION:
// --------------
// Webhooks can be missed (network issues, provider downtime). The reconciliation
// worker runs every 5 minutes to:
//   1. Find pending withdrawals older than 10 minutes
//   2. Call VerifyTransfer to check actual status
//   3. Finalize based on current status (success/failed)
//
// GRACEFUL SHUTDOWN:
// -----------------
// Worker listens for SIGINT/SIGTERM and shuts down gracefully:
//   1. Stop accepting new messages
//   2. Finish processing current messages
//   3. Close queue connections
//   4. Close database connections

func main() {
	ctx := context.Background()

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 1: Load Configuration
	// ═══════════════════════════════════════════════════════════════════════
	cfg := config.Load()

	// STEP 2: Initialize Logger
	logger.Init(cfg.AppEnv)
	log := logger.FromContext(ctx)

	log.Info("🚀 PropVest Background Worker starting...")

	if err := cfg.Validate(); err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	log.Info("configuration loaded",
		"env", cfg.AppEnv,
		"payment_provider", cfg.PaymentProvider)

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 3: Connect to Database
	// ═══════════════════════════════════════════════════════════════════════
	database.Connect(cfg)
	db := database.DB

	if db == nil {
		log.Error("failed to connect to database")
		os.Exit(1)
	}

	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	}()

	log.Info("✓ database connected")

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 4: Initialize Payment Provider
	// ═══════════════════════════════════════════════════════════════════════
	provider := payments.NewProvider(cfg)

	log.Info("✓ payment provider initialized", "provider", provider.Name())

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 5: Initialize Repositories
	// ═══════════════════════════════════════════════════════════════════════
	walletRepo := repositories.NewWalletRepository(db)
	userRepo := repositories.NewUserRepository(db)
	paymentRepo := repositories.NewPaymentRepository(db)

	log.Info("✓ repositories initialized")

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 6: Initialize Services
	// ═══════════════════════════════════════════════════════════════════════
	// Note: We pass nil for notifier and queue client since worker doesn't need them
	// for processing withdrawals (notifications are sent by the service itself)
	walletService := services.NewWalletService(
		walletRepo,
		userRepo,
		paymentRepo,
		provider,
		nil, // notifier - not needed in worker
		nil, // mq - worker doesn't publish, only consumes
		cfg,
		db,
	)

	log.Info("✓ services initialized")

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 7: Connect to Message Queue
	// ═══════════════════════════════════════════════════════════════════════
	mqClient := queue.New(cfg.RabbitMQURL)
	defer mqClient.Close()

	if !mqClient.Enabled() {
		log.Warn("⚠️ RabbitMQ not available; worker will only run reconciliation")
	} else {
		log.Info("✓ message queue connected")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 8: Start Withdrawal Processor
	// ═══════════════════════════════════════════════════════════════════════
	if mqClient.Enabled() {
		startWithdrawalProcessor(ctx, mqClient, provider, walletService, db)
		log.Info("✓ withdrawal processor started")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 9: Start Email Dispatcher (if needed)
	// ═══════════════════════════════════════════════════════════════════════
	if mqClient.Enabled() {
		mqClient.Consume(queue.QueueEmailDispatch, func(ctx context.Context, body []byte) error {
			var msg queue.EmailMessage
			if err := json.Unmarshal(body, &msg); err != nil {
				return err
			}
			logger.FromContext(ctx).Info("📧 would send email", "to", msg.To, "subject", msg.Subject)
			// TODO: Implement actual email sending
			return nil
		})
		log.Info("✓ email dispatcher started")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 10: Start SMS Dispatcher (if needed)
	// ═══════════════════════════════════════════════════════════════════════
	if mqClient.Enabled() {
		mqClient.Consume(queue.QueueSMSDispatch, func(ctx context.Context, body []byte) error {
			var msg queue.SMSMessage
			if err := json.Unmarshal(body, &msg); err != nil {
				return err
			}
			logger.FromContext(ctx).Info("📱 would send SMS", "to", msg.To)
			// TODO: Implement actual SMS sending
			return nil
		})
		log.Info("✓ SMS dispatcher started")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 11: Start Reconciliation Worker
	// ═══════════════════════════════════════════════════════════════════════
	stopReconciliation := startReconciliationWorker(ctx, db, provider, walletService)
	defer stopReconciliation()

	log.Info("✓ reconciliation worker started")

	// ═══════════════════════════════════════════════════════════════════════
	// STEP 12: Wait for Shutdown Signal
	// ═══════════════════════════════════════════════════════════════════════
	log.Info("✅ Worker is running. Press Ctrl+C to stop.")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("🛑 Shutting down worker gracefully...")
	log.Info("👋 Worker stopped")
}

// ═══════════════════════════════════════════════════════════════════════════
// WITHDRAWAL PROCESSOR
// ═══════════════════════════════════════════════════════════════════════════

// startWithdrawalProcessor consumes withdrawal messages and initiates bank transfers.
func startWithdrawalProcessor(
	ctx context.Context,
	mq *queue.Client,
	provider payments.Provider,
	walletService services.WalletService,
	db *gorm.DB,
) {
	mq.Consume(queue.QueueWithdrawalProcess, func(ctx context.Context, body []byte) error {
		log := logger.FromContext(ctx)

		// Parse message
		var msg queue.WithdrawalMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			log.Error("invalid withdrawal message", "error", err)
			return nil // Don't requeue invalid messages
		}

		// Parse UUIDs
		transactionID, err := uuid.Parse(msg.TransactionID)
		if err != nil {
			log.Error("invalid transaction ID", "transaction_id", msg.TransactionID, "error", err)
			return nil
		}

		_ = msg.UserID // Suppress unused warning

		log.Info("🏦 processing withdrawal",
			"transaction_id", transactionID,
			"amount", msg.AmountKobo,
			"reference", msg.Reference)

		// ───────────────────────────────────────────────────────────────────
		// STEP 1: Fetch transaction details
		// ───────────────────────────────────────────────────────────────────
		var txn models.WalletTransaction
		if err := db.Where("id = ?", transactionID).First(&txn).Error; err != nil {
			log.Error("transaction not found", "transaction_id", transactionID, "error", err)
			return nil // Don't requeue if transaction doesn't exist
		}

		// Check if already processed
		if txn.Status != "pending" {
			log.Info("transaction already processed",
				"transaction_id", transactionID,
				"status", txn.Status)
			return nil
		}

		// ───────────────────────────────────────────────────────────────────
		// STEP 2: Extract bank details from metadata
		// ───────────────────────────────────────────────────────────────────
		var metadata map[string]interface{}
		if err := json.Unmarshal(txn.Metadata, &metadata); err != nil {
			log.Error("failed to parse transaction metadata", "error", err)
			return fmt.Errorf("invalid metadata: %w", err) // Requeue
		}

		bankCode, _ := metadata["bank_code"].(string)
		accountNumber, _ := metadata["account_number"].(string)
		accountName, _ := metadata["account_name"].(string)

		if bankCode == "" || accountNumber == "" {
			log.Error("missing bank details in transaction", "transaction_id", transactionID)
			// Finalize as failed - can't process without bank details
			_ = walletService.FinalizeWithdrawal(ctx, transactionID, false, "", "Missing bank account details")
			return nil
		}

		// ───────────────────────────────────────────────────────────────────
		// STEP 3: Initiate transfer with Paystack
		// ───────────────────────────────────────────────────────────────────
		transferReq := &payments.TransferRequest{
			Amount:        msg.AmountKobo,
			AccountNumber: accountNumber,
			AccountName:   accountName,
			BankCode:      bankCode,
			Reference:     msg.Reference,
			Reason:        "Wallet withdrawal",
			Currency:      "NGN",
		}

		result, err := provider.InitiateTransfer(ctx, transferReq)
		if err != nil {
			log.Error("transfer initiation failed",
				"transaction_id", transactionID,
				"error", err)
			// Requeue for retry (might be temporary provider issue)
			return fmt.Errorf("transfer failed: %w", err)
		}

		log.Info("transfer initiated",
			"transaction_id", transactionID,
			"transfer_code", result.TransferCode,
			"status", result.Status)

		// ───────────────────────────────────────────────────────────────────
		// STEP 4: Save transfer code to transaction
		// ───────────────────────────────────────────────────────────────────
		if err := db.Model(&models.WalletTransaction{}).
			Where("id = ?", transactionID).
			Update("external_reference", result.TransferCode).Error; err != nil {
			log.Error("failed to save transfer code", "error", err)
			// Don't fail the job - transfer is initiated, we just couldn't save the code
		}

		// ───────────────────────────────────────────────────────────────────
		// STEP 5: Finalize immediately if already completed/failed
		// ───────────────────────────────────────────────────────────────────
		switch result.Status {
		case "success":
			log.Info("transfer completed immediately", "transaction_id", transactionID)
			_ = walletService.FinalizeWithdrawal(ctx, transactionID, true, result.TransferCode, "")
		case "failed":
			log.Warn("transfer failed immediately",
				"transaction_id", transactionID,
				"reason", result.FailureReason)
			_ = walletService.FinalizeWithdrawal(ctx, transactionID, false, result.TransferCode, result.FailureReason)
		case "pending":
			log.Info("transfer pending, will be finalized by webhook or reconciliation",
				"transaction_id", transactionID)
			// Webhook or reconciliation worker will finalize later
		}

		return nil // Success - message processed
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// RECONCILIATION WORKER
// ═══════════════════════════════════════════════════════════════════════════

// startReconciliationWorker polls pending withdrawals and verifies their status.
// This is a safety net for missed webhooks.
func startReconciliationWorker(
	ctx context.Context,
	db *gorm.DB,
	provider payments.Provider,
	walletService services.WalletService,
) func() {
	ticker := time.NewTicker(5 * time.Minute)
	done := make(chan struct{})

	go func() {
		// Run immediately on startup
		reconcilePendingWithdrawals(ctx, db, provider, walletService)

		// Then run every 5 minutes
		for {
			select {
			case <-ticker.C:
				reconcilePendingWithdrawals(ctx, db, provider, walletService)
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	// Return stop function
	return func() {
		close(done)
	}
}

// reconcilePendingWithdrawals finds stale pending withdrawals and checks their status.
func reconcilePendingWithdrawals(
	ctx context.Context,
	db *gorm.DB,
	provider payments.Provider,
	walletService services.WalletService,
) {
	log := logger.FromContext(ctx)
	log.Info("🔄 running reconciliation check...")

	// Find pending withdrawals older than 10 minutes
	// (give webhook a chance to arrive first)
	cutoff := time.Now().Add(-10 * time.Minute)

	var txns []models.WalletTransaction
	err := db.Where("type = ? AND status = ? AND created_at < ?",
		"withdrawal", "pending", cutoff).
		Find(&txns).Error

	if err != nil {
		log.Error("reconciliation query failed", "error", err)
		return
	}

	if len(txns) == 0 {
		log.Info("✓ no pending withdrawals to reconcile")
		return
	}

	log.Info("found pending withdrawals", "count", len(txns))

	for _, txn := range txns {
		// Skip if no transfer code (transfer not yet initiated)
		if txn.ExternalReference == nil || *txn.ExternalReference == "" {
			log.Warn("skipping transaction without transfer code",
				"transaction_id", txn.ID,
				"reference", txn.Reference)
			continue
		}

		log.Info("verifying withdrawal status",
			"transaction_id", txn.ID,
			"reference", txn.Reference,
			"transfer_code", *txn.ExternalReference)

		// Verify current status with provider
		result, err := provider.VerifyTransfer(ctx, *txn.ExternalReference)
		if err != nil {
			log.Error("verification failed",
				"transaction_id", txn.ID,
				"error", err)
			continue
		}

		log.Info("transfer status verified",
			"transaction_id", txn.ID,
			"status", result.Status)

		// Finalize based on status
		switch result.Status {
		case "success":
			log.Info("reconciling successful transfer", "transaction_id", txn.ID)
			if err := walletService.FinalizeWithdrawal(ctx, txn.ID, true, result.TransferCode, ""); err != nil {
				log.Error("finalization failed", "transaction_id", txn.ID, "error", err)
			}

		case "failed", "reversed":
			reason := result.FailureReason
			if reason == "" {
				reason = fmt.Sprintf("Transfer %s", result.Status)
			}
			log.Warn("reconciling failed transfer",
				"transaction_id", txn.ID,
				"reason", reason)
			if err := walletService.FinalizeWithdrawal(ctx, txn.ID, false, result.TransferCode, reason); err != nil {
				log.Error("finalization failed", "transaction_id", txn.ID, "error", err)
			}

		case "pending":
			log.Info("transfer still pending", "transaction_id", txn.ID)
			// Still pending - will check again next cycle

		default:
			log.Warn("unknown transfer status",
				"transaction_id", txn.ID,
				"status", result.Status)
		}
	}

	log.Info("✓ reconciliation complete")
}
