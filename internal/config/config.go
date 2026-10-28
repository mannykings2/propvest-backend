package config

import (
	"fmt"
	"log"
	"strings"
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

	// TrustedProxies is a comma-separated list of CIDRs (or IPs) whose
	// X-Forwarded-For header Gin may believe. Default is loopback only:
	// a direct client rotating forged XFF values would otherwise mint a
	// fresh rate-limit bucket per request (verified: 11/12 auth-endpoint
	// requests bypassed the 10/min limit by rotating XFF — Phase 5).
	// Behind a reverse proxy, list the proxy's CIDR here so real client
	// IPs keep flowing into rate limits and audit rows.
	TrustedProxies string `mapstructure:"TRUSTED_PROXIES"`

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
	EmailProvider string `mapstructure:"EMAIL_PROVIDER"` // "mock", "smtp" or "brevo"

	// Brevo (formerly Sendinblue) is configured with its own variable names in
	// .env. normalizeEmail() folds these into the generic SMTP_* fields so the
	// mailer has one contract to satisfy. EMAIL_FROM / EMAIL_FROM_NAME are the
	// friendly aliases used alongside BREVO_* in .env.
	BrevoSMTPHost     string `mapstructure:"BREVO_SMTP_HOST"`
	BrevoSMTPPort     int    `mapstructure:"BREVO_SMTP_PORT"`
	BrevoSMTPUsername string `mapstructure:"BREVO_SMTP_USERNAME"`
	BrevoSMTPPassword string `mapstructure:"BREVO_SMTP_PASSWORD"`
	EmailFrom         string `mapstructure:"EMAIL_FROM"`
	EmailFromName     string `mapstructure:"EMAIL_FROM_NAME"`

	// ── Payments (Paystack) ──────────────────────────────────────
	PaystackSecretKey     string `mapstructure:"PAYSTACK_SECRET_KEY"`
	PaystackPublicKey     string `mapstructure:"PAYSTACK_PUBLIC_KEY"`
	PaystackWebhookSecret string `mapstructure:"PAYSTACK_WEBHOOK_SECRET"`
	PaymentProvider       string `mapstructure:"PAYMENT_PROVIDER"` // "mock" or "paystack"

	// KYC_ENCRYPTION_KEY encrypts BVN/NIN at rest (AES-256-GCM, Phase 6.1).
	// Any non-empty passphrase works (SHA-256'd to the key); production boot
	// requires it, development falls back to a fixed local key.
	KYCEncryptionKey string `mapstructure:"KYC_ENCRYPTION_KEY"`

	// ── Feature flags ────────────────────────────────────────────
	// Phase 6.4: both default to TRUE in production (setDefaults below) —
	// a production deployment that forgets them gates investing behind
	// verification instead of silently leaving it open. Explicit env wins.
	RequireEmailVerification bool  `mapstructure:"REQUIRE_EMAIL_VERIFICATION"`
	RequireKYCForInvestment  bool  `mapstructure:"REQUIRE_KYC_FOR_INVESTMENT"`
	// Phase 9.1 (NDPA): signup must record acceptance of the terms + privacy
	// policy in production. Marketing consent is deliberately NOT enforced —
	// bundling it would make the consent not "freely given".
	RequireRegistrationConsent bool `mapstructure:"REQUIRE_REGISTRATION_CONSENT"`
	MinInvestmentAmount      int64 `mapstructure:"MIN_INVESTMENT_AMOUNT"` // kobo
	MaxInvestmentAmount      int64 `mapstructure:"MAX_INVESTMENT_AMOUNT"` // kobo
	MinDepositAmount         int64 `mapstructure:"MIN_DEPOSIT_AMOUNT"`    // kobo
	MaxDepositAmount         int64 `mapstructure:"MAX_DEPOSIT_AMOUNT"`    // kobo
	MinWithdrawalAmount      int64 `mapstructure:"MIN_WITHDRAWAL_AMOUNT"` // kobo
	MaxWithdrawalAmount      int64 `mapstructure:"MAX_WITHDRAWAL_AMOUNT"` // kobo
}

// Load reads the .env file (and OS env) and maps it into a Config struct.
// It is called once at startup — in main.go — and the result is passed down
// to every component that needs it.
func Load() *Config {
	viper.SetConfigFile(".env")

	// AutomaticEnv means: if an env variable is set in the OS (e.g. on a
	// production server), it takes priority over .env.
	viper.AutomaticEnv()

	// Explicitly bind environment variables so they are available
	// during viper.Unmarshal(), including when there is no .env file.
	bindEnvironmentVariables()

	// Sensible defaults so local dev works with a minimal .env.
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("No .env file found, reading from environment: %v", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalf("Failed to unmarshal config: %v", err)
	}

	normalizeEmail(&cfg)

	return &cfg
}

// normalizeEmail folds the provider-specific email variables (.env's
// EMAIL_PROVIDER=brevo + BREVO_SMTP_* + EMAIL_FROM*) into the generic SMTP_*
// contract the mailer reads.
//
// WHY THIS EXISTS: before it did, EMAIL_PROVIDER=brevo fell through mailer.New's
// switch to the default branch — the mock — so verification and password-reset
// links were only ever written to the log, in every environment, with no error.
// A provider name nobody recognized silently disabled email delivery.
func normalizeEmail(c *Config) {
	c.EmailProvider = strings.ToLower(strings.TrimSpace(c.EmailProvider))
	switch c.EmailProvider {
	case "", "brevo", "sendinblue", "smtp":
		// A configured SMTP relay (whatever it is called) means real sending.
		if c.BrevoSMTPHost != "" || c.SMTPHost != "" {
			c.EmailProvider = "smtp"
		} else {
			c.EmailProvider = "mock"
		}
	default:
		// A provider name nobody recognizes is kept verbatim so Validate() can
		// reject it loudly instead of the mailer quietly selecting the mock.
	}

	// BREVO_* fills the gaps only — explicit SMTP_* always wins.
	if c.SMTPHost == "" {
		c.SMTPHost = c.BrevoSMTPHost
	}
	if c.SMTPPort == 0 {
		c.SMTPPort = c.BrevoSMTPPort
	}
	if c.SMTPUsername == "" {
		c.SMTPUsername = c.BrevoSMTPUsername
	}
	if c.SMTPPassword == "" {
		c.SMTPPassword = c.BrevoSMTPPassword
	}
	if c.SMTPFromEmail == "" {
		c.SMTPFromEmail = c.EmailFrom
	}
	if c.SMTPFromName == "" {
		c.SMTPFromName = c.EmailFromName
	}
	if c.SMTPPort == 0 {
		c.SMTPPort = 587
	}
}

// setDefaults registers fallback values for optional tunables.
func setDefaults() {
	viper.SetDefault("APP_ENV", "development")

	// Phase 6.4 launch-gate default: production requires verified email + KYC
	// to invest unless the operator explicitly sets these to false. Computed
	// after the APP_ENV default so development keeps the open default.
	productionDefault := viper.GetString("APP_ENV") == "production"
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("BASE_URL", "http://localhost:8080")

	viper.SetDefault("DB_MAX_OPEN_CONNS", 25)
	viper.SetDefault("DB_MAX_IDLE_CONNS", 10)
	viper.SetDefault("DB_CONN_MAX_LIFETIME", "1h")
	viper.SetDefault("DB_CONN_MAX_IDLE_TIME", "15m")

	viper.SetDefault("ACCESS_TOKEN_TTL", "15m")
	viper.SetDefault("REFRESH_TOKEN_TTL", "720h") // 30 days
	viper.SetDefault("BCRYPT_COST", 12)

	// Loopback only: a direct client must not be able to mint fresh
	// rate-limit buckets by rotating X-Forwarded-For. Production behind
	// a reverse proxy overrides this with the proxy's CIDR.
	viper.SetDefault("TRUSTED_PROXIES", "127.0.0.1/32,::1/128")

	viper.SetDefault("SMS_PROVIDER", "mock")
	viper.SetDefault("EMAIL_PROVIDER", "mock")
	viper.SetDefault("PAYMENT_PROVIDER", "mock")

	viper.SetDefault("REQUIRE_EMAIL_VERIFICATION", productionDefault)
	viper.SetDefault("REQUIRE_KYC_FOR_INVESTMENT", productionDefault)
	viper.SetDefault("REQUIRE_REGISTRATION_CONSENT", productionDefault)
	viper.SetDefault("MIN_DEPOSIT_AMOUNT", 10000)         // ₦100
	viper.SetDefault("MAX_DEPOSIT_AMOUNT", 1000000000)    // ₦10,000,000
	viper.SetDefault("MIN_INVESTMENT_AMOUNT", 100000)     // ₦1,000
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
	if c.EmailProvider == "smtp" && c.SMTPHost == "" {
		return fmt.Errorf("EMAIL_PROVIDER=%s requires SMTP_HOST (or BREVO_SMTP_HOST)", c.EmailProvider)
	}
	if c.EmailProvider == "smtp" && c.SMTPFromEmail == "" {
		return fmt.Errorf("EMAIL_PROVIDER=%s requires SMTP_FROM_EMAIL (or EMAIL_FROM)", c.EmailProvider)
	}
	switch c.EmailProvider {
	case "mock", "smtp":
	default:
		return fmt.Errorf("unknown EMAIL_PROVIDER=%q (use smtp, brevo or mock)", c.EmailProvider)
	}
	// Fail fast rather than launch with verification/reset links that only reach
	// the log (that is exactly how the brevo/mock mismatch above went unnoticed).
	if c.IsProduction() && c.EmailProvider == "mock" {
		return fmt.Errorf("APP_ENV=production requires a real EMAIL_PROVIDER: set SMTP_HOST/SMTP_FROM_EMAIL (or BREVO_SMTP_HOST/EMAIL_FROM)")
	}

	// ── Phase 3.2: production configuration guards ─────────────────────────
	if c.IsProduction() {
		// A leaked access token must not double as a refresh token: the two
		// lifetimes exist precisely so short-lived tokens can't extend
		// themselves. Signing both with one key defeats that.
		if c.JWTSecret == c.JWTRefreshSecret {
			return fmt.Errorf("APP_ENV=production requires JWT_SECRET and JWT_REFRESH_SECRET to be different values")
		}

		// CORS must pin explicit origins. Empty means every browser request is
		// rejected (a silent outage); "*" means any website can call the API
		// with a user's session (a silent data leak). Neither is acceptable.
		if strings.TrimSpace(c.AllowedOrigins) == "" {
			return fmt.Errorf("APP_ENV=production requires ALLOWED_ORIGINS (comma-separated, e.g. https://propvest.com)")
		}
		if strings.Contains(c.AllowedOrigins, "*") {
			return fmt.Errorf("APP_ENV=production forbids a wildcard '*' in ALLOWED_ORIGINS")
		}

		// Payments must never silently run on the mock provider:
		// payments.New() falls back to MockProvider when PAYMENT_PROVIDER says
		// "paystack" but PAYSTACK_SECRET_KEY is empty — a fake payment layer
		// in production. Fail at boot instead.
		switch c.PaymentProvider {
		case "paystack", "hybrid":
			if c.PaystackSecretKey == "" {
				return fmt.Errorf("APP_ENV=production with PAYMENT_PROVIDER=%s requires PAYSTACK_SECRET_KEY (otherwise payments fall back to the mock provider)", c.PaymentProvider)
			}
		default:
			return fmt.Errorf("APP_ENV=production requires PAYMENT_PROVIDER=paystack or hybrid (got %q)", c.PaymentProvider)
		}

		// BVN/NIN sit in kyc_submissions: without an encryption key the app
		// would store them in the clear (Phase 6.1). Refuse to boot rather
		// than silently degrade to plaintext PII.
		if strings.TrimSpace(c.KYCEncryptionKey) == "" {
			return fmt.Errorf("APP_ENV=production requires KYC_ENCRYPTION_KEY (encrypts BVN/NIN at rest)")
		}
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

// TrustedProxiesList parses the comma-separated TRUSTED_PROXIES value into
// the CIDR/IP list Gin's SetTrustedProxies expects. Empty input yields nil,
// which Gin treats as "trust no proxies" (X-Forwarded-For ignored) — the
// safe direction: rate limits and audit IPs then key on the real peer
// address instead of a client-forged header (Phase 5 rate-limit bypass fix).
func (c *Config) TrustedProxiesList() []string {
	var out []string
	for _, part := range strings.Split(c.TrustedProxies, ",") {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// IsProduction reports whether the app is running in production mode.
func (c *Config) IsProduction() bool {
	return c.AppEnv == "production"
}

// IsStaging reports whether the app is running in the staging environment.
// Staging mirrors production: verbose debug output (Gin's debug route table,
// debug logging) must stay off there, while development keeps it for humans.
func (c *Config) IsStaging() bool {
	return c.AppEnv == "staging"
}

func bindEnvironmentVariables() {
	keys := []string{
		"APP_ENV",
		"PORT",
		"BASE_URL",

		"DATABASE_URL",
		"DB_MAX_OPEN_CONNS",
		"DB_MAX_IDLE_CONNS",
		"DB_CONN_MAX_LIFETIME",
		"DB_CONN_MAX_IDLE_TIME",

		"REDIS_URL",
		"RABBITMQ_URL",

		"JWT_SECRET",
		"JWT_REFRESH_SECRET",
		"KYC_ENCRYPTION_KEY",
		"ACCESS_TOKEN_TTL",
		"REFRESH_TOKEN_TTL",
		"BCRYPT_COST",
		"ALLOWED_ORIGINS",
		"TRUSTED_PROXIES",

		"CLOUDINARY_CLOUD_NAME",
		"CLOUDINARY_API_KEY",
		"CLOUDINARY_API_SECRET",
		"CLOUDINARY_UPLOAD_PRESET",

		"SMS_PROVIDER",
		"TWILIO_ACCOUNT_SID",
		"TWILIO_AUTH_TOKEN",
		"TWILIO_PHONE_NUMBER",
		"AFRICASTALKING_USERNAME",
		"AFRICASTALKING_API_KEY",
		"AFRICASTALKING_SENDER_ID",
		"TERMII_API_KEY",
		"TERMII_SENDER_ID", "SMTP_HOST",
		"SMTP_PORT",
		"SMTP_USERNAME",
		"SMTP_PASSWORD",
		"SMTP_FROM_EMAIL",
		"SMTP_FROM_NAME",
		"EMAIL_PROVIDER",
		"BREVO_SMTP_HOST",
		"BREVO_SMTP_PORT",
		"BREVO_SMTP_USERNAME",
		"BREVO_SMTP_PASSWORD",
		"EMAIL_FROM",
		"EMAIL_FROM_NAME",

		"PAYSTACK_SECRET_KEY",
		"PAYSTACK_PUBLIC_KEY",
		"PAYSTACK_WEBHOOK_SECRET",
		"PAYMENT_PROVIDER",

		"REQUIRE_EMAIL_VERIFICATION",
		"REQUIRE_KYC_FOR_INVESTMENT",
		"MIN_INVESTMENT_AMOUNT",
		"MAX_INVESTMENT_AMOUNT",
		"MIN_DEPOSIT_AMOUNT",
		"MAX_DEPOSIT_AMOUNT",
		"MIN_WITHDRAWAL_AMOUNT",
		"MAX_WITHDRAWAL_AMOUNT",
	}

	for _, key := range keys {
		if err := viper.BindEnv(key); err != nil {
			log.Fatalf("Failed to bind environment variable %s: %v", key, err)
		}
	}
}
