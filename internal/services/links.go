package services

import (
	"strings"
	"sync"

	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════════════════
// EMAIL LINK ORIGIN (Phase B2 — NDPA links inside messages)
// ═══════════════════════════════════════════════════════════════════════════
//
// Emails used to be plain "<p>body</p>" — the data-export download path and
// the marketing unsubscribe URL existed only as API paths, so neither was
// clickable. Every message that needs a link now builds an absolute URL from
// the SPA origin (FRONTEND_URL). Both the API and the worker call the Set
// functions once at boot; a zero value disables link rendering (tests).

var (
	linkMu           sync.RWMutex
	frontendURL      string
	unsubscribeToken func(uuid.UUID) string
)

// SetFrontendURL records the SPA origin (no trailing slash) used to build
// absolute links inside emails.
func SetFrontendURL(u string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	frontendURL = strings.TrimRight(strings.TrimSpace(u), "/")
}

// FrontendURL returns the configured SPA origin, or "" when unset.
func FrontendURL() string {
	linkMu.RLock()
	defer linkMu.RUnlock()
	return frontendURL
}

// SetUnsubscribeTokenFn injects the HMAC token builder (privacy service) so
// the bulk sender can embed a per-user one-click unsubscribe link without
// owning the key material itself.
func SetUnsubscribeTokenFn(fn func(uuid.UUID) string) {
	linkMu.Lock()
	defer linkMu.Unlock()
	unsubscribeToken = fn
}

// UnsubscribeLink returns the absolute SPA landing URL for a user's
// unsubscribe token, or "" when either the origin or the builder is missing.
func UnsubscribeLink(userID uuid.UUID) string {
	linkMu.RLock()
	base, fn := frontendURL, unsubscribeToken
	linkMu.RUnlock()
	if base == "" || fn == nil {
		return ""
	}
	return base + "/unsubscribe?token=" + fn(userID)
}

// ExportDownloadLink converts the API-relative download path
// ("/me/exports/download/<token>") returned at export time into the SPA
// landing URL, or "" when no SPA origin is configured.
func ExportDownloadLink(downloadPath string) string {
	linkMu.RLock()
	base := frontendURL
	linkMu.RUnlock()
	if base == "" || downloadPath == "" {
		return ""
	}
	const marker = "/download/"
	i := strings.LastIndex(downloadPath, marker)
	if i < 0 {
		return ""
	}
	return base + "/download/" + downloadPath[i+len(marker):]
}
