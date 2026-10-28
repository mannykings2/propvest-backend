package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

// This tool manually triggers a webhook to credit your wallet after a test payment.
// Use this during local development when Paystack cannot reach your localhost.
//
// Signature: the API verifies X-Paystack-Signature as HMAC-SHA512 of the raw
// body keyed with PAYSTACK_WEBHOOK_SECRET (falling back to PAYSTACK_SECRET_KEY).
// This tool loads the repo's .env and uses the SAME key, so a local webhook
// validates. Export the variable to override, or set PROPVEST_WEBHOOK_URL to
// target a non-default API origin.
//
// Usage:
//   go run tools/trigger_webhook <payment-reference>
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run tools/trigger_webhook.go <payment-reference>")
		fmt.Println("Example: go run tools/trigger_webhook.go DEP-759B35C0-EB0")
		os.Exit(1)
	}

	reference := os.Args[1]
	
	// This is the webhook payload Paystack sends when a payment succeeds
	// Matches the actual Paystack webhook format
	payload := map[string]interface{}{
		"event": "charge.success",
		"data": map[string]interface{}{
			"id":             12345678,
			"domain":         "test",
			"status":         "success",
			"reference":      reference,
			"amount":         150000, // In kobo (₦1,500)
			"message":        nil,
			"gateway_response": "Successful",
			"paid_at":        "2024-01-01T12:00:00.000Z",
			"created_at":     "2024-01-01T12:00:00.000Z",
			"channel":        "card",
			"currency":       "NGN",
			"ip_address":     "127.0.0.1",
			"metadata":       "",
			"fees":           2175,
			"customer": map[string]interface{}{
				"id":            1234,
				"first_name":    "Test",
				"last_name":     "User",
				"email":         "test@example.com",
				"customer_code": "CUS_test123",
				"phone":         nil,
				"metadata":      nil,
				"risk_action":   "default",
			},
			"authorization": map[string]interface{}{
				"authorization_code": "AUTH_test123",
				"bin":                "408408",
				"last4":              "4081",
				"exp_month":          "12",
				"exp_year":           "2025",
				"channel":            "card",
				"card_type":          "visa ",
				"bank":               "TEST BANK",
				"country_code":       "NG",
				"brand":              "visa",
				"reusable":           true,
				"signature":          "SIG_test123",
				"account_name":       nil,
			},
		},
	}

	// Convert to JSON
	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Failed to marshal payload: %v\n", err)
		os.Exit(1)
	}

	// Load the API's .env (same file the server reads) unless the variables are
	// already in the environment. A hard-coded placeholder secret can never
	// produce a signature the API accepts — that was the old failure mode.
	_ = godotenv.Load(".env")

	secret := os.Getenv("PAYSTACK_WEBHOOK_SECRET")
	if secret == "" {
		// Paystack signs webhooks with the secret key when no webhook secret is set.
		secret = os.Getenv("PAYSTACK_SECRET_KEY")
	}
	if secret == "" {
		fmt.Println("ERROR: PAYSTACK_WEBHOOK_SECRET (or PAYSTACK_SECRET_KEY) is not set.")
		fmt.Println("Run from the repo root so .env is loaded, or export the variable.")
		os.Exit(1)
	}
	signature := generateSignature(string(body), secret)

	// Send webhook to your local server
	url := os.Getenv("PROPVEST_WEBHOOK_URL")
	if url == "" {
		url = "http://localhost:8081/api/v1/webhooks/payment"
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Paystack-Signature", signature)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Failed to send webhook: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	fmt.Printf("✅ Webhook sent!\n")
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(respBody))
	
	if resp.StatusCode == 200 {
		fmt.Printf("\n✅ Success! Your wallet should now be credited.\n")
		fmt.Printf("Check your wallet balance with: GET /api/v1/wallet\n")
	} else {
		fmt.Printf("\n⚠️ Webhook failed. Check the server logs for details.\n")
	}
}

func generateSignature(payload, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
