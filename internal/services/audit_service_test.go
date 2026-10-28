package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestRedactSensitiveFields tests sensitive field redaction.
func TestRedactSensitiveFields(t *testing.T) {
	t.Run("redacts password fields", func(t *testing.T) {
		input := map[string]interface{}{
			"email":         "user@example.com",
			"password":      "secret123",
			"password_hash": "bcrypt$2a$10$...",
			"name":          "John Doe",
		}

		redacted := RedactSensitiveFields(input)

		assert.Contains(t, redacted, "email")
		assert.Contains(t, redacted, "name")
		assert.NotContains(t, redacted, "password")
		assert.NotContains(t, redacted, "password_hash")
	})

	t.Run("redacts token fields", func(t *testing.T) {
		input := map[string]interface{}{
			"user_id":       "123",
			"access_token":  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
			"refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
			"jwt":           "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
		}

		redacted := RedactSensitiveFields(input)

		assert.Contains(t, redacted, "user_id")
		assert.NotContains(t, redacted, "access_token")
		assert.NotContains(t, redacted, "refresh_token")
		assert.NotContains(t, redacted, "jwt")
	})

	t.Run("redacts secret fields", func(t *testing.T) {
		input := map[string]interface{}{
			"api_key":         "sk_live_123456",
			"private_key":     "-----BEGIN PRIVATE KEY-----",
			"paystack_secret": "sk_test_...",
			"config":          "value",
		}

		redacted := RedactSensitiveFields(input)

		assert.Contains(t, redacted, "config")
		assert.NotContains(t, redacted, "api_key")
		assert.NotContains(t, redacted, "private_key")
		assert.NotContains(t, redacted, "paystack_secret")
	})

	t.Run("redacts OTP and verification fields", func(t *testing.T) {
		input := map[string]interface{}{
			"email":             "user@example.com",
			"otp":               "123456",
			"otp_code":          "654321",
			"verification_code": "ABC123",
			"reset_token":       "token123",
		}

		redacted := RedactSensitiveFields(input)

		assert.Contains(t, redacted, "email")
		assert.NotContains(t, redacted, "otp")
		assert.NotContains(t, redacted, "otp_code")
		assert.NotContains(t, redacted, "verification_code")
		assert.NotContains(t, redacted, "reset_token")
	})

	t.Run("redacts payment fields", func(t *testing.T) {
		input := map[string]interface{}{
			"amount":      10000,
			"cvv":         "123",
			"card_number": "4111111111111111",
			"pin":         "1234",
		}

		redacted := RedactSensitiveFields(input)

		assert.Contains(t, redacted, "amount")
		assert.NotContains(t, redacted, "cvv")
		assert.NotContains(t, redacted, "card_number")
		assert.NotContains(t, redacted, "pin")
	})

	t.Run("handles case-insensitive matching", func(t *testing.T) {
		input := map[string]interface{}{
			"email":        "user@example.com",
			"Password":     "secret",
			"PASSWORD_HASH": "bcrypt...",
			"old_password": "old_secret",
		}

		redacted := RedactSensitiveFields(input)

		assert.Contains(t, redacted, "email")
		assert.NotContains(t, redacted, "Password")
		assert.NotContains(t, redacted, "PASSWORD_HASH")
		assert.NotContains(t, redacted, "old_password")
	})

	t.Run("handles substring matching", func(t *testing.T) {
		input := map[string]interface{}{
			"user_password":   "secret",
			"new_password":    "newsecret",
			"confirm_password": "newsecret",
			"username":        "john",
		}

		redacted := RedactSensitiveFields(input)

		assert.Contains(t, redacted, "username")
		assert.NotContains(t, redacted, "user_password")
		assert.NotContains(t, redacted, "new_password")
		assert.NotContains(t, redacted, "confirm_password")
	})

	t.Run("returns nil for nil input", func(t *testing.T) {
		redacted := RedactSensitiveFields(nil)
		assert.Nil(t, redacted)
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		input := map[string]interface{}{}
		redacted := RedactSensitiveFields(input)
		assert.Nil(t, redacted)
	})

	t.Run("preserves safe fields", func(t *testing.T) {
		input := map[string]interface{}{
			"id":         "123",
			"email":      "user@example.com",
			"name":       "John Doe",
			"role":       "admin",
			"status":     "active",
			"created_at": "2026-10-06",
		}

		redacted := RedactSensitiveFields(input)

		assert.Len(t, redacted, 6)
		assert.Equal(t, "123", redacted["id"])
		assert.Equal(t, "user@example.com", redacted["email"])
		assert.Equal(t, "John Doe", redacted["name"])
		assert.Equal(t, "admin", redacted["role"])
		assert.Equal(t, "active", redacted["status"])
	})
}

// TestAllowlistedFields tests allowlist-based field filtering.
func TestAllowlistedFields(t *testing.T) {
	t.Run("includes only allowlisted fields", func(t *testing.T) {
		input := map[string]interface{}{
			"id":            "123",
			"email":         "user@example.com",
			"password_hash": "bcrypt...",
			"internal_flag": true,
			"status":        "active",
		}

		allowlist := []string{"id", "email", "status"}
		allowed := AllowlistedFields(input, allowlist)

		assert.Len(t, allowed, 3)
		assert.Contains(t, allowed, "id")
		assert.Contains(t, allowed, "email")
		assert.Contains(t, allowed, "status")
		assert.NotContains(t, allowed, "password_hash")
		assert.NotContains(t, allowed, "internal_flag")
	})

	t.Run("handles missing fields gracefully", func(t *testing.T) {
		input := map[string]interface{}{
			"id":    "123",
			"email": "user@example.com",
		}

		allowlist := []string{"id", "email", "name", "phone"}
		allowed := AllowlistedFields(input, allowlist)

		assert.Len(t, allowed, 2)
		assert.Contains(t, allowed, "id")
		assert.Contains(t, allowed, "email")
		assert.NotContains(t, allowed, "name")
		assert.NotContains(t, allowed, "phone")
	})

	t.Run("returns nil for nil input", func(t *testing.T) {
		allowlist := []string{"id", "email"}
		allowed := AllowlistedFields(nil, allowlist)
		assert.Nil(t, allowed)
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		input := map[string]interface{}{}
		allowlist := []string{"id", "email"}
		allowed := AllowlistedFields(input, allowlist)
		assert.Nil(t, allowed)
	})

	t.Run("returns empty for empty allowlist", func(t *testing.T) {
		input := map[string]interface{}{
			"id":    "123",
			"email": "user@example.com",
		}

		allowlist := []string{}
		allowed := AllowlistedFields(input, allowlist)

		assert.NotNil(t, allowed)
		assert.Len(t, allowed, 0)
	})
}

// TestIsSensitiveField tests sensitive field pattern matching.
func TestIsSensitiveField(t *testing.T) {
	sensitiveFields := []string{
		"password",
		"password_hash",
		"passwordHash",
		"old_password",
		"new_password",
		"access_token",
		"refresh_token",
		"jwt",
		"api_key",
		"apikey",
		"secret",
		"otp",
		"otp_code",
		"verification_code",
		"reset_token",
		"cvv",
		"card_number",
		"pin",
	}

	for _, field := range sensitiveFields {
		t.Run("identifies "+field+" as sensitive", func(t *testing.T) {
			assert.True(t, isSensitiveField(field), field+" should be sensitive")
		})
	}

	safeFields := []string{
		"id",
		"email",
		"name",
		"role",
		"status",
		"created_at",
		"updated_at",
		"phone",
		"address",
	}

	for _, field := range safeFields {
		t.Run("identifies "+field+" as safe", func(t *testing.T) {
			assert.False(t, isSensitiveField(field), field+" should be safe")
		})
	}
}

// TestNewAdminAuditEvent tests admin audit event helper.
func TestNewAdminAuditEvent(t *testing.T) {
	actorID := MustParseUUID("550e8400-e29b-41d4-a716-446655440000")
	targetID := MustParseUUID("660e8400-e29b-41d4-a716-446655440000")

	event := NewAdminAuditEvent(
		actorID,
		"admin",
		"user.suspended",
		"user",
		&targetID,
		"Policy violation",
	)

	assert.Equal(t, actorID, *event.ActorID)
	assert.Equal(t, "admin", event.ActorType)
	assert.Equal(t, "admin", event.ActorRole)
	assert.Equal(t, "user.suspended", event.Action)
	assert.Equal(t, "user", event.TargetType)
	assert.Equal(t, targetID, *event.TargetID)
	assert.Equal(t, "Policy violation", event.Reason)
}

// TestNewSystemAuditEvent tests system audit event helper.
func TestNewSystemAuditEvent(t *testing.T) {
	targetID := MustParseUUID("770e8400-e29b-41d4-a716-446655440000")

	event := NewSystemAuditEvent(
		"investment.matured",
		"investment",
		&targetID,
	)

	assert.Nil(t, event.ActorID)
	assert.Equal(t, "system", event.ActorType)
	assert.Equal(t, "investment.matured", event.Action)
	assert.Equal(t, "investment", event.TargetType)
	assert.Equal(t, targetID, *event.TargetID)
}

// Helper function for tests
func MustParseUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}
