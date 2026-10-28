package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// The redaction rules must be conservative: sensitive keys are wiped, ordinary
// keys keep their values. A false negative here leaks credentials to the log
// store; a false positive makes logs useless during an incident.
func TestReplaceAttr_RedactsSensitiveKeys(t *testing.T) {
	sensitive := []string{
		"password", "Password", "PASSWORD",
		"refresh_token", "access_token", "token_hash",
		"PAYSTACK_SECRET_KEY", "jwt_secret",
		"Authorization", "Set-Cookie", "api_key", "apiKey",
		"otp", "OTP", "bvn", "nin", "cvv",
	}
	for _, key := range sensitive {
		got := replaceAttr(nil, slog.String(key, "super-secret-value"))
		if got.Value.String() != redactedPlaceholder {
			t.Errorf("key %q: want %q, got %q", key, redactedPlaceholder, got.Value.String())
		}
	}
}

func TestReplaceAttr_PassesOrdinaryKeysThrough(t *testing.T) {
	ordinary := []string{
		"user_id", "email", "request_id", "amount_kobo", "status",
		"error", "method", "path", "duration_ms",
		// near-misses that segment matching must NOT catch
		"beginning", "assigned_tokenless_count", "shipping", "opaque",
	}
	for _, key := range ordinary {
		got := replaceAttr(nil, slog.String(key, "value-123"))
		if got.Value.String() != "value-123" {
			t.Errorf("key %q was redacted but should pass through", key)
		}
	}
}

// Wire-level check: the exact opts Init builds must produce JSON where the
// secret is gone and the key (plus the ordinary value) survives.
func TestReplaceAttr_WiredHandlerProducesSafeJSON(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: replaceAttr,
	})
	l := slog.New(h)

	l.Info("login attempt",
		"user_id", "u-1",
		"password", "hunter2",
		"refresh_token", "eyJhbGciOi...",
		"note", "ok")

	out := buf.String()
	for _, leak := range []string{"hunter2", "eyJhbGciOi"} {
		if strings.Contains(out, leak) {
			t.Errorf("secret %q leaked into log output: %s", leak, out)
		}
	}
	if !strings.Contains(out, redactedPlaceholder) {
		t.Errorf("expected %q in output: %s", redactedPlaceholder, out)
	}
	if !strings.Contains(out, `"u-1"`) || !strings.Contains(out, `"ok"`) {
		t.Errorf("ordinary fields must survive redaction: %s", out)
	}

	// Key names stay so log queries/alerts still work.
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output must remain valid JSON: %v", err)
	}
	if _, ok := rec["password"]; !ok {
		t.Error("sensitive KEY should remain present (only the value is redacted)")
	}
}
