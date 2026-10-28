package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/models"
)

// LedgerService is the double-entry engine (Phase 7).
//
// ONE RULE: every naira has exactly one home. Every money mutation writes a
// balanced journal — sum(debits) == sum(credits) — inside the SAME database
// transaction as the wallet change it describes. The ledger is append-only:
// the database rejects UPDATE/DELETE, so a correction is a reversing journal.
//
// Account model: postings go to per-user sub-ledger accounts (2010 wallets,
// 2100 withdrawal reserve, 2020 earnings payable, 2200 holdings) so
//
//	sum(per-user entries) == sum(wallets columns)
//
// for every user, which is what the nightly reconciliation proves. 2000 is the
// control total that reports the ring-fenced custody, never posted to directly.
type LedgerService interface {
	// EnsureChartOfAccounts seeds the ten accounts (idempotent) and loads the
	// code→id map the posting path uses.
	EnsureChartOfAccounts(ctx context.Context) error

	// Post writes one balanced journal using the caller's transaction.
	Post(ctx context.Context, tx *gorm.DB, lines ...LedgerLine) (uuid.UUID, error)

	// Standard journals ------------------------------------------------------
	// Deposit:        DR 1010 clearing / CR 2010 user wallet
	Deposit(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error
	// Withdrawal reserve: DR 2010 / CR 2100 (money leaves available, waits in reserve)
	WithdrawalReserve(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error
	// Withdrawal paid: DR 2100 gross / CR 1010 net / CR 3000 fee
	WithdrawalPaid(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, gross, fee int64, ref string) error
	// Withdrawal released: DR 2100 / CR 2010 (provider failed, money goes back)
	WithdrawalReleased(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error
	// Invest/subscribe: DR 2010 user wallet / CR 2200 holdings
	Investment(ctx context.Context, tx *gorm.DB, userID uuid.UUID, walletTxnID *uuid.UUID, amountKobo int64, ref string) error
	// Investment refund: DR 2200 / CR 2010
	InvestmentRefund(ctx context.Context, tx *gorm.DB, userID uuid.UUID, walletTxnID *uuid.UUID, amountKobo int64, ref string) error
	// Earnings→main: DR 2020 / CR 2010
	EarningsToMain(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error
	// Distribution: DR 1010 / CR 4000, then DR 4000 / CR 2010 net + CR 3100 fee.
	// Returns the performance fee charged (0 when the product has no rate).
	Distribution(ctx context.Context, tx *gorm.DB, userID uuid.UUID, grossKobo int64, productCode *string, ref string) (feeKobo int64, err error)

	// Fees -------------------------------------------------------------------
	// WithdrawalFeeKobo = min(rate% × amount, cap) from fee_rules. Returns an
	// error when no active transaction rule exists — charging without a rule
	// is a bug, not a zero fee.
	WithdrawalFeeKobo(ctx context.Context, amountKobo int64) (feeKobo, netKobo int64, err error)
	// PerformanceFeeKobo = product rate (10–15%) of distributed profit. Never
	// touches principal. Zero when no rate is configured for the product.
	PerformanceFeeKobo(ctx context.Context, grossKobo int64, productCode *string) (feeKobo, netKobo int64, err error)

	// Trust checks ------------------------------------------------------------
	// RunReconciliation executes the nightly checks (trial balance, sub-ledger
	// mirrors, custody coverage, suspense) and stores the run.
	RunReconciliation(ctx context.Context, kind string) (*models.ReconciliationRun, error)
	// SweepFees moves earned fees (3000/3100) from clearing into the platform's
	// operating cash: DR 1000 / CR 1010. The only legal path for user money to
	// become platform money.
	SweepFees(ctx context.Context) (sweptKobo int64, err error)

	// BackfillFromWalletTransactions reconstructs journals for wallet history
	// that predates the ledger (Phase 7.3). Idempotent: references already
	// posted are skipped. Run automatically at boot while the ledger is empty.
	BackfillFromWalletTransactions(ctx context.Context) (posted int64, err error)
}

// LedgerLine is one leg of a journal. Exactly one of Debit/Credit is set.
type LedgerLine struct {
	AccountCode string
	UserID      *uuid.UUID
	WalletTxnID *uuid.UUID
	Debit       int64
	Credit      int64
	Description string
	Ref         string
}

type ledgerService struct {
	db *gorm.DB

	mu       sync.RWMutex
	accounts map[string]uuid.UUID // code → ledger_accounts.id

	feeMu       sync.RWMutex
	feeRules    map[string]models.FeeRule // code → rule
	feeLoadedAt time.Time
}

// feeRuleCacheTTL bounds how stale fee maths may be: a rule edit is picked up
// within five minutes without a restart, and withdrawals don't pay for a
// fee_rules query each time.
const feeRuleCacheTTL = 5 * time.Minute

func NewLedgerService(db *gorm.DB) LedgerService {
	return &ledgerService{db: db}
}

// ── Chart of accounts ──────────────────────────────────────────────────────

var chartOfAccounts = []models.LedgerAccount{
	{Code: models.AccountOperatingCash, Name: "Platform operating cash", Type: "asset"},
	{Code: models.AccountSettlementClearing, Name: "Settlement clearing", Type: "asset"},
	{Code: models.AccountSuspense, Name: "Suspense (unmatched settlement)", Type: "asset"},
	{Code: models.AccountUserCustody, Name: "User funds custody (control)", Type: "liability"},
	{Code: models.AccountUserWallets, Name: "User wallets", Type: "liability"},
	{Code: models.AccountWithdrawalReserve, Name: "Withdrawal reserve", Type: "liability"},
	{Code: models.AccountEarningsPayable, Name: "Earnings payable", Type: "liability"},
	// 2200 is the user's property POSITION: investments credit it (DR wallet /
	// CR holdings), so it carries a credit balance like the other user-money
	// accounts. The platform's property assets live in the properties tables,
	// not here — this account answers "what do we owe users who invested".
	{Code: models.AccountInvestmentHoldings, Name: "Investment holdings (user positions)", Type: "liability"},
	{Code: models.AccountFeeRevenueTransaction, Name: "Fee revenue — transaction fees", Type: "revenue"},
	{Code: models.AccountFeeRevenuePerformance, Name: "Fee revenue — performance fees", Type: "revenue"},
	{Code: models.AccountRentalIncome, Name: "Rental income (clearing)", Type: "income"},
}

func (s *ledgerService) EnsureChartOfAccounts(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, acct := range chartOfAccounts {
			a := acct
			// DoUpdates (not DoNothing) on name/type: chart metadata corrections
			// must propagate to already-seeded databases. Entries never change —
			// they reference account IDs, not names.
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "code"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "type"}),
			}).Create(&a).Error; err != nil {
				return fmt.Errorf("seed ledger account %s: %w", acct.Code, err)
			}
		}

		var rows []models.LedgerAccount
		if err := tx.Where("is_active = ?", true).Find(&rows).Error; err != nil {
			return err
		}
		m := make(map[string]uuid.UUID, len(rows))
		for _, r := range rows {
			m[r.Code] = r.ID
		}
		s.mu.Lock()
		s.accounts = m
		s.mu.Unlock()
		return nil
	})
}

func (s *ledgerService) accountID(code string) (uuid.UUID, error) {
	s.mu.RLock()
	id, ok := s.accounts[code]
	s.mu.RUnlock()
	if !ok {
		return uuid.Nil, fmt.Errorf("ledger: unknown account code %q (was EnsureChartOfAccounts called?)", code)
	}
	return id, nil
}

// ── Posting ────────────────────────────────────────────────────────────────

// Post writes one balanced journal. The caller owns the transaction, so the
// journal commits or rolls back together with the wallet mutation — a balance
// can never move without its entries (the Phase 0 lesson).
func (s *ledgerService) Post(ctx context.Context, tx *gorm.DB, lines ...LedgerLine) (uuid.UUID, error) {
	if len(lines) < 2 {
		return uuid.Nil, fmt.Errorf("ledger: a journal needs at least one debit and one credit")
	}
	var totalDr, totalCr int64
	for i, l := range lines {
		if l.Debit < 0 || l.Credit < 0 {
			return uuid.Nil, fmt.Errorf("ledger: line %d has a negative amount", i)
		}
		if (l.Debit == 0) == (l.Credit == 0) {
			return uuid.Nil, fmt.Errorf("ledger: line %d must be a debit OR a credit", i)
		}
		totalDr += l.Debit
		totalCr += l.Credit
	}
	if totalDr != totalCr {
		return uuid.Nil, fmt.Errorf("ledger: unbalanced journal (debits=%d credits=%d)", totalDr, totalCr)
	}

	journalID := uuid.New()
	for _, l := range lines {
		accountID, err := s.accountID(l.AccountCode)
		if err != nil {
			return uuid.Nil, err
		}
		entry := models.LedgerEntry{
			JournalID:   journalID,
			AccountID:   accountID,
			UserID:      l.UserID,
			WalletTxnID: l.WalletTxnID,
			Debit:       l.Debit,
			Credit:      l.Credit,
			Description: l.Description,
			Ref:         l.Ref,
		}
		if err := tx.WithContext(ctx).Create(&entry).Error; err != nil {
			return uuid.Nil, fmt.Errorf("ledger: insert entry: %w", err)
		}
	}
	return journalID, nil
}

func ptrID(id uuid.UUID) *uuid.UUID { return &id }

func (s *ledgerService) Deposit(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error {
	_, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountSettlementClearing, Debit: amountKobo, Description: "Deposit received", Ref: ref},
		LedgerLine{AccountCode: models.AccountUserWallets, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Credit: amountKobo, Description: "Wallet credited", Ref: ref},
	)
	return err
}

func (s *ledgerService) WithdrawalReserve(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error {
	_, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountUserWallets, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Debit: amountKobo, Description: "Withdrawal reserved", Ref: ref},
		LedgerLine{AccountCode: models.AccountWithdrawalReserve, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Credit: amountKobo, Description: "Withdrawal reserve", Ref: ref},
	)
	return err
}

func (s *ledgerService) WithdrawalPaid(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, gross, fee int64, ref string) error {
	lines := []LedgerLine{
		{AccountCode: models.AccountWithdrawalReserve, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Debit: gross, Description: "Withdrawal paid out", Ref: ref},
		{AccountCode: models.AccountSettlementClearing, Credit: gross - fee, Description: "Settlement to bank", Ref: ref},
	}
	if fee > 0 {
		lines = append(lines, LedgerLine{
			AccountCode: models.AccountFeeRevenueTransaction, Credit: fee,
			Description: "Transaction fee (1% capped at ₦2,000)", Ref: ref,
		})
	}
	_, err := s.Post(ctx, tx, lines...)
	return err
}

func (s *ledgerService) WithdrawalReleased(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error {
	_, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountWithdrawalReserve, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Debit: amountKobo, Description: "Withdrawal released", Ref: ref},
		LedgerLine{AccountCode: models.AccountUserWallets, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Credit: amountKobo, Description: "Funds returned to wallet", Ref: ref},
	)
	return err
}

func (s *ledgerService) Investment(ctx context.Context, tx *gorm.DB, userID uuid.UUID, walletTxnID *uuid.UUID, amountKobo int64, ref string) error {
	_, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountUserWallets, UserID: ptrID(userID), WalletTxnID: walletTxnID, Debit: amountKobo, Description: "Investment", Ref: ref},
		LedgerLine{AccountCode: models.AccountInvestmentHoldings, UserID: ptrID(userID), WalletTxnID: walletTxnID, Credit: amountKobo, Description: "Property principal", Ref: ref},
	)
	return err
}

func (s *ledgerService) InvestmentRefund(ctx context.Context, tx *gorm.DB, userID uuid.UUID, walletTxnID *uuid.UUID, amountKobo int64, ref string) error {
	_, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountInvestmentHoldings, UserID: ptrID(userID), WalletTxnID: walletTxnID, Debit: amountKobo, Description: "Investment refunded", Ref: ref},
		LedgerLine{AccountCode: models.AccountUserWallets, UserID: ptrID(userID), WalletTxnID: walletTxnID, Credit: amountKobo, Description: "Principal returned", Ref: ref},
	)
	return err
}

func (s *ledgerService) EarningsToMain(ctx context.Context, tx *gorm.DB, userID, walletTxnID uuid.UUID, amountKobo int64, ref string) error {
	_, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountEarningsPayable, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Debit: amountKobo, Description: "Earnings moved to main", Ref: ref},
		LedgerLine{AccountCode: models.AccountUserWallets, UserID: ptrID(userID), WalletTxnID: ptrID(walletTxnID), Credit: amountKobo, Description: "Earnings credited to main", Ref: ref},
	)
	return err
}

// Distribution books gross income, then splits it: investors get the net, the
// platform gets its performance fee. The fee NEVER touches principal.
func (s *ledgerService) Distribution(ctx context.Context, tx *gorm.DB, userID uuid.UUID, grossKobo int64, productCode *string, ref string) (int64, error) {
	fee, net, err := s.PerformanceFeeKobo(ctx, grossKobo, productCode)
	if err != nil {
		return 0, err
	}
	if _, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountSettlementClearing, Debit: grossKobo, Description: "Rental income received", Ref: ref},
		LedgerLine{AccountCode: models.AccountRentalIncome, Credit: grossKobo, Description: "Gross distribution", Ref: ref},
	); err != nil {
		return 0, err
	}
	if _, err := s.Post(ctx, tx,
		LedgerLine{AccountCode: models.AccountRentalIncome, Debit: grossKobo, Description: "Distribution split", Ref: ref},
		LedgerLine{AccountCode: models.AccountUserWallets, UserID: ptrID(userID), Credit: net, Description: "Investor share of profit", Ref: ref},
	); err != nil {
		return 0, err
	}
	if fee > 0 {
		if _, err := s.Post(ctx, tx,
			LedgerLine{AccountCode: models.AccountFeeRevenuePerformance, Credit: fee, Description: "Performance fee", Ref: ref},
			LedgerLine{AccountCode: models.AccountRentalIncome, Debit: fee, Description: "Performance fee", Ref: ref},
		); err != nil {
			return 0, err
		}
	}
	return fee, nil
}

// ── Fee engine ─────────────────────────────────────────────────────────────

// feeRuleSeeds are the ONLY fees this platform charges (Phase 7 decision):
// 1.0% on withdrawals capped at ₦2,000, and a performance fee inside 10–15%.
var feeRuleSeeds = []models.FeeRule{
	{
		Code: models.FeeRuleWithdrawal, Name: "Withdrawal transaction fee",
		FeeType: "transaction", Basis: "withdrawal",
		RateBP: 100, CapKobo: int64Ptr(200000), // 1% capped at ₦2,000
	},
	{
		Code: models.FeeRulePerformanceDefault, Name: "Performance fee (default)",
		FeeType: "performance", Basis: "distributed_profit",
		RateBP: 1250, // 12.5% — mid-band default until a product overrides it
	},
}

func int64Ptr(v int64) *int64 { return &v }

// EnsureFeeRules seeds the two rules (idempotent) — called with the chart.
func EnsureFeeRules(ctx context.Context, db *gorm.DB) error {
	for _, rule := range feeRuleSeeds {
		r := rule
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoNothing: true,
		}).Create(&r).Error; err != nil {
			return fmt.Errorf("seed fee rule %s: %w", rule.Code, err)
		}
	}
	return nil
}

func (s *ledgerService) loadFeeRules(ctx context.Context) (map[string]models.FeeRule, error) {
	s.feeMu.RLock()
	if s.feeRules != nil && time.Since(s.feeLoadedAt) < feeRuleCacheTTL {
		rules := s.feeRules
		s.feeMu.RUnlock()
		return rules, nil
	}
	s.feeMu.RUnlock()

	var rows []models.FeeRule
	if err := s.db.WithContext(ctx).Where("is_active = ?", true).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load fee rules: %w", err)
	}
	m := make(map[string]models.FeeRule, len(rows))
	for _, r := range rows {
		m[r.Code] = r
	}
	s.feeMu.Lock()
	s.feeRules = m
	s.feeLoadedAt = time.Now()
	s.feeMu.Unlock()
	return m, nil
}

func feeFromRule(rule models.FeeRule, basisKobo int64) int64 {
	if basisKobo <= 0 || rule.RateBP <= 0 {
		return 0
	}
	fee := basisKobo * int64(rule.RateBP) / 10000
	if rule.CapKobo != nil && fee > *rule.CapKobo {
		fee = *rule.CapKobo
	}
	if fee < rule.MinKobo {
		fee = rule.MinKobo
	}
	if fee > basisKobo {
		fee = basisKobo // a fee can never exceed the amount it is charged on
	}
	return fee
}

func (s *ledgerService) WithdrawalFeeKobo(ctx context.Context, amountKobo int64) (int64, int64, error) {
	rules, err := s.loadFeeRules(ctx)
	if err != nil {
		return 0, 0, err
	}
	rule, ok := rules[models.FeeRuleWithdrawal]
	if !ok {
		// Charging without a rule is a bug, not a zero fee.
		return 0, 0, fmt.Errorf("fee engine: no active withdrawal fee rule")
	}
	fee := feeFromRule(rule, amountKobo)
	return fee, amountKobo - fee, nil
}

func (s *ledgerService) PerformanceFeeKobo(ctx context.Context, grossKobo int64, productCode *string) (int64, int64, error) {
	rules, err := s.loadFeeRules(ctx)
	if err != nil {
		return 0, 0, err
	}
	rule, ok := rules[models.FeeRulePerformanceDefault]
	if productCode != nil {
		if productSpecific, found := rules["performance_fee_"+*productCode]; found {
			rule = productSpecific
			ok = true
		}
	}
	if !ok {
		return 0, 0, fmt.Errorf("fee engine: no active performance fee rule")
	}
	// Guard the decided band: 10–15% of distributed profit.
	if rule.RateBP < 1000 || rule.RateBP > 1500 {
		return 0, 0, fmt.Errorf("fee engine: performance rate %d bp outside the 10–15%% band", rule.RateBP)
	}
	fee := feeFromRule(rule, grossKobo)
	return fee, grossKobo - fee, nil
}

// ── Trust checks ───────────────────────────────────────────────────────────

type reconDetails struct {
	TrialBalanceOK   bool     `json:"trial_balance_ok"`
	MirrorsOK        bool     `json:"sub_ledgers_mirror_wallets"`
	MirrorMismatches int64    `json:"mirror_mismatches"`
	CustodyCoveredOK bool     `json:"custody_coverage_ok"`
	SuspenseOK       bool     `json:"suspense_zero"`
	Notes            []string `json:"notes,omitempty"`
}

// RunReconciliation executes the three trust checks the Money layer owes its
// users (Phase 7.6): the books balance, every user sub-ledger mirrors its
// wallet column, and Suspense is empty. A failure is stored AND alerted.
func (s *ledgerService) RunReconciliation(ctx context.Context, kind string) (*models.ReconciliationRun, error) {
	if kind == "" {
		kind = "nightly"
	}

	var totals struct {
		Debits  int64
		Credits int64
	}
	if err := s.db.WithContext(ctx).Raw(
		`SELECT COALESCE(SUM(debit),0) AS debits, COALESCE(SUM(credit),0) AS credits FROM ledger_entries`,
	).Scan(&totals).Error; err != nil {
		return nil, fmt.Errorf("reconciliation: totals: %w", err)
	}

	// Wallet-side user value (mirrors the wallets table): wallets + reserve + earnings.
	walletLiabilityRaw, err := s.sumAccountBalance(ctx,
		models.AccountUserWallets, models.AccountWithdrawalReserve, models.AccountEarningsPayable)
	if err != nil {
		return nil, err
	}
	// Liability/revenue are credit balances; sumAccountBalance is debit−credit.
	walletLiability := -walletLiabilityRaw

	positionsRaw, err := s.sumAccountBalance(ctx, models.AccountInvestmentHoldings)
	if err != nil {
		return nil, err
	}
	positions := -positionsRaw
	liability := walletLiability + positions

	// Custody assets are the cash side: bank + clearing + suspense.
	assets, err := s.sumAccountBalance(ctx, models.AccountOperatingCash, models.AccountSettlementClearing, models.AccountSuspense)
	if err != nil {
		return nil, err
	}
	suspense, err := s.sumAccountBalance(ctx, models.AccountSuspense)
	if err != nil {
		return nil, err
	}
	revenueRaw, err := s.sumAccountBalance(ctx, models.AccountFeeRevenueTransaction, models.AccountFeeRevenuePerformance)
	if err != nil {
		return nil, err
	}
	revenue := -revenueRaw

	var walletSum int64
	// Wallets mirror the sub-ledgers as: 2010 == available (main − locked),
	// 2100 == locked, 2020 == earnings — so the liability side is main + earnings.
	if err := s.db.WithContext(ctx).Raw(
		`SELECT COALESCE(SUM(main_balance + earnings_balance),0) FROM wallets`,
	).Scan(&walletSum).Error; err != nil {
		return nil, fmt.Errorf("reconciliation: wallet totals: %w", err)
	}	// Sub-ledger mirrors: per-user ledger balance must equal the wallet column
	// (2010 mirrors AVAILABLE = main − locked; 2100 mirrors locked; 2020 earnings).
	var mismatches int64
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT w.user_id
			FROM wallets w
			LEFT JOIN (
				SELECT e.user_id,
				       SUM(e.credit - e.debit) FILTER (WHERE a.code = '2010') AS bal_2010,
				       SUM(e.credit - e.debit) FILTER (WHERE a.code = '2100') AS bal_2100,
				       SUM(e.credit - e.debit) FILTER (WHERE a.code = '2020') AS bal_2020
				FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
				WHERE a.code IN ('2010','2100','2020') AND e.user_id IS NOT NULL
				GROUP BY e.user_id
			) b ON b.user_id = w.user_id
			WHERE COALESCE(b.bal_2010, 0) <> (w.main_balance - w.locked_balance)
			   OR COALESCE(b.bal_2100, 0) <> w.locked_balance
			   OR COALESCE(b.bal_2020, 0) <> w.earnings_balance
		) mismatched
	`).Scan(&mismatches).Error; err != nil {
		return nil, fmt.Errorf("reconciliation: mirror check: %w", err)
	}

	details := reconDetails{
		TrialBalanceOK:   totals.Debits == totals.Credits,
		MirrorsOK:        mismatches == 0 && walletLiability == walletSum,
		MirrorMismatches: mismatches,
		CustodyCoveredOK: assets == liability+revenue,
		SuspenseOK:       suspense == 0,
	}

	status := "pass"
	var notes []string
	if !details.TrialBalanceOK {
		status = "fail"
		notes = append(notes, fmt.Sprintf("trial balance off by %d kobo", totals.Debits-totals.Credits))
	}
	if !details.MirrorsOK {
		status = "fail"
		notes = append(notes, fmt.Sprintf("user sub-ledgers differ from wallets by %d kobo (%d mismatched users)", walletLiability-walletSum, mismatches))
	}
	if !details.CustodyCoveredOK {
		status = "fail"
		notes = append(notes, fmt.Sprintf("custody assets differ from liabilities+revenue by %d kobo", assets-(liability+revenue)))
	}
	if !details.SuspenseOK {
		status = "fail"
		notes = append(notes, fmt.Sprintf("suspense holds %d kobo that matched no payment", suspense))
	}
	details.Notes = notes

	raw, _ := json.Marshal(details)
	run := &models.ReconciliationRun{
		Kind:              kind,
		Status:            status,
		TotalDebitsKobo:   totals.Debits,
		TotalCreditsKobo:  totals.Credits,
		UserLiabilityKobo: liability,
		WalletBalanceKobo: walletSum,
		CustodyAssetsKobo: assets,
		SuspenseKobo:      suspense,
		DiscrepancyKobo:   assets - (liability + revenue),
		Details:           raw,
		Alerted:           status == "fail",
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		return nil, fmt.Errorf("reconciliation: store run: %w", err)
	}

	log := logger.FromContext(ctx)
	if status == "fail" {
		log.Error("reconciliation FAILED", "kind", kind, "notes", notes)
	} else {
		log.Info("reconciliation passed", "kind", kind, "liability_kobo", liability, "custody_kobo", assets)
	}
	return run, nil
}

// sumAccountBalance returns the net balance (debits − credits) of the given accounts.
func (s *ledgerService) sumAccountBalance(ctx context.Context, codes ...string) (int64, error) {
	var balance int64
	err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(e.debit - e.credit),0)
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
		WHERE a.code IN ?`, codes).Scan(&balance).Error
	if err != nil {
		return 0, fmt.Errorf("reconciliation: account balance: %w", err)
	}
	return balance, nil
}

// SweepFees is the only legal path for user money to become platform money
// (Phase 7.7): DR 1000 operating cash / CR 1010 clearing, for fees earned but
// not yet swept. Nothing else moves custody funds except a fee rule or refund.
func (s *ledgerService) SweepFees(ctx context.Context) (int64, error) {
	var sweptToDate int64
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(e.debit),0)
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id
		WHERE a.code = ? AND e.ref = 'fee-sweep'`, models.AccountOperatingCash).Scan(&sweptToDate).Error; err != nil {
		return 0, fmt.Errorf("fee sweep: already swept: %w", err)
	}
	revenue, err := s.sumAccountBalance(ctx, models.AccountFeeRevenueTransaction, models.AccountFeeRevenuePerformance)
	if err != nil {
		return 0, err
	}
	// sumAccountBalance returns debits−credits, so negate: revenue is a credit balance.
	earned := -revenue
	sweepable := earned - sweptToDate
	if sweepable <= 0 {
		return 0, nil
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, perr := s.Post(ctx, tx,
			LedgerLine{AccountCode: models.AccountOperatingCash, Debit: sweepable, Description: "Fee sweep to operating account", Ref: "fee-sweep"},
			LedgerLine{AccountCode: models.AccountSettlementClearing, Credit: sweepable, Description: "Fee sweep from clearing", Ref: "fee-sweep"},
		)
		return perr
	})
	if err != nil {
		return 0, fmt.Errorf("fee sweep: %w", err)
	}
	logger.FromContext(ctx).Info("fee sweep completed", "swept_kobo", sweepable)
	return sweepable, nil
}

// BackfillFromWalletTransactions reconstructs the double-entry history for
// wallet transactions recorded before the ledger existed (Phase 7.3).
//
// Reconstruction is by transaction type — the same accounts the live paths
// post to — so a backfilled ledger is indistinguishable from one that was
// posted all along. Withdrawals completed before Phase 7 carried no fee, so
// their journal is gross-to-clearing; fee revenue only starts when the fee
// engine went live.
//
// IDEMPOTENT: a reference that already has entries is skipped, so re-running
// (or a partially completed run) never double-books.
func (s *ledgerService) BackfillFromWalletTransactions(ctx context.Context) (int64, error) {
	var posted int64

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doneRefs []string
		if err := tx.Raw(`SELECT DISTINCT ref FROM ledger_entries WHERE ref IS NOT NULL`).Scan(&doneRefs).Error; err != nil {
			return err
		}
		done := make(map[string]bool, len(doneRefs))
		for _, r := range doneRefs {
			done[r] = true
		}

		var txns []models.WalletTransaction
		if err := tx.Where("status = ?", "completed").Order("created_at ASC").Find(&txns).Error; err != nil {
			return err
		}

		for _, txn := range txns {
			if done[txn.Reference] {
				continue
			}
			userID := txn.UserID
			var lines []LedgerLine
			switch txn.Type {
			case "deposit":
				lines = []LedgerLine{
					{AccountCode: models.AccountSettlementClearing, Debit: txn.Amount, Description: "Deposit (backfilled)", Ref: txn.Reference},
					{AccountCode: models.AccountUserWallets, UserID: &userID, Credit: txn.Amount, Description: "Wallet credited (backfilled)", Ref: txn.Reference},
				}
			case "withdrawal":
				lines = []LedgerLine{
					{AccountCode: models.AccountUserWallets, UserID: &userID, Debit: txn.Amount, Description: "Withdrawal (backfilled)", Ref: txn.Reference},
					{AccountCode: models.AccountSettlementClearing, Credit: txn.Amount - txn.FeeKobo, Description: "Settlement to bank (backfilled)", Ref: txn.Reference},
				}
				if txn.FeeKobo > 0 {
					lines = append(lines, LedgerLine{AccountCode: models.AccountFeeRevenueTransaction, Credit: txn.FeeKobo, Description: "Transaction fee (backfilled)", Ref: txn.Reference})
				}
			case "investment":
				lines = []LedgerLine{
					{AccountCode: models.AccountUserWallets, UserID: &userID, Debit: txn.Amount, Description: "Investment (backfilled)", Ref: txn.Reference},
					{AccountCode: models.AccountInvestmentHoldings, UserID: &userID, Credit: txn.Amount, Description: "Property principal (backfilled)", Ref: txn.Reference},
				}
			case "refund":
				lines = []LedgerLine{
					{AccountCode: models.AccountInvestmentHoldings, UserID: &userID, Debit: txn.Amount, Description: "Investment refunded (backfilled)", Ref: txn.Reference},
					{AccountCode: models.AccountUserWallets, UserID: &userID, Credit: txn.Amount, Description: "Principal returned (backfilled)", Ref: txn.Reference},
				}
			case "transfer":
				lines = []LedgerLine{
					{AccountCode: models.AccountEarningsPayable, UserID: &userID, Debit: txn.Amount, Description: "Earnings moved to main (backfilled)", Ref: txn.Reference},
					{AccountCode: models.AccountUserWallets, UserID: &userID, Credit: txn.Amount, Description: "Earnings credited (backfilled)", Ref: txn.Reference},
				}
			default:
				// Types with no custody meaning (fee rows book separately) are
				// left to the reconciliation report rather than guessed at.
				continue
			}
			if _, err := s.Post(ctx, tx, lines...); err != nil {
				return fmt.Errorf("backfill %s (%s): %w", txn.Type, txn.Reference, err)
			}
			done[txn.Reference] = true
			posted++
		}
		return nil
	})
	if err != nil {
		return posted, fmt.Errorf("ledger backfill: %w", err)
	}

	if posted > 0 {
		logger.FromContext(ctx).Info("ledger backfilled from wallet history", "journals_posted", posted)
	}
	return posted, nil
}
