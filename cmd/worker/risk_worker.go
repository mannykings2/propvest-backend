package main

import (
	"context"
	"time"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// RISK WORKER (Phase 8.2 / 8.8)
// ═══════════════════════════════════════════════════════════════════════════
//
// Two schedules, both starting immediately on boot so a freshly restarted
// worker catches up on whatever the last process missed:
//
//   - EVERY 5 MIN  AML monitoring scan — evaluates every enabled monitoring
//     rule over a 7-day transaction feed and files one live alert per pattern
//     window (velocity, cap, structuring, round-amount cycling, rapid
//     in-out, duplicate funding, KYC-name mismatch). Blocking rules are NOT
//     part of this pass: they run synchronously on the withdrawal path.
//   - HOURLY       CTR daily scan — per-user deposit totals against the
//     ₦5M/₦10M thresholds for the previous day, producing dated report rows
//     with a 7-day filing clock. Idempotent by unique key, so the hourly
//     repeat is free.
//
// The re-screen reminder (8.3) is deliberately NOT a job: it is a query the
// admin dashboard runs on demand (GET /admin/risk/rescreen).
const (
	riskScanInterval = 5 * time.Minute
	ctrScanInterval  = time.Hour
)

func startRiskWorker(ctx context.Context, risk services.RiskService) func() {
	scanTicker := time.NewTicker(riskScanInterval)
	ctrTicker := time.NewTicker(ctrScanInterval)
	done := make(chan struct{})

	runScan := func() {
		result, err := risk.RunScan(ctx)
		if err != nil {
			logger.Error("risk scan failed", "error", err)
			return
		}
		if result.Created > 0 || len(result.Breaches) > 0 {
			logger.Warn("risk scan: alerts filed",
				"created", result.Created,
				"duplicates", result.Duplicates,
				"rules_run", result.RulesRun,
				"breaches", result.Breaches)
		} else {
			logger.Info("risk scan complete",
				"facts", result.FactsScanned, "rules_run", result.RulesRun,
				"duplicates", result.Duplicates)
		}
	}

	runCTR := func() {
		// Yesterday: a day is complete before it can be reported.
		n, err := risk.RunCTRScan(ctx, time.Now().AddDate(0, 0, -1))
		if err != nil {
			logger.Error("ctr scan failed", "error", err)
			return
		}
		if n > 0 {
			logger.Warn("ctr scan: new report rows", "created", n)
		}
	}

	go func() {
		// Immediate first pass: a gap in monitoring is never recovered by
		// waiting for the first tick.
		runScan()
		runCTR()

		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-scanTicker.C:
				runScan()
			case <-ctrTicker.C:
				runCTR()
			}
		}
	}()

	return func() {
		close(done)
		scanTicker.Stop()
		ctrTicker.Stop()
	}
}
