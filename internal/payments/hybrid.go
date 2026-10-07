package payments

import (
	"context"
	"net/http"
	"time"

	"github.com/mannykings2/propvest-backend/internal/config"
)

// HybridProvider uses real Paystack for deposits and Mock for withdrawals.
// This allows testing withdrawal flows without a registered Paystack business account,
// while still testing real deposit flows with Paystack test cards.
type HybridProvider struct {
	paystack *PaystackProvider
	mock     *MockProvider
}

// NewHybridProvider creates a provider that uses Paystack for deposits and Mock for withdrawals.
func NewHybridProvider(cfg *config.Config) Provider {
	return &HybridProvider{
		paystack: &PaystackProvider{
			secretKey:     cfg.PaystackSecretKey,
			webhookSecret: cfg.PaystackWebhookSecret,
			callbackURL:   cfg.BaseURL + "/api/v1/wallet/deposit/callback",
			http:          &http.Client{Timeout: 15 * time.Second},
		},
		mock: &MockProvider{},
	}
}

// Name returns the provider name
func (h *HybridProvider) Name() string {
	return "hybrid"
}

// InitializeDeposit uses Paystack (for real deposit testing)
func (h *HybridProvider) InitializeDeposit(ctx context.Context, email string, amountKobo int64, reference string) (*InitResult, error) {
	return h.paystack.InitializeDeposit(ctx, email, amountKobo, reference)
}

// VerifyTransaction uses Paystack (for real deposit verification)
func (h *HybridProvider) VerifyTransaction(ctx context.Context, reference string) (*VerifyResult, error) {
	return h.paystack.VerifyTransaction(ctx, reference)
}

// VerifyWebhookSignature uses Paystack (for real webhook verification)
func (h *HybridProvider) VerifyWebhookSignature(signature string, body []byte) bool {
	return h.paystack.VerifyWebhookSignature(signature, body)
}

// ResolveAccountNumber uses Mock (for withdrawal testing)
func (h *HybridProvider) ResolveAccountNumber(ctx context.Context, accountNumber, bankCode string) (*AccountResolution, error) {
	return h.mock.ResolveAccountNumber(ctx, accountNumber, bankCode)
}

// InitiateTransfer uses Mock (for withdrawal testing without business account)
func (h *HybridProvider) InitiateTransfer(ctx context.Context, req *TransferRequest) (*TransferResult, error) {
	return h.mock.InitiateTransfer(ctx, req)
}

// VerifyTransfer uses Mock (for withdrawal status checking)
func (h *HybridProvider) VerifyTransfer(ctx context.Context, transferCodeOrReference string) (*TransferResult, error) {
	return h.mock.VerifyTransfer(ctx, transferCodeOrReference)
}
