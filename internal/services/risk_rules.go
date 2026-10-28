package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"gorm.io/datatypes"
)

// ═══════════════════════════════════════════════════════════════════════════
// RISK RULE ENGINE (Phase 8.2) — pure evaluators
//
// Every monitoring rule is a pure function: (facts, params) → breaches. No
// database, no clock reads (facts carry their own timestamps), so each rule is
// unit-testable by manufacturing a transaction pattern and asserting the
// breach — exactly the acceptance test the plan asks for.
//
// Thresholds never appear as literals in evaluation code: they arrive as
// ruleParams parsed from risk_rules.params (JSONB), which the admin UI edits
// through PATCH /admin/risk/rules/:code.
// ═══════════════════════════════════════════════════════════════════════════

// ruleParams is the parsed form of risk_rules.params. Zero values mean "use
// the default" so a rule row with '{}' still behaves sanely.
type ruleParams struct {
	MaxCount        int     `json:"max_count"`
	WindowMinutes   int     `json:"window_minutes"`
	WindowHours     int     `json:"window_hours"`
	WindowDays      int     `json:"window_days"`
	CapKobo         int64   `json:"cap_kobo"`
	MinCount        int     `json:"min_count"`
	LowerPct        float64 `json:"lower_pct"`
	UpperPct        float64 `json:"upper_pct"`
	ReferenceKobo   int64   `json:"reference_kobo"`
	GranularityKobo int64   `json:"granularity_kobo"`
	AgeDays         int     `json:"age_days"`
	AmountKobo      int64   `json:"amount_kobo"`
	Minutes         int     `json:"minutes"`
	MinUsers        int     `json:"min_users"`
}

// parseRuleParams decodes a rule's params JSONB. Malformed JSON yields zero
// params (defaults) rather than an error — a bad edit in the admin UI must
// never stop the scan; the rule just runs with defaults.
func parseRuleParams(raw []byte) ruleParams {
	var p ruleParams
	if len(raw) == 0 || string(raw) == "" || string(raw) == "null" {
		return p
	}
	_ = json.Unmarshal(raw, &p)
	return p
}

func (p ruleParams) or(def, v int) int {
	if v != 0 {
		return v
	}
	return def
}

// breach is one rule hit. Window is the dedupe partition: it changes at most
// once per pattern window (day/week), so a rule firing every scan tick lands
// as ONE live alert (partial unique index on risk_alerts.dedupe_key).
type breach struct {
	RuleCode  string
	UserID    uuid.UUID
	SubjectID *uuid.UUID
	Severity  string
	Window    string
	Details   map[string]interface{}
}

func (b breach) dedupeKey() string {
	return fmt.Sprintf("%s:%s:%s", b.RuleCode, b.UserID, b.Window)
}

// dayBucket / weekBucket partition time for dedupe (UTC).
func dayBucket(t time.Time) string { return t.UTC().Format("2006-01-02") }

func weekBucket(t time.Time) string {
	y, w := t.UTC().ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

// hourBucket partitions the blocking rules' denial alerts (a blocked user
// should not pile up one alert per attempt).
func hourBucket(t time.Time) string { return t.UTC().Format("2006-01-02T15") }

// splitByUser groups the feed per user, deposit and withdrawal streams kept
// apart and time-ordered (FeedSince has no ORDER BY guarantee).
func splitByUser(facts []repositories.TxnFact) (map[uuid.UUID][]repositories.TxnFact, map[uuid.UUID][]repositories.TxnFact) {
	deposits := map[uuid.UUID][]repositories.TxnFact{}
	withdrawals := map[uuid.UUID][]repositories.TxnFact{}
	for _, f := range facts {
		switch f.Type {
		case "deposit":
			deposits[f.UserID] = append(deposits[f.UserID], f)
		case "withdrawal":
			withdrawals[f.UserID] = append(withdrawals[f.UserID], f)
		}
	}
	// Insertion sort by time — feeds are already roughly ordered and each
	// slice is small; avoids pulling in sort.Slice's less boilerplate per map.
	for _, m := range []map[uuid.UUID][]repositories.TxnFact{deposits, withdrawals} {
		for _, list := range m {
			for i := 1; i < len(list); i++ {
				for j := i; j > 0 && list[j].CreatedAt.Before(list[j-1].CreatedAt); j-- {
					list[j], list[j-1] = list[j-1], list[j]
				}
			}
		}
	}
	return deposits, withdrawals
}

// completedDeposit keeps money-that-actually-arrived; a failed deposit never
// funded anything.
func completedDeposit(f repositories.TxnFact) bool { return f.Status == "completed" }

// attemptedWithdrawal counts every non-failed attempt: velocity controls are
// about attempts, not just successes (retry storms are the point).
func attemptedWithdrawal(f repositories.TxnFact) bool { return f.Status != "failed" }

// ── Rule: deposit velocity (10 min) ────────────────────────────────────────
func evalDepositVelocity(rule *models.RiskRule, facts []repositories.TxnFact) []breach {
	p := parseRuleParams(rule.Params)
	maxCount := p.or(3, p.MaxCount)
	minutes := p.or(10, p.WindowMinutes)

	deposits, _ := splitByUser(facts)
	var out []breach
	for userID, list := range deposits {
		for i, cur := range list {
			if !completedDeposit(cur) {
				continue
			}
			windowStart := cur.CreatedAt.Add(-time.Duration(minutes) * time.Minute)
			count := 0
			for j := i; j >= 0; j-- {
				if list[j].CreatedAt.Before(windowStart) {
					break
				}
				if completedDeposit(list[j]) {
					count++
				}
			}
			if count > maxCount {
				out = append(out, breach{
					RuleCode: rule.Code, UserID: userID,
					Severity: models.RiskSeverityMedium,
					Window:   dayBucket(cur.CreatedAt),
					Details: map[string]interface{}{
						"count": count, "window_minutes": minutes,
						"max_count": maxCount, "last_amount_kobo": cur.Amount,
						"at": cur.CreatedAt.UTC().Format(time.RFC3339),
					},
				})
				break // one breach per user per scan
			}
		}
	}
	return out
}

// ── Rule: cumulative deposit cap (rolling 24h) ─────────────────────────────
func evalDepositCap(rule *models.RiskRule, facts []repositories.TxnFact) []breach {
	p := parseRuleParams(rule.Params)
	hours := p.or(24, p.WindowHours)
	capKobo := p.CapKobo
	if capKobo <= 0 {
		capKobo = 500000000 // ₦5,000,000 default
	}

	deposits, _ := splitByUser(facts)
	var out []breach
	for userID, list := range deposits {
		for i, cur := range list {
			if !completedDeposit(cur) {
				continue
			}
			windowStart := cur.CreatedAt.Add(-time.Duration(hours) * time.Hour)
			var sum int64
			for j := i; j >= 0; j-- {
				if list[j].CreatedAt.Before(windowStart) {
					break
				}
				if completedDeposit(list[j]) {
					sum += list[j].Amount
				}
			}
			if sum > capKobo {
				out = append(out, breach{
					RuleCode: rule.Code, UserID: userID,
					Severity: models.RiskSeverityMedium,
					Window:   dayBucket(cur.CreatedAt),
					Details: map[string]interface{}{
						"sum_kobo": sum, "cap_kobo": capKobo,
						"window_hours": hours,
						"at":           cur.CreatedAt.UTC().Format(time.RFC3339),
					},
				})
				break
			}
		}
	}
	return out
}

// ── Rule: structuring near the reporting threshold (7d) ────────────────────
//
// 3+ transactions in 7 days each at 90–99% of a reference amount: just under
// the line, repeatedly — the classic structuring signature.
func evalStructuring(rule *models.RiskRule, facts []repositories.TxnFact) []breach {
	p := parseRuleParams(rule.Params)
	minCount := p.or(3, p.MinCount)
	lower := p.LowerPct
	if lower == 0 {
		lower = 90
	}
	upper := p.UpperPct
	if upper == 0 {
		upper = 99
	}
	reference := p.ReferenceKobo
	if reference <= 0 {
		reference = models.CTRThresholdIndividualKobo
	}

	lo := int64(float64(reference) * lower / 100)
	hi := int64(float64(reference) * upper / 100)

	deposits, _ := splitByUser(facts)
	var out []breach
	for userID, list := range deposits {
		var near []repositories.TxnFact
		for _, f := range list {
			if f.Status == "failed" {
				continue
			}
			if f.Amount >= lo && f.Amount <= hi {
				near = append(near, f)
			}
		}
		if len(near) < minCount {
			continue
		}
		last := near[len(near)-1]
		out = append(out, breach{
			RuleCode: rule.Code, UserID: userID,
			Severity:  models.RiskSeverityHigh,
			Window:    weekBucket(last.CreatedAt),
			SubjectID: &last.ID,
			Details: map[string]interface{}{
				"count": len(near), "min_count": minCount,
				"band":         fmt.Sprintf("%.0f%%-%.0f%% of %d kobo", lower, upper, reference),
				"amounts_kobo": amountsOf(near),
				"at":           last.CreatedAt.UTC().Format(time.RFC3339),
			},
		})
	}
	return out
}

// ── Rule: round-amount cycling (7d) ────────────────────────────────────────
func evalRoundAmountCycling(rule *models.RiskRule, facts []repositories.TxnFact) []breach {
	p := parseRuleParams(rule.Params)
	minCount := p.or(3, p.MinCount)
	gran := p.GranularityKobo
	if gran <= 0 {
		gran = 100000000 // ₦100,000
	}

	deposits, withdrawals := splitByUser(facts)
	var out []breach
	scan := func(perUser map[uuid.UUID][]repositories.TxnFact) {
		type group struct {
			count int
			last  repositories.TxnFact
		}
		// Grouping is PER USER and per amount: two users cycling the same
		// round figure are two separate signals, never one pooled one.
		for _, list := range perUser {
			if len(list) == 0 {
				continue
			}
			groups := map[int64]*group{}
			for _, f := range list {
				if f.Status == "failed" || f.Amount == 0 || f.Amount%gran != 0 {
					continue
				}
				g := groups[f.Amount]
				if g == nil {
					groups[f.Amount] = &group{count: 1, last: f}
					continue
				}
				g.count++
				g.last = f
			}
			for amount, g := range groups {
				if g.count < minCount {
					continue
				}
				out = append(out, breach{
					RuleCode: rule.Code, UserID: g.last.UserID,
					Severity:  models.RiskSeverityLow,
					Window:    weekBucket(g.last.CreatedAt),
					SubjectID: &g.last.ID,
					Details: map[string]interface{}{
						"amount_kobo": amount, "count": g.count,
						"min_count": minCount, "granularity_kobo": gran,
						"at": g.last.CreatedAt.UTC().Format(time.RFC3339),
					},
				})
			}
		}
	}
	scan(deposits)
	scan(withdrawals)
	return out
}

// ── Rule: rapid in-and-out (deposit → withdrawal within N minutes) ─────────
func evalRapidInOut(rule *models.RiskRule, facts []repositories.TxnFact) []breach {
	p := parseRuleParams(rule.Params)
	minutes := p.or(15, p.Minutes)

	deposits, withdrawals := splitByUser(facts)
	var out []breach
	for userID, wds := range withdrawals {
		deps := deposits[userID]
		for _, wd := range wds {
			if !attemptedWithdrawal(wd) {
				continue
			}
			cutoff := wd.CreatedAt.Add(-time.Duration(minutes) * time.Minute)
			for _, d := range deps {
				if !completedDeposit(d) || d.CreatedAt.Before(cutoff) {
					continue
				}
				out = append(out, breach{
					RuleCode: rule.Code, UserID: userID,
					Severity:  models.RiskSeverityMedium,
					Window:    dayBucket(wd.CreatedAt),
					SubjectID: &wd.ID,
					Details: map[string]interface{}{
						"minutes":      minutes,
						"deposit_kobo": d.Amount, "withdrawal_kobo": wd.Amount,
						"deposit_at": d.CreatedAt.UTC().Format(time.RFC3339),
						"at":         wd.CreatedAt.UTC().Format(time.RFC3339),
					},
				})
				break
			}
		}
	}
	return out
}

// ── Rule: duplicate account funding from one source (7d) ───────────────────
//
// One funding source (Paystack customer email in the deposit payload) moneying
// two or more distinct wallets — shared-source accounts, potentially mules.
func evalDuplicateFunding(rule *models.RiskRule, facts []repositories.TxnFact) []breach {
	p := parseRuleParams(rule.Params)
	minUsers := p.or(2, p.MinUsers)

	deposits, _ := splitByUser(facts)
	type srcState struct {
		users map[uuid.UUID]time.Time
	}
	sources := map[string]*srcState{}
	for _, list := range deposits {
		for _, f := range list {
			if !completedDeposit(f) || f.FundingEmail == "" {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(f.FundingEmail))
			if sources[key] == nil {
				sources[key] = &srcState{users: map[uuid.UUID]time.Time{}}
			}
			if prev, ok := sources[key].users[f.UserID]; !ok || f.CreatedAt.After(prev) {
				sources[key].users[f.UserID] = f.CreatedAt
			}
		}
	}

	var out []breach
	for email, st := range sources {
		if len(st.users) < minUsers {
			continue
		}
		var latest time.Time
		for _, t := range st.users {
			if t.After(latest) {
				latest = t
			}
		}
		for userID := range st.users {
			others := []string{}
			for other := range st.users {
				if other != userID {
					others = append(others, other.String())
				}
			}
			out = append(out, breach{
				RuleCode: rule.Code, UserID: userID,
				Severity: models.RiskSeverityHigh,
				Window:   weekBucket(latest),
				Details: map[string]interface{}{
					"funding_email":  email,
					"distinct_users": len(st.users),
					"other_user_ids": others,
					"at":             latest.UTC().Format(time.RFC3339),
				},
			})
		}
	}
	return out
}

// ── Rule: withdrawal account name vs KYC name (beyond tolerance) ───────────
//
// The bank-verified payout name (frozen into withdrawal metadata at
// initiation) compared against the KYC full name. A mismatch is not a block —
// banks return odd formats — but it is exactly what a reviewer must see.
func evalKYCNameMismatch(rule *models.RiskRule, names map[uuid.UUID]string, facts []repositories.TxnFact) []breach {
	_, withdrawals := splitByUser(facts)
	var out []breach
	for userID, list := range withdrawals {
		kycName := names[userID]
		if kycName == "" {
			continue
		}
		for _, wd := range list {
			if !attemptedWithdrawal(wd) || wd.BankAccountName == "" {
				continue
			}
			if accountNamesMatch(kycName, wd.BankAccountName) {
				continue
			}
			out = append(out, breach{
				RuleCode: rule.Code, UserID: userID,
				Severity:  models.RiskSeverityMedium,
				Window:    weekBucket(wd.CreatedAt),
				SubjectID: &wd.ID,
				Details: map[string]interface{}{
					"kyc_name": kycName, "bank_name": wd.BankAccountName,
					"withdrawal_kobo": wd.Amount,
					"at":              wd.CreatedAt.UTC().Format(time.RFC3339),
				},
			})
			break // one alert per user per scan
		}
	}
	return out
}

func amountsOf(list []repositories.TxnFact) []int64 {
	out := make([]int64, 0, len(list))
	for _, f := range list {
		out = append(out, f.Amount)
	}
	return out
}

// ═════════════════════════════════════════════════════════════════════════
// RULE SEEDS — the canonical v1 set, in Go.
//
// Migration 000032 inserts the same rows for a fresh database, but the code
// seed is authoritative: it is idempotent (ON CONFLICT DO NOTHING) and runs
// at every boot, which repairs a database that lost its rules — a
// `TRUNCATE users CASCADE` (test cleanup, a support script) silently wipes
// risk_rules through its updated_by FK, and golang-migrate never re-runs a
// finished migration. Same pattern as EnsureFeeRules / EnsureChartOfAccounts.
//
// Existing rows are never overwritten: thresholds are the admin's to edit.
// ═════════════════════════════════════════════════════════════════════════

func riskRuleSeeds() []models.RiskRule {
	mk := func(code, name, desc, mode, params string) models.RiskRule {
		return models.RiskRule{
			Code: code, Name: name, Description: desc, Mode: mode,
			Enabled: true, Params: datatypes.JSON(params),
		}
	}
	return []models.RiskRule{
		mk(models.RuleWithdrawVelocity24h, "Withdrawal velocity (24h)",
			"More than 5 withdrawals in any rolling 24 hours.",
			models.RiskModeBlocking, `{"max_count":5,"window_hours":24}`),
		mk(models.RuleRiskFlaggedUser, "Risk-flagged user withdrawal",
			"Withdrawals by a user flagged for risk review (manual flag or screening hit) are blocked until cleared.",
			models.RiskModeBlocking, `{}`),
		mk(models.RuleNewAccountHighValue, "New-account high-value withdrawal",
			"Withdrawal within 7 days of signup at or above the high-value threshold — classic mule/drain pattern.",
			models.RiskModeBlocking, `{"age_days":7,"amount_kobo":100000000}`),
		mk(models.RuleDepositVelocity10m, "Deposit velocity (10min)",
			"More than 3 deposits in any rolling 10 minutes.",
			models.RiskModeMonitoring, `{"max_count":3,"window_minutes":10}`),
		mk(models.RuleDepositCap24h, "Cumulative deposit cap (24h)",
			"Deposits totalling more than the cap in any rolling 24 hours.",
			models.RiskModeMonitoring, `{"cap_kobo":500000000}`),
		mk(models.RuleStructuring7d, "Structuring near limit (7d)",
			"3+ transactions in 7 days each at 90–99% of a reporting threshold — smurfing pattern.",
			models.RiskModeMonitoring, `{"min_count":3,"window_days":7,"lower_pct":90,"upper_pct":99,"reference_kobo":500000000}`),
		mk(models.RuleRoundAmountCycling, "Round-amount cycling (7d)",
			"The same round amount moved 3+ times in 7 days.",
			models.RiskModeMonitoring, `{"min_count":3,"window_days":7,"granularity_kobo":100000000}`),
		mk(models.RuleRapidInOut, "Rapid in-and-out (15min)",
			"Deposit followed by a withdrawal within 15 minutes — funds pass through rather than invest.",
			models.RiskModeMonitoring, `{"minutes":15}`),
		mk(models.RuleDuplicateFunding, "Duplicate account funding",
			"One funding source (Paystack customer email) moneying two or more distinct wallets in 7 days.",
			models.RiskModeMonitoring, `{"min_users":2,"window_days":7}`),
		mk(models.RuleKYCNameMismatch, "KYC name vs payout account mismatch",
			"Bank-verified withdrawal account name does not match the user's KYC name beyond tolerance. Evaluated at withdrawal initiation; alerts, never blocks.",
			models.RiskModeMonitoring, `{}`),
		mk(models.RuleScreeningHit, "Sanctions/PEP screening hit",
			"Raised directly by the manual screening flow when an admin records a HIT: freeze-review, escalated alert, STR drafted. Not scan-evaluated.",
			models.RiskModeMonitoring, `{}`),
	}
}
