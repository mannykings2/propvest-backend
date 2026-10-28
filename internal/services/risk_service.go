package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ═══════════════════════════════════════════════════════════════════════════
// RISK SERVICE (Phase 8 — AML, fraud monitoring & STR flow)
//
// Three jobs:
//   1. BLOCKING rules — checked synchronously on the withdrawal path
//      (CheckWithdrawalAllowed). A denial writes an alert so the queue shows
//      why a user was refused, then returns ErrRiskBlocked.
//   2. MONITORING rules — RunScan evaluates the pure functions in
//      risk_rules.go over a 7-day transaction feed and files one LIVE alert
//      per pattern window (dedupe via the partial unique index).
//   3. The administrative surface — triage, rule toggles, screening records,
//      STR drafts/filings and CTR scans. Every write is audited.
//
// Fail-open on rule-loading errors: a transient read failure must not freeze
// withdrawals (the withdrawal's own DB transaction is the real gate). Every
// such failure is logged at error level.
// ═══════════════════════════════════════════════════════════════════════════

// AlertQuery filters the triage queue.
type AlertQuery struct {
	UserID   *uuid.UUID
	Status   string
	Severity string
	RuleCode string
	Page     int
	Limit    int
}

// Actor is the admin on whose behalf an audited write happens.
type Actor struct {
	ID   uuid.UUID
	Role string
}

// AlertTriageInput is the PATCH /admin/risk/alerts/:id payload.
type AlertTriageInput struct {
	Status          *string `json:"status"`
	AssigneeID      *string `json:"assignee_id"`
	ResolutionNotes *string `json:"resolution_notes"`
}

// RuleUpdateInput is the PATCH /admin/risk/rules/:code payload. Both fields
// optional: pass Enabled to toggle, Params to edit thresholds (or both).
type RuleUpdateInput struct {
	Enabled *bool           `json:"enabled"`
	Params  json.RawMessage `json:"params"`
}

// STRUpdateInput is the PATCH /admin/risk/str/:id payload.
type STRUpdateInput struct {
	Narrative *string `json:"narrative"`
	Status    *string `json:"status"` // draft | filed
	GoAMLRef  *string `json:"goaml_ref"`
}

// RiskScanResult reports what one monitoring pass did (admin "run scan now"
// button and the worker's logs both consume it).
type RiskScanResult struct {
	FactsScanned int      `json:"facts_scanned"`
	Created      int      `json:"created"`
	Duplicates   int      `json:"duplicates"`
	RulesRun     int      `json:"rules_run"`
	Breaches     []string `json:"breaches,omitempty"` // human-readable hits
}

// RiskService is the Phase 8 contract.
type RiskService interface {
	// EnsureRules fails boot when risk_rules is empty (migrations seed it) —
	// silently running with zero rules would disable every control.
	EnsureRules(ctx context.Context) error

	// CheckWithdrawalAllowed runs every enabled BLOCKING rule plus the
	// sync-evaluated name-mismatch alert. Returns apperrors.ErrRiskBlocked
	// (or nil) — never partial data.
	CheckWithdrawalAllowed(ctx context.Context, userID uuid.UUID, amountKobo int64, bankAccountName string) error

	// RunScan evaluates every enabled MONITORING rule and files alerts.
	RunScan(ctx context.Context) (RiskScanResult, error)
	// RunCTRScan produces day's cash-threshold report rows (8.8), idempotent.
	RunCTRScan(ctx context.Context, day time.Time) (int, error)

	ListAlerts(ctx context.Context, q AlertQuery) ([]models.RiskAlert, int64, error)
	GetAlert(ctx context.Context, id uuid.UUID) (*models.RiskAlert, error)
	TriageAlert(ctx context.Context, actor Actor, id uuid.UUID, in AlertTriageInput) (*models.RiskAlert, error)
	FlagUser(ctx context.Context, actor Actor, userID uuid.UUID, flag bool, reason string) error

	ListRules(ctx context.Context) ([]models.RiskRule, error)
	UpdateRule(ctx context.Context, actor Actor, code string, in RuleUpdateInput) (*models.RiskRule, error)

	// RecordScreeningResult is the 8.3 hook behind the 6.3 attestation: it
	// writes the screening_results row and, on a hit, flags the user and
	// raises an ESCALATED alert with a drafted STR.
	RecordScreeningResult(ctx context.Context, actor Actor, userID uuid.UUID, source, result, notes string) error
	ListScreening(ctx context.Context, result string, page, limit int) ([]models.ScreeningResultRow, int64, error)
	RescreenDue(ctx context.Context) ([]repositories.StaleScreening, error)

	ListSTR(ctx context.Context, status string, page, limit int) ([]models.STRReport, int64, error)
	UpdateSTR(ctx context.Context, actor Actor, id uuid.UUID, in STRUpdateInput) (*models.STRReport, error)

	ListCTR(ctx context.Context, status string, page, limit int) ([]models.CTRReport, int64, error)
	FileCTR(ctx context.Context, actor Actor, id uuid.UUID, notes string) error
}

type riskService struct {
	riskRepo repositories.RiskRepository
	userRepo repositories.UserRepository
	auditSvc AuditService
	db       *gorm.DB
}

// NewRiskService constructs the service.
func NewRiskService(
	riskRepo repositories.RiskRepository,
	userRepo repositories.UserRepository,
	auditSvc AuditService,
	db *gorm.DB,
) RiskService {
	return &riskService{riskRepo: riskRepo, userRepo: userRepo, auditSvc: auditSvc, db: db}
}

// feedWindow is the widest monitoring window (the 7-day rules).
const feedWindow = 7 * 24 * time.Hour

func (s *riskService) EnsureRules(ctx context.Context) error {
	// Seed first (idempotent, never overwrites admin edits), then verify: a
	// rules table with zero rows would silently disable every control.
	for _, seed := range riskRuleSeeds() {
		row := seed
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoNothing: true,
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("risk: seed rule %s: %w", seed.Code, err)
		}
	}

	rules, err := s.riskRepo.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("risk: load rules: %w", err)
	}
	if len(rules) == 0 {
		return fmt.Errorf("risk: risk_rules is empty after seeding — check migrations (000032)")
	}
	var blocking int
	for _, r := range rules {
		if r.Mode == models.RiskModeBlocking {
			blocking++
		}
	}
	logger.Info("risk rules ready", "total", len(rules), "blocking", blocking)
	return nil
}

// ── Blocking path (8.2) ────────────────────────────────────────────────────

func (s *riskService) CheckWithdrawalAllowed(ctx context.Context, userID uuid.UUID, amountKobo int64, bankAccountName string) error {
	rules, err := s.riskRepo.ListRules(ctx)
	if err != nil {
		logger.Error("risk: rule load failed — allowing withdrawal (fail-open)", "error", err, "user_id", userID)
		return nil
	}
	enabled := map[string]*models.RiskRule{}
	for i := range rules {
		if rules[i].Enabled {
			enabled[rules[i].Code] = &rules[i]
		}
	}

	// Monitoring-evaluated here, synchronously, because the bank-verified
	// name only exists at initiation: alert, but never block.
	if rule, ok := enabled[models.RuleKYCNameMismatch]; ok && bankAccountName != "" {
		user, uerr := s.userRepo.FindByID(ctx, userID)
		if uerr == nil && user != nil && !accountNamesMatch(user.FullName, bankAccountName) {
			s.fileBreach(ctx, rule, breach{
				RuleCode: rule.Code, UserID: userID,
				Severity: models.RiskSeverityMedium,
				Window:   weekBucket(time.Now()),
				Details:  map[string]interface{}{"kyc_name": user.FullName, "bank_name": bankAccountName, "withdrawal_kobo": amountKobo},
			})
		}
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		logger.Error("risk: user load failed — allowing withdrawal (fail-open)", "error", err, "user_id", userID)
		return nil
	}

	// 1. Manual flag / screening hit — the account is under freeze-review.
	if rule, ok := enabled[models.RuleRiskFlaggedUser]; ok && user.RiskFlagged {
		reason := "account is flagged for risk review"
		if user.RiskFlagReason != nil && *user.RiskFlagReason != "" {
			reason = *user.RiskFlagReason
		}
		s.deny(ctx, rule, userID, amountKobo, "high",
			map[string]interface{}{"reason": reason})
		return apperrors.ErrRiskBlocked
	}

	// 2. New-account high-value withdrawal (mule/drain pattern).
	if rule, ok := enabled[models.RuleNewAccountHighValue]; ok {
		p := parseRuleParams(rule.Params)
		ageDays := p.or(7, p.AgeDays)
		threshold := p.AmountKobo
		if threshold <= 0 {
			threshold = models.CTRThresholdIndividualKobo / 5 // ₦1,000,000 default
		}
		if time.Since(user.CreatedAt) <= time.Duration(ageDays)*24*time.Hour && amountKobo >= threshold {
			s.deny(ctx, rule, userID, amountKobo, "high", map[string]interface{}{
				"account_age_hours": int(time.Since(user.CreatedAt).Hours()),
				"age_days":          ageDays, "threshold_kobo": threshold,
			})
			return apperrors.ErrRiskBlocked
		}
	}

	// 3. Withdrawal velocity: > max_count in the rolling window.
	if rule, ok := enabled[models.RuleWithdrawVelocity24h]; ok {
		p := parseRuleParams(rule.Params)
		maxCount := p.or(5, p.MaxCount)
		hours := p.or(24, p.WindowHours)
		since := time.Now().Add(-time.Duration(hours) * time.Hour)
		n, cerr := s.riskRepo.CountWithdrawalsSince(ctx, userID, since)
		if cerr != nil {
			logger.Error("risk: velocity count failed — allowing withdrawal (fail-open)", "error", cerr, "user_id", userID)
		} else if n >= int64(maxCount) {
			s.deny(ctx, rule, userID, amountKobo, "medium", map[string]interface{}{
				"count": n, "max_count": maxCount, "window_hours": hours,
			})
			return apperrors.ErrRiskBlocked
		}
	}

	return nil
}

// deny files the alert a blocking rule's refusal produced, then returns the
// user-facing error. The alert write is best-effort: a failed insert must not
// swallow the denial itself (the withdrawal is still refused either way).
func (s *riskService) deny(ctx context.Context, rule *models.RiskRule, userID uuid.UUID, amountKobo int64, severity string, details map[string]interface{}) {
	details["amount_kobo"] = amountKobo
	details["action"] = "withdrawal_denied"
	s.fileBreach(ctx, rule, breach{
		RuleCode: rule.Code, UserID: userID,
		Severity: severity,
		Window:   hourBucket(time.Now()),
		Details:  details,
	})
	logger.Warn("risk: withdrawal denied by rule",
		"rule", rule.Code, "user_id", userID, "amount_kobo", amountKobo)
}

// fileBreach persists one breach as an alert, swallowing the duplicate case.
func (s *riskService) fileBreach(ctx context.Context, rule *models.RiskRule, b breach) {
	details, _ := json.Marshal(b.Details)
	alert := &models.RiskAlert{
		UserID:      b.UserID,
		RuleCode:    b.RuleCode,
		Severity:    b.Severity,
		SubjectType: "user",
		SubjectID:   b.SubjectID,
		DedupeKey:   b.dedupeKey(),
		Details:     details,
		Status:      models.RiskAlertOpen,
	}
	if err := s.riskRepo.CreateAlert(ctx, alert); err != nil {
		if err == repositories.ErrAlertExists {
			return // already queued — the dedupe contract working
		}
		logger.Error("risk: alert write failed", "rule", b.RuleCode, "user_id", b.UserID, "error", err)
	}
}

// ── Monitoring scan (8.2) ──────────────────────────────────────────────────

func (s *riskService) RunScan(ctx context.Context) (RiskScanResult, error) {
	var result RiskScanResult

	rules, err := s.riskRepo.ListRules(ctx)
	if err != nil {
		return result, fmt.Errorf("risk: load rules: %w", err)
	}

	facts, err := s.riskRepo.FeedSince(ctx, time.Now().Add(-feedWindow))
	if err != nil {
		return result, fmt.Errorf("risk: load feed: %w", err)
	}
	result.FactsScanned = len(facts)

	// KYC names for the name-mismatch rule (only fetched if that rule runs).
	var names map[uuid.UUID]string
	needNames := false
	for i := range rules {
		if rules[i].Enabled && rules[i].Code == models.RuleKYCNameMismatch {
			needNames = true
		}
	}
	if needNames {
		names, err = s.kycNames(ctx)
		if err != nil {
			logger.Error("risk: kyc name load failed — name-mismatch rule skipped", "error", err)
		}
	}

	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.Mode != models.RiskModeMonitoring {
			continue
		}
		// screening_hit is flow-raised (RecordScreeningResult), never scanned.
		if rule.Code == models.RuleScreeningHit {
			continue
		}
		result.RulesRun++

		var hits []breach
		switch rule.Code {
		case models.RuleDepositVelocity10m:
			hits = evalDepositVelocity(rule, facts)
		case models.RuleDepositCap24h:
			hits = evalDepositCap(rule, facts)
		case models.RuleStructuring7d:
			hits = evalStructuring(rule, facts)
		case models.RuleRoundAmountCycling:
			hits = evalRoundAmountCycling(rule, facts)
		case models.RuleRapidInOut:
			hits = evalRapidInOut(rule, facts)
		case models.RuleDuplicateFunding:
			hits = evalDuplicateFunding(rule, facts)
		case models.RuleKYCNameMismatch:
			hits = evalKYCNameMismatch(rule, names, facts)
		default:
			// Unknown/extra rule row: nothing to evaluate — visible in the
			// admin rules table rather than crashing the scan.
			result.RulesRun--
			logger.Warn("risk: no evaluator for rule", "rule", rule.Code)
		}

		for _, b := range hits {
			err := s.fileBreachChecked(ctx, rule, b)
			switch err {
			case nil:
				result.Created++
				result.Breaches = append(result.Breaches, fmt.Sprintf("%s → user %s", b.RuleCode, b.UserID))
			case repositories.ErrAlertExists:
				result.Duplicates++
			default:
				logger.Error("risk: alert write failed", "rule", b.RuleCode, "error", err)
			}
		}
	}

	if result.Created > 0 {
		// One system audit row per productive pass — alert creation itself is
		// too chatty to audit individually, but the pass is a security event.
		s.audit(ctx, NewSystemAuditEvent("risk.scan.completed", "risk_alerts", nil))
	}
	return result, nil
}

// fileBreachChecked is fileBreach with the dedupe verdict returned.
func (s *riskService) fileBreachChecked(ctx context.Context, rule *models.RiskRule, b breach) error {
	details, _ := json.Marshal(b.Details)
	return s.riskRepo.CreateAlert(ctx, &models.RiskAlert{
		UserID:      b.UserID,
		RuleCode:    b.RuleCode,
		Severity:    b.Severity,
		SubjectType: "user",
		SubjectID:   b.SubjectID,
		DedupeKey:   b.dedupeKey(),
		Details:     details,
		Status:      models.RiskAlertOpen,
	})
}

// kycNames loads full names for the name-mismatch comparison.
func (s *riskService) kycNames(ctx context.Context) (map[uuid.UUID]string, error) {
	type row struct {
		ID       uuid.UUID
		FullName string
	}
	var rows []row
	err := s.db.WithContext(ctx).
		Raw(`SELECT id, full_name FROM users WHERE deleted_at IS NULL`).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.FullName
	}
	return out, nil
}

// ── Triage (8.4) ───────────────────────────────────────────────────────────

// allowedTransitions pins the lifecycle: open → in_review → escalated/cleared,
// with a reopen path (cleared → in_review) for false positives.
var allowedTransitions = map[string][]string{
	models.RiskAlertOpen:      {models.RiskAlertInReview, models.RiskAlertEscalated, models.RiskAlertCleared},
	models.RiskAlertInReview:  {models.RiskAlertEscalated, models.RiskAlertCleared, models.RiskAlertOpen},
	models.RiskAlertEscalated: {models.RiskAlertInReview, models.RiskAlertCleared},
	models.RiskAlertCleared:   {models.RiskAlertInReview},
}

func canTransition(from, to string) bool {
	for _, ok := range allowedTransitions[from] {
		if ok == to {
			return true
		}
	}
	return false
}

func (s *riskService) ListAlerts(ctx context.Context, q AlertQuery) ([]models.RiskAlert, int64, error) {
	page, limit := normalizePageLimit(q.Page, q.Limit)
	return s.riskRepo.ListAlerts(ctx, repositories.AlertFilter{
		UserID:   q.UserID,
		Status:   q.Status,
		Severity: q.Severity,
		RuleCode: q.RuleCode,
	}, limit, (page-1)*limit)
}

func (s *riskService) GetAlert(ctx context.Context, id uuid.UUID) (*models.RiskAlert, error) {
	return s.riskRepo.GetAlert(ctx, id)
}

func (s *riskService) TriageAlert(ctx context.Context, actor Actor, id uuid.UUID, in AlertTriageInput) (*models.RiskAlert, error) {
	alert, err := s.riskRepo.GetAlert(ctx, id)
	if err != nil {
		return nil, err
	}

	patch := map[string]interface{}{}

	// Status transition + rules around it.
	if in.Status != nil && *in.Status != alert.Status {
		to := *in.Status
		switch to {
		case models.RiskAlertOpen, models.RiskAlertInReview, models.RiskAlertEscalated, models.RiskAlertCleared:
		default:
			return nil, fmt.Errorf("unknown alert status %q", to)
		}
		if !canTransition(alert.Status, to) {
			return nil, fmt.Errorf("invalid transition %s → %s", alert.Status, to)
		}
		if to == models.RiskAlertCleared && strings.TrimSpace(strOrEmpty(in.ResolutionNotes)) == "" {
			return nil, fmt.Errorf("resolution notes are required to clear an alert")
		}
		patch["status"] = to
		if to == models.RiskAlertCleared {
			now := time.Now()
			patch["resolved_at"] = now
		} else {
			// Reopening from cleared (or moving forward) clears the stale stamp.
			patch["resolved_at"] = nil
		}
	}
	if in.ResolutionNotes != nil {
		patch["resolution_notes"] = *in.ResolutionNotes
	}
	if in.AssigneeID != nil {
		if *in.AssigneeID == "" {
			patch["assignee_id"] = nil
		} else {
			aid, perr := uuid.Parse(*in.AssigneeID)
			if perr != nil {
				return nil, fmt.Errorf("invalid assignee_id")
			}
			patch["assignee_id"] = aid
		}
	}
	if len(patch) == 0 {
		return alert, nil // nothing to change
	}

	// Escalation side-effect: draft the STR package with its 24h deadline.
	draftSTR := false
	if v, ok := patch["status"].(string); ok && v == models.RiskAlertEscalated {
		draftSTR = true
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.riskRepo.UpdateAlert(ctx, id, patch, tx); uerr != nil {
			return uerr
		}
		return s.auditTx(ctx, tx, NewAdminAuditEvent(
			actor.ID, actor.Role,
			models.AuditActionRiskAlertUpdated,
			"risk_alert", &id,
			alertReason(patch),
		))
	})
	if err != nil {
		return nil, err
	}

	if draftSTR {
		if serr := s.draftSTR(ctx, actor, id); serr != nil {
			logger.Error("risk: STR draft failed", "alert_id", id, "error", serr)
		}
	}

	return s.riskRepo.GetAlert(ctx, id)
}

func (s *riskService) draftSTR(ctx context.Context, actor Actor, alertID uuid.UUID) error {
	// Idempotent: an alert can be escalated only once (transition table), but
	// a reopen→re-escalate cycle must not duplicate the report.
	if _, err := s.riskRepo.GetSTRByAlert(ctx, alertID); err == nil {
		return nil
	}
	alert, err := s.riskRepo.GetAlert(ctx, alertID)
	if err != nil {
		return err
	}
	narrative := fmt.Sprintf(
		"Automated draft — alert %s (%s) escalated by %s at %s.\n\n"+
			"Rule: %s. Severity: %s. Subject user: %s.\n\n"+
			"Analyst narrative: ",
		alert.ID, alert.RuleCode, actor.Role, time.Now().UTC().Format(time.RFC3339),
		alert.RuleCode, alert.Severity, alert.UserID,
	)
	report := &models.STRReport{
		AlertID:    alertID,
		Narrative:  narrative,
		PreparedBy: &actor.ID,
		Status:     models.STRStatusDraft,
		DeadlineAt: time.Now().Add(24 * time.Hour), // NFIU: within 24h of determination
	}
	if err := s.riskRepo.CreateSTR(ctx, report); err != nil {
		return err
	}
	return s.audit(ctx, NewSystemAuditEvent("str.drafted", "str_report", &report.ID))
}

// FlagUser sets or clears the AML freeze marker (8.4). A flagged user's
// withdrawals are refused by the blocking rule.
func (s *riskService) FlagUser(ctx context.Context, actor Actor, userID uuid.UUID, flag bool, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("a reason is required")
	}
	if _, err := s.userRepo.FindByID(ctx, userID); err != nil {
		return err
	}

	patch := map[string]interface{}{"risk_flagged": flag}
	action := models.AuditActionUserRiskFlagged
	if flag {
		now := time.Now()
		patch["risk_flag_reason"] = reason
		patch["risk_flagged_at"] = now
		patch["risk_flagged_by"] = actor.ID
	} else {
		action = models.AuditActionUserRiskUnflagged
		patch["risk_flag_reason"] = nil
		patch["risk_flagged_at"] = nil
		patch["risk_flagged_by"] = nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.userRepo.UpdatePartial(ctx, userID, patch, tx); uerr != nil {
			return uerr
		}
		return s.auditTx(ctx, tx, NewAdminAuditEvent(actor.ID, actor.Role, action, "user", &userID, reason))
	})
}

// ── Rules (8.4) ────────────────────────────────────────────────────────────

func (s *riskService) ListRules(ctx context.Context) ([]models.RiskRule, error) {
	return s.riskRepo.ListRules(ctx)
}

func (s *riskService) UpdateRule(ctx context.Context, actor Actor, code string, in RuleUpdateInput) (*models.RiskRule, error) {
	if in.Enabled == nil && len(in.Params) == 0 {
		return nil, fmt.Errorf("nothing to update: send enabled and/or params")
	}
	var params []byte
	if len(in.Params) > 0 {
		if !json.Valid(in.Params) {
			return nil, fmt.Errorf("params must be valid JSON")
		}
		var probe map[string]interface{}
		if err := json.Unmarshal(in.Params, &probe); err != nil {
			return nil, fmt.Errorf("params must be a JSON object")
		}
		params = in.Params
	}

	var updated *models.RiskRule
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Track the previous state for the audit row.
		prev, gerr := s.riskRepo.GetRule(ctx, code)
		if gerr != nil {
			return gerr
		}
		upd, uerr := s.riskRepo.UpdateRule(ctx, code, in.Enabled, params, actor.ID, tx)
		if uerr != nil {
			return uerr
		}
		updated = upd

		changes := map[string]interface{}{}
		if in.Enabled != nil {
			changes["enabled"] = map[string]interface{}{"from": prev.Enabled, "to": upd.Enabled}
		}
		if params != nil {
			changes["params"] = map[string]interface{}{"from": string(prev.Params), "to": string(params)}
		}
		reason := fmt.Sprintf("rule %s updated", code)
		if in.Enabled != nil {
			if *in.Enabled {
				reason = fmt.Sprintf("rule %s enabled", code)
			} else {
				reason = fmt.Sprintf("rule %s DISABLED", code)
			}
		}
		evt := NewAdminAuditEvent(actor.ID, actor.Role, models.AuditActionRiskRuleUpdated, "risk_rule", nil, reason)
		evt.NewValues = changes
		evt.OldValues = map[string]interface{}{"enabled": prev.Enabled, "params": string(prev.Params)}
		return s.auditTx(ctx, tx, evt)
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ── Screening (8.3) ────────────────────────────────────────────────────────

func (s *riskService) RecordScreeningResult(ctx context.Context, actor Actor, userID uuid.UUID, source, result, notes string) error {
	if result != models.ScreeningClear && result != models.ScreeningHit {
		return fmt.Errorf("screening result must be 'clear' or 'hit'")
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("screening source is required")
	}

	var rowID uuid.UUID
	var alertID uuid.UUID

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// kind: first check ever = initial, anything after = rescreen.
		var prior int64
		if err := tx.Model(&models.ScreeningResultRow{}).Where("user_id = ?", userID).Count(&prior).Error; err != nil {
			return err
		}
		kind := models.ScreeningKindInitial
		if prior > 0 {
			kind = models.ScreeningKindRescreen
		}
		var notesPtr *string
		if strings.TrimSpace(notes) != "" {
			notesPtr = &notes
		}

		if result == models.ScreeningHit {
			// A hit is an immediate freeze-review: escalated alert, STR draft
			// on its heels, user flagged so withdrawals stop.
			alert := &models.RiskAlert{
				UserID:      userID,
				RuleCode:    models.RuleScreeningHit,
				Severity:    models.RiskSeverityCritical,
				SubjectType: "user",
				DedupeKey:   fmt.Sprintf("%s:%s:%s", models.RuleScreeningHit, userID, weekBucket(time.Now())),
				Status:      models.RiskAlertEscalated,
			}
			details, _ := json.Marshal(map[string]interface{}{
				"source": source, "notes": notes, "action": "freeze_review",
			})
			alert.Details = details
			if err := s.riskRepo.CreateAlert(ctx, alert, tx); err != nil && err != repositories.ErrAlertExists {
				return err
			}
			if alert.ID != uuid.Nil {
				alertID = alert.ID
			} else {
				// Deduped: reuse the live alert so the screening links to it.
				if live, lerr := s.riskRepo.GetAlertByDedupe(ctx, alert.DedupeKey); lerr == nil {
					alertID = live.ID
				}
			}

			if err := s.flagUserTx(ctx, tx, userID, true,
				"screening hit: "+source, actor.ID); err != nil {
				return err
			}
			// The freeze itself is security-relevant: audit it separately from
			// the attestation row below (the admin needs to see WHO froze the
			// account and why, independent of the screening record).
			flagReason := "screening hit: " + source
			if notes != "" {
				flagReason += " — " + notes
			}
			if err := s.auditTx(ctx, tx, NewAdminAuditEvent(actor.ID, actor.Role,
				models.AuditActionUserRiskFlagged, "user", &userID, flagReason)); err != nil {
				return err
			}
		}

		row := &models.ScreeningResultRow{
			UserID:     userID,
			Kind:       kind,
			Source:     source,
			Result:     result,
			Notes:      notesPtr,
			ScreenedBy: &actor.ID,
			ScreenedAt: time.Now(),
		}
		if alertID != uuid.Nil {
			row.AlertID = &alertID
		}
		if err := s.riskRepo.CreateScreening(ctx, row, tx); err != nil {
			return err
		}
		rowID = row.ID

		// Audit: the 6.3 endpoint audits its own kyc_submissions write; this
		// records the risk-layer side (freeze on hit is security-relevant).
		evt := NewAdminAuditEvent(actor.ID, actor.Role, models.AuditActionScreeningRecorded, "screening_result", &rowID,
			fmt.Sprintf("screening %s recorded (%s)", result, source))
		if result == models.ScreeningHit {
			evt.NewValues = map[string]interface{}{"hit": true, "user_flagged": true, "alert_id": alertID}
		}
		return s.auditTx(ctx, tx, evt)
	})
	if err != nil {
		return err
	}

	// Escalated hit → draft the STR (its own small tx; failure is logged and
	// visible as a missing draft on the STR screen).
	if alertID != uuid.Nil {
		if derr := s.draftSTR(ctx, actor, alertID); derr != nil {
			logger.Error("risk: STR draft after screening hit failed", "alert_id", alertID, "error", derr)
		}
	}
	return nil
}

// flagUserTx is the transactional half of FlagUser (used by the hit path).
func (s *riskService) flagUserTx(ctx context.Context, tx *gorm.DB, userID uuid.UUID, flag bool, reason string, actorID uuid.UUID) error {
	now := time.Now()
	patch := map[string]interface{}{
		"risk_flagged":     flag,
		"risk_flag_reason": reason,
		"risk_flagged_at":  now,
		"risk_flagged_by":  actorID,
	}
	return s.userRepo.UpdatePartial(ctx, userID, patch, tx)
}

func (s *riskService) ListScreening(ctx context.Context, result string, page, limit int) ([]models.ScreeningResultRow, int64, error) {
	page, limit = normalizePageLimit(page, limit)
	return s.riskRepo.ListScreening(ctx, result, limit, (page-1)*limit)
}

// rescreenInterval is the periodic re-check cadence (8.3: monthly reminder).
const rescreenInterval = 30 * 24 * time.Hour

func (s *riskService) RescreenDue(ctx context.Context) ([]repositories.StaleScreening, error) {
	return s.riskRepo.StaleScreenings(ctx, time.Now().Add(-rescreenInterval), 50)
}

// ── STR (8.7) ──────────────────────────────────────────────────────────────

func (s *riskService) ListSTR(ctx context.Context, status string, page, limit int) ([]models.STRReport, int64, error) {
	page, limit = normalizePageLimit(page, limit)
	return s.riskRepo.ListSTR(ctx, status, limit, (page-1)*limit)
}

func (s *riskService) UpdateSTR(ctx context.Context, actor Actor, id uuid.UUID, in STRUpdateInput) (*models.STRReport, error) {
	patch := map[string]interface{}{}

	if in.Narrative != nil {
		patch["narrative"] = *in.Narrative
	}
	if in.Status != nil {
		to := *in.Status
		if to != models.STRStatusDraft && to != models.STRStatusFiled {
			return nil, fmt.Errorf("unknown STR status %q", to)
		}
		patch["status"] = to
		if to == models.STRStatusFiled {
			ref := strings.TrimSpace(strOrEmpty(in.GoAMLRef))
			if ref == "" {
				return nil, fmt.Errorf("goaml_ref is required to mark an STR filed")
			}
			now := time.Now()
			patch["filed_at"] = now
			patch["goaml_ref"] = ref
		}
	}
	if len(patch) == 0 {
		return nil, fmt.Errorf("nothing to update")
	}

	// A filed STR can be reopened only by clearing its filing stamps.
	if in.Status != nil && *in.Status == models.STRStatusDraft {
		patch["filed_at"] = nil
		patch["goaml_ref"] = nil
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.riskRepo.UpdateSTR(ctx, id, patch, tx); uerr != nil {
			return uerr
		}
		action := models.AuditActionSTRUpdated
		reason := "STR report updated"
		if v, ok := patch["status"].(string); ok && v == models.STRStatusFiled {
			action = models.AuditActionSTRFiled
			reason = "STR filed with goAML reference " + fmt.Sprint(patch["goaml_ref"])
		}
		return s.auditTx(ctx, tx, NewAdminAuditEvent(actor.ID, actor.Role, action, "str_report", &id, reason))
	})
	if err != nil {
		return nil, err
	}

	var out models.STRReport
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// ── CTR (8.8) ──────────────────────────────────────────────────────────────

func (s *riskService) RunCTRScan(ctx context.Context, day time.Time) (int, error) {
	day = day.UTC()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	type total struct {
		UserID uuid.UUID
		Sum    int64
		Count  int64
	}
	var rows []total
	err := s.db.WithContext(ctx).Raw(`
		SELECT user_id, COALESCE(SUM(amount),0) AS sum, COUNT(*) AS count
		FROM wallet_transactions
		WHERE type = 'deposit' AND status = 'completed'
		  AND created_at >= ? AND created_at < ?
		GROUP BY user_id
		HAVING COALESCE(SUM(amount),0) >= ?`,
		start, end, models.CTRThresholdIndividualKobo).
		Scan(&rows).Error
	if err != nil {
		return 0, err
	}

	created := 0
	for _, r := range rows {
		// Highest applicable threshold: ≥₦10M is the corporate figure, else
		// the ₦5M individual one. One row per user/day.
		threshold := int64(models.CTRThresholdIndividualKobo)
		if r.Sum >= models.CTRThresholdCorporateKobo {
			threshold = models.CTRThresholdCorporateKobo
		}
		note := "Auto-detected: daily deposit volume reached the CTR threshold. " +
			"Electronic transfers — confirm whether a cash CTR applies before filing."
		report := &models.CTRReport{
			UserID:        r.UserID,
			WindowDate:    start,
			AmountKobo:    r.Sum,
			TxnCount:      int(r.Count),
			ThresholdKobo: threshold,
			Status:        models.CTRStatusOpen,
			DeadlineAt:    start.AddDate(0, 0, 7), // CTR within 7 days
			Notes:         &note,
		}
		made, cerr := s.riskRepo.CreateCTR(ctx, report)
		if cerr != nil {
			return created, cerr
		}
		if made {
			created++ // idempotent: an existing row for this day is not "new"
		}
	}
	return created, nil
}

func (s *riskService) ListCTR(ctx context.Context, status string, page, limit int) ([]models.CTRReport, int64, error) {
	page, limit = normalizePageLimit(page, limit)
	return s.riskRepo.ListCTR(ctx, status, limit, (page-1)*limit)
}

func (s *riskService) FileCTR(ctx context.Context, actor Actor, id uuid.UUID, notes string) error {
	if strings.TrimSpace(notes) == "" {
		return fmt.Errorf("filing notes are required")
	}
	report, err := s.riskRepo.GetCTR(ctx, id)
	if err != nil {
		return err
	}
	if report.Status == models.CTRStatusFiled {
		return fmt.Errorf("CTR report is already filed")
	}
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if uerr := s.riskRepo.UpdateCTR(ctx, id, map[string]interface{}{
			"status": "filed", "filed_at": now, "notes": notes,
		}); uerr != nil {
			return uerr
		}
		return s.auditTx(ctx, tx, NewAdminAuditEvent(actor.ID, actor.Role, models.AuditActionCTRFiled, "ctr_report", &id, notes))
	})
}

// ── audit helpers ──────────────────────────────────────────────────────────

func (s *riskService) audit(ctx context.Context, event AuditEvent) error {
	// auditRepository.Create dereferences the tx handle, so system events
	// (no admin actor) still need a real transaction to record against.
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.auditSvc.Record(ctx, tx, event)
	})
}

func (s *riskService) auditTx(ctx context.Context, tx *gorm.DB, event AuditEvent) error {
	return s.auditSvc.Record(ctx, tx, event)
}

func alertReason(patch map[string]interface{}) string {
	if v, ok := patch["status"].(string); ok {
		switch v {
		case models.RiskAlertEscalated:
			return "alert escalated (STR clock started)"
		case models.RiskAlertCleared:
			return "alert cleared"
		case models.RiskAlertInReview:
			return "alert moved to review"
		}
	}
	return "alert updated"
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func normalizePageLimit(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}
