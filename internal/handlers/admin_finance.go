package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// AdminFinanceHandler serves the Money layer's admin surface (Phase 7.8):
// fee revenue, settlement batches, and the nightly trust-reconciliation history.
// Everything the ledger knows about is inspectable — and where it makes sense,
// triggerable — from the admin dashboard.
type AdminFinanceHandler struct {
	finance services.FinanceService
	ledger  services.LedgerService
}

func NewAdminFinanceHandler(finance services.FinanceService, ledger services.LedgerService) *AdminFinanceHandler {
	return &AdminFinanceHandler{finance: finance, ledger: ledger}
}

// GetFees handles GET /api/v1/admin/finance/fees
// → fee revenue by type, the active rules, what's sweepable, recent charges.
func (h *AdminFinanceHandler) GetFees(c *gin.Context) {
	report, err := h.finance.FeeReport(c.Request.Context())
	if err != nil {
		logger.Error("fee report failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to build fee report", nil)
		return
	}
	response.Success(c, http.StatusOK, report)
}

// ListSettlements handles GET /api/v1/admin/finance/settlements?page=&limit=
func (h *AdminFinanceHandler) ListSettlements(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	batches, total, err := h.finance.ListSettlements(c.Request.Context(), page, limit)
	if err != nil {
		logger.Error("settlement list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list settlements", nil)
		return
	}
	response.Paginated(c, batches, page, limit, total)
}

// IngestSettlement handles POST /api/v1/admin/finance/settlements
//
// Books a provider payout report. Unmatched lines land in Suspense (9000) and
// flag the batch (Phase 7.5/7.9) instead of being silently absorbed.
func (h *AdminFinanceHandler) IngestSettlement(c *gin.Context) {
	var req services.SettlementBatchInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}
	if len(req.Items) == 0 {
		response.Error(c, http.StatusUnprocessableEntity, "a settlement batch needs at least one line", nil)
		return
	}

	batch, err := h.finance.IngestSettlementBatch(c.Request.Context(), req)
	if err != nil {
		logger.Error("settlement ingest failed", "error", err)
		response.Error(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusCreated, "settlement batch ingested", batch)
}

// ImportSettlements handles POST /api/v1/admin/finance/settlements/import
//
// Forces the same pull the worker runs on schedule (Paystack has no
// settlement webhook) — an operator can sync right after a payout lands
// instead of assembling the report by hand. Idempotent: already-imported
// batches are skipped, never double-booked.
func (h *AdminFinanceHandler) ImportSettlements(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days < 1 || days > 31 {
		days = 7
	}

	res, err := h.finance.ImportSettlements(c.Request.Context(), time.Now().AddDate(0, 0, -days))
	if err != nil {
		if errors.Is(err, services.ErrNoSettlementSource) {
			// The configured provider has no settlement feed (mock) — that is
			// an environment answer, not a server fault.
			response.Error(c, http.StatusNotImplemented, err.Error(), nil)
			return
		}
		logger.Error("settlement import failed", "error", err)
		response.Error(c, http.StatusBadGateway, "settlement import failed", nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "settlement import completed", res)
}

// ListReconciliation handles GET /api/v1/admin/finance/reconciliation?page=&limit=
func (h *AdminFinanceHandler) ListReconciliation(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	runs, total, err := h.finance.ListReconciliationRuns(c.Request.Context(), page, limit)
	if err != nil {
		logger.Error("reconciliation list failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "failed to list reconciliation runs", nil)
		return
	}
	response.Paginated(c, runs, page, limit, total)
}

// RunReconciliation handles POST /api/v1/admin/finance/reconciliation/run
//
// Executes the trust checks on demand — the worker runs the same code nightly;
// this lets an operator prove the books after an incident or a manual fix.
func (h *AdminFinanceHandler) RunReconciliation(c *gin.Context) {
	run, err := h.ledger.RunReconciliation(c.Request.Context(), "manual")
	if err != nil {
		logger.Error("reconciliation run failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "reconciliation failed to run", nil)
		return
	}
	// A failed check is still a successful run of the check — 200 with the
	// status inside, so the admin UI can render the discrepancy.
	response.SuccessWithMessage(c, http.StatusOK, "reconciliation completed", run)
}

// SweepFees handles POST /api/v1/admin/finance/fees/sweep
//
// The explicit move of earned fees into the operating account (Phase 7.7) —
// the only legal path for user money to become platform money.
func (h *AdminFinanceHandler) SweepFees(c *gin.Context) {
	swept, err := h.ledger.SweepFees(c.Request.Context())
	if err != nil {
		logger.Error("fee sweep failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "fee sweep failed", nil)
		return
	}
	response.SuccessWithMessage(c, http.StatusOK, "fee sweep completed", gin.H{"swept_kobo": swept})
}
