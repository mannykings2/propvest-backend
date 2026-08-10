-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000008: verification_tokens (email verification + password reset)
-- ═══════════════════════════════════════════════════════════════════════════
-- One table serves both flows via the `purpose` column. Only the SHA-256 hash of
-- the token is stored (never the plaintext token emailed to the user).
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS verification_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    purpose VARCHAR(32) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_verification_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chk_verification_tokens_purpose CHECK (purpose IN ('email_verification', 'password_reset'))
);

CREATE INDEX IF NOT EXISTS idx_verification_tokens_user_id ON verification_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_verification_tokens_purpose ON verification_tokens(purpose);
CREATE INDEX IF NOT EXISTS idx_verification_tokens_expires_at ON verification_tokens(expires_at);

COMMENT ON TABLE verification_tokens IS 'Single-use, expiring tokens for email verification and password reset';
COMMENT ON COLUMN verification_tokens.token_hash IS 'SHA-256 hex of the plaintext token (never store plaintext)';
COMMENT ON COLUMN verification_tokens.purpose IS 'email_verification | password_reset';
COMMENT ON COLUMN verification_tokens.used_at IS 'NULL until the token is consumed (single-use)';
