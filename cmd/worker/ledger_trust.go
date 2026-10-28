package main

import (
	"context"
	"time"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// LEDGER TRUST WORKER (Phase 7.6 / 7.7)
// ═══════════════════════════════════════════════════════════════════════════
//
// Two schedules, both starting immediately on boot so a freshly restarted
// worker proves the books before anything else needs them:
//
//   - HOURLY   fee sweep — the explicit DR 1000 / CR 1010 move of earned fees
//     into the operating account. The only legal path for user money to become
//     platform money; nothing else touches custody except a fee rule or refund.
//   - DAILY    trust reconciliation — trial balance, per-user sub-ledgers vs
//     wallet columns, custody coverage, suspense == 0. A failure is stored in
//     reconciliation_runs with alerted=true, which the admin dashboard shows.
const (
	feeSweepInterval   = time.Hour
	reconTrustInterval = 24 * time.Hour
)

func startLedgerTrustWorker(ctx context.Context, ledger services.LedgerService) func() {
	sweepTicker := time.NewTicker(feeSweepInterval)
	reconTicker := time.NewTicker(reconTrustInterval)
	done := make(chan struct{})

	runSweep := func() {
		swept, err := ledger.SweepFees(ctx)
		if err != nil {
			logger.Error("ledger trust: fee sweep failed", "error", err)
			return
		}
		if swept > 0 {
			logger.Info("ledger trust: fee swept", "swept_kobo", swept)
		}
	}
	runRecon := func() {
		if _, err := ledger.RunReconciliation(ctx, "nightly"); err != nil {
			logger.Error("ledger trust: reconciliation failed to run", "error", err)
		}
	}

	go func() {
		// Immediate first pass: prove the books at boot.
		runSweep()
		runRecon()

		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-sweepTicker.C:
				runSweep()
			case <-reconTicker.C:
				runRecon()
			}
		}
	}()

	return func() {
		close(done)
		sweepTicker.Stop()
		reconTicker.Stop()
	}
}
