package services

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// ═══════════════════════════════════════════════════════════════════════════
// RULE ENGINE UNIT TESTS (Phase 8.2)
//
// Each test manufactures the exact transaction pattern the rule exists to
// catch and asserts a breach — plus the negative case, because a rule that
// fires on everything is as bad as one that never fires. No database: the
// evaluators are pure.
// ═══════════════════════════════════════════════════════════════════════════

var ruleTestUser = uuid.MustParse("11111111-1111-4111-8111-111111111111")

func ruleWithParams(code string, params string) *models.RiskRule {
	return &models.RiskRule{
		Code:   code,
		Mode:   models.RiskModeMonitoring,
		Params: datatypes.JSON(params),
	}
}

// txn builds a fact with a timestamp relative to a fixed "now".
func txn(userID uuid.UUID, typ string, amountKobo int64, at time.Time) repositories.TxnFact {
	return repositories.TxnFact{
		ID: uuid.New(), UserID: userID, Type: typ, Amount: amountKobo,
		Status: "completed", CreatedAt: at,
	}
}

// ── Deposit velocity ───────────────────────────────────────────────────────

func TestDepositVelocity_BreachesAboveMax(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleDepositVelocity10m, `{"max_count":3,"window_minutes":10}`)

	// 3 deposits in 10 minutes is allowed (not ABOVE max)…
	var ok []repositories.TxnFact
	for i := 0; i < 3; i++ {
		ok = append(ok, txn(ruleTestUser, "deposit", 100000, now.Add(-time.Duration(i)*time.Minute)))
	}
	assert.Empty(t, evalDepositVelocity(rule, ok), "exactly max_count must not breach")

	// …a 4th inside the window breaks it.
	breaches := evalDepositVelocity(rule, append(ok, txn(ruleTestUser, "deposit", 100000, now)))
	require.Len(t, breaches, 1, "4th deposit in the window must alert")
	assert.Equal(t, rule.Code, breaches[0].RuleCode)
	assert.Equal(t, ruleTestUser, breaches[0].UserID)
	assert.Equal(t, models.RiskSeverityMedium, breaches[0].Severity)
}

func TestDepositVelocity_SpreadOverHoursDoesNotBreach(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleDepositVelocity10m, `{"max_count":3,"window_minutes":10}`)

	// 10 deposits, one per 15 minutes: never more than 1 in any 10-min window.
	var facts []repositories.TxnFact
	for i := 0; i < 10; i++ {
		facts = append(facts, txn(ruleTestUser, "deposit", 100000, now.Add(-time.Duration(i)*15*time.Minute)))
	}
	assert.Empty(t, evalDepositVelocity(rule, facts), "slow drip must not breach")
}

func TestDepositVelocity_FailedDepositsDoNotCount(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleDepositVelocity10m, `{"max_count":3,"window_minutes":10}`)

	var facts []repositories.TxnFact
	for i := 0; i < 5; i++ {
		f := txn(ruleTestUser, "deposit", 100000, now.Add(-time.Duration(i)*time.Minute))
		f.Status = "failed"
		facts = append(facts, f)
	}
	assert.Empty(t, evalDepositVelocity(rule, facts), "failed deposits never funded anything")
}

// ── Deposit cap (24h) ──────────────────────────────────────────────────────

func TestDepositCap_BreachesOverCap(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleDepositCap24h, `{"cap_kobo":500000000,"window_hours":24}`)

	facts := []repositories.TxnFact{
		txn(ruleTestUser, "deposit", 300000000, now.Add(-3*time.Hour)),
		txn(ruleTestUser, "deposit", 300000000, now), // total ₦6,000,000 > ₦5,000,000
	}
	breaches := evalDepositCap(rule, facts)
	require.Len(t, breaches, 1)
	assert.EqualValues(t, 600000000, breaches[0].Details["sum_kobo"])
}

func TestDepositCap_UnderCapStaysQuiet(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleDepositCap24h, `{"cap_kobo":500000000,"window_hours":24}`)

	facts := []repositories.TxnFact{
		txn(ruleTestUser, "deposit", 300000000, now.Add(-3*time.Hour)),
		txn(ruleTestUser, "deposit", 200000000, now), // exactly the cap: not over
	}
	assert.Empty(t, evalDepositCap(rule, facts))
}

// ── Structuring ────────────────────────────────────────────────────────────

func TestStructuring_ThreeNearLimitDepositsBreach(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleStructuring7d,
		`{"min_count":3,"lower_pct":90,"upper_pct":99,"reference_kobo":500000000}`)

	// Each at 95% of ₦5,000,000 — under the line, three times in a week.
	var facts []repositories.TxnFact
	for i := 0; i < 3; i++ {
		facts = append(facts, txn(ruleTestUser, "deposit", 475000000, now.Add(-time.Duration(i)*24*time.Hour)))
	}
	breaches := evalStructuring(rule, facts)
	require.Len(t, breaches, 1)
	assert.Equal(t, models.RiskSeverityHigh, breaches[0].Severity)
}

func TestStructuring_DoesNotFireBelowBandOrAboveLimit(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleStructuring7d,
		`{"min_count":3,"lower_pct":90,"upper_pct":99,"reference_kobo":500000000}`)

	// Two near-limit deposits: not enough.
	two := []repositories.TxnFact{
		txn(ruleTestUser, "deposit", 475000000, now),
		txn(ruleTestUser, "deposit", 475000000, now.Add(-24*time.Hour)),
	}
	assert.Empty(t, evalStructuring(rule, two))

	// Near-limit deposits but one is AT the threshold (100%): not structuring —
	// it would simply be reported normally.
	atLimit := append(two, txn(ruleTestUser, "deposit", 500000000, now.Add(-48*time.Hour)))
	assert.Empty(t, evalStructuring(rule, atLimit))

	// Far below the band (50%): ordinary activity.
	below := []repositories.TxnFact{
		txn(ruleTestUser, "deposit", 250000000, now),
		txn(ruleTestUser, "deposit", 250000000, now.Add(-24*time.Hour)),
		txn(ruleTestUser, "deposit", 250000000, now.Add(-48*time.Hour)),
	}
	assert.Empty(t, evalStructuring(rule, below))
}

// ── Round-amount cycling ───────────────────────────────────────────────────

func TestRoundAmountCycling_SameRoundAmountThriceBreaches(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleRoundAmountCycling, `{"min_count":3,"granularity_kobo":100000000}`)

	var facts []repositories.TxnFact
	for i := 0; i < 3; i++ {
		facts = append(facts, txn(ruleTestUser, "deposit", 100000000, now.Add(-time.Duration(i)*24*time.Hour)))
	}
	breaches := evalRoundAmountCycling(rule, facts)
	require.Len(t, breaches, 1, "3× ₦100k must alert once")
	assert.EqualValues(t, 100000000, breaches[0].Details["amount_kobo"])
}

func TestRoundAmountCycling_PerUserAndPerAmount(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleRoundAmountCycling, `{"min_count":3,"granularity_kobo":100000000}`)
	other := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	// User A cycles ₦100k, user B cycles ₦200k: two independent signals,
	// never one pooled count.
	var facts []repositories.TxnFact
	for i := 0; i < 3; i++ {
		at := now.Add(-time.Duration(i) * 24 * time.Hour)
		facts = append(facts,
			txn(ruleTestUser, "deposit", 100000000, at),
			txn(other, "deposit", 200000000, at),
		)
	}
	breaches := evalRoundAmountCycling(rule, facts)
	require.Len(t, breaches, 2, "two users cycling different round amounts are two alerts")

	// Two users each doing it twice: nothing (3 needed per user).
	var weak []repositories.TxnFact
	for i := 0; i < 2; i++ {
		at := now.Add(-time.Duration(i) * 24 * time.Hour)
		weak = append(weak,
			txn(ruleTestUser, "deposit", 100000000, at),
			txn(other, "deposit", 200000000, at),
		)
	}
	assert.Empty(t, evalRoundAmountCycling(rule, weak))
}

// ── Rapid in-and-out ───────────────────────────────────────────────────────

func TestRapidInOut_DepositThenWithdrawWithinWindowBreaches(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleRapidInOut, `{"minutes":15}`)

	facts := []repositories.TxnFact{
		txn(ruleTestUser, "deposit", 500000000, now.Add(-10*time.Minute)),
		txn(ruleTestUser, "withdrawal", 490000000, now),
	}
	breaches := evalRapidInOut(rule, facts)
	require.Len(t, breaches, 1)
	assert.Equal(t, models.RiskSeverityMedium, breaches[0].Severity)
}

func TestRapidInOut_InvestingInsteadOfWithdrawingStaysQuiet(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleRapidInOut, `{"minutes":15}`)

	facts := []repositories.TxnFact{
		txn(ruleTestUser, "deposit", 500000000, now.Add(-40*time.Minute)), // slow
		txn(ruleTestUser, "withdrawal", 490000000, now),
	}
	assert.Empty(t, evalRapidInOut(rule, facts), "40 minutes is outside the window")
}

// ── Duplicate funding ──────────────────────────────────────────────────────

func TestDuplicateFunding_OneSourceTwoWalletsBreaches(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleDuplicateFunding, `{"min_users":2}`)
	alice := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	bob := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")

	a := txn(alice, "deposit", 1000000, now.Add(-24*time.Hour))
	a.FundingEmail = "shared@example.com"
	b := txn(bob, "deposit", 1000000, now)
	b.FundingEmail = "shared@example.com"

	breaches := evalDuplicateFunding(rule, []repositories.TxnFact{a, b})
	require.Len(t, breaches, 2, "both accounts in the shared-source ring alert")
	for _, br := range breaches {
		assert.Equal(t, "shared@example.com", br.Details["funding_email"])
		assert.Equal(t, 2, br.Details["distinct_users"])
	}

	// Distinct sources: no signal.
	other := txn(bob, "deposit", 1000000, now)
	other.FundingEmail = "bob@example.com"
	assert.Empty(t, evalDuplicateFunding(rule, []repositories.TxnFact{a, other}))
}

// ── KYC name mismatch ──────────────────────────────────────────────────────

func TestKYCNameMismatch_MismatchAlertsMatchDoesNot(t *testing.T) {
	now := time.Now()
	rule := ruleWithParams(models.RuleKYCNameMismatch, `{}`)
	names := map[uuid.UUID]string{ruleTestUser: "JOHN DOE"}

	mismatch := txn(ruleTestUser, "withdrawal", 5000000, now)
	mismatch.BankAccountName = "IKECHUKWU OKAFOR"
	breaches := evalKYCNameMismatch(rule, names, []repositories.TxnFact{mismatch})
	require.Len(t, breaches, 1, "payout name unrelated to KYC name must alert")

	match := txn(ruleTestUser, "withdrawal", 5000000, now)
	match.BankAccountName = "JOHN DOE"
	assert.Empty(t, evalKYCNameMismatch(rule, names, []repositories.TxnFact{match}))

	// Bank formatting that still contains the KYC name matches (same
	// tolerance the wallet's own account-name check applies).
	tolerated := txn(ruleTestUser, "withdrawal", 5000000, now)
	tolerated.BankAccountName = "JOHN DOE JR"
	assert.Empty(t, evalKYCNameMismatch(rule, names, []repositories.TxnFact{tolerated}))
}

// ── Dedupe + params + transitions ──────────────────────────────────────────

func TestBreachDedupeKey_PartitionsByRuleUserAndWindow(t *testing.T) {
	a := breach{RuleCode: "r1", UserID: ruleTestUser, Window: "2026-10-27"}
	same := breach{RuleCode: "r1", UserID: ruleTestUser, Window: "2026-10-27"}
	otherDay := breach{RuleCode: "r1", UserID: ruleTestUser, Window: "2026-10-28"}
	otherRule := breach{RuleCode: "r2", UserID: ruleTestUser, Window: "2026-10-27"}

	assert.Equal(t, a.dedupeKey(), same.dedupeKey(), "same window → same slot → one live alert")
	assert.NotEqual(t, a.dedupeKey(), otherDay.dedupeKey())
	assert.NotEqual(t, a.dedupeKey(), otherRule.dedupeKey())
}

func TestParseRuleParams_DefaultsAndMalformed(t *testing.T) {
	empty := parseRuleParams(nil)
	assert.Equal(t, 0, empty.MaxCount, "no params → caller applies defaults")

	malformed := parseRuleParams([]byte("{not json"))
	assert.Equal(t, 0, malformed.MaxCount, "a bad admin edit must degrade to defaults, not crash")

	ok := parseRuleParams([]byte(`{"max_count":7}`))
	assert.Equal(t, 7, ok.MaxCount)
}

func TestAlertTransitions(t *testing.T) {
	assert.True(t, canTransition(models.RiskAlertOpen, models.RiskAlertInReview))
	assert.True(t, canTransition(models.RiskAlertOpen, models.RiskAlertEscalated))
	assert.True(t, canTransition(models.RiskAlertOpen, models.RiskAlertCleared))
	assert.True(t, canTransition(models.RiskAlertInReview, models.RiskAlertEscalated))
	assert.True(t, canTransition(models.RiskAlertEscalated, models.RiskAlertCleared))
	assert.True(t, canTransition(models.RiskAlertCleared, models.RiskAlertInReview), "false positives reopen")

	assert.False(t, canTransition(models.RiskAlertCleared, models.RiskAlertEscalated), "cleared → escalated must go through review")
	assert.False(t, canTransition(models.RiskAlertEscalated, models.RiskAlertOpen))
}

func TestSTROverdue(t *testing.T) {
	draft := &models.STRReport{Status: models.STRStatusDraft, DeadlineAt: time.Now().Add(-time.Hour)}
	assert.True(t, draft.IsOverdue(), "24h past escalation with no filing is overdue")

	fresh := &models.STRReport{Status: models.STRStatusDraft, DeadlineAt: time.Now().Add(23 * time.Hour)}
	assert.False(t, fresh.IsOverdue())

	filed := &models.STRReport{Status: models.STRStatusFiled, DeadlineAt: time.Now().Add(-time.Hour)}
	assert.False(t, filed.IsOverdue(), "filed reports are never overdue")
}
