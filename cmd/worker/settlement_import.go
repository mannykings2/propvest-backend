package main

import (
	"context"
	"time"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// SETTLEMENT IMPORT WORKER (Phase 7.5/7.9 — automatic payout ingestion)
// ═══════════════════════════════════════════════════════════════════════════
//
// Paystack has no settlement webhook, so batches arrive by polling its
// Settlements API. This worker pulls them into settlement_batches on a
// schedule instead of an operator POSTing the report by hand:
//
//   - BOOT     — catch up on anything that landed while the worker was down.
//   - 6 HOURLY — Paystack settles T+1 on working days; a 6-hour cadence books
//     each payout the same day it lands, with three tries per day in case one
//     fetch fails transiently.
//
// The pass is idempotent (deduped on external_ref), so overlapping runs and
// the admin "import now" button can never double-book a batch. The 7-day
// lookback means a batch skipped while "processing" gets picked up by a later
// pass without unbounded API history scans.
const (
	settlementImportInterval = 6 * time.Hour
	settlementImportLookback = 7 * 24 * time.Hour
)

func startSettlementImportWorker(ctx context.Context, finance services.FinanceService) func() {
	ticker := time.NewTicker(settlementImportInterval)
	done := make(chan struct{})

	run := func() {
		res, err := finance.ImportSettlements(ctx, time.Now().Add(-settlementImportLookback))
		if err != nil {
			logger.Error("settlement import failed", "error", err)
			return
		}
		if res.Created > 0 {
			logger.Info("settlement batches imported",
				"provider", res.Provider, "created", res.Created,
				"skipped", res.Skipped, "refs", res.BatchRefs)
		}
	}

	go func() {
		// Immediate first pass: a restarted worker still books yesterday's
		// payout without waiting six hours.
		run()

		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()

	return func() {
		close(done)
		ticker.Stop()
	}
}