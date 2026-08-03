package sms

import (
	"context"
	"log"

	"github.com/mannykings2/propvest-backend/internal/config"
)

// SMSService defines the interface for sending SMS messages
// Using an interface allows us to:
//   1. Mock SMS sending in tests
//   2. Switch SMS providers without changing service code
//   3. Use mock provider in development (no real SMS cost)
type SMSService interface {
	// SendOTP sends a one-time password to a phone number
	SendOTP(ctx context.Context, phone, code string) error

	// SendNotification sends a general notification SMS
	SendNotification(ctx context.Context, phone, message string) error
}

// MockSMSService is a development implementation that logs instead of sending
// Perfect for local development where you don't want to:
//   - Pay for real SMS
//   - Need a real phone number
//   - Set up SMS provider credentials
//
// In development, OTP codes are logged to console:
//   [SMS] Sending OTP to +2348012345678: 123456
//
// You can copy the code from logs and use it for testing
type MockSMSService struct{}

// NewMockSMSService creates a new mock SMS service
// Used in development environment
func NewMockSMSService() SMSService {
	return &MockSMSService{}
}

// SendOTP logs the OTP code instead of sending real SMS
// In development, you see:
//   [SMS] Sending OTP to +2348012345678: 123456
//
// Then you use "123456" to verify the phone number
func (s *MockSMSService) SendOTP(ctx context.Context, phone, code string) error {
	// Log the OTP code so developer can see it
	log.Printf("[SMS] Sending OTP to %s: %s", phone, code)
	log.Printf("[SMS] Message: Your PropVest verification code is: %s. Valid for 10 minutes.", code)
	return nil
}

// SendNotification logs the notification instead of sending real SMS
func (s *MockSMSService) SendNotification(ctx context.Context, phone, message string) error {
	log.Printf("[SMS] Sending notification to %s: %s", phone, message)
	return nil
}

// TwilioSMSService is a production implementation using Twilio
// Twilio is a popular SMS provider with:
//   - Global coverage
//   - Reliable delivery
//   - Reasonable pricing
//   - Good API
//
// Setup:
//   1. Sign up at https://www.twilio.com/
//   2. Get Account SID and Auth Token
//   3. Buy a phone number or use free trial
//   4. Add credentials to .env
type TwilioSMSService struct {
	accountSID  string
	authToken   string
	phoneNumber string
}

// NewTwilioSMSService creates a new Twilio SMS service
// Requires Twilio credentials from config
func NewTwilioSMSService(cfg *config.Config) SMSService {
	return &TwilioSMSService{
		accountSID:  cfg.TwilioAccountSID,
		authToken:   cfg.TwilioAuthToken,
		phoneNumber: cfg.TwilioPhoneNumber,
	}
}

// SendOTP sends OTP via Twilio
// This would use Twilio SDK to actually send SMS
// For now, it's a placeholder that you'll implement when integrating Twilio
func (s *TwilioSMSService) SendOTP(ctx context.Context, phone, code string) error {
	// TODO: Implement Twilio API call
	// Example:
	//   client := twilio.NewRestClient()
	//   params := &api.CreateMessageParams{}
	//   params.SetTo(phone)
	//   params.SetFrom(s.phoneNumber)
	//   params.SetBody(fmt.Sprintf("Your PropVest code is: %s", code))
	//   _, err := client.Api.CreateMessage(params)
	//   return err

	log.Printf("[Twilio] Would send OTP to %s: %s", phone, code)
	return nil
}

// SendNotification sends notification via Twilio
func (s *TwilioSMSService) SendNotification(ctx context.Context, phone, message string) error {
	// TODO: Implement Twilio API call
	log.Printf("[Twilio] Would send notification to %s: %s", phone, message)
	return nil
}

// TermiiSMSService is a Nigerian SMS provider
// Termii specializes in African markets with:
//   - Better delivery rates in Nigeria
//   - Cheaper pricing for African numbers
//   - Support for Nigerian sender IDs
//
// Setup:
//   1. Sign up at https://termii.com/
//   2. Get API Key
//   3. Register sender ID (e.g., "PROPVEST")
//   4. Add credentials to .env
type TermiiSMSService struct {
	apiKey   string
	senderID string
}

// NewTermiiSMSService creates a new Termii SMS service
func NewTermiiSMSService(cfg *config.Config) SMSService {
	return &TermiiSMSService{
		apiKey:   cfg.TermiiAPIKey,
		senderID: cfg.TermiiSenderID,
	}
}

// SendOTP sends OTP via Termii
func (s *TermiiSMSService) SendOTP(ctx context.Context, phone, code string) error {
	// TODO: Implement Termii API call
	// Example:
	//   url := "https://api.ng.termii.com/api/sms/send"
	//   payload := map[string]interface{}{
	//       "to":      phone,
	//       "from":    s.senderID,
	//       "sms":     fmt.Sprintf("Your PropVest code is: %s", code),
	//       "type":    "plain",
	//       "channel": "generic",
	//       "api_key": s.apiKey,
	//   }
	//   // Send HTTP POST request
	//   return sendHTTPRequest(url, payload)

	log.Printf("[Termii] Would send OTP to %s: %s", phone, code)
	return nil
}

// SendNotification sends notification via Termii
func (s *TermiiSMSService) SendNotification(ctx context.Context, phone, message string) error {
	// TODO: Implement Termii API call
	log.Printf("[Termii] Would send notification to %s: %s", phone, message)
	return nil
}

// NewSMSService creates an SMS service based on configuration
// This is the factory function that main.go calls
//
// It reads SMS_PROVIDER from .env and returns the appropriate implementation:
//   - "mock": MockSMSService (development)
//   - "twilio": TwilioSMSService (production, global)
//   - "termii": TermiiSMSService (production, Nigeria)
//
// Example usage in main.go:
//   smsService := sms.NewSMSService(cfg)
//   userService := services.NewUserService(userRepo, otpRepo, smsService, cloudinary)
func NewSMSService(cfg *config.Config) SMSService {
	switch cfg.SMSProvider {
	case "twilio":
		return NewTwilioSMSService(cfg)
	case "termii":
		return NewTermiiSMSService(cfg)
	case "mock":
		fallthrough
	default:
		// Default to mock for development
		return NewMockSMSService()
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: SMS Integration Strategy
// ═══════════════════════════════════════════════════════════════════════════
//
// Development Phase:
//   - Use MockSMSService
//   - See OTP codes in console logs
//   - No cost, no setup needed
//   - Fast iteration
//
// Testing Phase:
//   - Use mock SMS in automated tests
//   - No flaky tests from SMS delays
//   - No cost for thousands of test runs
//
// Production Phase:
//   - Switch to TwilioSMSService or TermiiSMSService
//   - Just change SMS_PROVIDER in .env
//   - No code changes needed
//
// SMS Provider Comparison:
//
// Twilio:
//   ✅ Global coverage
//   ✅ Reliable delivery
//   ✅ Great documentation
//   ❌ Expensive for African numbers
//   ❌ Sender ID registration takes time
//
// Termii (Nigeria):
//   ✅ Cheaper for Nigerian numbers
//   ✅ Better delivery in Nigeria
//   ✅ Fast sender ID approval
//   ❌ Limited to African countries
//   ❌ Smaller company (reliability risk)
//
// Africa's Talking:
//   ✅ Covers 15+ African countries
//   ✅ Good pricing
//   ✅ Strong in Kenya/Uganda
//   ❌ API is more complex
//
// Recommendation:
//   - Development: mock
//   - MVP/Nigeria: Termii
//   - Scale/Global: Twilio
//   - Africa-wide: Africa's Talking
//
// Cost Optimization:
//   - Use SMS only for critical verifications
//   - Email as primary notification channel
//   - Rate limit OTP requests (max 5/hour)
//   - Block disposable phone numbers
//   - Cache verification results
//
// Security:
//   - Never log real OTP codes in production
//   - Use HTTPS for webhook callbacks
//   - Validate phone numbers before sending
//   - Monitor for unusual patterns (spam detection)
//
// ═══════════════════════════════════════════════════════════════════════════
