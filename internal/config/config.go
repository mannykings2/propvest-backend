package config

import (
	"fmt"
	"log"
	"time"

	"github.com/spf13/viper"
)

// Config holds every environment variable the application needs.
// We define it as a struct so the rest of the app gets type-safe access —
// cfg.Port instead of os.Getenv("PORT").
//
// DECISION D8/D11 and the config-hardening item in the handoff: fields that the
// docs/.env.example referenced but that were never mapped (Paystack, SMTP, DB
// pool tunables, bcrypt cost, feature flags, RabbitMQ) are now represented here
// so they actually take effect instead of being silently ignored.
type Config struct {
	// ── Application ──────────────────────────────────────────────
	AppEnv  string `mapstructure:"APP_ENV"`
	Port    string `mapstructure:"PORT"`
	BaseURL string `mapstructure:"BASE_URL"` // public URL, used in email links

	// ── Database ─────────────────────────────────────────────────
	DatabaseURL       string `mapstructure:"DATABASE_URL"`
	DBMaxOpenConns    int    `mapstructure:"DB_MAX_OPEN_CONNS"`
	DBMaxIdleConns    int    `mapstructure:"DB_MAX_IDLE_CONNS"`
	DBConnMaxLifetime string `mapstructure:"DB_CONN_MAX_LIFETIME"`
	DBConnMaxIdleTime string `mapstructure:"DB_CONN_MAX_IDLE_TIME"`

	// ── Redis / RabbitMQ ─────────────────────────────────────────
	RedisURL    string `mapstructure:"REDIS_URL"`
	RabbitMQURL string `mapstructure:"RABBITMQ_URL"` // amqp://guest:guest@localhost:5672/

	// ── Auth / security ──────────────────────────────────────────
	JWTSecret        string `mapstructure:"JWT_SECRET"`
	JWTRefreshSecret string `mapstructure:"JWT_REFRESH_SECRET"`
	AccessTokenTTL   string `mapstructure:"ACCESS_TOKEN_TTL"`
	RefreshTokenTTL  string `mapstructure:"REFRESH_TOKEN_TTL"`
	BcryptCost       int    `mapstructure:"BCRYPT_COST"`
	AllowedOrigins   string `mapstructure:"ALLOWED_ORIGINS"`

	// ── Cloudinary (image uploads) ───────────────────────────────
	CloudinaryCloudName    string `mapstructure:"CLOUDINARY_CLOUD_NAME"`
	CloudinaryAPIKey       string `mapstructure:"CLOUDINARY_API_KEY"`
	CloudinaryAPISecret    string `mapstructure:"CLOUDINARY_API_SECRET"`
	CloudinaryUploadPreset string `mapstructure:"CLOUDINARY_UPLOAD_PRESET"`

	// ── SMS (OTP delivery) ───────────────────────────────────────
	SMSProvider            string `mapstructure:"SMS_PROVIDER"`
	TwilioAccountSID       string `mapstructure:"TWILIO_ACCOUNT_SID"`
	TwilioAuthToken        string `mapstructure:"TWILIO_AUTH_TOKEN"`
	TwilioPhoneNumber      string `mapstructure:"TWILIO_PHONE_NUMBER"`
	AfricasTalkingUsername string `mapstructure:"AFRICASTALKING_USERNAME"`
	AfricasTalkingAPIKey   string `mapstructure:"AFRICASTALKING_API_KEY"`
	AfricasTalkingSenderID string `mapstructure:"AFRICASTALKING_SENDER_ID"`
	TermiiAPIKey           string `mapstructure:"TERMII_API_KEY"`
	TermiiSenderID         string `mapstructure:"TERMII_SENDER_ID"`

	// ── Email (SMTP) ─────────────────────────────────────────────
	SMTPHost      string `mapstructure:"SMTP_HOST"`
	SMTPPort      int    `mapstructure:"SMTP_PORT"`
	SMTPUsername  string `mapstructure:"SMTP_USERNAME"`
	SMTPPassword  string `mapstructure:"SMTP_PASSWORD"`
	SMTPFromEmail string `mapstructure:"SMTP_FROM_EMAIL"`
	SMTPFromName  string `mapstructure:"SMTP_FROM_NAME"`
	EmailProvider string `mapstructure:"EMAIL_PROVIDER"` // "mock" or "smtp"

	// ── Payments (Paystack) ──────────────────────────────────────
	PaystackSecretKey     string `mapstructure:"PAYSTACK_SECRET_KEY"`
	PaystackPublicKey     string `mapstructure:"PAYSTACK_PUBLIC_KEY"`
	PaystackWebhookSecret string `mapstructure:"PAYSTACK_WEBHOOK_SECRET"`
	PaymentProvider       string `mapstructure:"PAYMENT_PROVIDER"` // "mock" or "paystack"

	// ── Feature flags ────────────────────────────────────────────
	RequireEmailVerification bool  `mapstructure:"REQUIRE_EMAIL_VERIFICATION"`
	RequireKYCForInvestment  bool  `mapstructure:"REQUIRE_KYC_FOR_INVESTMENT"`
	MinInvestmentAmount      int64 `mapstructure:"MIN_INVESTMENT_AMOUNT"` // kobo
	MaxInvestmentAmount      int64 `mapstructure:"MAX_INVESTMENT_AMOUNT"` // kobo
	MinDepositAmount         int64 `mapstructure:"MIN_DEPOSIT_AMOUNT"`    // kobo
	MaxDepositAmount         int64 `mapstructure:"MAX_DEPOSIT_AMOUNT"`    // kobo
}

// Load reads the .env file (and OS env) and maps it into a Config struct.
// It is called once at startup — in main.go — and the result is passed down
// to every component that needs it.
func Load() *Config {
	viper.SetConfigFile(".env")

	// AutomaticEnv means: if an env variable is set in the OS (e.g. on a
	// production server), it takes priority over .env. This is how the same
	// binary works in both local dev and production.
	viper.AutomaticEnv()

	// Sensible defaults so local dev works with a minimal .env.
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		// Not fatal — on a production server there may be no .env file,
		// just real environment variables injected by the platform.
		log.Printf("No .env file found, reading from environment: %v", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Failed to unmarshal config: %v", err)
	}

	return &cfg
}

// setDefaults registers fallback values for optional tunables.
func setDefaults() {
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("BASE_URL", "http://localhost:8080")

	viper.SetDefault("DB_MAX_OPEN_CONNS", 25)
	viper.SetDefault("DB_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DB_CONN_MAX_LIFETIME", "1h")
	viper.SetDefault("DB_CONN_MAX_IDLE_TIME", "15m")

	viper.SetDefault("ACCESS_TOKEN_TTL", "15m")
	viper.SetDefault("REFRESH_TOKEN_TTL", "720h") // 30 days
	viper.SetDefault("BCRYPT_COST", 12)

	viper.SetDefault("SMS_PROVIDER", "mock")
	viper.SetDefault("EMAIL_PROVIDER", "mock")
	viper.SetDefault("PAYMENT_PROVIDER", "mock")

	viper.SetDefault("REQUIRE_EMAIL_VERIFICATION", false)
	viper.SetDefault("REQUIRE_KYC_FOR_INVESTMENT", false)
	viper.SetDefault("MIN_DEPOSIT_AMOUNT", 10000)        // ₦100
	viper.SetDefault("MAX_DEPOSIT_AMOUNT", 1000000000)   // ₦10,000,000
	viper.SetDefault("MIN_INVESTMENT_AMOUNT", 100000)    // ₦1,000
	viper.SetDefault("MAX_INVESTMENT_AMOUNT", 5000000000) // ₦50,000,000
}

// Validate checks that required configuration is present and well-formed, and
// returns an error describing the first problem. main() should call this right
// after Load() and fail fast (docs 1.1 §10 "fail-fast").
//
// FIX-08 (D8): the auth service used to ignore time.ParseDuration errors on the
// token TTLs (accessTokenTTL, _ := ...), silently turning a typo into a 0s TTL.
// We validate the TTL strings here at boot so a bad value stops startup loudly.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 16 {
		return fmt.Errorf("JWT_SECRET must be set and at least 16 characters")
	}
	if len(c.JWTRefreshSecret) < 16 {
		return fmt.Errorf("JWT_REFRESH_SECRET must be set and at least 16 characters")
	}
	if _, err := time.ParseDuration(c.AccessTokenTTL); err != nil {
		return fmt.Errorf("ACCESS_TOKEN_TTL is not a valid duration (e.g. 15m): %w", err)
	}
	if _, err := time.ParseDuration(c.RefreshTokenTTL); err != nil {
		return fmt.Errorf("REFRESH_TOKEN_TTL is not a valid duration (e.g. 720h): %w", err)
	}
	if c.BcryptCost < 10 || c.BcryptCost > 15 {
		return fmt.Errorf("BCRYPT_COST must be between 10 and 15 (got %d)", c.BcryptCost)
	}
	return nil
}

// AccessTTL / RefreshTTL parse the validated duration strings. Safe to call
// after Validate() has succeeded.
func (c *Config) AccessTTL() time.Duration {
	d, _ := time.ParseDuration(c.AccessTokenTTL)
	return d
}

func (c *Config) RefreshTTL() time.Duration {
	d, _ := time.ParseDuration(c.RefreshTokenTTL)
	return d
}

// IsProduction reports whether the app is running in production mode.
func (c *Config) IsProduction() bool {
	return c.AppEnv == "production"
}
