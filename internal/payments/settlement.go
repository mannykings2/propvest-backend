package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/mannykings2/propvest-backend/internal/logger"
)

// ═══════════════════════════════════════════════════════════════════════════
// SETTLEMENT REPORT IMPORT
// ═══════════════════════════════════════════════════════════════════════════
//
// Paystack's documented webhook list has NO settlement event, so payout
// batches cannot arrive by push — the worker polls the Settlements API:
//
//   GET /settlement                   → payout batches (id, status, date)
//   GET /settlement/{id}/transactions → the transaction lines inside a batch
//
// Docs: https://paystack.com/docs/api/settlement/
//
// The finance service books what the fetcher returns, keyed on a stable
// external_ref derived from the provider's settlement id, so the scheduled
// worker, the admin "import now" button and a boot-time catch-up all share
// one idempotent path.
//
// UNITS: every amount here is Paystack's smallest unit (kobo) — the same
// convention as /transaction/* which this codebase already consumes. Batch
// totals are computed from the batch's own lines rather than trusting the
// summary fields, so the stored gross/fees/net always add up to the items.

// SettlementLine is one provider transaction inside a settlement batch.
type SettlementLine struct {
	Reference  string // Paystack transaction reference == our payment reference
	AmountKobo int64
	FeesKobo   int64
}

// SettlementReport is one provider payout batch, ready for booking.
type SettlementReport struct {
	ExternalRef string // stable identity: paystack-settlement-<id>
	Status      string // provider status; only "success" is bookable
	Currency    string
	BatchDate   time.Time
	GrossKobo   int64 // Σ line amount
	FeesKobo    int64 // Σ line fees
	NetKobo     int64 // gross - fees
	Items       []SettlementLine
}

// SettlementFetcher is the narrow slice of a payment provider the automatic
// settlement import needs. It is deliberately NOT part of Provider: providers
// without a settlement feed (or test fakes) simply don't implement it, and
// the importer degrades to "provider does not expose settlement reports"
// instead of failing to compile.
type SettlementFetcher interface {
	// Name identifies the provider for logs/results (Provider already has it).
	Name() string
	// FetchSettlements returns every batch with a settlement_date in
	// [from, to], each carrying its transaction lines.
	FetchSettlements(ctx context.Context, from, to time.Time) ([]SettlementReport, error)
}

// ── Paystack ───────────────────────────────────────────────────────────────

const (
	settlementListPerPage = 50
	settlementTxnPerPage  = 100
)

// paystackSettlementList is the GET /settlement envelope. Paystack returns
// offset pagination metadata (total/skipped/perPage/page/pageCount).
type paystackSettlementList struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    []struct {
		ID             int64  `json:"id"`
		Status         string `json:"status"`
		Currency       string `json:"currency"`
		SettlementDate string `json:"settlement_date"`
	} `json:"data"`
	Meta struct {
		PerPage   int `json:"perPage"`
		Page      int `json:"page"`
		PageCount int `json:"pageCount"`
	} `json:"meta"`
}

// paystackSettlementTxns is the GET /settlement/{id}/transactions envelope.
type paystackSettlementTxns struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    []struct {
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Fees      int64  `json:"fees"`
	} `json:"data"`
	Meta struct {
		PerPage   int `json:"perPage"`
		Page      int `json:"page"`
		PageCount int `json:"pageCount"`
	} `json:"meta"`
}

// FetchSettlements lists settlement batches in [from, to] and loads each
// successful batch's transaction lines. Non-success batches are returned
// without lines — the importer reports them as "not final yet" instead of
// booking a payout that may still fail.
func (p *PaystackProvider) FetchSettlements(ctx context.Context, from, to time.Time) ([]SettlementReport, error) {
	var out []SettlementReport

	for page := 1; ; page++ {
		var list paystackSettlementList
		path := fmt.Sprintf("/settlement?from=%s&to=%s&page=%d&perPage=%d",
			from.Format("2006-01-02"), to.Format("2006-01-02"), page, settlementListPerPage)
		if err := p.get(ctx, path, &list); err != nil {
			return nil, fmt.Errorf("paystack settlements page %d: %w", page, err)
		}
		if !list.Status {
			return nil, fmt.Errorf("paystack settlements: %s", list.Message)
		}

		for _, b := range list.Data {
			rep := SettlementReport{
				ExternalRef: fmt.Sprintf("paystack-settlement-%d", b.ID),
				Status:      b.Status,
				Currency:    b.Currency,
			}
			if ts, err := time.Parse(time.RFC3339, b.SettlementDate); err == nil {
				rep.BatchDate = ts
			}

			if b.Status == "success" {
				items, lerr := p.fetchSettlementLines(ctx, b.ID)
				if lerr != nil {
					// A batch we cannot line-item cannot be matched or booked;
					// surface the error rather than importing it empty.
					return nil, lerr
				}
				rep.Items = items
				for _, it := range items {
					rep.GrossKobo += it.AmountKobo
					rep.FeesKobo += it.FeesKobo
				}
				rep.NetKobo = rep.GrossKobo - rep.FeesKobo
			}

			out = append(out, rep)
		}

		if len(list.Data) < settlementListPerPage ||
			(list.Meta.PageCount > 0 && page >= list.Meta.PageCount) {
			break
		}
	}

	return out, nil
}

// fetchSettlementLines pages through one batch's transactions. Lines without a
// reference are dropped: they can never match a payment we booked, and
// importing them would park real money in Suspense with nothing to name it by
// (a provider-side bookkeeping line, not a payout we owe anyone).
func (p *PaystackProvider) fetchSettlementLines(ctx context.Context, settlementID int64) ([]SettlementLine, error) {
	var lines []SettlementLine
	var dropped int

	for page := 1; ; page++ {
		var out paystackSettlementTxns
		path := fmt.Sprintf("/settlement/%d/transactions?page=%d&perPage=%d",
			settlementID, page, settlementTxnPerPage)
		if err := p.get(ctx, path, &out); err != nil {
			return nil, fmt.Errorf("paystack settlement %d transactions page %d: %w", settlementID, page, err)
		}
		if !out.Status {
			return nil, fmt.Errorf("paystack settlement %d transactions: %s", settlementID, out.Message)
		}

		for _, t := range out.Data {
			if t.Reference == "" {
				dropped++
				continue
			}
			lines = append(lines, SettlementLine{
				Reference:  t.Reference,
				AmountKobo: t.Amount,
				FeesKobo:   t.Fees,
			})
		}

		if len(out.Data) < settlementTxnPerPage ||
			(out.Meta.PageCount > 0 && page >= out.Meta.PageCount) {
			break
		}
	}

	if dropped > 0 {
		logger.Warn("paystack settlement: lines without a reference dropped",
			"settlement_id", settlementID, "dropped", dropped)
	}
	return lines, nil
}