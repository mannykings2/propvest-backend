package main

import (
    "fmt"
    "log"

    "github.com/gin-gonic/gin"
    "github.com/mannykings2/propvest-backend/internal/config"
    "github.com/mannykings2/propvest-backend/internal/database"
    "github.com/mannykings2/propvest-backend/internal/handlers"
    "github.com/mannykings2/propvest-backend/internal/middleware"
    "github.com/mannykings2/propvest-backend/internal/repositories"
    v1 "github.com/mannykings2/propvest-backend/internal/routes/v1"
    "github.com/mannykings2/propvest-backend/internal/services"
    "github.com/mannykings2/propvest-backend/internal/utils/cloudinary"
    "github.com/mannykings2/propvest-backend/internal/utils/sms"
)

func main() {
	// ══════════════════════════════════════════════════════════════════
	// MILESTONE 0: FOUNDATION - COMPOSITION ROOT
	// ══════════════════════════════════════════════════════════════════
	// This is where EVERY dependency is created and wired together.
	// This is called the "Composition Root" pattern.
	//
	// Why here?
	//   1. Single source of truth for dependency wiring
	//   2. Easy to understand the entire application structure
	//   3. Easy to test (mock any dependency)
	//   4. Impossible to create hidden dependencies
	//
	// Layers are initialized bottom-up:
	//   Database → Repositories → Services → Handlers → Routes → Server

	// ──────────────────────────────────────────────────────────────────
	// 1. LOAD CONFIGURATION
	// ──────────────────────────────────────────────────────────────────
	// Configuration is always first. If config fails, nothing else works.
	log.Println("Loading configuration...")
	cfg := config.Load()

	// ──────────────────────────────────────────────────────────────────
	// 2. CONNECT TO DATABASE
	// ──────────────────────────────────────────────────────────────────
	log.Println("Connecting to database...")
	database.Connect(cfg.DatabaseURL)

	// ──────────────────────────────────────────────────────────────────
	// 3. RUN DATABASE MIGRATIONS
	// ──────────────────────────────────────────────────────────────────
	// RunMigrations uses golang-migrate for versioned, reversible migrations.
	// This is the production-grade approach — explicit SQL migrations
	// with full rollback support.
	//
	// Migration files live in: internal/database/migrations/
	// To create a new migration:
	//   make migrate-create name=add_feature
	//
	// To apply migrations:
	//   make migrate-up
	//
	// To rollback:
	//   make migrate-down
	log.Println("Running database migrations...")
	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// ──────────────────────────────────────────────────────────────────
	// 4. INITIALIZE REPOSITORIES (Data Access Layer)
	// ──────────────────────────────────────────────────────────────────
	// Repositories talk to the database. They receive *gorm.DB via
	// constructor injection, making them easy to test with mocks.
	log.Println("Initializing repositories...")
	userRepo := repositories.NewUserRepository(database.DB)
	walletRepo := repositories.NewWalletRepository(database.DB)
	refreshTokenRepo := repositories.NewRefreshTokenRepository(database.DB) // Milestone 1: Authentication
	otpRepo := repositories.NewOTPVerificationRepository(database.DB)      // Milestone 2: Phone verification

	// Future repositories (Milestone 3+):
	// propertyRepo := repositories.NewPropertyRepository(database.DB)
	// investmentRepo := repositories.NewInvestmentRepository(database.DB)
	// notificationRepo := repositories.NewNotificationRepository(database.DB)

	// ──────────────────────────────────────────────────────────────────
	// 5. INITIALIZE SERVICES (Business Logic Layer)
	// ──────────────────────────────────────────────────────────────────
	// Services contain business logic. They receive repositories and
	// config via constructor injection.
	log.Println("Initializing services...")

	// Initialize utilities
	cloudinaryService, err := cloudinary.NewCloudinaryService(cfg)
	if err != nil {
		log.Printf("Warning: Cloudinary not configured: %v", err)
		// Non-fatal - avatar uploads will fail but other features work
	}

	smsService := sms.NewSMSService(cfg) // SMS service (mock in development)

	// Initialize services
	authService := services.NewAuthService(userRepo, walletRepo, refreshTokenRepo, cfg, database.DB)
	userService := services.NewUserService(userRepo, otpRepo, refreshTokenRepo, smsService, cloudinaryService, cfg)
	walletService := services.NewWalletService(walletRepo, userRepo)

	// Future services (Milestone 4+):
	// propertyService := services.NewPropertyService(propertyRepo, userRepo)
	// investmentService := services.NewInvestmentService(investmentRepo, walletRepo, propertyRepo)
	// notificationService := services.NewNotificationService(notificationRepo)

	// ──────────────────────────────────────────────────────────────────
	// 6. INITIALIZE HANDLERS (HTTP Layer)
	// ──────────────────────────────────────────────────────────────────
	// Handlers parse HTTP requests and call services.
	// They receive services via constructor injection.
	log.Println("Initializing handlers...")
	authHandler := handlers.NewAuthHandler(authService)
	userHandler := handlers.NewUserHandler(userService)

	// Future handlers (Milestone 3+):
	// walletHandler := handlers.NewWalletHandler(walletService)
	// propertyHandler := handlers.NewPropertyHandler(propertyService)

	// Silence "unused variable" warnings for now
	_ = walletService

	// ──────────────────────────────────────────────────────────────────
	// 7. CONFIGURE GIN ROUTER
	// ──────────────────────────────────────────────────────────────────
	// Set Gin mode based on environment
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create router WITHOUT default middleware
	// We add our own middleware stack for full control
	r := gin.New()

	// ──────────────────────────────────────────────────────────────────
	// 8. REGISTER GLOBAL MIDDLEWARE
	// ──────────────────────────────────────────────────────────────────
	// Middleware runs in the order it's registered.
	// Order matters! Each middleware may depend on previous ones.
	//
	// Recommended production order:
	//   Recovery    → Catch panics (must be first)
	//   RequestID   → Generate unique request ID (for tracing)
	//   Logger      → Log every request (needs request ID)
	//   CORS        → Handle cross-origin requests
	//   RateLimit   → Prevent abuse (future)
	//   Auth        → Parse JWT (per-route via middleware.Auth())
	//   RBAC        → Check permissions (per-route via middleware.RequireRole())

	log.Println("Registering middleware...")
	r.Use(middleware.Recovery())   // Must be first to catch all panics
	r.Use(middleware.RequestID())  // Generate request ID for tracing
	r.Use(middleware.Logger())     // Log all requests

	// Parse CORS allowed origins from config
	allowedOrigins := middleware.ParseAllowedOrigins(cfg.AllowedOrigins)
	r.Use(middleware.CORS(allowedOrigins))

	// Future middleware (Milestone 1+):
	// r.Use(middleware.RateLimit())

	// ──────────────────────────────────────────────────────────────────
	// 9. REGISTER API ROUTES
	// ──────────────────────────────────────────────────────────────────
	// All routes are under /api/v1
	// Route registration is delegated to the routes package
	log.Println("Registering routes...")
	apiV1 := r.Group("/api/v1")
	v1.RegisterRoutes(apiV1, authHandler, userHandler, cfg)

	// Future: When we add v2, we create a new routes package
	// apiV2 := r.Group("/api/v2")
	// v2.RegisterRoutes(apiV2, authHandler, userHandler, cfg)

	// ──────────────────────────────────────────────────────────────────
	// 10. START HTTP SERVER
	// ──────────────────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("✓ PropVest API successfully initialized")
	log.Printf("✓ Environment: %s", cfg.AppEnv)
	log.Printf("✓ Database: Connected")
	log.Printf("✓ Migrations: Applied")
	log.Printf("✓ Server starting on %s", addr)
	log.Println("──────────────────────────────────────────────────────")

	if err := r.Run(addr); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// ══════════════════════════════════════════════════════════════════════
// TEACHING NOTES: Why Dependency Injection?
// ══════════════════════════════════════════════════════════════════════
//
// BEFORE (tight coupling):
//   func RegisterUser(email, password string) {
//       db := database.DB  // Global variable!
//       user := &User{...}
//       db.Create(user)    // Impossible to test without real database
//   }
//
// AFTER (dependency injection):
//   func (s *AuthService) RegisterUser(ctx context.Context, email, password string) {
//       user := &User{...}
//       s.userRepo.Create(ctx, user)  // Can be mocked in tests!
//   }
//
// Benefits:
//   1. Testability: Mock any dependency
//   2. Flexibility: Swap implementations without changing code
//   3. Clarity: All dependencies are explicit
//   4. Modularity: Each component is independent
//
// The only place that knows about ALL dependencies is main.go.
// Everything else only knows about its immediate dependencies.
// ══════════════════════════════════════════════════════════════════════
