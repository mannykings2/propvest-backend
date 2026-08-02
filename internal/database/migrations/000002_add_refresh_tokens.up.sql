-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Add Refresh Tokens Table
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000002_add_refresh_tokens
-- Description: Creates refresh_tokens table for JWT refresh token management
-- Author: Principal Software Architect
-- Date: 2026-07-29
-- ═══════════════════════════════════════════════════════════════════════════

-- ───────────────────────────────────────────────────────────────────────────
-- REFRESH_TOKENS TABLE
-- ───────────────────────────────────────────────────────────────────────────
-- Stores refresh tokens for session management
-- Refresh tokens allow users to get new access tokens without re-login
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Indexes for performance
    CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for refresh_tokens table
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_revoked_at ON refresh_tokens(revoked_at);

-- ═══════════════════════════════════════════════════════════════════════════
-- COMMENTS
-- ═══════════════════════════════════════════════════════════════════════════

COMMENT ON TABLE refresh_tokens IS 'Stores refresh tokens for JWT session management';
COMMENT ON COLUMN refresh_tokens.token_hash IS 'Hashed refresh token (never store plain tokens)';
COMMENT ON COLUMN refresh_tokens.expires_at IS 'Token expiration timestamp (typically 7-30 days)';
COMMENT ON COLUMN refresh_tokens.revoked_at IS 'NULL if active, timestamp if manually revoked';

-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
