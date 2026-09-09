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
	"time"
)

// Auto Webhook Watcher - Automatically credits wallet after payment callback
//
// This tool watches for new payment callbacks and automatically triggers
// the webhook to credit the wallet. Use this during local development
// to avoid manually running trigger_webhook.go after each payment.
//
// Usage:
//   go run tools/auto_webhook_watcher.go
//
// How it works:
//   1. Polls the database for pending payments
//   2. When a pending payment is found (user completed Paystack payment)
//   3. Automatically sends webhook to credit the wallet
//   4. Continues watching for new payments

const (
	webhookURL  = "http://localhost:8081/api/v1/webhooks/payment"
	secretKey   = "sk_test_bb30d89975e64d0787923e064e0af2df55478154"
	pollInterval = 3 * time.Second
)

func main() {
	fmt.Println("🔍 Auto Webhook Watcher Started")
	fmt.Println("Watching for new payments and auto-triggering webhooks...")
	fmt.Println("Press Ctrl+C to stop\n")

	processedRefs := make(map[string]bool)

	for {
		// Get pending payments from server
		payments, err := getPendingPayments()
		if err != nil {
			fmt.Printf("⚠️  Error fetching payments: %v\n", err)
			time.Sleep(pollInterval)
			continue
		}

		// Process each pending payment
		for _, ref := range payments {
			if processedRefs[ref] {
				continue // Already processed this reference
			}

			fmt.Printf("💰 New payment detected: %s\n", ref)
			fmt.Printf("   Triggering webhook...\n")

			if err := triggerWebhook(ref); err != nil {
				fmt.Printf("   ❌ Failed: %v\n", err)
			} else {
				fmt.Printf("   ✅ Wallet credited successfully!\n")
				processedRefs[ref] = true
			}
			fmt.Println()
		}

		time.Sleep(pollInterval)
	}
}

func getPendingPayments() ([]string, error) {
	// This is a simplified version - in reality you'd query the database
	// For now, return empty list (you can enhance this to connect to DB)
	return []string{}, nil
}

func triggerWebhook(reference string) error {
	// Create webhook payload matching Paystack format
	payload := map[string]interface{}{
		"event": "charge.success",
		"data": map[string]interface{}{
			"id":                 12345678,
			"domain":             "test",
			"status":             "success",
			"reference":          reference,
			"amount":             150000,
			"message":            nil,
			"gateway_response":   "Successful",
			"paid_at":            time.Now().Format(time.RFC3339),
			"created_at":         time.Now().Format(time.RFC3339),
			"channel":            "card",
			"currency":           "NGN",
			"ip_address":         "127.0.0.1",
			"metadata":           "",
			"fees":               2175,
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

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	signature := generateSignature(string(body), secretKey)

	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Paystack-Signature", signature)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func generateSignature(payload, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
