package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/payments"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
// MONEY CORRECTNESS UNDER CONCURRENCY (PRODUCTION_READINESS_PLAN Phase 1)
//
// These run against real Postgres because the guarantees under test are
// guarantees the DATABASE makes — row locks (SELECT FOR UPDATE), CHECK
// constraints and UNIQUE indexes. An in-memory double would happily agree with
// a broken transaction and prove nothing.
//
// Covered:
//   1.3  duplicate / concurrent webhook deliveries credit exactly once
//   1.5  two buyers race for the last slot → exactly one purchase
//   1.5  two withdrawals race for the same balance → never overdraw
// ═══════════════════════════════════════════════════════════════════════════

// ─── test doubles ──────────────────────────────────────────────────────────

// stubNotifier is a no-op services.NotificationService: notifications are a
// side effect, not the subject of these tests.
type stubNotifier struct{}

func (stubNotifier) Notify(context.Context, uuid.UUID, string, string, string, map[string]any) {}
func (stubNotifier) NotifyWithEmail(context.Context, uuid.UUID, string, string, string, string, map[string]any) {
}
func (stubNotifier) List(context.Context, uuid.UUID, bool, int, int) (*dto.NotificationListResponse, int64, error) {
	return &dto.NotificationListResponse{Notifications: []dto.NotificationResponse{}}, 0, nil
}
func (stubNotifier) MarkRead(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (stubNotifier) MarkAllRead(context.Context, uuid.UUID) error         { return nil }
func (stubNotifier) Delete(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (stubNotifier) StartRealtimeConsumer(context.Context)                {}

// ─── seeds ─────────────────────────────────────────────────────────────────

// seedWallet gives an existing user a funded main balance.
func seedWallet(t *testing.T, tdb *TestDB, userID uuid.UUID, mainBalance int64) {
	t.Helper()
	require.NoError(t, tdb.DB.Exec(`
		INSERT INTO wallets (id, user_id, main_balance, earnings_balance, currency)
		VALUES (?, ?, ?, 0, 'NGN')`,
		uuid.New(), userID, mainBalance,
	).Error)
}

// seedSingleSlotProperty creates an ACTIVE property with exactly one unit left
// to buy, so the only thing that can stop an oversell is the slot check.
func seedSingleSlotProperty(t *testing.T, tdb *TestDB, creator uuid.UUID, unitPriceKobo int64) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, tdb.DB.Exec(`
		INSERT INTO properties (
			id, title, slug, description, property_type, address, city, state,
			target_amount, roi_percent, duration_months, total_units, unit_price,
			minimum_investment, status, created_by
		) VALUES (
			?, 'Last Slot Property', ?, 'One unit available so two buyers can race for it.',
			'residential', '1 Last Street', 'Abuja', 'FCT',
			?, 12, 36, 1, ?, ?, 'active', ?
		)`,
		id, fmt.Sprintf("last-slot-%s", id.String()[:8]),
		unitPriceKobo, unitPriceKobo, unitPriceKobo, creator,
	).Error)
	return id
}

// seedPendingDeposit creates the payments row InitiateDeposit would have made
// (status pending), i.e. what the webhook arrives for.
func seedPendingDeposit(t *testing.T, tdb *TestDB, userID uuid.UUID, reference string, amountKobo int64) {
	t.Helper()
	require.NoError(t, tdb.DB.Exec(`
		INSERT INTO payments (id, user_id, provider, reference, amount_kobo, currency, status, channel)
		VALUES (?, ?, 'paystack', ?, ?, 'NGN', 'pending', 'deposit')`,
		uuid.New(), userID, reference, amountKobo,
	).Error)
}

// countRows is a tiny assertion helper for "exactly N rows exist".
func countRows(t *testing.T, tdb *TestDB, sql string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	require.NoError(t, tdb.DB.Raw(sql, args...).Scan(&n).Error)
	return n
}

// ─── 1.3 webhook idempotency ───────────────────────────────────────────────

// TestWebhook_DuplicateDeliveryCreditsOnce reproduces Paystack retrying a
// webhook it thinks we never received: the same signed payload delivered again
// (and, in the sub-test below, delivered again CONCURRENTLY) must credit the
// wallet exactly once.
func TestWebhook_DuplicateDeliveryCreditsOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()
	ctx := context.Background()

	walletRepo := repositories.NewWalletRepository(testDB.DB)
	userRepo := repositories.NewUserRepository(testDB.DB)
	paymentRepo := repositories.NewPaymentRepository(testDB.DB)
	outboxRepo := repositories.NewOutboxRepository(testDB.DB)

	// Phase 7: the ledger must be seeded before any money can move — an
	// unseeded chart makes every journal fail closed.
	ledgerSvc := services.NewLedgerService(testDB.DB)
	require.NoError(t, ledgerSvc.EnsureChartOfAccounts(ctx), "seed chart of accounts")
	require.NoError(t, services.EnsureFeeRules(ctx, testDB.DB), "seed fee rules")

	svc := services.NewWalletService(
		walletRepo, userRepo, paymentRepo, outboxRepo,
		payments.NewMockProvider(), stubNotifier{}, nil, /* no RabbitMQ */
		testDB.Config, testDB.DB, ledgerSvc,
		nil, // risk: blocking rules not under test here
	)

	t.Run("sequential duplicate delivery", func(t *testing.T) {
		owner := seedUser(t, testDB, "webhook-seq@example.com")
		seedWallet(t, testDB, owner, 0)
		const ref = "DEP-SEQ-0001"
		const amount = int64(250000) // ₦2,500
		seedPendingDeposit(t, testDB, owner, ref, amount)

		require.NoError(t, svc.CreditFromPaymentReference(ctx, ref, []byte(`{"event":"charge.success"}`)),
			"first delivery must credit")
		require.NoError(t, svc.CreditFromPaymentReference(ctx, ref, []byte(`{"event":"charge.success"}`)),
			"second delivery must be acknowledged as a no-op, not error")

		require.EqualValues(t, 1, countRows(t, testDB,
			`SELECT COUNT(*) FROM wallet_transactions WHERE reference = ?`, ref),
			"ledger must hold exactly one row for the reference")
		require.EqualValues(t, amount, countRows(t, testDB,
			`SELECT main_balance FROM wallets WHERE user_id = ?`, owner),
			"balance must equal the amount exactly once")
	})

	t.Run("concurrent duplicate delivery", func(t *testing.T) {
		owner := seedUser(t, testDB, "webhook-race@example.com")
		seedWallet(t, testDB, owner, 0)
		const ref = "DEP-RACE-0001"
		const amount = int64(300000) // ₦3,000
		seedPendingDeposit(t, testDB, owner, ref, amount)

		payload := []byte(`{"event":"charge.success"}`)
		var wg sync.WaitGroup
		var failures int64
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := svc.CreditFromPaymentReference(ctx, ref, payload); err != nil {
					atomic.AddInt64(&failures, 1)
				}
			}()
		}
		wg.Wait()

		require.EqualValues(t, 0, atomic.LoadInt64(&failures),
			"a duplicate webhook must never surface as an error (provider would retry forever)")
		require.EqualValues(t, 1, countRows(t, testDB,
			`SELECT COUNT(*) FROM wallet_transactions WHERE reference = ?`, ref),
			"even racing deliveries credit exactly once")
		require.EqualValues(t, amount, countRows(t, testDB,
			`SELECT main_balance FROM wallets WHERE user_id = ?`, owner),
			"balance must never exceed the paid amount")

		var paymentStatus string
		require.NoError(t, testDB.DB.Raw(`SELECT status FROM payments WHERE reference = ?`, ref).Scan(&paymentStatus).Error)
		require.Equal(t, "success", paymentStatus)
	})
}

// ─── 1.5 last-slot race ────────────────────────────────────────────────────

// TestConcurrentInvestment_NeverOversellsLastSlot fires two purchases at a
// property with one unit left, with a wallet deep enough to pay for both — so
// the balance check cannot be what stops the second buyer. Exactly one must
// succeed and the property must end with units_sold = 1.
func TestConcurrentInvestment_NeverOversellsLastSlot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()
	ctx := context.Background()

	const unitPrice = int64(500000) // ₦5,000 per slot

	creator := seedUser(t, testDB, "slot.creator@example.com")
	propertyID := seedSingleSlotProperty(t, testDB, creator, unitPrice)

	buyerA := seedUser(t, testDB, "slot.buyer.a@example.com")
	buyerB := seedUser(t, testDB, "slot.buyer.b@example.com")
	// Enough for two purchases: only the slot limit may stop the second one.
	seedWallet(t, testDB, buyerA, unitPrice*2)
	seedWallet(t, testDB, buyerB, unitPrice*2)

	// Phase 7: journals must be seedable before any money can move.
	ledgerSvc := services.NewLedgerService(testDB.DB)
	require.NoError(t, ledgerSvc.EnsureChartOfAccounts(ctx), "seed chart of accounts")
	require.NoError(t, services.EnsureFeeRules(ctx, testDB.DB), "seed fee rules")

	svc := services.NewInvestmentService(
		repositories.NewInvestmentRepository(testDB.DB),
		repositories.NewPropertyRepository(testDB.DB),
		repositories.NewWalletRepository(testDB.DB),
		repositories.NewOutboxRepository(testDB.DB),
		repositories.NewSaleVoteRepository(testDB.DB),
		stubNotifier{},
		testDB.DB,
		ledgerSvc,
	)

	buyers := []uuid.UUID{buyerA, buyerB}
	results := make([]error, len(buyers))

	var start sync.WaitGroup
	start.Add(1)
	var wg sync.WaitGroup
	for i, buyer := range buyers {
		wg.Add(1)
		go func(i int, buyer uuid.UUID) {
			defer wg.Done()
			start.Wait() // release both purchases at the same moment
			_, err := svc.CreateInvestment(ctx, buyer, dto.CreateInvestmentRequest{
				PropertyID: propertyID,
				Slots:      1,
			})
			results[i] = err
		}(i, buyer)
	}
	start.Done()
	wg.Wait()

	var successes int
	for i, err := range results {
		if err == nil {
			successes++
			continue
		}
		t.Logf("buyer %d rejected: %v", i, err)
	}
	require.Equal(t, 1, successes, "exactly one of two racing buyers may win the last slot")

	require.EqualValues(t, 1, countRows(t, testDB,
		`SELECT units_sold FROM properties WHERE id = ?`, propertyID),
		"units_sold must be exactly 1 — the property was oversold")
	require.EqualValues(t, 1, countRows(t, testDB,
		`SELECT COUNT(*) FROM investments WHERE property_id = ?`, propertyID),
		"exactly one investment row")
	require.EqualValues(t, 1, countRows(t, testDB,
		`SELECT COUNT(*) FROM wallet_transactions WHERE type = 'investment'`),
		"exactly one ledger debit for the purchase")

	// The winner paid exactly once: total debited across both wallets == unitPrice.
	var debited int64
	require.NoError(t, testDB.DB.Raw(`
		SELECT COALESCE(SUM(amount), 0) FROM wallet_transactions WHERE type = 'investment'`).Scan(&debited).Error)
	require.Equal(t, unitPrice, debited, "wallet must be debited exactly once")
}

// ─── 1.5 double-withdraw race ──────────────────────────────────────────────

// TestConcurrentWithdrawals_CannotOverdrawWallet fires two withdrawals at a
// wallet that can only cover one of them. Locking (FOR UPDATE on the wallet
// plus locked_balance reservation) must let exactly one through and never let
// the sum of reservations exceed the balance.
func TestConcurrentWithdrawals_CannotOverdrawWallet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()
	ctx := context.Background()

	const balance = int64(200000)    // ₦2,000 available
	const withdrawal = int64(150000) // ₦1,500 each — only one fits
	require.GreaterOrEqual(t, withdrawal, testDB.Config.MinWithdrawalAmount,
		"test amount must satisfy the configured minimum withdrawal")

	owner := seedUser(t, testDB, "withdraw.race@example.com")
	seedWallet(t, testDB, owner, balance)

	// Phase 7: withdrawals freeze their fee from fee_rules at initiation.
	ledgerSvc := services.NewLedgerService(testDB.DB)
	require.NoError(t, ledgerSvc.EnsureChartOfAccounts(ctx), "seed chart of accounts")
	require.NoError(t, services.EnsureFeeRules(ctx, testDB.DB), "seed fee rules")

	svc := services.NewWalletService(
		repositories.NewWalletRepository(testDB.DB),
		repositories.NewUserRepository(testDB.DB),
		repositories.NewPaymentRepository(testDB.DB),
		repositories.NewOutboxRepository(testDB.DB),
		payments.NewMockProvider(), stubNotifier{}, nil,
		testDB.Config, testDB.DB, ledgerSvc,
		nil, // risk: blocking rules not under test here
	)

	req := dto.WithdrawRequest{
		Amount:        withdrawal,
		BankCode:      "044",
		AccountNumber: "1234567890",
		AccountName:   "TEST USER", // what the mock bank returns
	}

	var start sync.WaitGroup
	start.Add(1)
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start.Wait()
			_, err := svc.InitiateWithdrawal(ctx, owner, req)
			results[i] = err
		}(i)
	}
	start.Done()
	wg.Wait()

	var successes int
	for i, err := range results {
		if err == nil {
			successes++
		} else {
			t.Logf("withdrawal %d rejected: %v", i, err)
		}
	}
	require.Equal(t, 1, successes, "only one of two racing withdrawals may be accepted")

	// Money conservation: nothing vanished, nothing was double-reserved.
	var w struct {
		MainBalance   int64
		LockedBalance int64
	}
	require.NoError(t, testDB.DB.Raw(
		`SELECT main_balance, locked_balance FROM wallets WHERE user_id = ?`, owner).
		Scan(&w).Error)
	mainBalance, lockedBalance := w.MainBalance, w.LockedBalance

	// Locking reserves against available (main - locked); main_balance itself
	// only drops when the payout settles.
	require.Equal(t, balance, mainBalance,
		"main_balance must be untouched until the payout settles")
	require.Equal(t, withdrawal, lockedBalance,
		"locked_balance must reserve exactly one withdrawal")
	require.Equal(t, balance-withdrawal, mainBalance-lockedBalance,
		"available balance must equal what is left after one withdrawal")

	require.EqualValues(t, 1, countRows(t, testDB,
		`SELECT COUNT(*) FROM wallet_transactions WHERE type = 'withdrawal' AND user_id = ?`, owner),
		"exactly one withdrawal may be recorded")
}
