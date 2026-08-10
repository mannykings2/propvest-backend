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
	emailSender := mailer.New(cfg)
	paymentProvider := payments.NewProvider(cfg)

	auditRecorder := audit.NewLogRecorder()

	// 5. REPOSITORIES.
	userRepo := repositories.NewUserRepository(database.DB)
	walletRepo := repositories.NewWalletRepository(database.DB)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(database.DB)
	otpRepo := repositories.NewOTPVerificationRepository(database.DB)
	tokenRepo := repositories.NewVerificationTokenRepository(database.DB)
	// propertyRepo := repositories.NewPropertyRepository(database.DB)
	// investmentRepo := repositories.NewInvestmentRepository(database.DB)
	// notificationRepo := repositories.NewNotificationRepository(database.DB)
	paymentRepo := repositories.NewPaymentRepository(database.DB)

	// 6. UTILITIES / SERVICES.
	cloudinaryService, err := cloudinary.NewCloudinaryService(cfg)
	if err != nil {
		logger.Warn("Cloudinary not configured; avatar/image uploads will fail", "error", err)
	}
	smsService := sms.NewSMSService(cfg)

	// Notification service: writes in-app rows, pushes over WebSocket, and
	// enqueues email/SMS work onto RabbitMQ.
	// TODO: Uncomment when notification module is implemented (Milestone 6)
	// notificationService := services.NewNotificationService(notificationRepo, hub, mq)

	// For now, use nil for notification service in wallet service (it's optional)
	var notificationService services.NotificationService = nil

	authService := services.NewAuthService(userRepo, walletRepo, refreshTokenRepo, tokenRepo, emailSender, auditRecorder, cfg, database.DB)
	userService := services.NewUserService(userRepo, otpRepo, refreshTokenRepo, smsService, cloudinaryService, cfg)
	walletService := services.NewWalletService(walletRepo, userRepo, paymentRepo, paymentProvider, notificationService, mq, cfg, database.DB)
	// propertyService := services.NewPropertyService(propertyRepo, cloudinaryService, auditRecorder)
	// investmentService := services.NewInvestmentService(investmentRepo, walletRepo, propertyRepo, notificationService, cfg, database.DB)
	// adminService := services.NewAdminService(userRepo, propertyRepo, investmentRepo, walletRepo, refreshTokenRepo, auditRecorder)

	// 7. HANDLERS.
	authHandler := handlers.NewAuthHandler(authService)
	userHandler := handlers.NewUserHandler(userService)
	walletHandler := handlers.NewWalletHandler(walletService)
	// propertyHandler := handlers.NewPropertyHandler(propertyService)
	// investmentHandler := handlers.NewInvestmentHandler(investmentService)
	// notificationHandler := handlers.NewNotificationHandler(notificationService)
	// adminHandler := handlers.NewAdminHandler(adminService)
	// webhookHandler := handlers.NewWebhookHandler(walletService, cfg)
	// wsHandler := handlers.NewWebSocketHandler(hub, cfg)

	// 8. ROUTER + MIDDLEWARE (order matters, docs 4.2 §13 / 1.4 §5).
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.Recovery())  // catch panics (first)
	r.Use(middleware.RequestID()) // correlation id
	r.Use(middleware.Logger())    // structured request log
	r.Use(middleware.CORS(middleware.ParseAllowedOrigins(cfg.AllowedOrigins)))

	// Root-level operational endpoints (not versioned): liveness/readiness.
	r.GET("/health", handlers.HealthCheck)
	r.GET("/health/ready", handlers.ReadyCheck)

	apiV1 := r.Group("/api/v1")
	v1.RegisterRoutes(apiV1, authHandler, userHandler, walletHandler, cfg)

	// 9. START THE API PROCESS'S REALTIME CONSUMER.
	// The worker (and webhook path) publish "realtime notification" messages to
	// RabbitMQ; the API process consumes them and fans them out to connected
	// WebSocket clients. This lets a separate worker trigger realtime pushes.
	// TODO: Uncomment when notification module is implemented (Milestone 6)
	// notificationService.StartRealtimeConsumer(context.Background())

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
