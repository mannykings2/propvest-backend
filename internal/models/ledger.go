package models

import (
	"time"

	"github.com/google/uuid"
)

// Chart-of-accounts codes (Phase 7). Postings reference accounts by code; the
// ledger service resolves code → row. See migration 000031 for the rationale.
const (
	// Asset
	AccountOperatingCash      = "1000" // platform bank — only fees may live here
	AccountSettlementClearing = "1010" // money in transit from the PSP
	AccountSuspense           = "9000" // unmatched settlement lines; must trend to 0

	// Liability (ring-fenced user money)
	AccountUserCustody       = "2000" // CONTROL total (reported, never posted to)
	AccountUserWallets       = "2010" // per-user; mirrors wallets.main_balance
	AccountWithdrawalReserve = "2100" // per-user; mirrors wallets.locked_balance
	AccountEarningsPayable   = "2020" // per-user; mirrors wallets.earnings_balance

	// Liability — users' property positions (investments credit it: the
	// platform owes those users property value, tracked in the investments tables)
	AccountInvestmentHoldings = "2200" // per-user property principal

	// Revenue — the platform's cut
	AccountFeeRevenueTransaction = "3000"
	AccountFeeRevenuePerformance = "3100"

	// Income — gross distributions before the split
	AccountRentalIncome = "4000"
)

// LedgerAccount is one row of the chart of accounts.
type LedgerAccount struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code      string    `gorm:"size:8;uniqueIndex;not null" json:"code"`
	Name      string    `gorm:"size:120;not null" json:"name"`
	Type      string    `gorm:"size:20;not null" json:"type"` // asset|liability|equity|revenue|income|expense
	IsActive  bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

func (LedgerAccount) TableName() string { return "ledger_accounts" }

// LedgerEntry is one leg of a balanced journal. Append-only: the table has a
// database trigger that rejects UPDATE and DELETE, so corrections are new
// (reversing) journals, never edits to history.
type LedgerEntry struct {
	ID        uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	JournalID uuid.UUID     `gorm:"type:uuid;not null;index" json:"journal_id"`
	AccountID uuid.UUID     `gorm:"type:uuid;not null;index" json:"account_id"`
	Account   LedgerAccount `gorm:"foreignKey:AccountID" json:"account,omitempty"`

	// UserID marks the sub-ledger owner for per-user accounts (2010/2100/2020/2200).
	UserID *uuid.UUID `gorm:"type:uuid;index" json:"user_id,omitempty"`
	// WalletTxnID links an entry to the wallet_transactions row it describes.
	WalletTxnID *uuid.UUID `gorm:"type:uuid" json:"wallet_txn_id,omitempty"`

	// Exactly one of Debit/Credit is non-zero (CHECK constraint in the DB).
	Debit  int64 `gorm:"not null;default:0;check:debit >= 0" json:"debit"`
	Credit int64 `gorm:"not null;default:0;check:credit >= 0" json:"credit"`

	Currency    string `gorm:"size:3;not null;default:'NGN'" json:"currency"`
	Description string `json:"description,omitempty"`
	Ref         string `gorm:"size:160;index" json:"ref,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

func (LedgerEntry) TableName() string { return "ledger_entries" }

// Fee-rule codes. Exactly two rules are seeded; the engine refuses to charge a
// withdrawal without an active transaction rule.
const (
	FeeRuleWithdrawal         = "withdrawal_transaction_fee"
	FeeRulePerformanceDefault = "performance_fee_default"
)

// FeeRule is a configured fee. rate_bp is basis points (100 = 1%); cap_kobo is
// the absolute ceiling in kobo (the ₦2,000 withdrawal cap is 200000).
type FeeRule struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Code      string    `gorm:"size:60;uniqueIndex;not null" json:"code"`
	Name      string    `gorm:"size:120;not null" json:"name"`
	FeeType   string    `gorm:"size:20;not null" json:"fee_type"` // transaction | performance
	Basis     string    `gorm:"size:40;not null" json:"basis"`    // withdrawal | distributed_profit
	RateBP    int       `gorm:"not null;default:0" json:"rate_bp"`
	CapKobo   *int64    `json:"cap_kobo,omitempty"`
	MinKobo   int64     `gorm:"not null;default:0" json:"min_kobo"`
	AppliesTo *string   `gorm:"size:60" json:"applies_to,omitempty"`
	IsActive  bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (FeeRule) TableName() string { return "fee_rules" }
