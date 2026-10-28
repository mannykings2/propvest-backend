package services

// emailOrigin returns the origin used for links inside auth emails
// (password reset, email verification). It prefers the SPA origin
// (FRONTEND_URL) — those routes live in the SPA, not the API — and falls
// back to the provided BASE_URL for deployments where both share one
// origin (and for unit tests that never wire SetFrontendURL).
func emailOrigin(fallback string) string {
	if u := FrontendURL(); u != "" {
		return u
	}
	return fallback
}
