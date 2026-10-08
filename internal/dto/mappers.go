package dto

import (
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
)

// UserToResponse converts a models.User into a UserResponse DTO.
// This is the ONLY place in the codebase that does this conversion.
//
// Why centralise this?
// If you later add a field to UserResponse (e.g. "profile_picture_url"),
// you add it here once — and every endpoint that returns a user is
// automatically updated. Without a mapper, you hunt through every handler
// making the same change repeatedly.
func UserToResponse(user models.User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		UserCode:  user.UserCode,
		FullName:  user.FullName,
		Email:     user.Email,
		Phone:     user.Phone,
		KYCStatus: user.KYCStatus,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}
}

// WalletToResponse converts a models.Wallet into a WalletResponse DTO.
func WalletToResponse(wallet models.Wallet) WalletResponse {
	return WalletResponse{
		ID:              wallet.ID,
		UserID:          wallet.UserID,
		MainBalance:     wallet.MainBalance,
		EarningsBalance: wallet.EarningsBalance,
		VirtualAcctNo:   wallet.VirtualAcctNo,
		VirtualBank:     wallet.VirtualBank,
		CreatedAt:       wallet.CreatedAt,
	}
}

// WalletToSummary converts a models.Wallet into the lighter summary shape.
func WalletToSummary(wallet models.Wallet) WalletSummaryResponse {
	return WalletSummaryResponse{
		MainBalance:     wallet.MainBalance,
		EarningsBalance: wallet.EarningsBalance,
	}
}

// InvestmentToResponse converts a models.Investment into an InvestmentResponse DTO.
// Optionally includes the property details if preloaded.
func InvestmentToResponse(inv models.Investment) InvestmentResponse {
	resp := InvestmentResponse{
		ID:            inv.ID,
		PropertyID:    inv.PropertyID,
		Slots:         inv.Slots,
		AmountKobo:    inv.AmountKobo,
		UnitPriceKobo: inv.UnitPriceKobo,
		Currency:      inv.Currency,
		Status:        inv.Status,
		Reference:     inv.Reference,
		CancelledAt:   inv.CancelledAt,
		CompletedAt:   inv.CompletedAt,
		RefundedAt:    inv.RefundedAt,
		CreatedAt:     inv.CreatedAt,
		UpdatedAt:     inv.UpdatedAt,
	}
	
	// TODO: Include property if preloaded once PropertyToResponse mapper exists
	// if inv.Property.ID != uuid.Nil {
	// 	property := PropertyToResponse(inv.Property)
	// 	resp.Property = &property
	// }
	
	return resp
}

// InvestmentsToResponse converts a slice of investments to DTOs.
func InvestmentsToResponse(investments []models.Investment) []InvestmentResponse {
	responses := make([]InvestmentResponse, len(investments))
	for i, inv := range investments {
		responses[i] = InvestmentToResponse(inv)
	}
	return responses
}

// InvestmentMetricsToResponse converts repository metrics to response DTO.
// Includes naira conversions for convenience (1 naira = 100 kobo).
func InvestmentMetricsToResponse(metrics *repositories.InvestmentMetrics) InvestmentMetricsResponse {
	return InvestmentMetricsResponse{
		TotalInvestments:       metrics.TotalInvestments,
		TotalAmountKobo:        metrics.TotalAmountKobo,
		TotalAmountNaira:       float64(metrics.TotalAmountKobo) / 100.0,
		ActiveInvestments:      metrics.ActiveInvestments,
		CompletedInvestments:   metrics.CompletedInvestments,
		CancelledInvestments:   metrics.CancelledInvestments,
		RefundedInvestments:    metrics.RefundedInvestments,
		UniqueInvestors:        metrics.UniqueInvestors,
		AverageInvestmentKobo:  metrics.AverageInvestmentKobo,
		AverageInvestmentNaira: float64(metrics.AverageInvestmentKobo) / 100.0,
	}
}
