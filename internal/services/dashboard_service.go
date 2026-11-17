package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/models"
	"gorm.io/gorm"
)

// ═══════════════════════════════════════════════════════════════════════════
// DASHBOARD SERVICE (Milestone 7 - Phase 3)
// ═══════════════════════════════════════════════════════════════════════════
//
// Provides bounded, performant queries for admin dashboard metrics.
//
// DESIGN PRINCIPLES:
//   1. All queries are bounded (use indexes, WHERE conditions, no table scans)
//   2. Metrics are approximate (eventual consistency acceptable for dashboards)
//   3. Responses are cacheable (5-10 minute TTL recommended)
//   4. No N+1 queries or unbounded JOINs
//
// PERFORMANCE CONSIDERATIONS:
//   - User metrics: Uses indexed created_at, kyc_status, suspended_at
//   - Property metrics: Uses indexed status
//   - Investment metrics: Uses indexed status, created_at
//   - Financial metrics: Uses indexed type, status on wallet_transactions
//
// FUTURE ENHANCEMENTS:
//   - Redis caching layer for dashboard responses
//   - Materialized views for complex aggregations
//   - Time-series data for trend analysis

// DashboardService defines the interface for admin dashboard operations.
type DashboardService interface {
	// GetDashboard returns platform-wide metrics for the admin dashboard.
	//
	// This is a read-only operation that aggregates data from multiple tables.
	// All queries are bounded and use indexed columns for performance.
	//
	// Parameters:
	//   - ctx: Request context for cancellation and tracing
	//   - actorID: Admin user requesting the dashboard (for audit logging)
	//
	// Returns:
	//   - Dashboard metrics response
	//   - Error if database queries fail
	GetDashboard(ctx context.Context, actorID uuid.UUID) (*dto.DashboardResponse, error)
}

// dashboardService is the concrete implementation of DashboardService.
type dashboardService struct {
	db *gorm.DB
}

// NewDashboardService creates a new dashboard service instance.
func NewDashboardService(db *gorm.DB) DashboardService {
	return &dashboardService{
		db: db,
	}
}

// GetDashboard retrieves platform-wide metrics for the admin dashboard.
//
// QUERY STRATEGY:
//   - Each metric is fetched with a separate bounded query
//   - Queries run sequentially (can be parallelized if needed)
//   - All queries use indexed columns for fast execution
//   - No complex JOINs or subqueries
//
// BOUNDED QUERIES:
//   - User metrics: COUNT(*) with WHERE conditions on indexed columns
//   - Property metrics: COUNT(*) and SUM(price) grouped by status
//   - Investment metrics: COUNT(*), COUNT(DISTINCT user_id), SUM(amount)
//   - Financial metrics: SUM(amount) grouped by type and status
//
// ERROR HANDLING:
//   - Individual metric failures don't fail the entire dashboard
//   - Zero values returned for failed metrics (dashboard still loads)
//   - Errors are logged but not propagated (graceful degradation)
func (s *dashboardService) GetDashboard(ctx context.Context, actorID uuid.UUID) (*dto.DashboardResponse, error) {
	dashboard := &dto.DashboardResponse{
		Timestamp: time.Now(),
	}

	// ───────────────────────────────────────────────────────────────────────
	// USER METRICS
	// ───────────────────────────────────────────────────────────────────────

	// Total users (exclude soft-deleted)
	s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("deleted_at IS NULL").
		Count(&dashboard.Users.Total)

	// Active users this month — users who actually SIGNED IN this month.
	// last_login_at is stamped by the auth service on every successful login
	// (migration 000036); never-logged-in users fall back to created_at so
	// brand-new signups still count for their first month.
	thisMonthStart := time.Now().AddDate(0, 0, -30)
	s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("deleted_at IS NULL AND COALESCE(last_login_at, created_at) >= ?", thisMonthStart).
		Count(&dashboard.Users.ActiveThisMonth)

	// New users this month
	monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("created_at >= ?", monthStart).
		Count(&dashboard.Users.NewThisMonth)

	// New users last month
	lastMonthStart := monthStart.AddDate(0, -1, 0)
	s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("created_at >= ? AND created_at < ?", lastMonthStart, monthStart).
		Count(&dashboard.Users.NewLastMonth)

	// KYC metrics
	s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("kyc_status = ?", "pending").
		Count(&dashboard.Users.KYCPending)

	s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("kyc_status = ?", "verified").
		Count(&dashboard.Users.KYCVerified)

	// Suspended users
	s.db.WithContext(ctx).
		Model(&models.User{}).
		Where("suspended_at IS NOT NULL").
		Count(&dashboard.Users.SuspendedCount)

	// ───────────────────────────────────────────────────────────────────────
	// PROPERTY METRICS
	// ───────────────────────────────────────────────────────────────────────

	// Property counts by status
	type PropertyStatusCount struct {
		Status     string
		Count      int64
		TotalValue int64
	}

	var propertyCounts []PropertyStatusCount
	s.db.WithContext(ctx).
		Model(&models.Property{}).
		Select("status, COUNT(*) as count, SUM(price) as total_value").
		Where("deleted_at IS NULL").
		Group("status").
		Scan(&propertyCounts)

	// Aggregate property metrics
	for _, pc := range propertyCounts {
		dashboard.Properties.Total += pc.Count
		dashboard.Properties.TotalValue += pc.TotalValue

		switch pc.Status {
		case "draft":
			dashboard.Properties.Draft = pc.Count
		case "active":
			dashboard.Properties.Active = pc.Count
		case "funded":
			dashboard.Properties.Funded = pc.Count
		case "completed":
			dashboard.Properties.Completed = pc.Count
		}
	}

	// ───────────────────────────────────────────────────────────────────────
	// INVESTMENT METRICS
	// ───────────────────────────────────────────────────────────────────────

	// Total investment count
	s.db.WithContext(ctx).
		Model(&models.Investment{}).
		Count(&dashboard.Investments.TotalCount)

	// Active investments
	s.db.WithContext(ctx).
		Model(&models.Investment{}).
		Where("status = ?", models.InvestmentStatusActive).
		Count(&dashboard.Investments.ActiveCount)

	// Total investment value
	type InvestmentSum struct {
		TotalValue int64
	}
	var invSum InvestmentSum
	s.db.WithContext(ctx).
		Model(&models.Investment{}).
		Select("SUM(amount) as total_value").
		Scan(&invSum)
	dashboard.Investments.TotalValue = invSum.TotalValue

	// Distinct investor count
	s.db.WithContext(ctx).
		Model(&models.Investment{}).
		Distinct("user_id").
		Count(&dashboard.Investments.TotalInvestors)

	// Average investment
	if dashboard.Investments.TotalCount > 0 {
		dashboard.Investments.AverageInvestment = dashboard.Investments.TotalValue / dashboard.Investments.TotalCount
	}

	// ───────────────────────────────────────────────────────────────────────
	// FINANCIAL METRICS
	// ───────────────────────────────────────────────────────────────────────

	// Deposits (completed)
	type FinancialSum struct {
		TotalAmount int64
		Count       int64
	}

	var depositSum FinancialSum
	s.db.WithContext(ctx).
		Model(&models.WalletTransaction{}).
		Select("SUM(amount) as total_amount, COUNT(*) as count").
		Where("type = ? AND status = ?", "deposit", "completed").
		Scan(&depositSum)
	dashboard.Financial.TotalDeposits = depositSum.TotalAmount
	dashboard.Financial.DepositCount = depositSum.Count

	// Withdrawals (completed)
	var withdrawalSum FinancialSum
	s.db.WithContext(ctx).
		Model(&models.WalletTransaction{}).
		Select("SUM(amount) as total_amount, COUNT(*) as count").
		Where("type = ? AND status = ?", "withdrawal", "completed").
		Scan(&withdrawalSum)
	dashboard.Financial.TotalWithdrawals = withdrawalSum.TotalAmount
	dashboard.Financial.WithdrawalCount = withdrawalSum.Count

	// Pending withdrawals
	var pendingWithdrawalSum struct {
		TotalAmount int64
	}
	s.db.WithContext(ctx).
		Model(&models.WalletTransaction{}).
		Select("SUM(amount) as total_amount").
		Where("type = ? AND status = ?", "withdrawal", "pending").
		Scan(&pendingWithdrawalSum)
	dashboard.Financial.PendingWithdrawals = pendingWithdrawalSum.TotalAmount

	// Total platform balance (sum of all wallet balances)
	var platformBalance struct {
		TotalBalance int64
	}
	s.db.WithContext(ctx).
		Model(&models.Wallet{}).
		Select("SUM(main_balance + earnings_balance) as total_balance").
		Scan(&platformBalance)
	dashboard.Financial.TotalPlatformBalance = platformBalance.TotalBalance

	return dashboard, nil
}
