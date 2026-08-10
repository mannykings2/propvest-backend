package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/dto"
	"github.com/mannykings2/propvest-backend/internal/response"
	"github.com/mannykings2/propvest-backend/internal/services"
)

// ═══════════════════════════════════════════════════════════════════════════
// WALLET HANDLER - HTTP LAYER
// ═══════════════════════════════════════════════════════════════════════════
//
// PURPOSE OF THIS FILE:
// This is the HTTP layer for wallet operations. It sits between the HTTP client
// (mobile app, web browser) and the business logic (WalletService).
//
// RESPONSIBILITIES:
//   1. Parse HTTP requests (JSON body, query parameters, headers)
//   2. Validate request data (Gin's binding validation)
//   3. Extract user identity from JWT (set by auth middleware)
//   4. Call appropriate service methods
//   5. Transform service responses into HTTP responses
//   6. Map errors to HTTP status codes
//
// WHAT THIS LAYER DOES NOT DO:
//   - NO business logic (that's in service layer)
//   - NO database queries (that's in repository layer)
//   - NO external API calls (that's in service layer)
//   - NO complex calculations
//
// This is the "thin layer" pattern - handlers should be short and simple.
// All complexity lives in service/repository layers where it can be tested
// without HTTP.

// WalletHandler handles wallet-related HTTP requests.
//
// STRUCT EXPLANATION:
// This struct holds dependencies needed by handler methods.
// We use dependency injection (pass dependencies in constructor) rather than
// global variables for:
//   1. Testability: Can inject mock service in tests
//   2. Clarity: Dependencies are explicit
//   3. Flexibility: Can use different service implementations
//   4. Safety: No global mutable state
type WalletHandler struct {
	// walletService is the business logic layer for wallet operations.
	// Injected via constructor (NewWalletHandler).
	walletService services.WalletService
}

// NewWalletHandler creates a new wallet handler instance.
//
// CONSTRUCTOR PATTERN:
// This is Go's way of creating objects with dependencies. Instead of:
//   handler := &WalletHandler{} // Fields are nil, will crash!
//
// We use:
//   handler := NewWalletHandler(service) // Properly initialized
//
// WHY RETURN POINTER (*WalletHandler)?
//   1. Methods use pointer receivers (consistency)
//   2. More efficient than copying struct
//   3. Standard Go convention for handlers
//
// CALLED FROM:
// cmd/api/main.go during application startup:
//   walletService := services.NewWalletService(...)
//   walletHandler := handlers.NewWalletHandler(walletService)
//   v1.SetupRoutes(router, authHandler, userHandler, walletHandler, cfg)
func NewWalletHandler(walletService services.WalletService) *WalletHandler {
	return &WalletHandler{
		walletService: walletService,
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// HANDLER METHOD 1: GetWallet
// ═══════════════════════════════════════════════════════════════════════════

// GetWallet handles GET /api/v1/wallet
//
// PURPOSE:
// Retrieves the authenticated user's wallet information including current
// balances (main + earnings) and formatted display values.
//
// AUTHENTICATION:
// This endpoint requires authentication (auth middleware must be applied).
// User's ID is extracted from JWT token and stored in Gin context by middleware.
//
// REQUEST:
//   GET /api/v1/wallet
//   Headers:
//     Authorization: Bearer <access_token>
//
// SUCCESS RESPONSE (200 OK):
//   {
//     "success": true,
//     "message": "Request successful",
//     "data": {
//       "id": "123e4567-e89b-12d3-a456-426614174000",
//       "main_balance": 150000,
//       "main_balance_formatted": "₦1,500.00",
//       "earnings_balance": 50000,
//       "earnings_balance_formatted": "₦500.00",
//       "currency": "NGN",
//       "virtual_acct_no": null,
//       "virtual_bank": null
//     }
//   }
//
// ERROR RESPONSES:
//   - 401 Unauthorized: Missing or invalid access token
//   - 404 Not Found: Wallet doesn't exist (shouldn't happen - created on registration)
//   - 500 Internal Server Error: Database error
//
// FLOW:
//   1. Extract user_id from Gin context (set by auth middleware)
//   2. Call walletService.GetWallet(userID)
//   3. Service queries database for wallet
//   4. Return wallet data or error
//
// Example curl:
//   curl -X GET http://localhost:8080/api/v1/wallet \
//     -H "Authorization: Bearer eyJhbGc..."
func (h *WalletHandler) GetWallet(c *gin.Context) {
	// Step 1: Extract authenticated user's ID from context
	//
	// HOW getUserIDFromContext WORKS:
	//   1. Auth middleware runs before this handler
	//   2. Middleware validates JWT token
	//   3. Middleware extracts user_id claim from token
	//   4. Middleware stores user_id in Gin context: c.Set("user_id", "...")
	//   5. We retrieve it here: c.Get("user_id")
	//
	// WHY THIS IS SECURE:
	//   - User cannot fake this value (it comes from signed JWT)
	//   - JWT is cryptographically verified by middleware
	//   - If JWT is invalid/expired, middleware rejects request before this runs
	userID, err := getUserIDFromContext(c)
	if err != nil {
		// Failed to extract user ID (shouldn't happen if middleware is working)
		// getUserIDFromContext returns apperrors.ErrUnauthorized on failure
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Step 2: Call service layer to get wallet
	//
	// CONTEXT PASSING:
	// We pass c.Request.Context() which carries:
	//   - Request ID (for tracing logs)
	//   - Cancellation signal (if client disconnects)
	//   - Deadline (if timeout is configured)
	//
	// Service can check ctx.Done() to stop work if client is gone.
	wallet, err := h.walletService.GetWallet(c.Request.Context(), userID)
	if err != nil {
		// Service returned an error (wallet not found, database error, etc.)
		// handleError maps service error to appropriate HTTP status code
		h.handleError(c, err)
		return
	}

	// Step 3: Return success response
	//
	// response.Success() is a helper that formats the standard response:
	//   {"success": true, "message": "Request successful", "data": {...}}
	//
	// Status code defaults to 200 OK for GET requests.
	response.Success(c, wallet)
}

// ═══════════════════════════════════════════════════════════════════════════
// HANDLER METHOD 2: InitiateDeposit
// ═══════════════════════════════════════════════════════════════════════════

// InitiateDeposit handles POST /api/v1/wallet/deposit
//
// PURPOSE:
// Starts the deposit flow by creating a payment with the provider (Paystack/
// Flutterwave) and returning an authorization_url where the user can complete
// the payment.
//
// AUTHENTICATION:
// Requires authentication (user must be logged in).
//
// REQUEST:
//   POST /api/v1/wallet/deposit
//   Headers:
//     Authorization: Bearer <access_token>
//     Content-Type: application/json
//   Body:
//     {
//       "amount_kobo": 150000,
//       "idempotency_key": "optional-unique-key"
//     }
//
// SUCCESS RESPONSE (200 OK):
//   {
//     "success": true,
//     "message": "Request successful",
//     "data": {
//       "authorization_url": "https://checkout.paystack.com/abc123",
//       "access_code": "abc123",
//       "reference": "DEP-123e4567",
//       "amount_kobo": 150000,
//       "amount_formatted": "₦1,500.00"
//     }
//   }
//
// ERROR RESPONSES:
//   - 400 Bad Request: Invalid amount (negative, zero, too large)
//   - 401 Unauthorized: Missing or invalid access token
//   - 422 Unprocessable Entity: Amount below minimum (₦100 = 10000 kobo)
//   - 500 Internal Server Error: Payment provider error, database error
//   - 503 Service Unavailable: Payment provider is down
//
// FLOW:
//   1. Extract user_id from context
//   2. Parse and validate request body
//   3. Call walletService.InitiateDeposit()
//   4. Service creates Payment record in database
//   5. Service calls provider.InitializeDeposit()
//   6. Provider returns authorization_url
//   7. Return authorization_url to client
//   8. Client redirects user to authorization_url
//   9. User completes payment on provider's page
//   10. Provider sends webhook to /webhooks/payment
//   11. Webhook handler credits wallet
//
// IDEMPOTENCY:
// Optional idempotency_key prevents duplicate deposits if client retries.
// If same key is used twice, second call returns existing payment session.
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/wallet/deposit \
//     -H "Authorization: Bearer eyJhbGc..." \
//     -H "Content-Type: application/json" \
//     -d '{"amount_kobo": 150000}'
func (h *WalletHandler) InitiateDeposit(c *gin.Context) {
	// Step 1: Extract user ID
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Step 2: Parse and validate request body
	//
	// DTO (Data Transfer Object) pattern:
	// dto.DepositRequest defines:
	//   - Which fields are required (binding:"required")
	//   - Validation rules (min, max, gt=0)
	//   - JSON field names (json:"amount_kobo")
	//
	// ShouldBindJSON does two things:
	//   1. Unmarshals JSON into struct
	//   2. Runs validation based on binding tags
	//
	// If validation fails, err contains detailed validation errors:
	//   "amount_kobo is required"
	//   "amount_kobo must be greater than 0"
	var req dto.DepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// response.ValidationError formats validator errors nicely:
		//   {"success": false, "error": "amount_kobo is required"}
		response.ValidationError(c, err)
		return
	}

	// Step 3: Call service to initiate deposit
	//
	// SERVICE LAYER RESPONSIBILITIES:
	//   1. Validate business rules (amount >= minimum)
	//   2. Get user's email from database
	//   3. Generate unique reference (DEP-{uuid})
	//   4. Create Payment record with status "pending"
	//   5. Call provider.InitializeDeposit()
	//   6. Return authorization_url
	result, err := h.walletService.InitiateDeposit(
		c.Request.Context(),
		userID,
		req.Amount,
		c.GetHeader("Idempotency-Key"), // Optional, can be empty string
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Step 4: Return success response with authorization URL
	// Client should redirect user to authorization_url
	response.Success(c, result)
}

// ═══════════════════════════════════════════════════════════════════════════
// HANDLER METHOD 3: RequestWithdrawal
// ═══════════════════════════════════════════════════════════════════════════

// RequestWithdrawal handles POST /api/v1/wallet/withdraw
//
// PURPOSE:
// Debits the user's wallet and creates a pending withdrawal transaction.
// The actual payout to the user's bank account is processed asynchronously
// by the worker process.
//
// AUTHENTICATION:
// Requires authentication.
//
// REQUEST:
//   POST /api/v1/wallet/withdraw
//   Headers:
//     Authorization: Bearer <access_token>
//     Content-Type: application/json
//   Body:
//     {
//       "amount_kobo": 50000,
//       "account_number": "0123456789",
//       "account_name": "John Doe",
//       "bank_code": "058",
//       "bank_name": "GTBank"
//     }
//
// SUCCESS RESPONSE (200 OK):
//   {
//     "success": true,
//     "message": "Withdrawal initiated successfully",
//     "data": {
//       "id": "123e4567-e89b-12d3-a456-426614174000",
//       "reference": "WDR-123e4567",
//       "type": "withdrawal",
//       "amount": 50000,
//       "amount_formatted": "₦500.00",
//       "status": "pending",
//       "balance_before": 150000,
//       "balance_after": 100000,
//       "created_at": "2026-08-05T12:00:00Z"
//     }
//   }
//
// ERROR RESPONSES:
//   - 400 Bad Request: Invalid request body, invalid bank account format
//   - 401 Unauthorized: Missing or invalid access token
//   - 422 Unprocessable Entity: Insufficient balance, amount below minimum
//   - 500 Internal Server Error: Database error
//
// WITHDRAWAL FLOW:
//   1. User submits withdrawal request
//   2. Handler validates request and calls service
//   3. Service locks wallet row (SELECT FOR UPDATE)
//   4. Service checks balance >= amount
//   5. Service debits wallet atomically
//   6. Service creates WalletTransaction with status "pending"
//   7. Service queues payout job (if queue client exists)
//   8. Handler returns transaction record
//   9. Worker picks up job and processes payout with provider
//   10. Worker updates transaction status ("completed" or "failed")
//
// FINANCIAL SAFETY:
//   - Balance is debited immediately (no overdraft possible)
//   - Debit and ledger creation happen in one database transaction
//   - If payout fails later, money is still gone from wallet (refund is manual)
//   - This prevents "double spending" (user can't withdraw same money twice)
//
// Example curl:
//   curl -X POST http://localhost:8080/api/v1/wallet/withdraw \
//     -H "Authorization: Bearer eyJhbGc..." \
//     -H "Content-Type: application/json" \
//     -d '{"amount_kobo": 50000, "account_number": "0123456789", "account_name": "John Doe", "bank_code": "058", "bank_name": "GTBank"}'
func (h *WalletHandler) RequestWithdrawal(c *gin.Context) {
	// Step 1: Extract user ID
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Step 2: Parse and validate request body
	//
	// dto.WithdrawRequest validates:
	//   - amount_kobo > 0 (binding:"required,gt=0")
	//   - account_number format (binding:"required,len=10" for Nigerian banks)
	//   - account_name not empty
	//   - bank_code not empty (3-digit code)
	var req dto.WithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err)
		return
	}

	// Step 3: Call service to process withdrawal
	//
	// SERVICE RESPONSIBILITIES:
	//   1. Get wallet with row lock (SELECT FOR UPDATE)
	//   2. Validate business rules:
	//      - amount >= minimum withdrawal (e.g., ₦500)
	//      - balance >= amount (insufficient funds check)
	//   3. Debit wallet balance
	//   4. Create WalletTransaction ledger entry
	//   5. Queue payout job (if message queue exists)
	//   6. Return transaction record
	//
	// ATOMICITY:
	// All database operations happen in one transaction:
	//   BEGIN
	//     UPDATE wallets SET main_balance = main_balance - 50000 WHERE id = ...
	//     INSERT INTO wallet_transactions (...)
	//   COMMIT
	//
	// If any step fails, everything rolls back (balance not debited).
	transaction, err := h.walletService.InitiateWithdrawal(c.Request.Context(), userID, req)
	if err != nil {
		// Common errors:
		//   - apperrors.ErrInsufficientBalance (422)
		//   - apperrors.ErrAmountTooSmall (422)
		//   - Database errors (500)
		h.handleError(c, err)
		return
	}

	// Step 4: Return success response
	//
	// Response includes:
	//   - Transaction ID (for tracking)
	//   - Reference (DEP-xxx format, user-visible)
	//   - Status ("pending" - payout not processed yet)
	//   - Balance before/after (for user confirmation)
	//
	// Client should show:
	//   "Withdrawal of ₦500.00 initiated. Processing may take 1-2 business days."
	response.SuccessWithMessage(c, http.StatusOK, "Withdrawal initiated successfully", transaction)
}

// ═══════════════════════════════════════════════════════════════════════════
// HANDLER METHOD 4: GetTransactions
// ═══════════════════════════════════════════════════════════════════════════

// GetTransactions handles GET /api/v1/wallet/transactions
//
// PURPOSE:
// Returns the user's transaction history (ledger) with filtering and pagination.
//
// AUTHENTICATION:
// Requires authentication.
//
// REQUEST:
//   GET /api/v1/wallet/transactions?type=deposit&status=completed&page=1&limit=20
//   Headers:
//     Authorization: Bearer <access_token>
//   Query Parameters:
//     - type: Filter by transaction type (deposit, withdrawal, credit, debit)
//     - status: Filter by status (pending, completed, failed)
//     - page: Page number (default: 1)
//     - limit: Items per page (default: 20, max: 100)
//
// SUCCESS RESPONSE (200 OK):
//   {
//     "success": true,
//     "message": "Request successful",
//     "data": {
//       "transactions": [
//         {
//           "id": "123e4567-e89b-12d3-a456-426614174000",
//           "reference": "DEP-123e4567",
//           "type": "deposit",
//           "amount": 150000,
//           "amount_formatted": "₦1,500.00",
//           "status": "completed",
//           "balance_before": 0,
//           "balance_after": 150000,
//           "description": "Deposit via Paystack",
//           "created_at": "2026-08-05T12:00:00Z"
//         }
//       ],
//       "total": 42,
//       "page": 1,
//       "limit": 20,
//       "pages": 3
//     }
//   }
//
// ERROR RESPONSES:
//   - 400 Bad Request: Invalid query parameters (negative page, invalid type)
//   - 401 Unauthorized: Missing or invalid access token
//   - 500 Internal Server Error: Database error
//
// PAGINATION:
// Uses offset-based pagination:
//   - page=1, limit=20 → OFFSET 0 LIMIT 20 (rows 1-20)
//   - page=2, limit=20 → OFFSET 20 LIMIT 20 (rows 21-40)
//   - page=3, limit=20 → OFFSET 40 LIMIT 20 (rows 41-60)
//
// PERFORMANCE NOTES:
//   - Query includes index on (wallet_id, created_at DESC)
//   - Filtering by type/status also uses indexes
//   - Large offsets (page 1000) are slow - consider cursor pagination for scale
//
// Example curl:
//   curl -X GET "http://localhost:8080/api/v1/wallet/transactions?type=deposit&page=1&limit=10" \
//     -H "Authorization: Bearer eyJhbGc..."
func (h *WalletHandler) GetTransactions(c *gin.Context) {
	// Step 1: Extract user ID
	userID, err := getUserIDFromContext(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Step 2: Parse query parameters
	//
	// QUERY PARAMETERS IN GIN:
	// c.Query("key") returns string value or "" if not present
	// c.DefaultQuery("key", "default") returns value or default
	//
	// We need to convert strings to appropriate types:
	//   "1" → 1 (int)
	//   "deposit" → "deposit" (string, no conversion)

	// Transaction type filter (optional)
	// Valid values: "", "deposit", "withdrawal", "credit", "debit"
	// Empty string means "all types"
	txType := c.Query("type")

	// Status filter (optional)
	// Valid values: "", "pending", "completed", "failed"
	// Empty string means "all statuses"
	status := c.Query("status")

	// Page number (default: 1)
	// strconv.Atoi converts string to int:
	//   "1" → 1, nil
	//   "abc" → 0, error
	//   "" → 0, error
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		// Invalid page number (negative, non-numeric)
		page = 1 // Fallback to first page
	}

	// Items per page (default: 20, max: 100)
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		// Prevent excessive database load
		// Client requesting limit=10000 would load 10000 rows
		limit = 100
	}

	// Step 3: Call service to get transactions
	//
	// SERVICE RESPONSIBILITIES:
	//   1. Build query with filters (WHERE type = ? AND status = ?)
	//   2. Calculate offset (page=2, limit=20 → offset=20)
	//   3. Execute query with pagination (LIMIT 20 OFFSET 20)
	//   4. Count total matching rows (for pagination metadata)
	//   5. Transform models to DTOs
	//   6. Return transactions + total count
	transactions, total, err := h.walletService.GetTransactionHistory(
		c.Request.Context(),
		userID,
		txType,
		status,
		page,
		limit,
	)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Step 4: Build response with pagination metadata
	//
	// PAGINATION METADATA:
	// Client needs this to build pagination UI:
	//   - total: 42 → "Showing 1-20 of 42 results"
	//   - page: 2 → Highlight page 2 button
	//   - pages: 3 → Show page buttons 1, 2, 3
	//   - limit: 20 → "Show 20 per page"
	//
	// CALCULATING TOTAL PAGES:
	// Ceiling division: total / limit rounded up
	//   - 42 / 20 = 2.1 → 3 pages
	//   - 40 / 20 = 2.0 → 2 pages
	//   - 1 / 20 = 0.05 → 1 page
	//
	// Formula: (total + limit - 1) / limit
	//   - (42 + 20 - 1) / 20 = 61 / 20 = 3
	//   - (40 + 20 - 1) / 20 = 59 / 20 = 2
	//   - (1 + 20 - 1) / 20 = 20 / 20 = 1
	pages := (total + int64(limit) - 1) / int64(limit)

	// Build response map
	result := map[string]interface{}{
		"transactions": transactions,
		"total":        total,
		"page":         page,
		"limit":        limit,
		"pages":        pages,
	}

	response.Success(c, result)
}

// ═══════════════════════════════════════════════════════════════════════════
// HANDLER METHOD 5: HandleWebhook
// ═══════════════════════════════════════════════════════════════════════════

// HandleWebhook handles POST /api/v1/webhooks/payment
//
// PURPOSE:
// This is the endpoint that payment providers (Paystack, Flutterwave) call to
// notify us when a payment is completed. This is how we know to credit the
// user's wallet.
//
// CRITICAL SECURITY NOTES:
// 1. This endpoint is PUBLIC (no authentication middleware)
// 2. Anyone on the internet can send POST requests to it
// 3. We MUST verify the webhook signature to prevent fraud
// 4. We MUST verify the payment with the provider's API
// 5. NEVER trust webhook data without verification
//
// AUTHENTICATION:
// NO authentication required (external system calls this).
// Security comes from signature verification instead.
//
// REQUEST:
//   POST /api/v1/webhooks/payment
//   Headers:
//     X-Paystack-Signature: abc123def456...  (or X-Flutterwave-Signature)
//     Content-Type: application/json
//   Body:
//     {
//       "event": "charge.success",
//       "data": {
//         "reference": "DEP-123e4567-e89b-12d3-a456-426614174000",
//         "amount": 150000,
//         "currency": "NGN",
//         "status": "success",
//         "customer": {...},
//         ...
//       }
//     }
//
// SUCCESS RESPONSE (200 OK):
//   {
//     "success": true,
//     "message": "Webhook processed successfully"
//   }
//
// ERROR RESPONSES:
//   - 400 Bad Request: Invalid signature, malformed body
//   - 500 Internal Server Error: Database error, verification failed
//
// WHY WEBHOOKS?
// After user completes payment on provider's page:
//   1. Provider redirects user back to our app (callback URL)
//   2. Provider ALSO sends webhook to our server (this endpoint)
//
// Callback alone is not enough because:
//   - User might close browser before redirect completes
//   - User might lose internet connection
//   - Attacker could fake the callback URL parameters
//
// Webhooks are more reliable:
//   - Server-to-server (no user browser involved)
//   - Provider retries if our server is down
//   - Cryptographically signed (can't be faked)
//
// IDEMPOTENCY:
// This endpoint is idempotent - safe to call multiple times with same reference.
// Provider might send the same webhook multiple times:
//   - Network retry (we didn't respond fast enough)
//   - Provider's internal retry logic
//   - Duplicate events in provider's system
//
// Our service checks if transaction already exists before crediting wallet.
//
// FLOW:
//   1. Read raw request body (needed for signature verification)
//   2. Extract signature from header
//   3. Verify signature matches body (prevents forgery)
//   4. Parse body as JSON
//   5. Extract payment reference from webhook data
//   6. Call service to verify and credit wallet
//   7. Service calls provider API to verify payment
//   8. Service credits wallet if payment is valid (idempotent)
//   9. Return 200 OK (so provider stops retrying)
//
// PROVIDER RETRY BEHAVIOR:
// If we return non-200 status, provider will retry:
//   - Paystack: Retries for 3 days (exponential backoff)
//   - Flutterwave: Retries for 24 hours
//
// So we should return 200 OK even for duplicate webhooks (idempotent success).
//
// Example curl (simulating provider):
//   curl -X POST http://localhost:8080/api/v1/webhooks/payment \
//     -H "X-Paystack-Signature: abc123..." \
//     -H "Content-Type: application/json" \
//     -d '{"event":"charge.success","data":{"reference":"DEP-123","amount":150000}}'
func (h *WalletHandler) HandleWebhook(c *gin.Context) {
	// Step 1: Read raw request body
	//
	// WHY RAW BODY?
	// Signature is computed over the exact bytes the provider sent.
	// If we parse JSON first, then re-serialize, byte order might change
	// and signature verification would fail.
	//
	// EXAMPLE PROBLEM:
	//   Provider sends: {"amount":1000,"status":"success"}
	//   Provider signature: HMAC(secret, '{"amount":1000,"status":"success"}')
	//
	//   We parse: map{"amount": 1000, "status": "success"}
	//   We re-serialize: {"status":"success","amount":1000} (different order!)
	//   We compute: HMAC(secret, '{"status":"success","amount":1000}')
	//   Signatures don't match! ❌
	//
	// SOLUTION: Keep original raw bytes for signature verification.
	//
	// io.ReadAll reads entire request body into memory.
	// This is safe for webhooks (small payloads, few KB).
	// For large uploads, use streaming instead.
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		// Failed to read body (network error, client disconnected)
		response.Error(c, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// Step 2: Extract signature from header
	//
	// Different providers use different header names:
	//   - Paystack: X-Paystack-Signature
	//   - Flutterwave: verif-hash
	//   - Stripe: Stripe-Signature
	//
	// For now, we support Paystack. When adding more providers,
	// check multiple headers or add provider detection.
	signature := c.GetHeader("X-Paystack-Signature")
	if signature == "" {
		// No signature header (attacker, misconfigured provider, or testing)
		// In production, reject this immediately
		// In development with mock provider, might need different header
		
		// Try mock provider header for development
		signature = c.GetHeader("X-Mock-Signature")
		if signature == "" {
			response.Error(c, http.StatusBadRequest, "Missing signature header")
			return
		}
	}

	// Step 3: Verify signature
	//
	// CRITICAL SECURITY CHECK:
	// This proves the webhook came from the payment provider, not an attacker.
	//
	// HOW SIGNATURE VERIFICATION WORKS:
	//   1. Provider computes: signature = HMAC-SHA512(secret_key, body)
	//   2. Provider sends signature in header
	//   3. We compute: expected = HMAC-SHA512(our_secret_key, body)
	//   4. We compare: signature == expected
	//   5. If match → authentic, if different → forged
	//
	// ATTACK PREVENTED:
	//   Attacker doesn't know secret_key, so can't create valid signature.
	//   Without signature verification, attacker could send:
	//     {"reference": "VICTIM-123", "amount": 999999999}
	//   And we'd credit victim's wallet ₦9,999,999.99!
	if !h.walletService.VerifyWebhookSignature(signature, body) {
		// Invalid signature (forged webhook or wrong secret key)
		// LOG THIS! It might indicate an attack.
		//
		// In production, also:
		//   - Alert security team
		//   - Rate limit this IP
		//   - Log attacker's IP for investigation
		response.Error(c, http.StatusBadRequest, "Invalid signature")
		return
	}

	// Step 4: Parse webhook payload
	//
	// Now that signature is verified, we can trust the data.
	//
	// WEBHOOK PAYLOAD STRUCTURE (Paystack):
	//   {
	//     "event": "charge.success",
	//     "data": {
	//       "reference": "DEP-123",
	//       "amount": 150000,
	//       "status": "success",
	//       ...
	//     }
	//   }
	//
	// We need to extract the reference to look up the payment.
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		// Malformed JSON (shouldn't happen - provider sends valid JSON)
		// But signature was valid, so this is weird...
		// Maybe network corruption? Log this.
		response.Error(c, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Step 5: Extract reference from payload
	//
	// Navigate nested JSON structure: payload["data"]["reference"]
	//
	// TYPE ASSERTIONS IN GO:
	// JSON unmarshals to map[string]interface{}
	// interface{} is "any type" - we need to assert specific types:
	//   value.(string) - "I assert this is a string"
	//   value.(map[string]interface{}) - "I assert this is a map"
	//
	// If assertion is wrong, program panics (crashes).
	// Use safe form with ok check:
	//   str, ok := value.(string)
	//   if !ok { handle error }
	
	// Get "data" field (should be an object/map)
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		response.Error(c, http.StatusBadRequest, "Invalid webhook payload structure")
		return
	}

	// Get "reference" field from data (should be a string)
	reference, ok := data["reference"].(string)
	if !ok || reference == "" {
		response.Error(c, http.StatusBadRequest, "Missing payment reference in webhook")
		return
	}

	// Step 6: Process webhook - verify payment and credit wallet
	//
	// SERVICE RESPONSIBILITIES:
	//   1. Look up Payment record by reference
	//   2. If already processed (status != pending), return early (idempotency)
	//   3. Call provider.VerifyTransaction(reference) - server-side verification
	//   4. If provider says "success" and amount matches:
	//      a. Start database transaction
	//      b. Lock wallet row (SELECT FOR UPDATE)
	//      c. Check if WalletTransaction with this reference exists
	//      d. If exists, return (duplicate webhook)
	//      e. If not exists, credit wallet and create ledger entry
	//      f. Update Payment status to "success"
	//      g. Commit transaction
	//   5. If verification fails or amount mismatch, mark Payment as "failed"
	//
	// IDEMPOTENCY GUARANTEE:
	// Even if provider sends same webhook 100 times, wallet is credited once.
	// This is safe because:
	//   - WalletTransaction has unique constraint on reference
	//   - Service checks TransactionExists() before creating
	//   - All checks happen inside locked transaction
	err = h.walletService.CreditFromPaymentReference(c.Request.Context(), reference, body)
	if err != nil {
		// Error processing webhook
		// Could be:
		//   - Payment not found (reference doesn't exist)
		//   - Verification failed (provider returned "failed")
		//   - Database error
		//   - Amount mismatch (fraud attempt)
		//
		// DON'T return 500! Return 200 so provider stops retrying.
		// Log the error for investigation but acknowledge webhook.
		// Otherwise provider will spam us with retries.
		//
		// Exception: If it's a temporary error (database down), return 500
		// so provider retries later when database is back.
		h.handleError(c, err)
		return
	}

	// Step 7: Return success
	//
	// Return 200 OK so provider knows we processed it successfully.
	// Provider will stop retrying this webhook.
	//
	// Even for duplicate webhooks (already processed), return 200.
	// Idempotency means we can safely say "yes, we handled this."
	response.SuccessWithMessage(c, http.StatusOK, "Webhook processed successfully")
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

// handleError maps service-layer errors to HTTP responses.
//
// PURPOSE:
// Centralized error handling for all wallet endpoints.
// Transforms internal service errors into appropriate HTTP responses.
//
// HOW IT WORKS:
//   1. Service layer returns domain errors (apperrors.ErrInsufficientBalance, etc.)
//   2. This function maps errors to HTTP status codes
//   3. response.Error() formats the final JSON response
//
// ERROR MAPPING:
// The response.Error() helper (from internal/response package) automatically:
//   1. Maps error to HTTP status code (via errors.HTTPStatusFromError)
//      - apperrors.ErrInsufficientBalance → 422 Unprocessable Entity
//      - apperrors.ErrNotFound → 404 Not Found
//      - apperrors.ErrUnauthorized → 401 Unauthorized
//      - Unknown errors → 500 Internal Server Error
//
//   2. Sanitizes error message for client (via errors.ClientMessage)
//      - Internal error: "database connection failed" → "Internal server error"
//      - User error: "insufficient balance" → "Insufficient balance"
//
//   3. Generates error code for programmatic handling (via errors.ErrorCode)
//      - apperrors.ErrInsufficientBalance → "insufficient_balance"
//      - Client can check: if (response.code === "insufficient_balance") { ... }
//
//   4. Includes request_id for tracing (from middleware)
//      - Logs and responses both have same request_id
//      - Makes debugging easy: grep logs for request_id to see full flow
//
// WHY CENTRALIZE ERROR HANDLING?
//   1. Consistency: Same error always gets same status code
//   2. DRY: Don't repeat error mapping logic in every handler
//   3. Security: Automatic sanitization of internal errors
//   4. Maintainability: Change mapping in one place
//
// EXAMPLE FLOW:
//   Service returns: apperrors.ErrInsufficientBalance
//   HTTPStatusFromError(): 422
//   ClientMessage(): "Insufficient balance"
//   ErrorCode(): "insufficient_balance"
//   Response:
//     {
//       "success": false,
//       "error": "Insufficient balance",
//       "code": "insufficient_balance",
//       "request_id": "abc123"
//     }
//
// PARAMETERS:
//   - c: Gin context (for HTTP response)
//   - err: Error from service layer
//
// SIDE EFFECTS:
//   - Writes HTTP response to client
//   - Sets appropriate status code
//   - Logs error (via response.Error)
func (h *WalletHandler) handleError(c *gin.Context, err error) {
	// Delegate to response package's centralized error handler
	// This ensures consistent error responses across all handlers
	response.Error(c, err)
}

// ═══════════════════════════════════════════════════════════════════════════
// SUMMARY FOR JUNIOR DEVELOPERS
// ═══════════════════════════════════════════════════════════════════════════
//
// WHAT IS A HANDLER?
// A handler is a function that processes HTTP requests. It's the "controller"
// in MVC architecture, sitting between the HTTP layer and business logic.
//
// HANDLER RESPONSIBILITIES:
//   1. Parse HTTP request (body, query params, headers)
//   2. Validate input (format, required fields)
//   3. Extract authentication (user_id from JWT)
//   4. Call service layer (business logic)
//   5. Format response (JSON structure)
//   6. Set HTTP status code
//   7. Handle errors
//
// HANDLERS SHOULD NOT:
//   - Contain business logic (that's in service layer)
//   - Query database directly (that's in repository layer)
//   - Compute complex values (that's in service layer)
//   - Make external API calls (that's in service layer)
//
// WALLET HANDLER METHODS:
//   1. GetWallet - Retrieve user's wallet balance
//   2. InitiateDeposit - Start deposit flow with payment provider
//   3. RequestWithdrawal - Debit wallet and queue payout
//   4. GetTransactions - Retrieve transaction history with pagination
//   5. HandleWebhook - Process payment provider notifications (PUBLIC!)
//
// AUTHENTICATION:
// Methods 1-4 require authentication (auth middleware must be applied).
// Method 5 (webhook) is public but uses signature verification instead.
//
// TESTING CHECKLIST:
//   ✓ Authenticated requests work
//   ✓ Unauthenticated requests are rejected (except webhook)
//   ✓ Invalid input is rejected with 400 Bad Request
//   ✓ Insufficient balance returns 422 Unprocessable Entity
//   ✓ Webhooks with invalid signature are rejected
//   ✓ Duplicate webhooks are idempotent (wallet credited once)
//   ✓ Error messages are user-friendly (no internal details)
//
// NEXT STEPS:
//   1. Register routes (internal/routes/v1/routes.go)
//   2. Wire dependencies (cmd/api/main.go)
//   3. Test in Postman
//   4. Celebrate! 🎉
