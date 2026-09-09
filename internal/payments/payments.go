// Package payments integrates external payment providers behind a single
// interface, so the wallet service never talks to Paystack (or Flutterwave)
// directly.
//
// WHY AN INTERFACE?
// -----------------
// The docs (1.3) put payment providers behind an abstraction (payments/paystack,
// payments/flutterwave, payments/shared). Depending on an interface means:
//   - We can run a MockProvider in development/tests (no real money, no network).
//   - We can add Flutterwave later without changing the wallet service.
//   - The wallet service stays testable (inject a fake provider).
//
// New() selects the implementation from PAYMENT_PROVIDER ("mock" by default).
package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/logger"
)

// Type definitions moved to provider.go to avoid duplication

// New selects the provider from config.
func New(cfg *config.Config) Provider {
	if cfg.PaymentProvider == "paystack" && cfg.PaystackSecretKey != "" {
		logger.Info("payment provider: Paystack")
		return &PaystackProvider{
			secretKey:     cfg.PaystackSecretKey,
			webhookSecret: cfg.PaystackWebhookSecret,
			callbackURL:   cfg.BaseURL + "/api/v1/wallet/deposit/callback",
			http:          &http.Client{Timeout: 15 * time.Second},
		}
	}
	logger.Info("payment provider: mock (no real charges)")
	return &MockProvider{}
}

// NewProvider is an alias kept for the composition root's readability.
func NewProvider(cfg *config.Config) Provider { return New(cfg) }

// MockProvider implementation moved to mock.go to avoid duplication

// ── Paystack provider ───────────────────────────────────────────────────────

// PaystackProvider talks to the real Paystack REST API.
type PaystackProvider struct {
	secretKey     string
	webhookSecret string
	callbackURL   string
	http          *http.Client
}

func (p *PaystackProvider) Name() string { return "paystack" }

// InitializeDeposit calls POST /transaction/initialize. Paystack expects the
// amount in kobo (the smallest unit) — which is exactly how we store money.
func (p *PaystackProvider) InitializeDeposit(ctx context.Context, email string, amountKobo int64, reference string) (*InitResult, error) {
	payload := map[string]any{
		"email":     email,
		"amount":    amountKobo,
		"reference": reference,
		"callback_url": p.callbackURL,
	}
	var out struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AuthorizationURL string `json:"authorization_url"`
			AccessCode       string `json:"access_code"`
			Reference        string `json:"reference"`
		} `json:"data"`
	}
	if err := p.post(ctx, "/transaction/initialize", payload, &out); err != nil {
		return nil, err
	}
	if !out.Status {
		return nil, fmt.Errorf("paystack initialize failed: %s", out.Message)
	}
	return &InitResult{
		AuthorizationURL: out.Data.AuthorizationURL,
		AccessCode:       out.Data.AccessCode,
		Reference:        out.Data.Reference,
	}, nil
}

// VerifyTransaction calls GET /transaction/verify/:reference.
func (p *PaystackProvider) VerifyTransaction(ctx context.Context, reference string) (*VerifyResult, error) {
	var out struct {
		Status bool `json:"status"`
		Data   struct {
			Status   string `json:"status"`
			Reference string `json:"reference"`
			Amount   int64  `json:"amount"`
			Customer struct {
				Email string `json:"email"`
			} `json:"customer"`
		} `json:"data"`
	}
	if err := p.get(ctx, "/transaction/verify/"+reference, &out); err != nil {
		return nil, err
	}
	return &VerifyResult{
		Reference:  out.Data.Reference,
		AmountKobo: out.Data.Amount,
		Status:     out.Data.Status,
		Email:      out.Data.Customer.Email,
	}, nil
}

// VerifyWebhookSignature validates the X-Paystack-Signature header, which is the
// HMAC-SHA512 of the raw request body keyed by our secret key.
func (p *PaystackProvider) VerifyWebhookSignature(signature string, body []byte) bool {
	key := p.webhookSecret
	if key == "" {
		key = p.secretKey // Paystack signs webhooks with the secret key
	}
	mac := hmac.New(sha512.New, []byte(key))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// post/get are tiny helpers around the Paystack REST API with bearer auth.
func (p *PaystackProvider) post(ctx context.Context, path string, body any, out any) error {
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.paystack.co"+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")
	return p.do(req, out)
}

func (p *PaystackProvider) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.paystack.co"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	return p.do(req, out)
}

func (p *PaystackProvider) do(req *http.Request, out any) error {
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("paystack API %s returned %d: %s", req.URL.Path, resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, out)
}

// ResolveAccountNumber verifies a bank account using Paystack's Bank Account Resolution API.
// API Docs: https://paystack.com/docs/identity-verification/resolve-account-number/
//
// This API calls the Nigerian Interbank Settlement System (NIBSS) to verify:
//   1. Account exists
//   2. Account name matches
//
// Example Response:
// {
//   "status": true,
//   "message": "Account number resolved",
//   "data": {
//     "account_number": "0123456789",
//     "account_name": "John Doe",
//     "bank_id": 9
//   }
// }
func (p *PaystackProvider) ResolveAccountNumber(ctx context.Context, accountNumber, bankCode string) (*AccountResolution, error) {
	// Build query parameters
	path := fmt.Sprintf("/bank/resolve?account_number=%s&bank_code=%s", accountNumber, bankCode)
	
	var response struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			AccountNumber string `json:"account_number"`
			AccountName   string `json:"account_name"`
			BankID        int    `json:"bank_id"`
		} `json:"data"`
	}
	
	if err := p.get(ctx, path, &response); err != nil {
		return nil, fmt.Errorf("failed to resolve account: %w", err)
	}
	
	if !response.Status {
		return nil, fmt.Errorf("account resolution failed: %s", response.Message)
	}
	
	// Map bank code to bank name (Paystack doesn't return this in resolution)
	bankNames := map[string]string{
		"044": "Access Bank", "014": "Afribank", "023": "Citibank",
		"050": "Ecobank", "084": "Enterprise Bank", "070": "Fidelity Bank",
		"011": "First Bank", "214": "First City Monument Bank",
		"058": "Guaranty Trust Bank", "030": "Heritage Bank",
		"301": "Jaiz Bank", "082": "Keystone Bank", "526": "Parallex Bank",
		"076": "Polaris Bank", "101": "Providus Bank", "125": "Rubies MFB",
		"221": "Stanbic IBTC Bank", "068": "Standard Chartered Bank",
		"232": "Sterling Bank", "100": "Suntrust Bank", "032": "Union Bank",
		"033": "United Bank for Africa", "215": "Unity Bank",
		"035": "Wema Bank", "057": "Zenith Bank",
	}
	
	bankName := bankNames[bankCode]
	if bankName == "" {
		bankName = fmt.Sprintf("Bank %s", bankCode)
	}
	
	return &AccountResolution{
		AccountNumber: response.Data.AccountNumber,
		AccountName:   response.Data.AccountName,
		BankCode:      bankCode,
		BankName:      bankName,
	}, nil
}

// InitiateTransfer sends money to a bank account using Paystack's Transfer API.
// API Docs: https://paystack.com/docs/transfers/single-transfers/
//
// IMPORTANT: Before using this API, you must:
//   1. Enable Transfers in your Paystack Dashboard
//   2. Set a Transfer PIN (Dashboard → Settings → API Keys & Webhooks)
//   3. Have sufficient balance in your Paystack account
//
// Request:
// POST /transfer
// {
//   "source": "balance",
//   "reason": "Wallet withdrawal",
//   "amount": 50000,
//   "recipient": "RCP_xxxxx"
// }
//
// Response:
// {
//   "status": true,
//   "message": "Transfer has been queued",
//   "data": {
//     "reference": "xxxxx",
//     "integration": 123456,
//     "domain": "test",
//     "amount": 50000,
//     "currency": "NGN",
//     "source": "balance",
//     "reason": "Wallet withdrawal",
//     "recipient": 123456,
//     "status": "pending",
//     "transfer_code": "TRF_xxxxx",
//     "created_at": "2024-01-01T00:00:00.000Z",
//     "updated_at": "2024-01-01T00:00:00.000Z"
//   }
// }
func (p *PaystackProvider) InitiateTransfer(ctx context.Context, req *TransferRequest) (*TransferResult, error) {
	// Validate request
	if req.Amount <= 0 {
		return nil, fmt.Errorf("invalid amount: must be positive")
	}
	if req.Reference == "" {
		return nil, fmt.Errorf("reference is required")
	}
	if req.AccountNumber == "" {
		return nil, fmt.Errorf("account number is required")
	}
	if req.BankCode == "" {
		return nil, fmt.Errorf("bank code is required")
	}
	
	// Set default currency
	currency := req.Currency
	if currency == "" {
		currency = "NGN"
	}
	
	// Set default reason
	reason := req.Reason
	if reason == "" {
		reason = "Wallet withdrawal"
	}
	
	// Paystack requires creating a transfer recipient first
	// We'll create an inline recipient (one-time use)
	recipientPayload := map[string]any{
		"type":           "nuban", // Nigerian Uniform Bank Account Number
		"name":           req.AccountName,
		"account_number": req.AccountNumber,
		"bank_code":      req.BankCode,
		"currency":       currency,
	}
	
	var recipientResponse struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			RecipientCode string `json:"recipient_code"`
			Type          string `json:"type"`
			Name          string `json:"name"`
			Active        bool   `json:"active"`
		} `json:"data"`
	}
	
	// Create transfer recipient
	if err := p.post(ctx, "/transferrecipient", recipientPayload, &recipientResponse); err != nil {
		return nil, fmt.Errorf("failed to create transfer recipient: %w", err)
	}
	
	if !recipientResponse.Status {
		return nil, fmt.Errorf("recipient creation failed: %s", recipientResponse.Message)
	}
	
	// Now initiate the actual transfer
	transferPayload := map[string]any{
		"source":    "balance",
		"reason":    reason,
		"amount":    req.Amount, // in kobo
		"recipient": recipientResponse.Data.RecipientCode,
		"reference": req.Reference,
	}
	
	var transferResponse struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Reference    string `json:"reference"`
			TransferCode string `json:"transfer_code"`
			Amount       int64  `json:"amount"`
			Currency     string `json:"currency"`
			Status       string `json:"status"` // pending, success, failed, reversed
			Reason       string `json:"reason"`
			RecipientID  int    `json:"recipient"`
			CreatedAt    string `json:"created_at"`
		} `json:"data"`
	}
	
	if err := p.post(ctx, "/transfer", transferPayload, &transferResponse); err != nil {
		return nil, fmt.Errorf("failed to initiate transfer: %w", err)
	}
	
	if !transferResponse.Status {
		return nil, fmt.Errorf("transfer initiation failed: %s", transferResponse.Message)
	}
	
	// Map Paystack status to our standard status
	// Paystack returns: pending, success, failed, reversed
	status := transferResponse.Data.Status
	failureReason := ""
	
	if status == "failed" {
		failureReason = transferResponse.Message
	}
	
	return &TransferResult{
		Status:        status,
		TransferCode:  transferResponse.Data.TransferCode,
		Reference:     transferResponse.Data.Reference,
		Amount:        transferResponse.Data.Amount,
		RecipientName: req.AccountName,
		BankName:      "", // We'd need to look this up
		CreatedAt:     transferResponse.Data.CreatedAt,
		FailureReason: failureReason,
	}, nil
}

// VerifyTransfer checks the status of a transfer using Paystack's Transfer Verification API.
// API Docs: https://paystack.com/docs/transfers/single-transfers/#verify-transfer
//
// GET /transfer/verify/:reference
//
// Response:
// {
//   "status": true,
//   "message": "Transfer retrieved",
//   "data": {
//     "reference": "xxxxx",
//     "integration": 123456,
//     "domain": "test",
//     "amount": 50000,
//     "currency": "NGN",
//     "source": "balance",
//     "reason": "Wallet withdrawal",
//     "recipient": 123456,
//     "status": "success",
//     "transfer_code": "TRF_xxxxx",
//     "created_at": "2024-01-01T00:00:00.000Z",
//     "updated_at": "2024-01-01T00:00:00.000Z"
//   }
// }
func (p *PaystackProvider) VerifyTransfer(ctx context.Context, transferCodeOrReference string) (*TransferResult, error) {
	// Paystack accepts either transfer_code or reference
	path := fmt.Sprintf("/transfer/verify/%s", transferCodeOrReference)
	
	var response struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Reference    string `json:"reference"`
			TransferCode string `json:"transfer_code"`
			Amount       int64  `json:"amount"`
			Currency     string `json:"currency"`
			Status       string `json:"status"`
			Reason       string `json:"reason"`
			FailureReason string `json:"failure_reason,omitempty"`
			CreatedAt    string `json:"created_at"`
			UpdatedAt    string `json:"updated_at"`
			Recipient    struct {
				Name string `json:"name"`
				Details struct {
					BankName string `json:"bank_name"`
				} `json:"details"`
			} `json:"recipient"`
		} `json:"data"`
	}
	
	if err := p.get(ctx, path, &response); err != nil {
		return nil, fmt.Errorf("failed to verify transfer: %w", err)
	}
	
	if !response.Status {
		return nil, fmt.Errorf("transfer verification failed: %s", response.Message)
	}
	
	failureReason := response.Data.FailureReason
	if failureReason == "" && response.Data.Status == "failed" {
		failureReason = response.Message
	}
	
	return &TransferResult{
		Status:        response.Data.Status,
		TransferCode:  response.Data.TransferCode,
		Reference:     response.Data.Reference,
		Amount:        response.Data.Amount,
		RecipientName: response.Data.Recipient.Name,
		BankName:      response.Data.Recipient.Details.BankName,
		CreatedAt:     response.Data.CreatedAt,
		FailureReason: failureReason,
	}, nil
}
