package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════
// TRANSFER WEBHOOK TRIGGER TOOL
// ═══════════════════════════════════════════════════════════════════════════
//
// PURPOSE:
// Manually trigger transfer webhooks for testing withdrawal finalization
// without waiting for real Paystack transfers to complete.
//
// USAGE:
//   go run trigger_transfer_webhook.go --reference WD-ABC123 --event success
//   go run trigger_transfer_webhook.go --reference WD-ABC123 --event failed
//
// This simulates what Paystack would send when a transfer completes.

func main() {
	// Command-line flags
	reference := flag.String("reference", "", "Withdrawal reference (e.g., WD-ABC123)")
	event := flag.String("event", "success", "Event type: success, failed, or reversed")
	transferCode := flag.String("transfer-code", "", "Paystack transfer code (auto-generated if empty)")
	reason := flag.String("reason", "", "Failure reason (for failed events)")
	url := flag.String("url", "http://localhost:8081/api/v1/webhooks/paystack/transfer", "Webhook URL")
	secretKey := flag.String("secret", "", "Paystack secret key (from .env if empty)")

	flag.Parse()

	// Validate required flags
	if *reference == "" {
		fmt.Println("❌ Error: --reference is required")
		fmt.Println("\nUsage:")
		fmt.Println("  go run trigger_transfer_webhook.go --reference WD-ABC123 --event success")
		fmt.Println("  go run trigger_transfer_webhook.go --reference WD-ABC123 --event failed --reason 'Invalid account'")
		os.Exit(1)
	}

	// Get secret key from environment if not provided
	if *secretKey == "" {
		*secretKey = os.Getenv("PAYSTACK_SECRET_KEY")
		if *secretKey == "" {
			// Try to read from .env file
			*secretKey = readSecretFromEnv()
			if *secretKey == "" {
				fmt.Println("❌ Error: PAYSTACK_SECRET_KEY not found")
				fmt.Println("Set it via --secret flag or PAYSTACK_SECRET_KEY environment variable")
				os.Exit(1)
			}
		}
	}

	// Auto-generate transfer code if not provided
	if *transferCode == "" {
		*transferCode = fmt.Sprintf("TRF_test_%d", time.Now().Unix())
	}

	// Map event to status
	var status string
	switch *event {
	case "success":
		status = "success"
	case "failed":
		status = "failed"
	case "reversed":
		status = "reversed"
	default:
		fmt.Printf("❌ Error: Invalid event type '%s'\n", *event)
		fmt.Println("Valid events: success, failed, reversed")
		os.Exit(1)
	}

	// Build webhook payload
	payload := map[string]interface{}{
		"event": fmt.Sprintf("transfer.%s", *event),
		"data": map[string]interface{}{
			"reference":     *reference,
			"transfer_code": *transferCode,
			"status":        status,
			"amount":        5000000, // ₦50,000 (example amount)
			"currency":      "NGN",
			"reason":        *reason,
			"recipient": map[string]interface{}{
				"type":           "nuban",
				"account_number": "0123456789",
				"bank_code":      "058",
				"bank_name":      "GTBank Plc",
			},
			"createdAt": time.Now().Format(time.RFC3339),
		},
	}

	// Marshal to JSON
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Printf("❌ Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	// Compute HMAC signature
	mac := hmac.New(sha512.New, []byte(*secretKey))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	// Print details
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("📤 SENDING TRANSFER WEBHOOK")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Reference:     %s\n", *reference)
	fmt.Printf("Event:         transfer.%s\n", *event)
	fmt.Printf("Status:        %s\n", status)
	fmt.Printf("Transfer Code: %s\n", *transferCode)
	if *reason != "" {
		fmt.Printf("Reason:        %s\n", *reason)
	}
	fmt.Printf("URL:           %s\n", *url)
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Println("\n📋 Payload:")
	fmt.Println(string(body))
	fmt.Println("\n🔐 Signature:")
	fmt.Println(signature)
	fmt.Println("═══════════════════════════════════════════════════════════")

	// Send HTTP request
	req, err := http.NewRequest("POST", *url, bytes.NewReader(body))
	if err != nil {
		fmt.Printf("❌ Error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Paystack-Signature", signature)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Error sending request: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Error reading response: %v\n", err)
		os.Exit(1)
	}

	// Print response
	fmt.Println("\n📥 RESPONSE")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Status:  %d %s\n", resp.StatusCode, resp.Status)
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Println("Body:")
	
	// Pretty print JSON response if possible
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(respBody, &prettyJSON); err == nil {
		formatted, _ := json.MarshalIndent(prettyJSON, "", "  ")
		fmt.Println(string(formatted))
	} else {
		fmt.Println(string(respBody))
	}
	fmt.Println("═══════════════════════════════════════════════════════════")

	// Check result
	if resp.StatusCode == 200 {
		fmt.Println("\n✅ Webhook delivered successfully!")
		
		// Provide helpful next steps
		fmt.Println("\n💡 Next Steps:")
		fmt.Println("1. Check API logs for processing confirmation")
		fmt.Println("2. Check worker logs for finalization")
		fmt.Println("3. Verify wallet balance updated")
		fmt.Println("4. Check transaction status in database:")
		fmt.Printf("   SELECT * FROM wallet_transactions WHERE reference = '%s';\n", *reference)
	} else {
		fmt.Printf("\n⚠️ Webhook returned status %d\n", resp.StatusCode)
		fmt.Println("Check API logs for error details")
		os.Exit(1)
	}
}

// readSecretFromEnv tries to read PAYSTACK_SECRET_KEY from .env file
func readSecretFromEnv() string {
	data, err := os.ReadFile(".env")
	if err != nil {
		return ""
	}

	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("PAYSTACK_SECRET_KEY=")) {
			parts := bytes.SplitN(line, []byte("="), 2)
			if len(parts) == 2 {
				return string(bytes.Trim(parts[1], `"`))
			}
		}
	}
	return ""
}
