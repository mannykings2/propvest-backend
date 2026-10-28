package services

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The link helpers share package-level state (SPA origin + token builder);
// save and restore it so these tests never leak into other tests.
func withLinksReset(t *testing.T) {
	t.Helper()
	linkMu.RLock()
	prevURL, prevFn := frontendURL, unsubscribeToken
	linkMu.RUnlock()
	t.Cleanup(func() {
		linkMu.Lock()
		frontendURL, unsubscribeToken = prevURL, prevFn
		linkMu.Unlock()
	})
}

func TestUnsubscribeLink(t *testing.T) {
	withLinksReset(t)

	userID := uuid.New()
	// No origin → no link (message degrades to the plain body).
	SetUnsubscribeTokenFn(func(id uuid.UUID) string { return "tok-" + id.String()[:8] })
	if got := UnsubscribeLink(userID); got != "" {
		t.Fatalf("expected empty link without FRONTEND_URL, got %q", got)
	}

	// Origin set (trailing slash is normalised away).
	SetFrontendURL("https://app.example.com/")
	got := UnsubscribeLink(userID)
	want := "https://app.example.com/unsubscribe?token=tok-" + userID.String()[:8]
	if got != want {
		t.Fatalf("UnsubscribeLink = %q, want %q", got, want)
	}

	// No token builder (privacy service never wired) → no link.
	SetUnsubscribeTokenFn(nil)
	if got := UnsubscribeLink(userID); got != "" {
		t.Fatalf("expected empty link without a token builder, got %q", got)
	}
}

func TestExportDownloadLink(t *testing.T) {
	withLinksReset(t)
	SetFrontendURL("https://app.example.com")

	got := ExportDownloadLink("/me/exports/download/abc123")
	if got != "https://app.example.com/download/abc123" {
		t.Fatalf("ExportDownloadLink = %q", got)
	}
	// Degenerate inputs never produce a broken half-link.
	if got := ExportDownloadLink(""); got != "" {
		t.Fatalf("empty path must yield empty link, got %q", got)
	}
	if got := ExportDownloadLink("/something-else"); got != "" {
		t.Fatalf("non-download path must yield empty link, got %q", got)
	}
}

func TestEmailBodyWithLinks(t *testing.T) {
	withLinksReset(t)
	SetFrontendURL("https://app.example.com")

	// With a download path: clickable link + expiry note in both parts.
	html, text := emailBodyWithLinks("Your export is ready.", map[string]any{
		"download_path": "/me/exports/download/xyz",
	})
	for _, part := range []string{html, text} {
		if !strings.Contains(part, "https://app.example.com/download/xyz") {
			t.Fatalf("message must contain the absolute download link:\n%s", part)
		}
	}

	// Without metadata: plain body, no dangling markup.
	html, text = emailBodyWithLinks("Plain notification.", nil)
	if html != "<p>Plain notification.</p>" || text != "Plain notification." {
		t.Fatalf("plain body changed: %q / %q", html, text)
	}
}

func TestEmailOriginFallsBack(t *testing.T) {
	withLinksReset(t)
	if got := emailOrigin("http://localhost:8081"); got != "http://localhost:8081" {
		t.Fatalf("fallback origin expected, got %q", got)
	}
	SetFrontendURL("https://app.example.com")
	if got := emailOrigin("http://localhost:8081"); got != "https://app.example.com" {
		t.Fatalf("FRONTEND_URL must win, got %q", got)
	}
}