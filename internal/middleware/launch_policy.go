package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mannykings2/propvest-backend/internal/config"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/mannykings2/propvest-backend/internal/response"
)

// RequireVerifiedAccount enforces the launch security policy flags on
// money-moving routes (buy a slot, off-plan subscribe, withdraw).
//
// WHY MIDDLEWARE: the two policy values — email_verified and kyc_status — are
// already loaded by Auth into the request context as the AccountState, so this
// check costs zero extra queries. Putting it in the service layer would have
// meant a second user lookup per purchase.
//
// WHY ROUTE-LEVEL, NOT GLOBAL: an unverified user must still be able to open
// the app, read the verification email, upload KYC documents and browse
// listings. The gate sits only where funds leave the platform or an investment
// is created. That matches the flags' names (REQUIRE_KYC_FOR_INVESTMENT) and
// keeps the verification flows reachable.
//
// FLAGS (config):
//   - REQUIRE_EMAIL_VERIFICATION  → 403 code=email_not_verified
//   - REQUIRE_KYC_FOR_INVESTMENT  → 403 code=kyc_required
//
// Plus one UNCONDITIONAL rule (Phase 9.3): a user with a live erasure request
// cannot move funds or invest — the 30-day window exists so they can CANCEL,
// not so they can empty the account under the wire.
//
// Both flags default to false in development; set them to true in production .env.
func RequireVerifiedAccount(cfg *config.Config, userRepo repositories.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Auth stores the account row it just read; if it isn't there the route
		// chain forgot Auth (programming error) — refuse rather than pass open.
		raw, ok := c.Get("account_state")
		if !ok {
			response.Error(c, http.StatusUnauthorized, "Authorization required", "unauthorized")
			c.Abort()
			return
		}
		state, ok := raw.(*repositories.AccountState)
		if !ok || state == nil {
			response.Error(c, http.StatusUnauthorized, "Authorization required", "unauthorized")
			c.Abort()
			return
		}

		if cfg != nil && cfg.RequireEmailVerification && !state.EmailVerified {
			response.Error(c, http.StatusForbidden,
				"Verify your email address before moving funds", "email_not_verified")
			c.Abort()
			return
		}

		if cfg != nil && cfg.RequireKYCForInvestment && state.KYCStatus != "verified" {
			response.Error(c, http.StatusForbidden,
				"KYC verification required to invest", "kyc_required")
			c.Abort()
			return
		}

		// Phase 9.3: erasure pending → funds are frozen. One indexed lookup,
		// only on routes that move money.
		if userRepo != nil {
			pending, err := userRepo.HasPendingDeletion(c.Request.Context(), state.UserID)
			if err == nil && pending {
				response.Error(c, http.StatusForbidden,
					"Your account is scheduled for deletion — cancel the request to keep investing",
					"account_deletion_pending")
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
