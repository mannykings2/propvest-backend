package config

import "testing"

// TestNormalizeEmail pins the folding of the .env-specific Brevo variables into
// the generic SMTP contract. Before this existed, EMAIL_PROVIDER=brevo selected
// the mock mailer and no email was ever delivered, silently.
func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		name         string
		in           *Config
		wantProvider string
		wantHost     string
		wantPort     int
		wantFrom     string
	}{
		{
			name: "brevo variables fold into SMTP",
			in: &Config{
				EmailProvider:     "Brevo",
				BrevoSMTPHost:     "smtp-relay.brevo.com",
				BrevoSMTPPort:     587,
				BrevoSMTPUsername: "user@smtp-brevo.com",
				BrevoSMTPPassword: "secret",
				EmailFrom:         "noreply@propvest.com",
				EmailFromName:     "PropVest",
			},
			wantProvider: "smtp",
			wantHost:     "smtp-relay.brevo.com",
			wantPort:     587,
			wantFrom:     "noreply@propvest.com",
		},
		{
			name: "explicit SMTP wins over brevo",
			in: &Config{
				EmailProvider: "brevo",
				SMTPHost:      "smtp.example.com",
				SMTPPort:      2525,
				SMTPFromEmail: "real@example.com",
				BrevoSMTPHost: "smtp-relay.brevo.com",
				EmailFrom:     "fallback@example.com",
			},
			wantProvider: "smtp",
			wantHost:     "smtp.example.com",
			wantPort:     2525,
			wantFrom:     "real@example.com",
		},
		{
			name:         "nothing configured stays mock",
			in:           &Config{EmailProvider: "mock"},
			wantProvider: "mock",
		},
		{
			name:         "unknown provider is kept verbatim so Validate can reject it",
			in:           &Config{EmailProvider: "sendgrid"},
			wantProvider: "sendgrid",
		},
		{
			name:         "smtp host without explicit provider still enables smtp",
			in:           &Config{SMTPHost: "localhost", SMTPPort: 1025},
			wantProvider: "smtp",
			wantHost:     "localhost",
			wantPort:     1025,
		},
		{
			name:         "missing port defaults to 587",
			in:           &Config{EmailProvider: "smtp", SMTPHost: "smtp.example.com"},
			wantProvider: "smtp",
			wantHost:     "smtp.example.com",
			wantPort:     587,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			normalizeEmail(tt.in)
			if tt.in.EmailProvider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", tt.in.EmailProvider, tt.wantProvider)
			}
			if tt.in.SMTPHost != tt.wantHost {
				t.Errorf("host = %q, want %q", tt.in.SMTPHost, tt.wantHost)
			}
			if tt.wantPort != 0 && tt.in.SMTPPort != tt.wantPort {
				t.Errorf("port = %d, want %d", tt.in.SMTPPort, tt.wantPort)
			}
			if tt.wantFrom != "" && tt.in.SMTPFromEmail != tt.wantFrom {
				t.Errorf("from = %q, want %q", tt.in.SMTPFromEmail, tt.wantFrom)
			}
		})
	}
}

// TestValidate_EmailRules covers the fail-fast rules added with the fix.
func TestValidate_EmailRules(t *testing.T) {
	base := func() *Config {
		return &Config{
			DatabaseURL:      "postgres://localhost/x",
			JWTSecret:        "0123456789abcdef0123456789abcdef",
			JWTRefreshSecret: "fedcba9876543210fedcba9876543210",
			AccessTokenTTL:   "15m",
			RefreshTokenTTL:  "720h",
			BcryptCost:       12,
		}
	}

	// validProduction returns a base config that also satisfies the Phase 3.2
	// production guards (CORS origins, payment provider, distinct JWT keys), so
	// each subtest can break exactly one rule.
	validProduction := func() *Config {
		c := base()
		c.AppEnv = "production"
		c.EmailProvider = "smtp"
		c.SMTPHost = "smtp-relay.brevo.com"
		c.SMTPFromEmail = "noreply@propvest.com"
		c.AllowedOrigins = "https://propvest.com"
		c.PaymentProvider = "paystack"
		c.PaystackSecretKey = "sk_live_placeholder"
		// Phase 6.1: production refuses to boot without a BVN/NIN key.
		c.KYCEncryptionKey = "production-test-kyc-key"
		return c
	}

	t.Run("production refuses mock email", func(t *testing.T) {
		c := base()
		c.AppEnv = "production"
		c.EmailProvider = "mock"
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error for APP_ENV=production with EMAIL_PROVIDER=mock")
		}
	})

	t.Run("production accepts configured smtp", func(t *testing.T) {
		c := validProduction()
		if err := c.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("production requires KYC encryption key", func(t *testing.T) {
		c := validProduction()
		c.KYCEncryptionKey = "  "
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error for APP_ENV=production without KYC_ENCRYPTION_KEY")
		}
	})

	// ── Phase 3.2 production guards ─────────────────────────────────────
	t.Run("production requires distinct JWT secrets", func(t *testing.T) {
		c := validProduction()
		c.JWTRefreshSecret = c.JWTSecret
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error when JWT_SECRET == JWT_REFRESH_SECRET in production")
		}
	})

	t.Run("production requires ALLOWED_ORIGINS", func(t *testing.T) {
		c := validProduction()
		c.AllowedOrigins = ""
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error for empty ALLOWED_ORIGINS in production")
		}
	})

	t.Run("production forbids wildcard origins", func(t *testing.T) {
		c := validProduction()
		c.AllowedOrigins = "*"
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error for wildcard ALLOWED_ORIGINS in production")
		}
	})

	t.Run("production forbids mock payments", func(t *testing.T) {
		c := validProduction()
		c.PaymentProvider = "mock"
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error for PAYMENT_PROVIDER=mock in production")
		}
	})

	t.Run("production requires PAYSTACK_SECRET_KEY", func(t *testing.T) {
		c := validProduction()
		c.PaystackSecretKey = ""
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error for paystack mode without a secret key in production")
		}
	})

	t.Run("development may use mock payments and shared origins", func(t *testing.T) {
		c := base()
		c.AppEnv = "development"
		c.EmailProvider = "mock"
		c.PaymentProvider = "mock"
		c.AllowedOrigins = "http://localhost:3000"
		if err := c.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("development may run without email", func(t *testing.T) {
		c := base()
		c.AppEnv = "development"
		c.EmailProvider = "mock"
		if err := c.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown provider is rejected", func(t *testing.T) {
		c := base()
		c.AppEnv = "development"
		c.EmailProvider = "sendgrid"
		if err := c.Validate(); err == nil {
			t.Fatal("expected an error for an unrecognized EMAIL_PROVIDER")
		}
	})
}
