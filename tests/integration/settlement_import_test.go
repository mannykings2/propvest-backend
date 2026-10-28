package integration

import (
	"context"
	"testing"
	"time"

	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/payments"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// stubSettlementFetcher plays the provider side of the automatic import: it
// returns exactly what GET /settlement would hand us, without the network.
type stubSettlementFetcher struct {
	reports []payments.SettlementReport
	calls   int
}

func (s *stubSettlementFetcher) Name() string { return "paystack" }

func (s *stubSettlementFetcher) FetchSettlements(_ context.Context, _, _ time.Time) ([]payments.SettlementReport, error) {
	s.calls++
	return s.reports, nil
}

// suspenseCredit is the natural credit balance of Suspense (9000): money that
// matched no payment. Other tests may have left rows behind, so assertions
// here compare deltas, not absolute values.
func suspenseCredit(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var out int64
	require.NoError(t, db.Raw(`
		SELECT COALESCE(SUM(e.credit - e.debit), 0)
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
		WHERE a.code = '9000'`).Scan(&out).Error)
	return out
}

// TestSettlementImport_AutomaticBookAndDedupe exercises the whole automatic
// path the worker runs (Phase 7.5/7.9): a fetched payout batch is matched
// against booked payments, unmatched cash parks in Suspense with the batch
// flagged, a non-final batch waits, and a second pass books nothing twice —
// the guarantee that makes the scheduled worker and the admin "import now"
// button safe to share one path.
func TestSettlementImport_AutomaticBookAndDedupe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()
	ctx := context.Background()

	ledgerSvc := services.NewLedgerService(testDB.DB)
	require.NoError(t, ledgerSvc.EnsureChartOfAccounts(ctx), "seed chart of accounts")
	paymentRepo := repositories.NewPaymentRepository(testDB.DB)

	fetcher := &stubSettlementFetcher{reports: []payments.SettlementReport{
		{
			ExternalRef: "paystack-settlement-88001",
			Status:      "success",
			Currency:    "NGN",
			BatchDate:   time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
			GrossKobo:   800000,
			FeesKobo:    2400,
			NetKobo:     797600,
			Items: []payments.SettlementLine{
				{Reference: "SETT-IMP-MATCH", AmountKobo: 500000, FeesKobo: 1500}, // booked below
				{Reference: "SETT-IMP-GHOST", AmountKobo: 300000, FeesKobo: 900},  // no payment
			},
		},
		{ExternalRef: "paystack-settlement-88002", Status: "processing", Currency: "NGN"},
	}}
	finance := services.NewFinanceService(testDB.DB, paymentRepo, ledgerSvc, fetcher)

	// A payment we booked — the MATCH line must tie back to it.
	investor := seedUser(t, testDB, "settlement.import@example.com")
	seedWallet(t, testDB, investor, 0)
	seedPendingDeposit(t, testDB, investor, "SETT-IMP-MATCH", 500000)

	suspenseBefore := suspenseCredit(t, testDB.DB)

	// ── First pass: books the final batch, waits on the processing one ──────
	res, err := finance.ImportSettlements(ctx, time.Now().AddDate(0, 0, -7))
	require.NoError(t, err)
	assert.Equal(t, "paystack", res.Provider)
	assert.Equal(t, 1, res.Created, "only the success batch is bookable")
	assert.Equal(t, 1, res.Skipped, "the processing batch must wait for a later pass")
	require.Equal(t, []string{"paystack-settlement-88001"}, res.BatchRefs)

	batches, total, err := finance.ListSettlements(ctx, 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	batch := batches[0]
	assert.Equal(t, "paystack-settlement-88001", batch.ExternalRef)
	assert.Equal(t, "disputed", batch.Status, "an unmatched line must flag the batch")
	require.NotNil(t, batch.Alert)
	assert.Contains(t, *batch.Alert, "1 line")

	require.Len(t, batch.Items, 2)
	var matched, unmatched *models.SettlementItem
	for i := range batch.Items {
		switch batch.Items[i].ExternalRef {
		case "SETT-IMP-MATCH":
			matched = &batch.Items[i]
		case "SETT-IMP-GHOST":
			unmatched = &batch.Items[i]
		}
	}
	require.NotNil(t, matched, "booked payment line must be present")
	require.NotNil(t, unmatched, "ghost line must be present")
	assert.True(t, matched.Matched, "line naming a booked payment ties to it")
	require.NotNil(t, matched.PaymentID)
	assert.False(t, unmatched.Matched, "line naming nothing stays unmatched")
	assert.Nil(t, unmatched.PaymentID)

	assert.Equal(t, suspenseBefore+300000, suspenseCredit(t, testDB.DB),
		"unmatched cash parks in Suspense exactly once")

	// ── Second pass: idempotent — nothing re-booked, nothing double-posted ──
	res2, err := finance.ImportSettlements(ctx, time.Now().AddDate(0, 0, -7))
	require.NoError(t, err)
	assert.Equal(t, 0, res2.Created, "an already-imported batch must never re-book")
	assert.Equal(t, 2, res2.Skipped, "existing batch counts as skipped")

	_, total2, err := finance.ListSettlements(ctx, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total2, "no duplicate batch row")
	assert.Equal(t, suspenseBefore+300000, suspenseCredit(t, testDB.DB),
		"no double-post of the unmatched line")

	// ── A provider without a settlement feed answers a typed error ──────────
	noFeed := services.NewFinanceService(testDB.DB, paymentRepo, ledgerSvc, nil)
	_, err = noFeed.ImportSettlements(ctx, time.Now())
	assert.ErrorIs(t, err, services.ErrNoSettlementSource,
		"mock providers must report 'no feed', not crash or silently no-op")
}