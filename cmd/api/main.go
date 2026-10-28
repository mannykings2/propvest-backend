package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/audit"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/database"
	"github.com/mannykings2/propvest-backend/internal/handlers"
	"github.com/mannykings2/propvest-backend/internal/logger"
	"github.com/mannykings2/propvest-backend/internal/mailer"
	"github.com/mannykings2/propvest-backend/internal/middleware"
	"github.com/mannykings2/propvest-backend/internal/payments"
	"github.com/mannykings2/propvest-backend/internal/queue"
	"github.com/mannykings2/propvest-backend/internal/realtime"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	v1 "github.com/mannykings2/propvest-backend/internal/routes/v1"
	"github.com/mannykings2/propvest-backend/internal/services"
	"github.com/mannykings2/propvest-backend/internal/utils/cloudinary"
	"github.com/mannykings2/propvest-backend/internal/utils/piienc"
	"github.com/mannykings2/propvest-backend/internal/utils/sms"
)

// main is the COMPOSITION ROOT: the one place that constructs and wires every
// dependency. Layers are initialised bottom-up:
//
//	config → logger → database → infra (queue/realtime/mailer/payments)
//	       → repositories → services → handlers → router → HTTP server
//
// Nothing below main.go knows about anything above it; each component only
// receives the collaborators it needs via its constructor.
func main() {
	// 1. CONFIG — first, because everything depends on it.
	cfg := config.Load()

	// 2. LOGGER — structured JSON logs (docs 6.1/6.2). Init as early as possible.
	logger.Init(cfg.AppEnv)

	// Fail fast on bad configuration (docs 1.1 §10). This also validates the
	// token TTL strings so a typo can't silently become a 0s TTL (FIX-08).
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// Launch gates (Phase 6.4): in production both default ON — unverified or
	// KYC-pending accounts cannot invest or withdraw. An explicit false in the
	// production .env is a decision someone should see in the logs.
	if cfg.AppEnv == "production" {
		if !cfg.RequireEmailVerification {
			logger.Warn("REQUIRE_EMAIL_VERIFICATION=false in production — unverified accounts can move funds")
		}
		if !cfg.RequireKYCForInvestment {
			logger.Warn("REQUIRE_KYC_FOR_INVESTMENT=false in production — accounts without approved KYC can invest")
		}
	}

	logger.Info("starting PropVest API", "env", cfg.AppEnv)

	// 3. DATABASE + MIGRATIONS.
	database.Connect(cfg)
	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// 4. INFRASTRUCTURE ADAPTERS.
	//
	// RabbitMQ: used for asynchronous, retryable work (emails, SMS, withdrawal
	// processing) and for fanning realtime notifications back to the API. If the
	// broker is unavailable the client runs in a disabled/no-op mode so local dev
	// works without RabbitMQ (see internal/queue).
	mq := queue.New(cfg.RabbitMQURL)
	defer mq.Close()

	// WebSocket hub: keeps track of connected clients and pushes realtime events.
	hub := realtime.NewHub()
	go hub.Run()

	// Mailer + payment provider are chosen by config ("mock" in dev).
	// The mailer is QUEUED (Phase 5): request-path sends publish to
	// QueueEmailDispatch and the worker delivers with retry — register used
	// to block ~2.2s on an untimed SMTP dial before answering.
	emailSender := mailer.NewQueuedSender(mq, mailer.New(cfg))
	paymentProvider := payments.NewProvider(cfg)

	auditRecorder := audit.NewLogRecorder()

	// PII cipher for BVN/NIN at rest (Phase 6.1). Production refuses to boot
	// without KYC_ENCRYPTION_KEY (cfg.Validate); development falls back to a
	// fixed key so the local stack keeps working — loudly, so it can't ship.
	kycKey := cfg.KYCEncryptionKey
	if kycKey == "" {
		kycKey = "propvest-development-only-kyc-key"
		logger.Warn("KYC_ENCRYPTION_KEY not set — using the insecure development key (never run production without it)")
	}
	kycCipher, err := piienc.New(kycKey)
	if err != nil {
		logger.Error("failed to initialise PII cipher", "error", err)
		os.Exit(1)
	}

	// 5. REPOSITORIES.
	userRepo := repositories.NewUserRepository(database.DB)
	walletRepo := repositories.NewWalletRepository(database.DB)
	outboxRepo := repositories.NewOutboxRepository(database.DB)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(database.DB)
	otpRepo := repositories.NewOTPVerificationRepository(database.DB)
	tokenRepo := repositories.NewVerificationTokenRepository(database.DB)
	paymentRepo := repositories.NewPaymentRepository(database.DB)

	// Property repositories (Milestone 4)
	propertyRepo := repositories.NewPropertyRepository(database.DB)
	propertyImageRepo := repositories.NewPropertyImageRepository(database.DB)
	propertyDocumentRepo := repositories.NewPropertyDocumentRepository(database.DB)

	// Investment repository (Milestone 5)
	investmentRepo := repositories.NewInvestmentRepository(database.DB)

	// Phase 2 repositories (Integration Phase 2 plan)
	saleVoteRepo := repositories.NewSaleVoteRepository(database.DB)
	kycSubmissionRepo := repositories.NewKYCSubmissionRepository(database.DB, kycCipher)
	offplanRepo := repositories.NewOffplanRepository(database.DB)

	// Notification repositories (Milestone 6)
	notificationRepo := repositories.NewNotificationRepository(database.DB)
	notificationPreferenceRepo := repositories.NewNotificationPreferenceRepository(database.DB)

	// Admin repositories (Milestone 7)
	auditRepo := repositories.NewAuditRepository(database.DB)

	// Future repositories
	// ...

	// 6. UTILITIES / SERVICES.
	cloudinaryService, err := cloudinary.NewCloudinaryService(cfg)
	if err != nil {
		logger.Warn("Cloudinary not configured; avatar/image uploads will fail", "error", err)
	}
	smsService := sms.NewSMSService(cfg)

	// Notification service: writes in-app rows, pushes over WebSocket, and
	// enqueues email/SMS work onto RabbitMQ.
	notificationService := services.NewNotificationService(notificationRepo, hub, mq)

	authService := services.NewAuthService(userRepo, walletRepo, refreshTokenRepo, tokenRepo, emailSender, auditRecorder, cfg, database.DB)
	userService := services.NewUserService(userRepo, otpRepo, refreshTokenRepo, kycSubmissionRepo, smsService, cloudinaryService, cfg)

	// Phase 7 (Money layer): double-entry ledger. Seeded BEFORE any service can
	// post a journal — an unseeded chart would make every posting fail closed.
	ledgerService := services.NewLedgerService(database.DB)
	if err := ledgerService.EnsureChartOfAccounts(context.Background()); err != nil {
		logger.Error("failed to seed chart of accounts", "error", err)
		os.Exit(1)
	}
	if err := services.EnsureFeeRules(context.Background(), database.DB); err != nil {
		logger.Error("failed to seed fee rules", "error", err)
		os.Exit(1)
	}

	// Phase 7.3: one-time backfill — reconstruct journals for wallet history
	// that predates the ledger. Runs only while the ledger is empty, and is
	// idempotent by reference, so a restart never double-books.
	var entryCount, txnCount int64
	database.DB.Raw(`SELECT COUNT(*) FROM ledger_entries`).Scan(&entryCount)
	database.DB.Raw(`SELECT COUNT(*) FROM wallet_transactions`).Scan(&txnCount)
	if entryCount == 0 && txnCount > 0 {
		if posted, err := ledgerService.BackfillFromWalletTransactions(context.Background()); err != nil {
			logger.Error("ledger backfill failed", "error", err)
			os.Exit(1)
		} else {
			logger.Info("ledger backfill complete", "journals_posted", posted)
		}
	}

	logger.Info("ledger ready: chart of accounts + fee rules seeded")

	// Phase 8 (Risk layer): AML rule engine. Seeded rules are verified at boot
	// — an empty risk_rules would silently disable every blocking control.
	riskRepo := repositories.NewRiskRepository(database.DB)
	riskService := services.NewRiskService(riskRepo, userRepo, services.NewAuditService(auditRepo), database.DB)
	if err := riskService.EnsureRules(context.Background()); err != nil {
		logger.Error("risk rules not ready", "error", err)
		os.Exit(1)
	}

	walletService := services.NewWalletService(walletRepo, userRepo, paymentRepo, outboxRepo, paymentProvider, notificationService, mq, cfg, database.DB, ledgerService, riskService)

	// Property service (Milestone 4)
	propertyService := services.NewPropertyService(propertyRepo, propertyImageRepo, propertyDocumentRepo, outboxRepo, cloudinaryService, database.DB)

	// Investment service (Milestone 5) — takes the sale-vote repo + notifier
	// for Phase 2 governance (trigger/cast vote, co-owner fan-out).
	investmentService := services.NewInvestmentService(investmentRepo, propertyRepo, walletRepo, outboxRepo, saleVoteRepo, notificationService, database.DB, ledgerService)

	// Off-plan service (Phase 2, Workstream B)
	offplanService := services.NewOffplanService(offplanRepo, walletRepo, database.DB, ledgerService)

	// Notification services (Milestone 6)
	notificationPreferenceService := services.NewNotificationPreferenceService(notificationPreferenceRepo)
	adminMessagingService := services.NewAdminMessagingService(userRepo, notificationRepo, mq, database.DB)

	// Admin services (Milestone 7)
	auditService := services.NewAuditService(auditRepo)
	dashboardService := services.NewDashboardService(database.DB)
	adminUserService := services.NewAdminUserService(userRepo, refreshTokenRepo, investmentRepo, kycSubmissionRepo, auditService, database.DB)
	// Phase 8.3: every sanctions/PEP attestation is mirrored into the risk
	// layer (screening_results + hit handling) after it commits.
	adminUserService.SetScreeningRiskHook(riskService)
	adminFinancialService := services.NewAdminFinancialService(database.DB)
	adminReportsService := services.NewAdminReportsService(database.DB, auditRepo, auditService)
	adminHealthService := services.NewAdminHealthService(database.DB, cfg)

	// Phase 9 (Regulation layer): consent evidence, NDPA data export, deferred
	// erasure, marketing suppression. The KYC cipher lets the OWNER's own
	// export carry decrypted BVN/NIN — the one place decryption is legal
	// without an admin role.
	consentService := services.NewConsentService(database.DB)
	// Email links (Phase B2): absolute SPA URLs for reset/verify/export/
	// unsubscribe. Falls back to BASE_URL for single-origin deployments.
	frontendURL := cfg.FrontendURL
	if frontendURL == "" {
		frontendURL = cfg.BaseURL
	}
	services.SetFrontendURL(frontendURL)

	privacyService := services.NewPrivacyService(database.DB, notificationService, auditService, cfg.JWTSecret).
		WithKYCCipher(cfg.KYCEncryptionKey)
	services.SetUnsubscribeTokenFn(privacyService.UnsubscribeToken)

	// Future services
	// adminService := services.NewAdminService(userRepo, propertyRepo, investmentRepo, walletRepo, refreshTokenRepo, auditRecorder)

	// 7. HANDLERS.
	authHandler := handlers.NewAuthHandler(authService)
	userHandler := handlers.NewUserHandler(userService)
	walletHandler := handlers.NewWalletHandler(walletService)

	// Property handler (Milestone 4)
	propertyHandler := handlers.NewPropertyHandler(propertyService)

	// Investment handler (Milestone 5)
	investmentHandler := handlers.NewInvestmentHandler(investmentService)

	// Off-plan handler (Phase 2, Workstream B)
	offplanHandler := handlers.NewOffplanHandler(offplanService)

	// Support tickets (Phase 4.2): one service powers both the user-facing
	// conversation and the staff/admin queue (RBAC gated at the route).
	supportRepo := repositories.NewSupportRepository(database.DB)
	supportService := services.NewSupportService(supportRepo, notificationService)
	supportHandler := handlers.NewSupportHandler(supportService)
	adminSupportHandler := handlers.NewAdminSupportHandler(supportService)

	// Notification handlers (Milestone 6)
	notificationHandler := handlers.NewNotificationHandler(notificationService, notificationPreferenceService)
	adminMessagingHandler := handlers.NewAdminMessagingHandler(adminMessagingService)

	// Admin handlers (Milestone 7)
	adminDashboardHandler := handlers.NewAdminDashboardHandler(dashboardService)
	adminUserHandler := handlers.NewAdminUserHandler(adminUserService)
	adminFinancialHandler := handlers.NewAdminFinancialHandler(adminFinancialService)
	adminReportsHandler := handlers.NewAdminReportsHandler(adminReportsService)
	adminHealthHandler := handlers.NewAdminHealthHandler(adminHealthService)

	// Phase 7 (Money layer): admin fee/settlement/reconciliation surface.
	// The settlement import needs the provider's settlement feed when it has
	// one (Paystack/hybrid do; mock doesn't — import then reports 501).
	settlementFetcher, _ := paymentProvider.(payments.SettlementFetcher)
	financeService := services.NewFinanceService(database.DB, paymentRepo, ledgerService, settlementFetcher)
	adminFinanceHandler := handlers.NewAdminFinanceHandler(financeService, ledgerService)

	// Phase 9: user-facing data rights + the admin Privacy tab.
	privacyHandler := handlers.NewPrivacyHandler(consentService, privacyService)
	adminPrivacyHandler := handlers.NewAdminPrivacyHandler(privacyService)

	// Phase 8: admin Risk surface — alert triage, rule toggles, screening,
	// STR/CTR filings.
	adminRiskHandler := handlers.NewAdminRiskHandler(riskService)

	// Future handlers
	// adminHandler := handlers.NewAdminHandler(adminService)
	// webhookHandler := handlers.NewWebhookHandler(walletService, cfg)
	// wsHandler := handlers.NewWebSocketHandler(hub, cfg)

	// 8. ROUTER + MIDDLEWARE (order matters, docs 4.2 §13 / 1.4 §5).
	// Staging mirrors production: no debug route dump, no debug logging.
	if cfg.IsProduction() || cfg.IsStaging() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()

	// Pin which peers may set X-Forwarded-For (Phase 5). Gin defaults to
	// trusting every caller, so a direct client could rotate forged XFF
	// values and mint a fresh rate-limit bucket per request — measured
	// bypass of the 10/min auth limit before this line. Default is
	// loopback (config TRUSTED_PROXIES); behind a reverse proxy list its
	// CIDR so real client IPs still reach rate limits and audit rows.
	// Invalid entries are a config error → fail fast at boot.
	if err := r.SetTrustedProxies(cfg.TrustedProxiesList()); err != nil {
		logger.Error("invalid TRUSTED_PROXIES", "error", err)
		os.Exit(1)
	}

	r.Use(middleware.Recovery())  // catch panics (first)
	r.Use(middleware.RequestID()) // correlation id
	r.Use(middleware.Logger())    // structured request log
	r.Use(middleware.CORS(middleware.ParseAllowedOrigins(cfg.AllowedOrigins)))
	r.Use(middleware.SecurityHeaders(cfg.IsProduction())) // security headers
	r.Use(middleware.RateLimit(middleware.RateLimitConfig{
		AnonymousLimit:     100, // 100 requests per minute for anonymous
		AuthenticatedLimit: 300, // 300 requests per minute for authenticated
		AuthEndpointLimit:  10,  // 10 requests per minute for login/register
	}))

	// Root-level operational endpoints (not versioned): liveness/readiness.
	r.GET("/health", handlers.HealthCheck)            // liveness: process is up (no deps)
	readinessHandler := handlers.NewReadinessHandler( // readiness: deps are up
		handlers.ReadinessProbe{Name: "database", Critical: true, Check: handlers.DatabaseReadiness(database.DB)},
		handlers.ReadinessProbe{Name: "queue", Critical: false, Check: handlers.QueueReadiness(mq)},
	)
	r.GET("/health/ready", readinessHandler)

	apiV1 := r.Group("/api/v1")
	v1.RegisterRoutes(apiV1, authHandler, userHandler, walletHandler, propertyHandler, investmentHandler, notificationHandler, adminMessagingHandler, adminDashboardHandler, adminUserHandler, adminFinancialHandler, adminReportsHandler, adminHealthHandler, adminFinanceHandler, adminRiskHandler, offplanHandler, supportHandler, adminSupportHandler, privacyHandler, adminPrivacyHandler, userRepo, cfg, readinessHandler)

	// 9. START THE API PROCESS'S REALTIME CONSUMER.
	// The worker (and webhook path) publish "realtime notification" messages to
	// RabbitMQ; the API process consumes them and fans them out to connected
	// WebSocket clients. This lets a separate worker trigger realtime pushes.
	notificationService.StartRealtimeConsumer(context.Background())

	// 10. HTTP SERVER with timeouts + graceful shutdown (FIX-07).
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Block until we receive an interrupt/terminate signal.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutdown signal received; draining connections")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	if sqlDB, derr := database.DB.DB(); derr == nil {
		_ = sqlDB.Close()
	}
	logger.Info("shutdown complete")
}
