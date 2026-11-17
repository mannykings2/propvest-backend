-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000036: Login history + last_login_at
--
-- Why: admins saw "last login: never" (the DTO field existed but was always
-- nil), and users had no way to notice a compromised account. Every
-- successful login now stamps users.last_login_at AND appends an
-- ip/user-agent/request_id row, so the admin user list answers "when did they
-- last sign in" and the user's own Profile can show "recent sign-ins".
--
-- Retention: rows are small; keep 90 days (privacy schedule). The worker's
-- retention sweeper can prune; until then unbounded rows are acceptable at
-- this scale and the table is trivially truncated in an emergency.
-- ═══════════════════════════════════════════════════════════════════════════

ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS login_history (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip_address VARCHAR(64),
    user_agent TEXT,
    request_id VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- "My recent sign-ins" reads newest-first for one user.
CREATE INDEX IF NOT EXISTS idx_login_history_user_created
    ON login_history (user_id, created_at DESC);
