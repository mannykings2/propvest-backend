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
