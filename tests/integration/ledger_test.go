package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/payments"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ledgerSum returns the natural balance of an account code: debit − credit
// for assets, credit − debit for liabilities/revenue, so assertions read the
// way the account actually stands. With userID it scopes to one sub-ledger.
func ledgerSum(t *testing.T, db *gorm.DB, code string, userID *uuid.UUID) int64 {
	t.Helper()
	var out int64
	query := `
		SELECT COALESCE(SUM((e.debit - e.credit) *
		       CASE WHEN a.type = 'asset' THEN 1 ELSE -1 END), 0)
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
		WHERE a.code = ?`
	args := []interface{}{code}
	if userID != nil {
		query += ` AND e.user_id = ?`
		args = append(args, *userID)
	}
	require.NoError(t, db.Raw(query, args...).Scan(&out).Error)
	return out
}

// requireJournalBalanced asserts the double-entry invariant for one business
// reference: every journal's debits equal its credits.
func requireJournalBalanced(t *testing.T, db *gorm.DB, ref string) {
	t.Helper()
	var out struct {
		Dr int64
		Cr int64
	}
	require.NoError(t, db.Raw(`
		SELECT COALESCE(SUM(e.debit),0) AS dr, COALESCE(SUM(e.credit),0) AS cr
		FROM ledger_entries e WHERE e.ref = ?`, ref).Scan(&out).Error)
	assert.Equal(t, out.Dr, out.Cr, "journal %s must balance (dr=%d cr=%d)", ref, out.Dr, out.Cr)
	assert.Greater(t, out.Dr, int64(0), "journal %s must actually post money", ref)
}

// mirrorMismatches counts users whose ledger sub-ledger disagrees with their
// wallet column — the same check the nightly reconciliation runs.
func mirrorMismatches(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT w.user_id
			FROM wallets w
			LEFT JOIN (
				SELECT e.user_id,
				       SUM(e.credit - e.debit) FILTER (WHERE a.code = '2010') AS bal_2010,
				       SUM(e.credit - e.debit) FILTER (WHERE a.code = '2100') AS bal_2100,
				       SUM(e.credit - e.debit) FILTER (WHERE a.code = '2020') AS bal_2020
				FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
				WHERE a.code IN ('2010','2100','2020') AND e.user_id IS NOT NULL
				GROUP BY e.user_id
			) b ON b.user_id = w.user_id
			WHERE COALESCE(b.bal_2010, 0) <> (w.main_balance - w.locked_balance)
			   OR COALESCE(b.bal_2100, 0) <> w.locked_balance
			   OR COALESCE(b.bal_2020, 0) <> w.earnings_balance
		) mismatched
	`).Scan(&n).Error)
	return n
}

// TestLedger_EndToEnd posts real money through deposit → invest → withdraw and
// proves the Phase 7 invariants: balanced journals per reference, per-user
// sub-ledgers that mirror the wallet columns, the 1% capped fee landing in
// revenue, a passing reconciliation, and a fee sweep.
func TestLedger_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()
	ctx := context.Background()

	ledgerSvc := services.NewLedgerService(testDB.DB)
	require.NoError(t, ledgerSvc.EnsureChartOfAccounts(ctx), "seed chart of accounts")
	require.NoError(t, services.EnsureFeeRules(ctx, testDB.DB), "seed fee rules")

	walletRepo := repositories.NewWalletRepository(testDB.DB)
	userRepo := repositories.NewUserRepository(testDB.DB)
	paymentRepo := repositories.NewPaymentRepository(testDB.DB)
	outboxRepo := repositories.NewOutboxRepository(testDB.DB)

	walletSvc := services.NewWalletService(
		walletRepo, userRepo, paymentRepo, outboxRepo,
		payments.NewMockProvider(), stubNotifier{}, nil,
		testDB.Config, testDB.DB, ledgerSvc,
		nil, // risk: blocking rules not under test here
	)

	// Wallet starts empty: every naira must arrive through a booked deposit,
	// otherwise the sub-ledger mirror cannot hold.
	investor := seedUser(t, testDB, "ledger.investor@example.com")
	seedWallet(t, testDB, investor, 0)

	// ── Deposits: DR 1010 clearing / CR 2010 wallet ──────────────────────────
	const deposit = int64(500000) // ₦5,000
	seedPendingDeposit(t, testDB, investor, "LED-DEP-0001", deposit)
	require.NoError(t, walletSvc.CreditFromPaymentReference(ctx, "LED-DEP-0001", []byte(`{"event":"charge.success"}`)),
		"deposit credit must succeed")
	seedPendingDeposit(t, testDB, investor, "LED-DEP-0002", deposit)
	require.NoError(t, walletSvc.CreditFromPaymentReference(ctx, "LED-DEP-0002", []byte(`{"event":"charge.success"}`)))

	requireJournalBalanced(t, testDB.DB, "LED-DEP-0001")
	requireJournalBalanced(t, testDB.DB, "LED-DEP-0002")
	assert.Equal(t, deposit*2, ledgerSum(t, testDB.DB, "2010", &investor),
		"user wallet account must equal the two deposits")

	// ── Investment: DR 2010 wallet / CR 2200 holdings ────────────────────────
	const unitPrice = int64(200000) // ₦2,000 per slot
	creator := seedUser(t, testDB, "ledger.creator@example.com")
	propertyID := seedSingleSlotProperty(t, testDB, creator, unitPrice)

	invSvc := services.NewInvestmentService(
		repositories.NewInvestmentRepository(testDB.DB),
		repositories.NewPropertyRepository(testDB.DB),
		walletRepo, outboxRepo,
		repositories.NewSaleVoteRepository(testDB.DB),
		stubNotifier{}, testDB.DB, ledgerSvc,
	)
	inv, err := invSvc.CreateInvestment(ctx, investor, dto.CreateInvestmentRequest{
		PropertyID: propertyID,
		Slots:      1,
	})
	require.NoError(t, err, "investment must succeed")
	require.NotNil(t, inv)

	// The investment's own reference is what its journal is keyed by — read it
	// back rather than guessing the generated id.
	var invRef string
	require.NoError(t, testDB.DB.Raw(
		`SELECT reference FROM investments WHERE user_id = ? ORDER BY created_at DESC LIMIT 1`, investor).
		Scan(&invRef).Error)
	requireJournalBalanced(t, testDB.DB, invRef)
	assert.Equal(t, deposit*2-unitPrice, ledgerSum(t, testDB.DB, "2010", &investor),
		"investment moves principal out of the wallet account")
	assert.Equal(t, unitPrice, ledgerSum(t, testDB.DB, "2200", &investor),
		"principal lives in investment holdings")

	// ── Withdrawal: reserve, then paid with the frozen 1% fee ────────────────
	const withdraw = int64(150000) // ₦1,500 → fee ₦15 (1,500 kobo)
	wd, err := walletSvc.InitiateWithdrawal(ctx, investor, dto.WithdrawRequest{
		Amount:        withdraw,
		BankCode:      "044",
		AccountNumber: "1234567890",
		AccountName:   "TEST USER", // what the mock bank returns
	})
	require.NoError(t, err, "withdrawal initiation must succeed")

	// Fee frozen at initiation: 1% of ₦1,500 = ₦15.
	var frozen struct {
		Fee int64
		Net int64
	}
	require.NoError(t, testDB.DB.Raw(
		`SELECT fee_kobo AS fee, net_kobo AS net FROM wallet_transactions WHERE id = ?`, wd.ID).
		Scan(&frozen).Error)
	feeKobo, netKobo := frozen.Fee, frozen.Net
	assert.Equal(t, int64(1500), feeKobo, "1% of ₦1,500")
	assert.Equal(t, withdraw-1500, netKobo, "net payout = gross − fee")

	// Reserving moves the money into the withdrawal reserve (2100).
	assert.Equal(t, withdraw, ledgerSum(t, testDB.DB, "2100", &investor),
		"reserve mirrors the locked balance while the payout is in flight")

	require.NoError(t, walletSvc.FinalizeWithdrawal(ctx, wd.ID, true, "TRF-MOCK-1", ""),
		"finalisation must succeed")
	requireJournalBalanced(t, testDB.DB, wd.Reference)

	assert.Equal(t, int64(0), ledgerSum(t, testDB.DB, "2100", &investor),
		"reserve is emptied once the payout settles")
	assert.Equal(t, int64(1500), ledgerSum(t, testDB.DB, "3000", nil),
		"the 1% fee lands in transaction-fee revenue")
	assert.Equal(t, deposit*2-unitPrice-withdraw, ledgerSum(t, testDB.DB, "2010", &investor),
		"wallet account drops by the gross withdrawal")

	// ── Trust checks: reconciliation passes, then the sweep moves the fee ────
	assert.EqualValues(t, 0, mirrorMismatches(t, testDB.DB),
		"every user sub-ledger must mirror its wallet column")

	run, err := ledgerSvc.RunReconciliation(ctx, "test")
	require.NoError(t, err, "reconciliation must run")
	assert.Equal(t, "pass", run.Status, "books must reconcile: %+v", run)
	assert.EqualValues(t, 0, run.SuspenseKobo, "suspense must be empty")
	assert.Equal(t, run.TotalDebitsKobo, run.TotalCreditsKobo, "trial balance")

	swept, err := ledgerSvc.SweepFees(ctx)
	require.NoError(t, err, "fee sweep must run")
	assert.Equal(t, int64(1500), swept, "exactly the earned fee is swept to operating cash")
	assert.Equal(t, int64(1500), ledgerSum(t, testDB.DB, "1000", nil),
		"operating cash holds the swept fee")

	// A second sweep has nothing left to move — the sweep is idempotent.
	sweptAgain, err := ledgerSvc.SweepFees(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, sweptAgain, "no double sweep")
}

// TestLedger_BackfillReconstructsHistory proves Phase 7.3: wallet transactions
// that predate the ledger are reconstructed so sub-ledgers equal balances.
func TestLedger_BackfillReconstructsHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()
	ctx := context.Background()

	ledgerSvc := services.NewLedgerService(testDB.DB)
	require.NoError(t, ledgerSvc.EnsureChartOfAccounts(ctx))

	user := seedUser(t, testDB, "ledger.backfill@example.com")
	// Pre-ledger history: a deposit then an investment, recorded only in
	// wallet_transactions (exactly what production looks like on cutover day).
	seedWallet(t, testDB, user, 300000)
	require.NoError(t, testDB.DB.Exec(`
		INSERT INTO wallet_transactions
			(wallet_id, user_id, type, amount, balance_before, balance_after, reference, description, status)
		SELECT id, user_id, 'deposit', 500000, 0, 500000, 'OLD-DEP-1', 'legacy deposit', 'completed'
		FROM wallets WHERE user_id = ?`, user).Error)
	require.NoError(t, testDB.DB.Exec(`
		INSERT INTO wallet_transactions
			(wallet_id, user_id, type, amount, balance_before, balance_after, reference, description, status)
		SELECT id, user_id, 'investment', 200000, 500000, 300000, 'OLD-INV-1', 'legacy investment', 'completed'
		FROM wallets WHERE user_id = ?`, user).Error)

	posted, err := ledgerSvc.BackfillFromWalletTransactions(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, posted, "both legacy transactions reconstruct")

	// Idempotent: a second run books nothing new.
	postedAgain, err := ledgerSvc.BackfillFromWalletTransactions(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, postedAgain, "backfill must never double-book")

	requireJournalBalanced(t, testDB.DB, "OLD-DEP-1")
	requireJournalBalanced(t, testDB.DB, "OLD-INV-1")
	assert.Equal(t, int64(300000), ledgerSum(t, testDB.DB, "2010", &user),
		"wallet account equals the wallet's main balance after reconstruction")
}
