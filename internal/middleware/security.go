package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds security-related HTTP headers to all responses.
//
// These headers protect against common web vulnerabilities:
//
// 1. X-Frame-Options: DENY
//    Prevents clickjacking attacks by ensuring the site cannot be embedded in
//    an iframe on another domain.
//
// 2. X-Content-Type-Options: nosniff
//    Prevents MIME-sniffing attacks. Forces browsers to respect the declared
//    Content-Type rather than trying to guess it.
//
// 3. X-XSS-Protection: 1; mode=block
//    Enables browser XSS filtering. If a cross-site scripting attack is
//    detected, the browser will block the page entirely.
//
// 4. Strict-Transport-Security (HSTS)
//    Forces browsers to always use HTTPS for future requests to this domain.
//    Only applied in production when using HTTPS.
//    - max-age=31536000: Enforce HTTPS for 1 year
//    - includeSubDomains: Apply to all subdomains
//
// 5. Content-Security-Policy (CSP)
//    Restricts which resources can be loaded. This basic policy:
//    - default-src 'self': Only load resources from same origin
//    - For API servers, this is sufficient
//    - Frontend apps need more complex CSP policies
//
// 6. Referrer-Policy: strict-origin-when-cross-origin
//    Controls how much referrer information is sent with requests.
//    Balances privacy with functionality.
//
// 7. Permissions-Policy
//    Disables browser features that an API doesn't need (camera, microphone, etc.)
//
// PRODUCTION CONSIDERATIONS:
//   - HSTS is only applied when APP_ENV=production and using HTTPS
//   - Consider using a reverse proxy (Nginx/Caddy) to handle some headers
//   - Adjust CSP based on your specific needs
//
// REFERENCES:
//   - OWASP Secure Headers Project: https://owasp.org/www-project-secure-headers/
//   - Mozilla Observatory: https://observatory.mozilla.org/
func SecurityHeaders(isProduction bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ───────────────────────────────────────────────────────────────────
		// ALWAYS APPLY (Development + Production)
		// ───────────────────────────────────────────────────────────────────

		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Prevent MIME-sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Enable XSS protection (legacy browsers)
		c.Header("X-XSS-Protection", "1; mode=block")

		// Control referrer information
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable unnecessary browser features
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

		// Basic CSP for API server
		// Note: Frontend apps need more specific policies
		c.Header("Content-Security-Policy", "default-src 'self'")

		// ───────────────────────────────────────────────────────────────────
		// PRODUCTION ONLY (HTTPS required)
		// ───────────────────────────────────────────────────────────────────

		if isProduction {
			// Force HTTPS for 1 year (including subdomains)
			// CRITICAL: Only enable this when you have a valid SSL certificate
			//           and are running behind HTTPS. Enabling this prematurely
			//           can lock users out of your site!
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

