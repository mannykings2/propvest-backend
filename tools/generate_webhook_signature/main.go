package main

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// This tool generates HMAC-SHA512 signatures for testing webhooks in Postman.
//
// Usage:
//   go run tools/generate_webhook_signature.go
//
// It will output the signature you need to put in the X-Paystack-Signature header.

func main() {
	// The secret key used by the mock provider
	// In production, this would be your actual Paystack webhook secret
	secretKey := "mock-secret-key"

	// Example webhook payload
	// Modify this to match your test scenario
	payload := map[string]interface{}{
		"event": "charge.success",
		"data": map[string]interface{}{
			"reference":  "DEP-ABC123",
			"amount":     1500000, // Amount in kobo
			"status":     "success",
			"paid_at":    "2026-08-10T05:08:06.000Z",
			"channel":    "card",
			"currency":   "NGN",
			"ip_address": "::1",
			"customer": map[string]interface{}{
				"email": "user@example.com",
			},
		},
	}

	// Convert payload to JSON
	jsonBody, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		os.Exit(1)
	}

	// Generate signature
	signature := generateSignature(string(jsonBody), secretKey)

	// Output results
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("WEBHOOK SIGNATURE GENERATOR FOR POSTMAN TESTING")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("SECRET KEY:")
	fmt.Println(secretKey)
	fmt.Println()
	fmt.Println("REQUEST BODY (copy this to Postman Body > raw > JSON):")
	fmt.Println(string(jsonBody))
	fmt.Println()
	fmt.Println("SIGNATURE (copy this to X-Paystack-Signature header):")
	fmt.Println(signature)
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("POSTMAN SETUP:")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("1. Method: POST")
	fmt.Println("2. URL: http://localhost:8081/api/v1/webhooks/payment")
	fmt.Println("3. Headers:")
	fmt.Println("   Content-Type: application/json")
	fmt.Println("   X-Paystack-Signature:", signature)
	fmt.Println("4. Body: (paste the JSON above)")
	fmt.Println("═══════════════════════════════════════════════════════════")
}

func generateSignature(body, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}
