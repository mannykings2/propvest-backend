-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000022: Create Notifications System
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Create tables for Notification System (Milestone 6)
-- - notifications: In-app notification center messages (docs 5.5, 2.2 §16)
-- - notification_preferences: User channel preferences per notification type
-- ═══════════════════════════════════════════════════════════════════════════

-- ═══════════════════════════════════════════════════════════════════════════
-- Table: notifications
-- ═══════════════════════════════════════════════════════════════════════════
-- Stores in-app notifications shown in user's notification center
-- Email/SMS delivery is handled asynchronously by worker via RabbitMQ
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Type is a machine label for frontend icon switching
    -- e.g., "deposit_success", "investment_created", "property_approved"
    type VARCHAR(100) NOT NULL,
    
    -- Display content
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    
    -- Read tracking
    read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMP WITH TIME ZONE,
    
    -- Metadata carries structured context (ids, amounts) for deep-linking
    metadata JSONB,
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE notifications IS 'In-app notification center messages (Milestone 6)';
COMMENT ON COLUMN notifications.type IS 'Machine label for notification type (e.g., deposit_success)';
COMMENT ON COLUMN notifications.title IS 'Notification title shown in notification center';
COMMENT ON COLUMN notifications.body IS 'Notification body/message content';
COMMENT ON COLUMN notifications.read IS 'Unread/read flag for unread count badge';
COMMENT ON COLUMN notifications.read_at IS 'Timestamp when notification was marked as read';
COMMENT ON COLUMN notifications.metadata IS 'Structured JSON context for deep-linking (property_id, investment_id, etc.)';

-- ═══════════════════════════════════════════════════════════════════════════
-- Table: notification_preferences
-- ═══════════════════════════════════════════════════════════════════════════
-- User preferences for notification delivery channels per notification type
-- Defaults: in_app=true, email=true, sms=false (unless set)
CREATE TABLE notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Notification type this preference applies to
    -- NULL means "global default for unspecified types"
    notification_type VARCHAR(100),
    
    -- Channel preferences
    in_app_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    email_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sms_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE notification_preferences IS 'User notification delivery channel preferences per type';
COMMENT ON COLUMN notification_preferences.notification_type IS 'Notification type (e.g., deposit_success). NULL = global default';
COMMENT ON COLUMN notification_preferences.in_app_enabled IS 'Enable in-app notification center notifications';
COMMENT ON COLUMN notification_preferences.email_enabled IS 'Enable email notifications';
COMMENT ON COLUMN notification_preferences.sms_enabled IS 'Enable SMS notifications';

-- ═══════════════════════════════════════════════════════════════════════════
-- Constraints
-- ═══════════════════════════════════════════════════════════════════════════

-- Each user can have only one preference row per notification type
-- (includes NULL type for global default)
CREATE UNIQUE INDEX idx_notification_prefs_user_type 
ON notification_preferences(user_id, COALESCE(notification_type, ''));

-- Type must be non-empty string if not NULL
ALTER TABLE notifications
ADD CONSTRAINT chk_notifications_type_not_empty 
CHECK (type <> '');

ALTER TABLE notification_preferences
ADD CONSTRAINT chk_notification_prefs_type_not_empty 
CHECK (notification_type IS NULL OR notification_type <> '');

-- Title and body must not be empty
ALTER TABLE notifications
ADD CONSTRAINT chk_notifications_title_not_empty 
CHECK (title <> '');

ALTER TABLE notifications
ADD CONSTRAINT chk_notifications_body_not_empty 
CHECK (body <> '');

-- read_at must be set when read=true (enforced at application layer, not DB constraint)

-- ═══════════════════════════════════════════════════════════════════════════
-- Indexes for Performance
-- ═══════════════════════════════════════════════════════════════════════════

-- Index for fetching user's notifications (most common query)
CREATE INDEX idx_notifications_user_created 
ON notifications(user_id, created_at DESC);

-- Index for unread count badge query
CREATE INDEX idx_notifications_user_unread 
ON notifications(user_id, read) 
WHERE read = FALSE;

-- Index for notification type filtering
CREATE INDEX idx_notifications_type ON notifications(type);

-- Index for read_at queries (analytics)
CREATE INDEX idx_notifications_read_at 
ON notifications(read_at) 
WHERE read_at IS NOT NULL;

-- Index for fetching user preferences
CREATE INDEX idx_notification_prefs_user 
ON notification_preferences(user_id);

-- Index for querying preferences by type
CREATE INDEX idx_notification_prefs_type 
ON notification_preferences(notification_type) 
WHERE notification_type IS NOT NULL;

-- GIN index for metadata JSONB queries
CREATE INDEX idx_notifications_metadata 
ON notifications USING GIN (metadata);

-- ═══════════════════════════════════════════════════════════════════════════
-- Seed Data: Default Global Preferences for Existing Users
-- ═══════════════════════════════════════════════════════════════════════════
-- Create global default preferences (notification_type=NULL) for all existing users
-- in_app=true, email=true, sms=false (system defaults)
INSERT INTO notification_preferences (user_id, notification_type, in_app_enabled, email_enabled, sms_enabled)
SELECT id, NULL, TRUE, TRUE, FALSE
FROM users
WHERE NOT EXISTS (
    SELECT 1 FROM notification_preferences WHERE notification_preferences.user_id = users.id AND notification_type IS NULL
);

-- ═══════════════════════════════════════════════════════════════════════════
-- Trigger: Update notification_preferences.updated_at on row modification
-- ═══════════════════════════════════════════════════════════════════════════
CREATE OR REPLACE FUNCTION update_notification_preferences_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_notification_preferences_updated_at
BEFORE UPDATE ON notification_preferences
FOR EACH ROW
EXECUTE FUNCTION update_notification_preferences_updated_at();

-- ═══════════════════════════════════════════════════════════════════════════
-- Verification
-- ═══════════════════════════════════════════════════════════════════════════
DO $$
DECLARE
    users_count INTEGER;
    prefs_count INTEGER;
BEGIN
    -- Verify all existing users have global default preferences
    SELECT COUNT(*) INTO users_count FROM users;
    SELECT COUNT(*) INTO prefs_count 
    FROM notification_preferences 
    WHERE notification_type IS NULL;
    
    IF users_count > 0 AND prefs_count <> users_count THEN
        RAISE WARNING 'Expected % global preferences, found %. Some users may be missing defaults.', users_count, prefs_count;
    END IF;
    
    RAISE NOTICE 'Migration 000022 complete: % notifications table, % notification_preferences table created, % users have default preferences', 
        (SELECT COUNT(*) FROM notifications), 
        (SELECT COUNT(*) FROM notification_preferences),
        prefs_count;
END $$;
