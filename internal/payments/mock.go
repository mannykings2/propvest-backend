package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
)

// ═══════════════════════════════════════════════════════════════════════════
// MOCK PAYMENT PROVIDER
// ═══════════════════════════════════════════════════════════════════════════
//
// PURPOSE OF THIS FILE:
// This is a fake payment provider for development and testing. It simulates
// the behavior of a real provider (Paystack, Flutterwave) without making actual
// API calls or processing real money.
//
// WHY DO WE NEED THIS?
//   1. DEVELOPMENT: Can't test with real money every time we make a code change
//   2. TESTING: Automated tests shouldn't charge real credit cards
//   3. SPEED: No network latency, instant responses
//   4. RELIABILITY: Tests don't fail because payment provider is down
//   5. COST: Avoid transaction fees during development
//   6. SAFETY: No risk of accidentally charging real cards
//
// WHEN TO USE MOCK VS REAL PROVIDER:
//   - Development environment: Use MockProvider
//   - Test environment: Use MockProvider
//   - Staging environment: Use real provider with test API keys
//   - Production environment: Use real provider with live API keys
//
// HOW IT SIMULATES REAL PROVIDERS:
//   1. Returns fake authorization URLs (no actual payment page)
//   2. Marks all payments as "success" immediately
//   3. Implements signature verification (so webhook code works)
//   4. Uses same interface as real providers (drop-in replacement)

// MockProvider is a fake payment provider for development and testing.
//
// STRUCT EXPLANATION:
// This struct holds configuration data needed by the mock provider.
// Right now it only has SecretKey, but we could add more fields later
// (e.g., FailureRate to simulate random payment failures).
type MockProvider struct {
	// SecretKey is used for webhook signature verification.
	//
	// WHY DO WE NEED THIS IN A MOCK?
	// Our webhook handler code calls VerifyWebhookSignature() regardless of
	// which provider is being used. If we don't implement it correctly,
	// our webhook handler will reject all webhooks during development!
	//
	// By implementing signature verification in the mock, we ensure:
	//   1. Webhook handler code is exercised during development
	//   2. Tests can verify signature validation works
	//   3. Switching to real provider doesn't break webhook handling
	SecretKey string
}

// NewMockProvider creates a new mock payment provider instance.
//
// WHAT IS A CONSTRUCTOR?
// In Go, we use factory functions (functions that return struct instances)
// instead of constructors like in Java/C++. By convention, they're named
// "New" + struct name.
//
// WHY USE A CONSTRUCTOR INSTEAD OF DIRECT INITIALIZATION?
//   1. Encapsulation: Hide initialization logic
//   2. Validation: Ensure struct is properly configured
//   3. Defaults: Set sensible default values
//   4. Future-proofing: Can add initialization logic later without breaking callers
//
// EXAMPLE WITHOUT CONSTRUCTOR:
//   provider := &MockProvider{SecretKey: "test-secret"} // Fragile, easy to forget fields
//
// EXAMPLE WITH CONSTRUCTOR:
//   provider := NewMockProvider() // Always correctly initialized
//
// RETURN TYPE:
// Returns *MockProvider (pointer) not MockProvider (value) because:
//   1. Efficiency: Passing pointers is cheaper than copying entire struct
//   2. Consistency: All methods use pointer receivers (see below)
//   3. Convention: Factory functions in Go typically return pointers
func NewMockProvider() *MockProvider {
	return &MockProvider{
		// Use a fixed secret key for development
		// In real providers, this would come from config/environment variables
		SecretKey: "mock-secret-key",
	}
}

// Name returns the provider identifier.
//
// INTERFACE METHOD:
// This method is part of the Provider interface contract.
// Every provider MUST implement this method.
//
// RECEIVER SYNTAX: (m *MockProvider)
// This is called a "method receiver" in Go. It's like "this" or "self" in other languages.
//   - m: variable name (convention: first letter of type name)
//   - *MockProvider: pointer receiver (can modify m's fields)
//
// POINTER VS VALUE RECEIVER:
//   - Pointer (*MockProvider): Can modify struct fields, more efficient
//   - Value (MockProvider): Cannot modify fields, copies entire struct
//
// RULE OF THUMB: Use pointer receivers unless you have a specific reason not to.
//
// RETURNS:
// String "mock" to identify this provider in logs and database records.
func (m *MockProvider) Name() string {
	return "mock"
}

// InitializeDeposit simulates creating a payment session with a provider.
//
// IN A REAL PROVIDER (e.g., Paystack):
//   1. Make HTTP POST to https://api.paystack.co/transaction/initialize
//   2. Send: {amount: 150000, email: "user@example.com", reference: "DEP-ABC123"}
//   3. Receive: {authorization_url: "https://checkout.paystack.com/xyz", reference: "DEP-ABC123"}
//   4. Return authorization_url to frontend
//   5. Frontend redirects user to authorization_url
//   6. User completes payment on Paystack's page
//
// IN THE MOCK:
//   1. No HTTP call (just return fake URL)
//   2. No actual payment page (URL is just a placeholder)
//   3. No actual payment processing
//   4. Developer manually triggers webhook to simulate completion
//
// HOW TO TEST WITH MOCK:
//   1. Call InitializeDeposit() → get fake URL
//   2. In Postman, call the webhook endpoint manually:
//      POST /webhooks/payment
//      {
//        "reference": "DEP-ABC123",
//        "status": "success",
//        "amount": 150000
//      }
//   3. Backend processes webhook and credits wallet
//
// PARAMETERS EXPLAINED:
//   - ctx: Context for request lifecycle (cancellation, timeouts, deadlines)
//   - email: User's email (real providers need this for receipts/notifications)
//   - amountKobo: Amount to charge in kobo (₦1500.00 = 150000 kobo)
//   - reference: Our unique reference for this transaction (e.g., "DEP-ABC123")
//
// WHY CONTEXT?
// Context is a Go standard library type that carries deadlines, cancellation
// signals, and request-scoped values across API boundaries.
//
// COMMON USE CASES:
//   - HTTP requests: If client disconnects, cancel expensive operations
//   - Database queries: Set timeouts to prevent hanging
//   - Background jobs: Cancel work when shutting down gracefully
//
// EXAMPLE:
//   ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//   defer cancel() // Release resources
//   result, err := provider.InitializeDeposit(ctx, email, amount, ref)
//   // If 5 seconds pass, ctx.Done() closes and query is cancelled
func (m *MockProvider) InitializeDeposit(ctx context.Context, email string, amountKobo int64, reference string) (*InitResult, error) {
	// In a real provider, we'd check ctx.Done() before making HTTP calls
	// to avoid wasting resources if request was cancelled
	select {
	case <-ctx.Done():
		// Context was cancelled (client disconnected, timeout reached, etc.)
		return nil, ctx.Err() // Returns context.Canceled or context.DeadlineExceeded
	default:
		// Context still active, proceed with operation
	}

	// Simulate the response a real provider would return
	//
	// MEMORY ALLOCATION:
	// Using &InitResult{...} creates a new InitResult on the heap and returns
	// its address (pointer). The heap is Go's memory area for dynamically
	// allocated data that outlives the function call.
	//
	// HEAP VS STACK:
	//   - Stack: Fast, automatic cleanup, limited size, function-scoped
	//   - Heap: Slower, garbage collected, unlimited size, outlives function
	//
	// When we return a pointer, Go's escape analysis automatically allocates
	// on the heap (because the value "escapes" the function).
	return &InitResult{
		// Fake authorization URL (in reality, this would be a real Paystack/Flutterwave URL)
		AuthorizationURL: fmt.Sprintf("https://mock-provider.com/pay/%s", reference),

		// AccessCode is optional
		AccessCode: "mock_access_code",

		// Echo back the reference we received (real providers do this for validation)
		Reference: reference,
	}, nil // nil error means success
}

// VerifyTransaction simulates server-side payment verification.
//
// IN A REAL PROVIDER:
//   1. Make HTTP GET to https://api.paystack.co/transaction/verify/{reference}
//   2. Authenticate with secret key in Authorization header
//   3. Receive: {status: "success", amount: 150000, reference: "1234567890"}
//   4. Return result to caller
//
// IN THE MOCK:
//   1. No HTTP call
//   2. Always returns "success" status
//   3. Returns the reference we received
//
// WHY VERIFY?
// CRITICAL SECURITY RULE: Never trust webhooks alone!
//
// ATTACK SCENARIO WITHOUT VERIFICATION:
//   1. Attacker intercepts a legitimate webhook
//   2. Attacker replays webhook 100 times
//   3. Without verification, we'd credit wallet 100 times for one payment
//   4. Attacker withdraws money → company loses money
//
// WITH VERIFICATION:
//   1. Webhook arrives (potentially forged)
//   2. We call VerifyTransaction(reference) with our secret key
//   3. Provider returns actual status from their database
//   4. If status != "success" or webhook reference not in provider DB, reject
//   5. Attacker cannot forge provider's database response
//
// PARAMETERS:
//   - ctx: Request context (for cancellation/timeout)
//   - reference: Our unique reference from InitializeDeposit
//
// RETURNS:
//   - *VerifyResult: Payment status, amount, provider reference
//   - error: Only if API call fails (network error, invalid auth, etc.)
//
// NOTE: Payment failure is NOT an error! It's a valid result with Status="failed"
//   - error: API call failed (couldn't reach provider)
//   - Status="failed": API call succeeded, but payment failed (card declined, etc.)
func (m *MockProvider) VerifyTransaction(ctx context.Context, reference string) (*VerifyResult, error) {
	// Check if context was cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Mock always returns success
	//
	// FOR TESTING FAILURE SCENARIOS:
	// You could enhance this by checking the reference:
	//   if strings.HasPrefix(reference, "FAIL-") {
	//       return &VerifyResult{Status: "failed", ...}, nil
	//   }
	//
	// This allows testing both success and failure cases in development.
	return &VerifyResult{
		// Status: "success" means payment was completed successfully
		// Other possible values: "failed", "pending", "abandoned"
		Status: "success",

		// AmountKobo in kobo (we'd need to look this up in reality)
		// For the mock, we just return a dummy amount
		// In real providers, this comes from their database
		AmountKobo: 0, // Caller should verify this matches expected amount

		// Provider's reference (in reality, this is different from our reference)
		// Example: Our reference = "DEP-ABC123", Provider reference = "1234567890"
		Reference: reference,

		// Email associated with the payment
		Email: "",

		// Currency code (ISO 4217)
		// NGN = Nigerian Naira
		Currency: "NGN",
	}, nil
}

// VerifyWebhookSignature validates webhook authenticity.
//
// WEBHOOK SECURITY PROBLEM:
// Webhooks are just HTTP POST requests. Anyone can send HTTP POST requests.
// Without authentication, attacker could send:
//
//   POST /webhooks/payment
//   {
//     "reference": "VICTIM-123",
//     "status": "success",
//     "amount": 1000000000
//   }
//
// And we'd credit victim's wallet ₦10,000,000!
//
// HOW SIGNATURE VERIFICATION SOLVES THIS:
//   1. Provider knows a secret key (only provider and us know it)
//   2. Provider computes: signature = HMAC-SHA512(secret_key, request_body)
//   3. Provider sends signature in HTTP header: X-Paystack-Signature
//   4. We compute same HMAC with our copy of secret_key
//   5. If signatures match → webhook is authentic (only someone with secret_key could create it)
//   6. If signatures differ → webhook is forged, reject it
//
// WHAT IS HMAC?
// HMAC (Hash-based Message Authentication Code) is a cryptographic algorithm that:
//   - Takes a message (request body) and secret key
//   - Produces a fixed-length signature
//   - Cannot be reversed (can't get secret_key from signature)
//   - Changes completely if message changes even 1 bit
//
// EXAMPLE:
//   message = "hello"
//   secret = "key123"
//   signature = HMAC-SHA512(secret, message) = "a1b2c3d4e5f6..."
//
//   message = "hellp" (changed 1 letter)
//   signature = HMAC-SHA512(secret, message) = "z9y8x7w6v5u4..." (completely different!)
//
// PARAMETERS:
//   - signature: Value from webhook's HTTP header (hex-encoded string)
//   - body: Raw request body bytes (BEFORE JSON parsing!)
//
// WHY RAW BYTES NOT PARSED JSON?
// JSON formatting matters! These produce different signatures:
//   {"status":"success","amount":1000}
//   {"amount":1000,"status":"success"}
//
// Provider computed signature from original JSON byte order.
// If we parse then re-serialize, order might change → signature mismatch!
//
// RULE: Always verify signature against raw body bytes.
func (m *MockProvider) VerifyWebhookSignature(signature string, body []byte) bool {
	// Compute expected signature using our secret key
	//
	// STEP-BY-STEP:
	// 1. Create new HMAC hasher with SHA-512 algorithm and our secret key
	mac := hmac.New(sha512.New, []byte(m.SecretKey))

	// 2. Write the message (request body) into the hasher
	//    This updates the hash state without returning anything yet
	mac.Write(body)

	// 3. Compute the final hash
	//    Sum(nil) returns the computed hash as a byte slice
	//    nil parameter means "don't append to anything, just return the hash"
	expectedMAC := mac.Sum(nil)

	// 4. Convert bytes to hex string for comparison
	//    Paystack sends signatures as hex-encoded strings like "a1b2c3..."
	//    not raw bytes like []byte{161, 178, 195, ...}
	expectedSignature := hex.EncodeToString(expectedMAC)

	// 5. Compare signatures in constant time
	//
	// WHY CONSTANT TIME COMPARISON?
	// Prevents timing attacks. Simple comparison (signature == expectedSignature)
	// stops at first different byte, which leaks information about how many
	// bytes matched.
	//
	// TIMING ATTACK:
	//   Attacker tries: "a..." → fails in 1ms
	//   Attacker tries: "b..." → fails in 1ms
	//   ...
	//   Attacker tries: "z..." → fails in 2ms (first byte matched!)
	//   Attacker now knows first byte is 'z', repeats for second byte
	//
	// hmac.Equal() always compares all bytes, preventing this attack.
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// ═══════════════════════════════════════════════════════════════════════════
// USAGE EXAMPLE
// ═══════════════════════════════════════════════════════════════════════════
//
// // In cmd/api/main.go:
// var paymentProvider payments.Provider
//
// if config.Environment == "development" {
//     paymentProvider = payments.NewMockProvider()
// } else {
//     paymentProvider = payments.NewPaystackProvider(config.PaystackSecretKey)
// }
//
// walletService := services.NewWalletService(
//     walletRepo,
//     userRepo,
//     paymentRepo,
//     paymentProvider, // <- Injected here
//     notificationService,
//     nil,
//     config,
//     db,
// )
//
// // In tests:
// func TestWalletDeposit(t *testing.T) {
//     mockProvider := payments.NewMockProvider()
//     service := services.NewWalletService(
//         mockRepo,
//         mockUserRepo,
//         mockPaymentRepo,
//         mockProvider, // <- No real API calls!
//         mockNotificationService,
//         nil,
//         mockConfig,
//         mockDB,
//     )
//
//     // Test deposit flow without hitting real payment provider
//     result, err := service.InitiateDeposit(ctx, userID, 150000)
//     assert.NoError(t, err)
//     assert.Contains(t, result.AuthorizationURL, "mock-provider.com")
// }
//
// ═══════════════════════════════════════════════════════════════════════════
// TESTING WEBHOOK FLOW WITH MOCK
// ═══════════════════════════════════════════════════════════════════════════
//
// 1. Start backend: make run
//
// 2. Register user and login (get auth token)
//
// 3. Initialize deposit:
//    POST /api/v1/wallet/deposit
//    {
//      "amount_kobo": 150000
//    }
//    Response:
//    {
//      "authorization_url": "https://mock-provider.com/pay/DEP-ABC123",
//      "reference": "DEP-ABC123"
//    }
//
// 4. Simulate webhook (pretend to be the payment provider):
//    POST /api/v1/webhooks/payment
//    X-Mock-Signature: <computed signature>
//    {
//      "reference": "DEP-ABC123",
//      "status": "success",
//      "amount": 150000
//    }
//
// 5. Check wallet balance:
//    GET /api/v1/wallet
//    Response:
//    {
//      "main_balance": 150000,
//      "main_balance_formatted": "₦1,500.00"
//    }
//
// ═══════════════════════════════════════════════════════════════════════════
// FUTURE ENHANCEMENTS
// ═══════════════════════════════════════════════════════════════════════════
//
// 1. Simulate random failures:
//    type MockProvider struct {
//        SecretKey   string
//        FailureRate float64 // 0.0 - 1.0 (0% - 100% chance of failure)
//    }
//
// 2. Simulate delays (test timeout handling):
//    time.Sleep(2 * time.Second) before returning
//
// 3. Simulate specific error codes:
//    if reference == "INSUFFICIENT_FUNDS" {
//        return &VerifyResult{Status: "failed", Message: "Insufficient funds"}, nil
//    }
//
// 4. Track calls for testing:
//    type MockProvider struct {
//        Calls []string // Records all method calls
//    }
//    func (m *MockProvider) InitializeDeposit(...) {
//        m.Calls = append(m.Calls, "InitializeDeposit")
//        // ...
//    }
