-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000022: Rollback - Create Notifications System
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Rollback notification system tables from migration 000022
-- WARNING: This will drop notifications and notification_preferences tables.
--          All notification data and user preferences will be lost.
-- ═══════════════════════════════════════════════════════════════════════════

-- Drop trigger and function
DROP TRIGGER IF EXISTS trigger_update_notification_preferences_updated_at ON notification_preferences;
DROP FUNCTION IF EXISTS update_notification_preferences_updated_at();

-- Drop indexes (in reverse order of creation)
DROP INDEX IF EXISTS idx_notifications_metadata;
DROP INDEX IF EXISTS idx_notification_prefs_type;
DROP INDEX IF EXISTS idx_notification_prefs_user;
DROP INDEX IF EXISTS idx_notifications_read_at;
DROP INDEX IF EXISTS idx_notifications_type;
DROP INDEX IF EXISTS idx_notifications_user_unread;
DROP INDEX IF EXISTS idx_notifications_user_created;
DROP INDEX IF EXISTS idx_notification_prefs_user_type;

-- Drop tables (notification_preferences first due to no foreign keys, then notifications)
DROP TABLE IF EXISTS notification_preferences CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
