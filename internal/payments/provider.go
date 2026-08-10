package payments

import "context"

// ═══════════════════════════════════════════════════════════════════════════
// PAYMENT PROVIDER ABSTRACTION
// ═══════════════════════════════════════════════════════════════════════════
//
// PURPOSE OF THIS FILE:
// This file defines the contract (interface) that all payment providers must
// implement. By programming to an interface instead of a concrete implementation,
// we can:
//   1. Swap providers without changing business logic (Paystack → Flutterwave)
//   2. Test our code with mock providers (no real API calls in tests)
//   3. Support multiple providers simultaneously (user choice)
//   4. Add new providers without modifying existing code (Open/Closed Principle)
//
// This is a core principle of SOLID design: Dependency Inversion Principle (DIP).
// High-level modules (WalletService) should not depend on low-level modules
// (PaystackProvider). Both should depend on abstractions (Provider interface).

// Provider is the abstraction for any payment gateway integration.
//
// WHAT IS A PAYMENT PROVIDER?
// A payment provider (like Paystack, Flutterwave, Stripe) is a service that:
//   - Hosts payment pages where users enter card details
//   - Processes the actual payment securely
//   - Sends webhooks to notify us of payment status
//   - Handles the complexity of payment processing (PCI compliance, 3D Secure, etc.)
//
// WHY USE A PROVIDER?
// Building payment processing yourself is:
//   - Expensive (requires PCI DSS compliance - costs $50k+/year)
//   - Risky (card data breaches = massive fines + reputation damage)
//   - Complex (fraud detection, chargebacks, multiple payment methods)
//   - Time-consuming (integrations with banks, card networks)
//
// Providers charge ~1.5-3% per transaction but handle ALL of this for you.
type Provider interface {
	// Name returns a unique identifier for this provider ("paystack", "flutterwave", "mock").
	// Used for:
	//   - Logging which provider processed a payment
	//   - Storing provider name in the payments table
	//   - Debugging webhook delivery issues
	Name() string

	// InitializeDeposit starts the payment flow by asking the provider for a
	// hosted payment URL where the user can enter their card details.
	//
	// WHAT HAPPENS:
	//   1. We call provider API with amount + user email + our reference
	//   2. Provider creates a payment session and returns a URL
	//   3. We redirect user to that URL
	//   4. User enters card details on provider's secure page
	//   5. Provider processes payment
	//   6. Provider redirects user back to our app
	//   7. Provider sends webhook to notify us of result
	//
	// PARAMETERS:
	//   - ctx: Request context for cancellation and deadlines
	//   - email: User's email (provider needs this for receipts)
	//   - amountKobo: Amount in kobo (NGN smallest unit, like cents)
	//   - reference: Our unique reference for idempotency and reconciliation
	//
	// RETURNS:
	//   - *InitResult: Contains the authorization_url to redirect user to
	//   - error: If provider API fails (network issue, invalid amount, etc.)
	//
	// WHY KOBO NOT NAIRA?
	// Financial rule: NEVER use floats for money (0.1 + 0.2 = 0.30000000000000004).
	// Use smallest currency unit as integer: ₦15.50 = 1550 kobo.
	InitializeDeposit(ctx context.Context, email string, amountKobo int64, reference string) (*InitResult, error)

	// VerifyTransaction does server-side verification of a payment after the user
	// completes (or abandons) the flow.
	//
	// WHY VERIFY?
	// NEVER trust the webhook alone! Webhooks can be:
	//   - Spoofed by attackers (fake "payment successful" to credit their wallet)
	//   - Delayed or lost (network issues, provider downtime)
	//   - Replayed (attacker intercepts legitimate webhook, sends it multiple times)
	//
	// ALWAYS verify with the provider's API using your secret key.
	//
	// WHEN TO CALL:
	//   1. User returns from payment page (callback URL)
	//   2. We receive a webhook notification
	//
	// FLOW:
	//   1. Extract reference from webhook/callback
	//   2. Call VerifyTransaction(reference)
	//   3. Provider returns current status from their database
	//   4. If status = "success" AND amount matches, credit wallet
	//
	// RETURNS:
	//   - *VerifyResult: Status ("success", "failed", "pending"), amount, provider reference
	//   - error: If API call fails (not if payment failed - that's in result.Status)
	VerifyTransaction(ctx context.Context, reference string) (*VerifyResult, error)

	// VerifyWebhookSignature validates that a webhook request actually came from
	// the provider and hasn't been tampered with.
	//
	// HOW WEBHOOKS WORK:
	//   1. Payment completes on provider's side
	//   2. Provider makes HTTP POST to our /webhooks/payment endpoint
	//   3. Request includes: payment data + signature header
	//   4. Signature = HMAC-SHA512(secret_key, request_body)
	//   5. We compute same signature and compare
	//   6. If match → webhook is authentic
	//   7. If different → webhook is fake, ignore it
	//
	// WHY THIS MATTERS:
	// Without signature verification, attacker could:
	//   1. Send fake webhook: {"reference": "DEP-VICTIM123", "status": "success"}
	//   2. Our system credits victim's wallet
	//   3. Attacker steals money from victim
	//   4. We lose the money (provider never received payment)
	//
	// PARAMETERS:
	//   - signature: Value from webhook's "X-Paystack-Signature" header (or equivalent)
	//   - body: Raw request body bytes (before JSON parsing)
	//
	// RETURNS:
	//   - bool: true if signature valid, false if invalid (reject webhook)
	//
	// CRITICAL SECURITY RULE:
	// ALWAYS verify signature BEFORE processing webhook data!
	VerifyWebhookSignature(signature string, body []byte) bool
}

// ═══════════════════════════════════════════════════════════════════════════
// RESULT TYPES
// ═══════════════════════════════════════════════════════════════════════════

// InitResult is returned by InitializeDeposit.
//
// STRUCT EXPLANATION:
// A struct is a composite data type that groups related fields together.
// Think of it like a container that holds multiple pieces of related information.
type InitResult struct {
	// AuthorizationURL is the provider's hosted payment page URL.
	// We redirect the user to this URL where they enter card details.
	//
	// Example: "https://checkout.paystack.com/abc123xyz"
	//
	// WHY A URL NOT A FORM?
	// Security and compliance:
	//   - We NEVER see user's card details (PCI DSS requirement)
	//   - Provider handles 3D Secure, fraud checks, etc.
	//   - Provider is liable if card data is breached, not us
	AuthorizationURL string

	// AccessCode is an optional token some providers return.
	// Paystack uses this, Flutterwave doesn't.
	AccessCode string

	// Reference is our internal reference echoed back.
	// Useful for validation (ensure provider returns what we sent).
	Reference string
}

// VerifyResult is returned by VerifyTransaction.
type VerifyResult struct {
	// Status is the payment status: "success", "failed", "pending", "abandoned"
	//
	// PAYMENT LIFECYCLE:
	//   pending → user hasn't completed payment yet
	//   abandoned → user closed payment page
	//   failed → payment declined (insufficient funds, card blocked, etc.)
	//   success → payment completed successfully
	//
	// WE ONLY CREDIT WALLET ON "success"!
	Status string

	// AmountKobo is the actual amount charged in kobo.
	// CRITICAL: Always verify this matches what we requested!
	//
	// WHY VERIFY AMOUNT?
	// Attacker could:
	//   1. Initialize payment for ₦100
	//   2. Modify webhook to say ₦10,000
	//   3. Without verification, we'd credit ₦10,000 for ₦100 payment
	//
	// ALWAYS: if result.AmountKobo != payment.AmountKobo { return error }
	AmountKobo int64

	// Reference is provider's unique reference for this transaction.
	// Different from our reference:
	//   - Our reference: DEP-ABC123 (we generated)
	//   - Provider reference: 1234567890 (provider generated)
	//
	// WHY STORE BOTH?
	//   - Our reference: for idempotency and user-facing display
	//   - Provider reference: for disputes, refunds, provider support tickets
	Reference string

	// Email is the customer's email associated with the payment
	Email string

	// Currency is the currency code (should always be "NGN" for PropVest).
	Currency string
}

// ═══════════════════════════════════════════════════════════════════════════
// SUMMARY FOR JUNIOR DEVELOPERS
// ═══════════════════════════════════════════════════════════════════════════
//
// WHAT IS AN INTERFACE?
// An interface in Go defines a contract - a set of methods that a type must
// implement. It's like a job description: "any type that can do these 4 things
// is a Provider."
//
// WHY USE INTERFACES?
// 1. Flexibility: Swap implementations without changing code
// 2. Testability: Use mocks in tests instead of real API calls
// 3. Decoupling: Business logic doesn't depend on specific provider
// 4. Extensibility: Add new providers without modifying existing code
//
// PAYMENT FLOW OVERVIEW:
// 1. User clicks "Deposit ₦5000"
// 2. Backend calls InitializeDeposit() → get authorization_url
// 3. Frontend redirects user to authorization_url
// 4. User enters card details on provider's secure page
// 5. Provider processes payment
// 6. Provider sends webhook to our /webhooks/payment endpoint
// 7. Backend verifies webhook signature (security)
// 8. Backend calls VerifyTransaction() to confirm (never trust webhook alone)
// 9. If verified, credit user's wallet atomically with transaction record
//
// SECURITY PRINCIPLES:
// 1. NEVER trust webhooks without signature verification
// 2. ALWAYS verify payment with provider API (VerifyTransaction)
// 3. NEVER see or store card details (PCI DSS violation)
// 4. ALWAYS use HTTPS for API calls (man-in-the-middle protection)
// 5. ALWAYS verify amount matches what we requested (prevent fraud)
//
// NEXT FILES:
// - mock.go: Mock provider for development/testing
// - paystack.go: Real Paystack integration (production)
// - flutterwave.go: Alternative provider (optional)
