package config

import (
	"log"

	"github.com/spf13/viper"
)

// Config holds every environment variable the application needs.
// We define it as a struct so that the rest of the app gets
// type-safe access — cfg.Port instead of os.Getenv("PORT").
type Config struct {
	AppEnv             string `mapstructure:"APP_ENV"`
	Port               string `mapstructure:"PORT"`
	DatabaseURL        string `mapstructure:"DATABASE_URL"`
	RedisURL           string `mapstructure:"REDIS_URL"`
	JWTSecret          string `mapstructure:"JWT_SECRET"`
	JWTRefreshSecret   string `mapstructure:"JWT_REFRESH_SECRET"`
	AccessTokenTTL     string `mapstructure:"ACCESS_TOKEN_TTL"`
	RefreshTokenTTL    string `mapstructure:"REFRESH_TOKEN_TTL"`
	AllowedOrigins     string `mapstructure:"ALLOWED_ORIGINS"`
	
	// Cloudinary configuration for image uploads
	CloudinaryCloudName  string `mapstructure:"CLOUDINARY_CLOUD_NAME"`
	CloudinaryAPIKey     string `mapstructure:"CLOUDINARY_API_KEY"`
	CloudinaryAPISecret  string `mapstructure:"CLOUDINARY_API_SECRET"`
	CloudinaryUploadPreset string `mapstructure:"CLOUDINARY_UPLOAD_PRESET"`
	
	// SMS configuration for OTP verification
	SMSProvider           string `mapstructure:"SMS_PROVIDER"`
	TwilioAccountSID      string `mapstructure:"TWILIO_ACCOUNT_SID"`
	TwilioAuthToken       string `mapstructure:"TWILIO_AUTH_TOKEN"`
	TwilioPhoneNumber     string `mapstructure:"TWILIO_PHONE_NUMBER"`
	AfricasTalkingUsername string `mapstructure:"AFRICASTALKING_USERNAME"`
	AfricasTalkingAPIKey   string `mapstructure:"AFRICASTALKING_API_KEY"`
	AfricasTalkingSenderID string `mapstructure:"AFRICASTALKING_SENDER_ID"`
	TermiiAPIKey          string `mapstructure:"TERMII_API_KEY"`
	TermiiSenderID        string `mapstructure:"TERMII_SENDER_ID"`
}

// Load reads the .env file and maps it into a Config struct.
// It is called once at startup — in main.go — and the result
// is passed down to every component that needs it.
func Load() *Config {
	viper.SetConfigFile(".env")

	// AutomaticEnv means: if an env variable is set in the OS
	// (e.g. on a production server), it takes priority over .env.
	// This is how the same binary works in both local dev and production.
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		// Not fatal — on a production server there may be no .env file,
		// just real environment variables injected by the platform.
		log.Printf("No .env file found, reading from environment: %v", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		// This IS fatal — if we can't read config, nothing works.
		log.Fatalf("Failed to unmarshal config: %v", err)
	}

	return &cfg
}