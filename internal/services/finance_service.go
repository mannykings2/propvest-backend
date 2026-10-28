package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/payments"
)

// ErrNoSettlementSource is returned by ImportSettlements when the configured
// payment provider doesn't expose a settlement feed (mock). It is a
// configuration answer, not a crash — the handler maps it to 501.
var ErrNoSettlementSource = fmt.Errorf("payment provider does not expose settlement reports")

// FinanceService is the admin-facing view of the Money layer (Phase 7.5/7.8/7.9):
// fee revenue, settlement batches against provider payouts, and the history of
// nightly trust reconciliations.
type FinanceService interface {
	// FeeReport summarises revenue by fee type, the active rules, and the
	// per-transaction fees charged (withdrawals carry fee_kobo since 7.4).
	FeeReport(ctx context.Context) (*dtoFeeReport, error)

	// ListSettlements returns provider payout batches newest-first with their
	// match state.
	ListSettlements(ctx context.Context, page, limit int) ([]models.SettlementBatch, int64, error)

	// IngestSettlementBatch books a provider payout report: every line is tied
	// to a payment we know about; anything unmatched lands in Suspense (9000)
	// and flags the batch so an operator sees it the same day (7.5/7.9).
	IngestSettlementBatch(ctx context.Context, in SettlementBatchInput) (*models.SettlementBatch, error)

	// ImportSettlements pulls the provider's settlement reports since `since`
	// and books every batch not already imported. Idempotent by external_ref:
	// the scheduled worker, the admin "import now" button and a boot catch-up
	// share this one path. Returns ErrNoSettlementSource when the provider has
	// no settlement feed.
	ImportSettlements(ctx context.Context, since time.Time) (*SettlementImportResult, error)

	// ListReconciliationRuns returns the trust-check history (newest-first).
	ListReconciliationRuns(ctx context.Context, page, limit int) ([]models.ReconciliationRun, int64, error)
}

// SettlementItemInput is one line of a provider settlement report.
type SettlementItemInput struct {
	ExternalRef string `json:"external_ref"`
	AmountKobo  int64  `json:"amount_kobo"`
}

// SettlementBatchInput is an uploaded/ingested provider payout.
type SettlementBatchInput struct {
	Provider    string                `json:"provider"`
	ExternalRef string                `json:"external_ref"`
	BatchDate   *time.Time            `json:"batch_date"`
	GrossKobo   int64                 `json:"gross_kobo"`
	FeesKobo    int64                 `json:"fees_kobo"`
	NetKobo     int64                 `json:"net_kobo"`
	Items       []SettlementItemInput `json:"items"`
}

// SettlementImportResult summarises one automatic import pass for the worker
// log and the admin response.
type SettlementImportResult struct {
	Provider  string   `json:"provider"`
	Created   int      `json:"created"` // batches newly booked
	Skipped   int      `json:"skipped"` // already imported / not final / no lines
	BatchRefs []string `json:"batch_refs,omitempty"` // refs newly booked
}

// dtoFeeReport is the shape GET /admin/finance/fees returns.
type dtoFeeReport struct {
	TransactionFeeKobo int64            `json:"transaction_fee_kobo"`
	PerformanceFeeKobo int64            `json:"performance_fee_kobo"`
	SweptToOpsKobo     int64            `json:"swept_to_ops_kobo"`
	SweepableFees      int64            `json:"sweepable_kobo"` // earned, not yet swept
	Rules              []models.FeeRule `json:"rules"`
	RecentFees         []FeeChargeRow   `json:"recent_charges"`
	TotalCharged       int64            `json:"total_charged_kobo"`
}

// FeeChargeRow is one fee actually charged, joined to its business reference.
type FeeChargeRow struct {
	Reference  string    `json:"reference"`
	Type       string    `json:"type"`
	AmountKobo int64     `json:"amount_kobo"`
	FeeKobo    int64     `json:"fee_kobo"`
	CreatedAt  time.Time `json:"created_at"`
}

type financeService struct {
	db          *gorm.DB
	paymentRepo PaymentRepositoryLike
	ledger      LedgerService
	// settlements is the provider's settlement feed (nil when the configured
	// provider has none — import then answers ErrNoSettlementSource).
	settlements payments.SettlementFetcher
}

// PaymentRepositoryLike is the slice of the payment repo settlement matching
// needs (reference lookup) — kept narrow so this service doesn't drag the full
// repository contract in.
type PaymentRepositoryLike interface {
	FindByReference(ctx context.Context, reference string) (*models.Payment, error)
}

func NewFinanceService(db *gorm.DB, paymentRepo PaymentRepositoryLike, ledger LedgerService, settlements payments.SettlementFetcher) FinanceService {
	return &financeService{db: db, paymentRepo: paymentRepo, ledger: ledger, settlements: settlements}
}

// ── Fee report ─────────────────────────────────────────────────────────────

func (s *financeService) FeeReport(ctx context.Context) (*dtoFeeReport, error) {
	report := &dtoFeeReport{Rules: []models.FeeRule{}, RecentFees: []FeeChargeRow{}}

	// Revenue balances: 3000/3100 are credit balances, so invert.
	rev, err := s.accountRevenue(ctx, models.AccountFeeRevenueTransaction)
	if err != nil {
		return nil, err
	}
	report.TransactionFeeKobo = rev

	rev, err = s.accountRevenue(ctx, models.AccountFeeRevenuePerformance)
	if err != nil {
		return nil, err
	}
	report.PerformanceFeeKobo = rev
	report.TotalCharged = report.TransactionFeeKobo + report.PerformanceFeeKobo

	var swept int64
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(e.debit),0) FROM ledger_entries e
		JOIN ledger_accounts a ON a.id = e.account_id
		WHERE a.code = ? AND e.ref = 'fee-sweep'`, models.AccountOperatingCash).Scan(&swept).Error; err != nil {
		return nil, fmt.Errorf("fee report: swept: %w", err)
	}
	report.SweptToOpsKobo = swept
	report.SweepableFees = report.TotalCharged - swept

	if err := s.db.WithContext(ctx).Order("code").Find(&report.Rules).Error; err != nil {
		return nil, fmt.Errorf("fee report: rules: %w", err)
	}

	// Recent charges straight from the wallet ledger (withdrawals freeze their
	// fee at initiation; off-plan fees are on their subscription rows).
	if err := s.db.WithContext(ctx).Raw(`
		SELECT reference, type, amount, fee_kobo, created_at
		FROM wallet_transactions
		WHERE fee_kobo > 0 OR type = 'fee'
		ORDER BY created_at DESC LIMIT 20`).Scan(&report.RecentFees).Error; err != nil {
		return nil, fmt.Errorf("fee report: charges: %w", err)
	}

	return report, nil
}

func (s *financeService) accountRevenue(ctx context.Context, code string) (int64, error) {
	var net int64
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(e.credit - e.debit),0)
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
		WHERE a.code = ?`, code).Scan(&net).Error; err != nil {
		return 0, fmt.Errorf("fee report: %s: %w", code, err)
	}
	return net, nil
}

// ── Settlement ─────────────────────────────────────────────────────────────

func (s *financeService) ListSettlements(ctx context.Context, page, limit int) ([]models.SettlementBatch, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.SettlementBatch{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.SettlementBatch
	err := s.db.WithContext(ctx).
		Preload("Items").
		Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Find(&rows).Error
	return rows, total, err
}

// IngestSettlementBatch books a payout report. Matching rule (7.9): an item is
// matched when its provider reference names a payment we already booked.
// Everything else is money we cannot explain — parked in Suspense (9000) with
// an alert on the batch, so a payout Paystack says settled but we never booked
// is visible instead of quietly absorbed.
func (s *financeService) IngestSettlementBatch(ctx context.Context, in SettlementBatchInput) (*models.SettlementBatch, error) {
	if in.ExternalRef == "" {
		return nil, fmt.Errorf("settlement: external_ref is required")
	}
	if in.Provider == "" {
		in.Provider = "paystack"
	}

	batch := &models.SettlementBatch{
		Provider:    in.Provider,
		ExternalRef: in.ExternalRef,
		BatchDate:   in.BatchDate,
		GrossKobo:   in.GrossKobo,
		FeesKobo:    in.FeesKobo,
		NetKobo:     in.NetKobo,
		Status:      models.SettlementStatusReceived,
	}

	var unmatched []SettlementItemInput
	var matchedCount int

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, it := range in.Items {
			payment, perr := s.paymentRepo.FindByReference(ctx, it.ExternalRef)
			item := models.SettlementItem{
				BatchID:     batch.ID,
				ExternalRef: it.ExternalRef,
				AmountKobo:  it.AmountKobo,
			}
			if perr == nil && payment != nil {
				item.Matched = true
				item.PaymentID = &payment.ID
				matchedCount++
			} else {
				unmatched = append(unmatched, it)
			}
			batch.Items = append(batch.Items, item)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("settlement: match items: %w", err)
	}

	// The batch row must exist before its children — GORM handles the order in
	// one create when Items are attached to the parent.
	if err := s.db.WithContext(ctx).Create(batch).Error; err != nil {
		return nil, fmt.Errorf("settlement: store batch: %w", err)
	}

	// Unmatched money → Suspense, in one journal per item so each line is
	// individually traceable when an operator later explains it.
	var suspenseTotal int64
	if len(unmatched) > 0 {
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, it := range unmatched {
				if _, perr := s.ledger.Post(ctx, tx,
					LedgerLine{AccountCode: models.AccountSettlementClearing, Debit: it.AmountKobo,
						Description: "Settlement line matched no booked payment", Ref: it.ExternalRef},
					LedgerLine{AccountCode: models.AccountSuspense, Credit: it.AmountKobo,
						Description: "Unmatched settlement → suspense", Ref: it.ExternalRef},
				); perr != nil {
					return perr
				}
				suspenseTotal += it.AmountKobo
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("settlement: suspense posting: %w", err)
		}
	}

	status := models.SettlementStatusMatched
	var alert *string
	if len(unmatched) > 0 {
		status = models.SettlementStatusDisputed
		msg := fmt.Sprintf("%d line(s) totalling ₦%.2f matched no booked payment — held in suspense",
			len(unmatched), float64(suspenseTotal)/100.0)
		alert = &msg
	}
	updates := map[string]interface{}{"status": status}
	if alert != nil {
		updates["alert"] = *alert
	}
	if err := s.db.WithContext(ctx).Model(&models.SettlementBatch{}).
		Where("id = ?", batch.ID).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("settlement: finalise batch: %w", err)
	}
	batch.Status = status
	batch.Alert = alert

	log := logger.FromContext(ctx)
	log.Info("settlement batch ingested",
		"external_ref", in.ExternalRef, "items", len(in.Items),
		"matched", matchedCount, "unmatched", len(unmatched), "status", status)

	return batch, nil
}

// ImportSettlements pulls the provider's settlement reports in [since, now]
// and books each batch we haven't imported yet. This is the automatic path the
// worker runs on schedule (Paystack has no settlement webhook), and the same
// code serves the admin "import now" button.
//
// Idempotency is keyed on external_ref (UNIQUE in settlement_batches): the
// pre-check skips batches a previous pass booked, and a duplicate-key race
// between the worker and a manual click counts as a skip, never a double book.
func (s *financeService) ImportSettlements(ctx context.Context, since time.Time) (*SettlementImportResult, error) {
	if s.settlements == nil {
		return nil, ErrNoSettlementSource
	}

	res := &SettlementImportResult{Provider: s.settlements.Name()}
	reports, err := s.settlements.FetchSettlements(ctx, since, time.Now())
	if err != nil {
		return nil, fmt.Errorf("settlement import: fetch: %w", err)
	}

	log := logger.FromContext(ctx)
	for _, rep := range reports {
		switch {
		case rep.Status != "success":
			// Not final — Paystack may still fail it. Next tick re-checks.
			res.Skipped++
			log.Info("settlement import: batch not final yet",
				"ref", rep.ExternalRef, "status", rep.Status)
			continue
		case rep.Currency != "" && rep.Currency != "NGN":
			res.Skipped++
			log.Warn("settlement import: non-NGN batch skipped",
				"ref", rep.ExternalRef, "currency", rep.Currency)
			continue
		case len(rep.Items) == 0:
			// Nothing matchable — booking it would record a payout with no
			// lines to reconcile, which is exactly what the check warns about.
			res.Skipped++
			log.Warn("settlement import: batch has no matchable lines", "ref", rep.ExternalRef)
			continue
		}

		var existing int64
		if err := s.db.WithContext(ctx).Model(&models.SettlementBatch{}).
			Where("external_ref = ?", rep.ExternalRef).Count(&existing).Error; err != nil {
			return nil, fmt.Errorf("settlement import: dedupe: %w", err)
		}
		if existing > 0 {
			res.Skipped++
			continue
		}

		in := SettlementBatchInput{
			Provider:    s.settlements.Name(),
			ExternalRef: rep.ExternalRef,
			GrossKobo:   rep.GrossKobo,
			FeesKobo:    rep.FeesKobo,
			NetKobo:     rep.NetKobo,
		}
		if !rep.BatchDate.IsZero() {
			in.BatchDate = &rep.BatchDate
		}
		for _, it := range rep.Items {
			in.Items = append(in.Items, SettlementItemInput{
				ExternalRef: it.Reference,
				AmountKobo:  it.AmountKobo,
			})
		}

		if _, err := s.IngestSettlementBatch(ctx, in); err != nil {
			// Worker and admin button racing the same batch: the unique index
			// won — the other pass booked it, so this one is a skip.
			if isDuplicateKey(err) {
				res.Skipped++
				continue
			}
			return nil, fmt.Errorf("settlement import: book %s: %w", rep.ExternalRef, err)
		}
		res.Created++
		res.BatchRefs = append(res.BatchRefs, rep.ExternalRef)
	}

	log.Info("settlement import pass complete",
		"provider", res.Provider, "created", res.Created,
		"skipped", res.Skipped, "since", since.Format("2006-01-02"))
	return res, nil
}

// isDuplicateKey recognises Postgres unique violations through GORM's wrapper
// — the race signal that another pass already booked this batch.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "duplicate key") ||
		strings.Contains(s, "UNIQUE constraint failed")
}

func (s *financeService) ListReconciliationRuns(ctx context.Context, page, limit int) ([]models.ReconciliationRun, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := s.db.WithContext(ctx).Model(&models.ReconciliationRun{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.ReconciliationRun
	err := s.db.WithContext(ctx).Order("run_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Find(&rows).Error
	return rows, total, err
}
