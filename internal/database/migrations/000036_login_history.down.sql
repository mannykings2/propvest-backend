DROP INDEX IF EXISTS idx_login_history_user_created;
DROP TABLE IF EXISTS login_history;
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
