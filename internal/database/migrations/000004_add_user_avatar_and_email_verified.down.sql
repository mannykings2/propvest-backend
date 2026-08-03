-- Rollback migration: Remove avatar_url and email_verified from users table
-- This drops the columns and their data
-- Use this if you need to undo the user profile enhancements

-- ═══════════════════════════════════════════════════════════════════════════
-- DROP INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop the email_verified index first
DROP INDEX IF EXISTS idx_users_email_verified;

-- ═══════════════════════════════════════════════════════════════════════════
-- DROP COLUMNS
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop email_verified column
-- This removes the verification tracking feature
ALTER TABLE users 
DROP COLUMN IF EXISTS email_verified;

-- Drop avatar_url column
-- This removes all user profile pictures
-- Users would need to re-upload if you add the column back
ALTER TABLE users 
DROP COLUMN IF EXISTS avatar_url;

-- ═══════════════════════════════════════════════════════════════════════════
-- NOTES
-- ═══════════════════════════════════════════════════════════════════════════
--
-- What happens when you run this migration?
--   1. All avatar URLs are deleted (Cloudinary images remain but links are lost)
--   2. Email verification status is lost
--   3. Users will appear unverified even if they were verified
--   4. Frontend will show default avatars for all users
--
-- When to use this rollback?
--   - Testing/development: resetting database state
--   - Removing profile features entirely
--   - Replacing with different image storage solution
--
-- ⚠️ WARNING: This is a destructive operation
-- In production:
--   - Back up avatar_url data first
--   - Consider keeping email_verified for audit purposes
--   - Notify users about avatar removal
--
-- ═══════════════════════════════════════════════════════════════════════════
