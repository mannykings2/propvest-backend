package main

import (
	"context"
	"time"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// PRIVACY SWEEPER (Phase 9 — NDPA retention + erasure)
// ═══════════════════════════════════════════════════════════════════════════
//
// Runs at boot (catch up on a downtime that spanned a deadline) and daily:
//
//   - EXPIRE data-export bundles whose 24h link window passed. The bundle PII
//     is wiped from the row, not merely unlinked — retention minimalism.
//   - EXECUTE erasure requests whose 30-day grace has ended: identity
//     anonymised, financial ledger retained (statutory exception), sessions
//     revoked. Approving a request in the admin tab accelerates execute_after
//     to now, so the next sweep (or the boot pass) picks it up.
const privacySweepInterval = 24 * time.Hour

func startPrivacySweeper(ctx context.Context, privacy services.PrivacyService) func() {
	ticker := time.NewTicker(privacySweepInterval)
	done := make(chan struct{})

	run := func() {
		if n, err := privacy.ExpireExportBundles(ctx); err != nil {
			logger.Error("privacy sweep: export expiry failed", "error", err)
		} else if n > 0 {
			logger.Info("privacy sweep: export bundles expired and wiped", "count", n)
		}

		if n, err := privacy.ExecuteDueDeletions(ctx); err != nil {
			logger.Error("privacy sweep: erasure execution failed", "error", err)
		} else if n > 0 {
			logger.Info("privacy sweep: erasures executed", "count", n)
		}
	}

	go func() {
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