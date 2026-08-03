-- Migration: Add avatar_url and email_verified to users table
-- Purpose: Support user profile pictures and email verification tracking

-- ═══════════════════════════════════════════════════════════════════════════
-- ADD COLUMNS TO USERS TABLE
-- ═══════════════════════════════════════════════════════════════════════════

-- Add avatar_url column for storing Cloudinary image URLs
-- Nullable because not all users will upload avatars
-- Example value: "https://res.cloudinary.com/propvest/image/upload/v1234567890/avatars/user-abc123.jpg"
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS avatar_url TEXT;

-- Add email_verified column to track email verification status
-- Default false means: new users have unverified emails
-- Set to true when user clicks verification link sent via email
-- NOT NULL with default ensures consistency
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS email_verified BOOLEAN DEFAULT false NOT NULL;

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

-- Index on email_verified for queries like:
--   "Show me all unverified users" (admin dashboard)
--   "Find users who haven't verified email in 7 days" (reminder emails)
CREATE INDEX IF NOT EXISTS idx_users_email_verified 
ON users(email_verified);

-- ═══════════════════════════════════════════════════════════════════════════
-- NOTES
-- ═══════════════════════════════════════════════════════════════════════════
--
-- avatar_url:
--   - Stores Cloudinary URLs (not local file paths)
--   - Frontend displays default avatar if NULL
--   - Cloudinary handles image optimization and CDN delivery
--   - Format: https://res.cloudinary.com/{cloud_name}/image/upload/{transformation}/{public_id}
--
-- email_verified:
--   - Set to true after user clicks verification link
--   - Can be enforced before sensitive actions (future)
--   - Admin can manually verify users if needed
--   - Default false for backward compatibility (existing users not verified)
--
-- ═══════════════════════════════════════════════════════════════════════════
