package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	apperrors "github.com/mannykings2/propvest-backend/internal/errors"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════════════
// RISK LAYER INTEGRATION (Phase 8.1 / 8.2 / 8.4 / 8.7 / 8.8)
//
// Real Postgres, because the guarantees under test are DB guarantees: the
// partial unique index (exactly ONE live alert per pattern), the ON CONFLICT
// dedupe, the audit rows written inside the triage transaction, and the CTR
// unique key. Every acceptance line in the plan's Phase 8 table gets exercised
// here through the real service.
// ═══════════════════════════════════════════════════════════════════════════

// riskDeps wires the real service: repository + audit service, no stubs.
// EnsureRules re-seeds the rule set first — the cleanup's
// `TRUNCATE users CASCADE` wipes risk_rules through its updated_by FK, and
// golang-migrate never re-runs a finished migration.
func riskDeps(t *testing.T, tdb *TestDB) services.RiskService {
	t.Helper()
	svc := services.NewRiskService(
		repositories.NewRiskRepository(tdb.DB),
		repositories.NewUserRepository(tdb.DB),
		services.NewAuditService(repositories.NewAuditRepository(tdb.DB)),
		tdb.DB,
	)
	require.NoError(t, svc.EnsureRules(context.Background()), "seed risk rules")
	return svc
}

// seedRiskTxn inserts a wallet transaction with a controllable timestamp —
// the feed the monitoring rules read. metadata may be "" (stored NULL).
func seedRiskTxn(t *testing.T, tdb *TestDB, walletID, userID uuid.UUID, typ string, amountKobo int64, at time.Time, metadata string) {
	t.Helper()
	require.NoError(t, tdb.DB.Exec(`
		INSERT INTO wallet_transactions
			(id, wallet_id, user_id, type, amount, balance_before, balance_after,
			 reference, description, status, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, 0, 0, ?, '', 'completed', NULLIF(?, '')::jsonb, ?)`,
		uuid.New(), walletID, userID, typ, amountKobo,
		"RISK-"+uuid.New().String(), metadata, at,
	).Error)
}

// seedRiskUser creates a user + wallet row (FK targets).
func seedRiskUser(t *testing.T, tdb *TestDB, email string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	userID := seedUser(t, tdb, email)
	seedWallet(t, tdb, userID, 0)
	// Scan via string: gorm's raw scan does not route through google/uuid's
	// Scanner here, so parse explicitly rather than fight the driver.
	var walletIDStr string
	require.NoError(t, tdb.DB.Raw(`SELECT id::text FROM wallets WHERE user_id = ?`, userID).Scan(&walletIDStr).Error)
	walletID, err := uuid.Parse(walletIDStr)
	require.NoError(t, err)
	return userID, walletID
}

func auditCount(t *testing.T, tdb *TestDB, action string) int64 {
	t.Helper()
	return countRows(t, tdb,
		`SELECT COUNT(*) FROM audit_logs WHERE action = ?`, action)
}

// ── 8.1: seeding + "exactly one alert" + audited resolution ────────────────

func TestRisk_ScanCreatesExactlyOneAlertAndResolutionIsAudited(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	risk := riskDeps(t, tdb)

	// The migration seeded the whole rule set.
	rules, err := risk.ListRules(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rules), 9, "migration 000032 must seed every v1 rule")
	var blocking, monitoring int
	for _, r := range rules {
		switch r.Mode {
		case models.RiskModeBlocking:
			blocking++
		case models.RiskModeMonitoring:
			monitoring++
		}
	}
	assert.GreaterOrEqual(t, blocking, 3, "the three blocking rules")
	assert.GreaterOrEqual(t, monitoring, 6, "the monitoring rules")

	// Manufacture a deposit-velocity breach: 4 deposits inside 10 minutes
	// (rule default max_count=3).
	userID, walletID := seedRiskUser(t, tdb, "risk.velocity@example.com")
	now := time.Now()
	for i := 0; i < 4; i++ {
		seedRiskTxn(t, tdb, walletID, userID, "deposit", 500000, now.Add(-time.Duration(i)*time.Minute), "")
	}

	// First scan files the alert…
	res1, err := risk.RunScan(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, res1.Created, 1, "scan must file the velocity alert")

	live := countRows(t, tdb, `
		SELECT COUNT(*) FROM risk_alerts
		WHERE user_id = ? AND rule_code = ? AND status IN ('open','in_review')`,
		userID, models.RuleDepositVelocity10m)
	require.EqualValues(t, 1, live, "8.1: a rule breach creates EXACTLY one alert")

	// …and the second scan dedupes instead of duplicating.
	_, err = risk.RunScan(ctx)
	require.NoError(t, err)
	live = countRows(t, tdb, `
		SELECT COUNT(*) FROM risk_alerts
		WHERE user_id = ? AND rule_code = ? AND status IN ('open','in_review')`,
		userID, models.RuleDepositVelocity10m)
	assert.EqualValues(t, 1, live, "re-running the scan must not duplicate a live alert")

	// Triage it: open → in_review → cleared (notes required for cleared).
	alerts, _, err := risk.ListAlerts(ctx, services.AlertQuery{
		Status: models.RiskAlertOpen, UserID: &userID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, alerts)
	alertID := alerts[0].ID

	actor := services.Actor{ID: userID, Role: "admin"}

	_, err = risk.TriageAlert(ctx, actor, alertID, services.AlertTriageInput{
		Status: strPtr(models.RiskAlertCleared),
	})
	require.Error(t, err, "clearing without resolution notes must be refused")

	_, err = risk.TriageAlert(ctx, actor, alertID, services.AlertTriageInput{
		Status: strPtr(models.RiskAlertInReview),
	})
	require.NoError(t, err)

	cleared, err := risk.TriageAlert(ctx, actor, alertID, services.AlertTriageInput{
		Status:          strPtr(models.RiskAlertCleared),
		ResolutionNotes: strPtr("verified: legitimate salary credits"),
	})
	require.NoError(t, err)
	assert.Equal(t, models.RiskAlertCleared, cleared.Status)
	require.NotNil(t, cleared.ResolutionNotes)

	// 8.1: resolving wrote an audit row (same transaction as the change).
	assert.GreaterOrEqual(t, auditCount(t, tdb, models.AuditActionRiskAlertUpdated), int64(2),
		"each triage write leaves an audit row")
}

// ── 8.4: rule toggle stops alerts; re-enabling resumes ─────────────────────

func TestRisk_RuleToggleStopsAlertsThenResumes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	risk := riskDeps(t, tdb)
	actor := services.Actor{ID: seedUser(t, tdb, "risk.admin@example.com"), Role: "admin"}

	// Round-amount pattern ready to fire: 3× ₦100k in a week.
	userID, walletID := seedRiskUser(t, tdb, "risk.round@example.com")
	now := time.Now()
	for i := 0; i < 3; i++ {
		seedRiskTxn(t, tdb, walletID, userID, "deposit", 100000000, now.Add(-time.Duration(i)*24*time.Hour), "")
	}

	// Disable the rule FIRST.
	updated, err := risk.UpdateRule(ctx, actor, models.RuleRoundAmountCycling,
		services.RuleUpdateInput{Enabled: boolPtr(false)})
	require.NoError(t, err)
	assert.False(t, updated.Enabled)

	_, err = risk.RunScan(ctx)
	require.NoError(t, err)
	assert.Zero(t, countRows(t, tdb,
		`SELECT COUNT(*) FROM risk_alerts WHERE user_id = ? AND rule_code = ?`,
		userID, models.RuleRoundAmountCycling),
		"a disabled rule must not raise alerts")

	// Re-enable → the very next scan files it.
	_, err = risk.UpdateRule(ctx, actor, models.RuleRoundAmountCycling,
		services.RuleUpdateInput{Enabled: boolPtr(true)})
	require.NoError(t, err)

	res, err := risk.RunScan(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, res.Created, 1, "re-enabled rule must resume alerting")
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM risk_alerts WHERE user_id = ? AND rule_code = ?`,
		userID, models.RuleRoundAmountCycling))

	// The toggle itself is audited (both directions).
	assert.GreaterOrEqual(t, auditCount(t, tdb, models.AuditActionRiskRuleUpdated), int64(2),
		"disable and enable each leave an audit row")

	// Threshold edits take effect immediately (max_count raised above supply).
	_, err = risk.UpdateRule(ctx, actor, models.RuleRoundAmountCycling, services.RuleUpdateInput{
		Params: []byte(`{"min_count":10,"granularity_kobo":100000000}`),
	})
	require.NoError(t, err)
	// Clear the live alert, rescan with the stricter threshold → no new alert.
	_, err = risk.TriageAlert(ctx, actor, mustAlertID(t, tdb, userID, models.RuleRoundAmountCycling),
		services.AlertTriageInput{
			Status:          strPtr(models.RiskAlertCleared),
			ResolutionNotes: strPtr("rule tuned"),
		})
	require.NoError(t, err)
	_, err = risk.RunScan(ctx)
	require.NoError(t, err)
	assert.Zero(t, countRows(t, tdb,
		`SELECT COUNT(*) FROM risk_alerts WHERE user_id = ? AND rule_code = ? AND status IN ('open','in_review')`,
		userID, models.RuleRoundAmountCycling), "edited threshold (min_count=10) must stop the alert")
}

// ── 8.2 blocking: velocity, flag, new-account, name mismatch ───────────────

func TestRisk_BlockingRulesDenyWithdrawals(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	risk := riskDeps(t, tdb)

	// 1. Velocity: 5 withdrawals in 24h already → the 6th is denied.
	vUser, vWallet := seedRiskUser(t, tdb, "risk.velocity.block@example.com")
	now := time.Now()
	for i := 0; i < 5; i++ {
		seedRiskTxn(t, tdb, vWallet, vUser, "withdrawal", 10000000, now.Add(-time.Duration(i)*time.Hour), "")
	}
	err := risk.CheckWithdrawalAllowed(ctx, vUser, 10000000, "Test Investor") // ₦100k
	require.ErrorIs(t, err, apperrors.ErrRiskBlocked, "6th withdrawal in 24h must be denied")
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM risk_alerts WHERE user_id = ? AND rule_code = ?`,
		vUser, models.RuleWithdrawVelocity24h),
		"the denial itself files an alert explaining why")

	// Under the limit: allowed (same account, 1 withdrawal so far).
	fUser, fWallet := seedRiskUser(t, tdb, "risk.fresh@example.com")
	seedRiskTxn(t, tdb, fWallet, fUser, "withdrawal", 10000000, now.Add(-time.Hour), "")
	require.NoError(t, risk.CheckWithdrawalAllowed(ctx, fUser, 10000000, "Test Investor"),
		"one withdrawal against a max of five must pass")

	// 2. New-account high value: fresh account + ₦2,000,000 → denied.
	err = risk.CheckWithdrawalAllowed(ctx, fUser, 200000000, "Test Investor") // ₦2,000,000
	require.ErrorIs(t, err, apperrors.ErrRiskBlocked, "new account high-value withdrawal must be denied")
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM risk_alerts WHERE user_id = ? AND rule_code = ?`,
		fUser, models.RuleNewAccountHighValue))

	// 3. Manual flag: small amount, old account — only the flag can deny it.
	gUser, _ := seedRiskUser(t, tdb, "risk.flagged@example.com")
	require.NoError(t, tdb.DB.Exec(`UPDATE users SET created_at = now() - INTERVAL '90 days' WHERE id = ?`, gUser).Error)
	actor := services.Actor{ID: seedUser(t, tdb, "risk.admin2@example.com"), Role: "admin"}
	require.NoError(t, risk.FlagUser(ctx, actor, gUser, true, "pending source-of-funds check"))
	err = risk.CheckWithdrawalAllowed(ctx, gUser, 10000000, "Test Investor")
	require.ErrorIs(t, err, apperrors.ErrRiskBlocked, "flagged user must be denied")
	assert.GreaterOrEqual(t, auditCount(t, tdb, models.AuditActionUserRiskFlagged), int64(1))

	// Clearing the flag reopens the tap.
	require.NoError(t, risk.FlagUser(ctx, actor, gUser, false, "source of funds received"))
	require.NoError(t, risk.CheckWithdrawalAllowed(ctx, gUser, 10000000, "Test Investor"))

	// 4. Name mismatch: ALERTS but never blocks.
	nUser, _ := seedRiskUser(t, tdb, "risk.namemismatch@example.com")
	require.NoError(t, tdb.DB.Exec(`UPDATE users SET created_at = now() - INTERVAL '90 days' WHERE id = ?`, nUser).Error)
	err = risk.CheckWithdrawalAllowed(ctx, nUser, 10000000, "SOMEONE ELSE ENTIRELY")
	require.NoError(t, err, "a name mismatch is monitoring, not a block")
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM risk_alerts WHERE user_id = ? AND rule_code = ?`,
		nUser, models.RuleKYCNameMismatch))
}

// ── 8.3 + 8.7: screening hit → flag + escalated alert + STR draft ──────────

func TestRisk_ScreeningHitFreezesUserAndDraftsSTR(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	risk := riskDeps(t, tdb)
	actor := services.Actor{ID: seedUser(t, tdb, "risk.kyc.admin@example.com"), Role: "admin"}

	userID, _ := seedRiskUser(t, tdb, "risk.hit@example.com")
	require.NoError(t, tdb.DB.Exec(`UPDATE users SET created_at = now() - INTERVAL '90 days' WHERE id = ?`, userID).Error)

	// A clear screening changes nothing.
	require.NoError(t, risk.RecordScreeningResult(ctx, actor, userID, "OFAC SDN (manual check)", "clear", ""))
	var flagged bool
	require.NoError(t, tdb.DB.Raw(`SELECT risk_flagged FROM users WHERE id = ?`, userID).Scan(&flagged).Error)
	require.False(t, flagged, "a clear screening must not freeze anyone")

	// A HIT freezes the user, escalates an alert and drafts the STR.
	require.NoError(t, risk.RecordScreeningResult(ctx, actor, userID, "NFIU watchlist (manual check)", "hit", "name + DOB match"))

	require.NoError(t, tdb.DB.Raw(`SELECT risk_flagged FROM users WHERE id = ?`, userID).Scan(&flagged).Error)
	require.True(t, flagged, "8.3: a hit must freeze the account for review")
	assert.GreaterOrEqual(t, auditCount(t, tdb, models.AuditActionUserRiskFlagged), int64(1))

	assert.EqualValues(t, 1, countRows(t, tdb, `
		SELECT COUNT(*) FROM risk_alerts
		WHERE user_id = ? AND rule_code = ? AND status = 'escalated' AND severity = 'critical'`,
		userID, models.RuleScreeningHit), "the hit lands as an escalated critical alert")

	// 8.7: escalation auto-drafted an STR with a 24-hour deadline.
	var str models.STRReport
	require.NoError(t, tdb.DB.Raw(`
		SELECT s.* FROM str_reports s
		JOIN risk_alerts a ON a.id = s.alert_id
		WHERE a.user_id = ? AND a.rule_code = ?`, userID, models.RuleScreeningHit).Scan(&str).Error)
	assert.Equal(t, models.STRStatusDraft, str.Status)
	assert.False(t, str.IsOverdue(), "fresh draft is inside the 24h window")
	assert.WithinDuration(t, time.Now().Add(24*time.Hour), str.DeadlineAt, time.Hour,
		"NFIU window is 24 hours from determination")

	// screening_results captured the row and linked it to the alert.
	assert.EqualValues(t, 2, countRows(t, tdb,
		`SELECT COUNT(*) FROM screening_results WHERE user_id = ?`, userID),
		"clear + hit both land in screening_results (8.3)")
	assert.EqualValues(t, 1, countRows(t, tdb, `
		SELECT COUNT(*) FROM screening_results
		WHERE user_id = ? AND result = 'hit' AND alert_id IS NOT NULL`, userID),
		"the hit row links to its escalated alert")

	// Withdrawals now blocked by the flagged-user rule.
	err := risk.CheckWithdrawalAllowed(ctx, userID, 10000000, "Test Investor")
	require.ErrorIs(t, err, apperrors.ErrRiskBlocked, "freeze blocks withdrawals")

	// A second hit in the same week dedupes: still one live alert.
	require.NoError(t, risk.RecordScreeningResult(ctx, actor, userID, "NFIU watchlist (manual check)", "hit", "re-check"))
	assert.EqualValues(t, 1, countRows(t, tdb, `
		SELECT COUNT(*) FROM risk_alerts WHERE user_id = ? AND rule_code = ?`,
		userID, models.RuleScreeningHit), "repeat hits must not flood the queue")
	assert.EqualValues(t, 1, countRows(t, tdb,
		`SELECT COUNT(*) FROM str_reports s JOIN risk_alerts a ON a.id = s.alert_id WHERE a.user_id = ?`, userID),
		"one STR per escalated alert, never a second draft")
}

// ── 8.7: triage escalation → STR draft; filing needs the goAML ref ─────────

func TestRisk_EscalationDraftsSTRAndFilingIsAudited(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	risk := riskDeps(t, tdb)
	adminID := seedUser(t, tdb, "risk.escalate.admin@example.com")
	actor := services.Actor{ID: adminID, Role: "admin"}

	// Structuring pattern → open alert.
	userID, walletID := seedRiskUser(t, tdb, "risk.structuring@example.com")
	now := time.Now()
	for i := 0; i < 3; i++ {
		seedRiskTxn(t, tdb, walletID, userID, "deposit", 475000000, now.Add(-time.Duration(i)*24*time.Hour), "")
	}
	_, err := risk.RunScan(ctx)
	require.NoError(t, err)
	alertID := mustAlertID(t, tdb, userID, models.RuleStructuring7d)

	// Escalate → STR drafted with its 24h clock.
	esc, err := risk.TriageAlert(ctx, actor, alertID, services.AlertTriageInput{
		Status: strPtr(models.RiskAlertEscalated),
	})
	require.NoError(t, err)
	assert.Equal(t, models.RiskAlertEscalated, esc.Status)

	strs, _, err := risk.ListSTR(ctx, models.STRStatusDraft, 1, 10)
	require.NoError(t, err)
	require.NotEmpty(t, strs, "escalation must auto-draft an STR")
	draft := strs[0]
	require.NotNil(t, draft.PreparedBy)
	assert.Equal(t, adminID, *draft.PreparedBy)
	assert.Contains(t, draft.Narrative, models.RuleStructuring7d, "narrative names the rule")

	// Filing without the goAML reference is refused.
	_, err = risk.UpdateSTR(ctx, actor, draft.ID, services.STRUpdateInput{
		Status: strPtr(models.STRStatusFiled),
	})
	require.Error(t, err, "filing without a goAML reference must be refused")

	// With the reference it files and audits.
	filed, err := risk.UpdateSTR(ctx, actor, draft.ID, services.STRUpdateInput{
		Status:    strPtr(models.STRStatusFiled),
		GoAMLRef:  strPtr("GOAML-2026-000123"),
		Narrative: strPtr("Three deposits at 95% of the ₦5m threshold within 72 hours. Reported under MLPPA 2022."),
	})
	require.NoError(t, err)
	assert.Equal(t, models.STRStatusFiled, filed.Status)
	require.NotNil(t, filed.FiledAt)
	require.NotNil(t, filed.GoAMLRef)
	assert.Equal(t, "GOAML-2026-000123", *filed.GoAMLRef)
	assert.False(t, filed.IsOverdue())

	assert.GreaterOrEqual(t, auditCount(t, tdb, models.AuditActionSTRFiled), int64(1),
		"filing the STR leaves an audit row")

	// Closing the alert still requires resolution notes — the rule never
	// relaxes, not even after the STR has been filed.
	_, err = risk.TriageAlert(ctx, actor, alertID, services.AlertTriageInput{
		Status: strPtr(models.RiskAlertCleared),
	})
	require.Error(t, err, "clearing without notes stays refused even post-filing")

	_, err = risk.TriageAlert(ctx, actor, alertID, services.AlertTriageInput{
		Status:          strPtr(models.RiskAlertCleared),
		ResolutionNotes: strPtr("STR filed with NFIU; case closed"),
	})
	require.NoError(t, err)
}

// ── 8.8: CTR daily scan → dated row with a 7-day clock, idempotent ─────────

func TestRisk_CTRScanProducesIdempotentDatedRows(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	risk := riskDeps(t, tdb)
	actor := services.Actor{ID: seedUser(t, tdb, "risk.ctr.admin@example.com"), Role: "admin"}

	userID, walletID := seedRiskUser(t, tdb, "risk.ctr@example.com")

	// Yesterday: ₦6,000,000 in deposits → above the ₦5M individual threshold.
	yesterday := time.Now().AddDate(0, 0, -1)
	seedRiskTxn(t, tdb, walletID, userID, "deposit", 400000000, yesterday.Add(10*time.Hour), "")
	seedRiskTxn(t, tdb, walletID, userID, "deposit", 200000000, yesterday.Add(12*time.Hour), "")

	created, err := risk.RunCTRScan(ctx, yesterday)
	require.NoError(t, err)
	require.Equal(t, 1, created, "one CTR row per user per day")

	// Hourly worker repeats the same scan → no second row.
	created, err = risk.RunCTRScan(ctx, yesterday)
	require.NoError(t, err)
	assert.Zero(t, created, "the scan is idempotent across hourly runs")

	// The row carries the threshold, totals and the 7-day clock.
	ctr, _, err := risk.ListCTR(ctx, models.CTRStatusOpen, 1, 10)
	require.NoError(t, err)
	require.Len(t, ctr, 1)
	row := ctr[0]
	assert.EqualValues(t, 600000000, row.AmountKobo)
	assert.EqualValues(t, models.CTRThresholdIndividualKobo, row.ThresholdKobo)
	assert.Equal(t, 2, row.TxnCount)
	assert.WithinDuration(t, row.WindowDate.AddDate(0, 0, 7), row.DeadlineAt, time.Minute,
		"CTR filing window is 7 days")

	// Filing requires notes and is audited.
	require.Error(t, risk.FileCTR(ctx, actor, row.ID, ""), "filing without notes must be refused")
	require.NoError(t, risk.FileCTR(ctx, actor, row.ID, "confirmed filed with NFIU"))
	assert.GreaterOrEqual(t, auditCount(t, tdb, models.AuditActionCTRFiled), int64(1))
}

// ── 8.3: the re-screen reminder (monthly) ──────────────────────────────────

func TestRisk_RescreenReminderListsStaleApprovedUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tdb := SetupTestDB(t)
	defer tdb.Cleanup()
	ctx := context.Background()
	risk := riskDeps(t, tdb)
	actor := services.Actor{ID: seedUser(t, tdb, "risk.rescreen.admin@example.com"), Role: "admin"}

	stale, _ := seedRiskUser(t, tdb, "risk.stale@example.com")
	fresh, _ := seedRiskUser(t, tdb, "risk.fresh.screen@example.com")
	never, _ := seedRiskUser(t, tdb, "risk.never.screened@example.com")

	// All three are approved KYC holders (the reminder only covers them).
	for _, id := range []uuid.UUID{stale, fresh, never} {
		require.NoError(t, tdb.DB.Exec(
			`UPDATE users SET kyc_status = 'approved', is_active = true WHERE id = ?`, id).Error)
	}
	// stale: last checked 45 days ago; fresh: checked today.
	require.NoError(t, tdb.DB.Exec(`
		INSERT INTO screening_results (id, user_id, kind, source, result, screened_at)
		VALUES (?, ?, 'initial', 'OFAC SDN (manual check)', 'clear', now() - INTERVAL '45 days'),
		       (?, ?, 'initial', 'OFAC SDN (manual check)', 'clear', now())`,
		uuid.New(), stale, uuid.New(), fresh).Error)

	require.NoError(t, risk.RecordScreeningResult(ctx, actor, fresh, "OFAC SDN (manual check)", "clear", ""))

	due, err := risk.RescreenDue(ctx)
	require.NoError(t, err)

	ids := map[uuid.UUID]bool{}
	for _, d := range due {
		ids[d.UserID] = true
	}
	assert.True(t, ids[stale], "a 45-day-old check is due for re-screening")
	assert.True(t, ids[never], "a never-screened approved user is due")
	assert.False(t, ids[fresh], "checked today → not on the reminder")
}

// ── helpers ────────────────────────────────────────────────────────────────

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// mustAlertID loads the live alert for a (user, rule) pair.
func mustAlertID(t *testing.T, tdb *TestDB, userID uuid.UUID, rule string) uuid.UUID {
	t.Helper()
	var idStr string
	require.NoError(t, tdb.DB.Raw(`
		SELECT id::text FROM risk_alerts
		WHERE user_id = ? AND rule_code = ? AND status IN ('open','in_review')
		ORDER BY created_at DESC LIMIT 1`, userID, rule).Scan(&idStr).Error)
	id, err := uuid.Parse(idStr)
	require.NoError(t, err, "expected a live alert for rule %s", rule)
	return id
}
