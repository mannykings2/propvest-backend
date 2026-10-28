package services

import (
	"testing"

	"github.com/mannykings2/propvest-backend/internal/models"
)

// The fee maths is the part of the Money layer a user can feel, so it is
// pinned against the worked examples from the plan (Phase 7, fee model):
//
//	deposit/withdrawal fee: 1.0% capped at ₦2,000
//	performance fee:        10–15% of distributed profit, never principal
func TestWithdrawalFee_WorksExamples(t *testing.T) {
	rule := modelsFeeWithdrawal()

	cases := []struct {
		name       string
		amountKobo int64
		wantFee    int64
		wantNet    int64
	}{
		{"₦1,000 withdrawal → ₦10 fee", 100000, 1000, 99000},
		{"₦500,000 withdrawal → cap binds at exactly ₦2,000", 50000000, 200000, 49800000},
		{"₦100,000 withdrawal → ₦1,000 fee", 10000000, 100000, 9900000},
		{"tiny withdrawal: 1%% of 50 kobo rounds down to nothing (integer maths never rounds up against the user)", 50, 0, 50},
		{"zero withdrawal charges nothing", 0, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fee := feeFromRule(rule, tc.amountKobo)
			if fee != tc.wantFee {
				t.Fatalf("fee = %d kobo, want %d", fee, tc.wantFee)
			}
			if net := tc.amountKobo - fee; net != tc.wantNet {
				t.Fatalf("net = %d kobo, want %d", net, tc.wantNet)
			}
		})
	}
}

func TestPerformanceFee_DistributedProfitOnly(t *testing.T) {
	rule := modelsFeePerformance(1250) // 12.5%, the mid-band default

	const gross = int64(10000) // ₦100 gross distribution
	fee := feeFromRule(rule, gross)
	if fee != 1250 {
		t.Fatalf("fee at 12.5%% of ₦100 = %d kobo, want 1250 (₦12.50)", fee)
	}
	if net := gross - fee; net != 8750 {
		t.Fatalf("investor share = %d kobo, want 8750 (₦87.50)", net)
	}

	// The band endpoints must both be legal and charge only the profit.
	for _, bp := range []int{1000, 1500} {
		f := feeFromRule(modelsFeePerformance(bp), gross)
		want := gross * int64(bp) / 10000
		if f != want {
			t.Fatalf("rate %d bp: fee = %d, want %d", bp, f, want)
		}
	}

	// No distribution, no fee — the engine never touches principal.
	if f := feeFromRule(rule, 0); f != 0 {
		t.Fatalf("zero gross must charge nothing, got %d", f)
	}
}

// A fee can never exceed the amount it was charged on, whatever the rules say.
func TestFeeFromRule_CappedAtBasis(t *testing.T) {
	greedy := modelsFeeWithdrawal()
	greedy.RateBP = 20000 // 200% — would take more than the withdrawal itself
	if f := feeFromRule(greedy, 1000); f != 1000 {
		t.Fatalf("fee must be capped at the basis, got %d", f)
	}
}

// helpers: the seeded rule shapes, without needing a database.
func modelsFeeWithdrawal() models.FeeRule {
	return models.FeeRule{Code: models.FeeRuleWithdrawal, FeeType: "transaction", Basis: "withdrawal", RateBP: 100, CapKobo: int64Ptr(200000)}
}

func modelsFeePerformance(rateBP int) models.FeeRule {
	return models.FeeRule{Code: models.FeeRulePerformanceDefault, FeeType: "performance", Basis: "distributed_profit", RateBP: rateBP}
}
