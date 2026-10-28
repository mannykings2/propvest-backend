package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// These tests pin the settlement-import parsing contract against a local
// server: pagination through both list endpoints, kobo math derived from the
// lines, the stable external_ref, and honest handling of provider errors.
// They run with no network and no real keys (baseURL override).

func newSettlementFetcher(t *testing.T, handler http.HandlerFunc) *PaystackProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &PaystackProvider{
		secretKey: "sk_test_dummy",
		http:      &http.Client{Timeout: 5 * time.Second},
		baseURL:   srv.URL,
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

// TestFetchSettlements_MapsBatchAndLines covers the happy path: batch list →
// per-batch transactions, totals computed from lines (not summary fields),
// empty-reference lines dropped, non-final batches returned without lines.
func TestFetchSettlements_MapsBatchAndLines(t *testing.T) {
	const batchDate = "2026-10-26T00:00:00.000Z"

	p := newSettlementFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/settlement":
			q := r.URL.Query()
			if q.Get("from") != "2026-10-20" || q.Get("to") != "2026-10-27" {
				t.Errorf("date window not sent: from=%q to=%q", q.Get("from"), q.Get("to"))
			}
			if got := r.Header.Get("Authorization"); got != "Bearer sk_test_dummy" {
				t.Errorf("missing bearer auth: %q", got)
			}
			writeJSON(t, w, map[string]any{
				"status": true,
				"data": []map[string]any{
					{"id": 3090024, "status": "success", "currency": "NGN", "settlement_date": batchDate},
					{"id": 3090025, "status": "processing", "currency": "NGN", "settlement_date": batchDate},
				},
				"meta": map[string]any{"perPage": 50, "page": 1, "pageCount": 1},
			})
		case "/settlement/3090024/transactions":
			writeJSON(t, w, map[string]any{
				"status": true,
				"data": []map[string]any{
					{"reference": "dep-1001", "amount": 500000, "fees": 1500},
					{"reference": "dep-1002", "amount": 250000, "fees": 750},
					// A provider bookkeeping line with no reference can never
					// match a payment — must be dropped, not booked.
					{"reference": "", "amount": 100, "fees": 0},
				},
				"meta": map[string]any{"perPage": 100, "page": 1, "pageCount": 1},
			})
		case "/settlement/3090025/transactions":
			t.Errorf("non-final batch must not have its lines fetched")
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	from := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)

	reports, err := p.FetchSettlements(context.Background(), from, to)
	if err != nil {
		t.Fatalf("FetchSettlements: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("want 2 reports, got %d", len(reports))
	}

	ok := reports[0]
	if ok.ExternalRef != "paystack-settlement-3090024" {
		t.Errorf("external_ref = %q", ok.ExternalRef)
	}
	if ok.Status != "success" || ok.Currency != "NGN" {
		t.Errorf("status/currency = %q/%q", ok.Status, ok.Currency)
	}
	if want := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC); !ok.BatchDate.Equal(want) {
		t.Errorf("batch date = %v, want %v", ok.BatchDate, want)
	}
	if len(ok.Items) != 2 {
		t.Errorf("want 2 items (empty-ref dropped), got %d", len(ok.Items))
	}
	// Totals come from the lines: 750000 gross, 2250 fees, 747750 net.
	if ok.GrossKobo != 750000 || ok.FeesKobo != 2250 || ok.NetKobo != 747750 {
		t.Errorf("totals = %d/%d/%d, want 750000/2250/747750",
			ok.GrossKobo, ok.FeesKobo, ok.NetKobo)
	}

	pending := reports[1]
	if pending.ExternalRef != "paystack-settlement-3090025" {
		t.Errorf("second ref = %q", pending.ExternalRef)
	}
	if pending.Status != "processing" {
		t.Errorf("second status = %q, want processing", pending.Status)
	}
	if len(pending.Items) != 0 || pending.GrossKobo != 0 {
		t.Errorf("non-final batch must come back without lines/totals")
	}
}

// TestFetchSettlements_PaginatesBothEndpoints proves neither loop truncates:
// the batch list spans 2 pages (50 + 10) and one batch's lines span 2 pages
// (100 + 7).
func TestFetchSettlements_PaginatesBothEndpoints(t *testing.T) {
	// The single success batch lives on page 2 of the list, id 999.
	successBatch := map[string]any{
		"id": 999, "status": "success", "currency": "NGN",
		"settlement_date": "2026-10-26T00:00:00.000Z",
	}

	p := newSettlementFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		page := 1
		fmt.Sscanf(q.Get("page"), "%d", &page)

		switch r.URL.Path {
		case "/settlement":
			var data []map[string]any
			switch page {
			case 1:
				for i := 0; i < 50; i++ {
					data = append(data, map[string]any{
						"id": 1000 + i, "status": "processing",
						"currency": "NGN", "settlement_date": "2026-10-25T00:00:00.000Z",
					})
				}
			case 2:
				for i := 0; i < 9; i++ {
					data = append(data, map[string]any{
						"id": 2000 + i, "status": "processing",
						"currency": "NGN", "settlement_date": "2026-10-25T00:00:00.000Z",
					})
				}
				data = append(data, successBatch)
			default:
				t.Errorf("list page %d requested out of range", page)
			}
			writeJSON(t, w, map[string]any{
				"status": true, "data": data,
				"meta": map[string]any{"perPage": 50, "page": page, "pageCount": 2},
			})
		case "/settlement/999/transactions":
			var data []map[string]any
			switch page {
			case 1:
				for i := 0; i < 100; i++ {
					data = append(data, map[string]any{
						"reference": fmt.Sprintf("txn-%03d", i), "amount": 1000, "fees": 10,
					})
				}
			case 2:
				for i := 100; i < 107; i++ {
					data = append(data, map[string]any{
						"reference": fmt.Sprintf("txn-%03d", i), "amount": 1000, "fees": 10,
					})
				}
			default:
				t.Errorf("transactions page %d requested out of range", page)
			}
			writeJSON(t, w, map[string]any{
				"status": true, "data": data,
				"meta": map[string]any{"perPage": 100, "page": page, "pageCount": 2},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	reports, err := p.FetchSettlements(context.Background(),
		time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchSettlements: %v", err)
	}
	if len(reports) != 60 {
		t.Fatalf("want all 60 batches across 2 pages, got %d", len(reports))
	}

	last := reports[59]
	if last.ExternalRef != "paystack-settlement-999" {
		t.Errorf("last ref = %q, want the page-2 success batch", last.ExternalRef)
	}
	if len(last.Items) != 107 {
		t.Errorf("want all 107 lines across 2 pages, got %d", len(last.Items))
	}
	if last.GrossKobo != 107*1000 || last.FeesKobo != 107*10 {
		t.Errorf("totals = %d/%d, want %d/%d", last.GrossKobo, last.FeesKobo, 107*1000, 107*10)
	}
}

// TestFetchSettlements_SurfacesProviderErrors: a failed fetch must error (the
// importer logs and retries next tick), never return a silently-empty batch.
func TestFetchSettlements_SurfacesProviderErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"api says no", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, map[string]any{"status": false, "message": "Invalid API key"})
		}},
		{"http failure", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}},
		{"transactions endpoint fails", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/settlement" {
				writeJSON(t, w, map[string]any{
					"status": true,
					"data": []map[string]any{
						{"id": 7, "status": "success", "currency": "NGN", "settlement_date": "2026-10-26T00:00:00.000Z"},
					},
					"meta": map[string]any{"perPage": 50, "page": 1, "pageCount": 1},
				})
				return
			}
			http.Error(w, "boom", http.StatusBadGateway)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newSettlementFetcher(t, tc.handler)
			if _, err := p.FetchSettlements(context.Background(),
				time.Now().AddDate(0, 0, -7), time.Now()); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
}