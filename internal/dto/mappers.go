package dto

import (
	"github.com/mannykings2/propvest-backend/internal/models"
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