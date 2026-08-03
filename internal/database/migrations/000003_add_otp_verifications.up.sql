-- Migration: Add OTP verification table for phone number changes
-- Purpose: Store one-time passwords sent via SMS for phone verification
-- Security: OTP codes are hashed before storage (like passwords)

-- ═══════════════════════════════════════════════════════════════════════════
-- TABLE: otp_verifications
-- ═══════════════════════════════════════════════════════════════════════════
-- Stores OTP codes sent to users for phone number verification
-- Each OTP is:
--   - Hashed before storage (SHA-256)
--   - Valid for 10 minutes
--   - Single-use (marked as used after verification)
--   - Tied to specific user and phone number

CREATE TABLE IF NOT EXISTS otp_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Foreign key to users table
    -- Which user requested this OTP?
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Phone number this OTP was sent to
    -- We store this separately because user might be changing their phone
    -- Format: E.164 (+2348012345678)
    phone VARCHAR(20) NOT NULL,
    
    -- Hashed OTP code (SHA-256)
    -- We hash OTP codes like passwords for security
    -- If database is breached, attacker can't use the OTP codes
    -- Original OTP: "123456"
    -- Stored hash: "8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92"
    code_hash VARCHAR(64) NOT NULL,
    
    -- OTP expiration timestamp
    -- Typically 10 minutes from creation
    -- After this time, OTP is rejected even if correct
    expires_at TIMESTAMP NOT NULL,
    
    -- Whether this OTP has been used
    -- Once verified, OTP cannot be reused (prevents replay attacks)
    -- NULL = not used yet
    -- Timestamp = when it was used
    verified_at TIMESTAMP,
    
    -- Number of failed verification attempts
    -- After 3 failed attempts, OTP is blocked (prevents brute-force)
    -- Resets to 0 when new OTP is generated
    attempt_count INTEGER DEFAULT 0 NOT NULL,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW() NOT NULL
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

-- Index on user_id for fast lookup during verification
-- Query: "Show me all OTP codes for this user"
-- Used when: User requests verification, checking rate limits
CREATE INDEX IF NOT EXISTS idx_otp_verifications_user_id 
ON otp_verifications(user_id);

-- Index on phone for fast lookup
-- Query: "Show me active OTP for this phone number"
-- Used when: Preventing multiple OTPs to same phone
CREATE INDEX IF NOT EXISTS idx_otp_verifications_phone 
ON otp_verifications(phone);

-- Index on code_hash for fast verification lookup
-- Query: "Find OTP by its hash"
-- Used when: User submits OTP code for verification
CREATE INDEX IF NOT EXISTS idx_otp_verifications_code_hash 
ON otp_verifications(code_hash);

-- Index on expires_at for cleanup jobs
-- Query: "Delete all expired OTP codes"
-- Used when: Background job removes old OTP records
CREATE INDEX IF NOT EXISTS idx_otp_verifications_expires_at 
ON otp_verifications(expires_at);

-- Composite index for verification queries
-- Query: "Find active OTP for this user and phone"
-- Used when: Verifying OTP during phone change
-- This index covers the most common query pattern
CREATE INDEX IF NOT EXISTS idx_otp_verifications_user_phone 
ON otp_verifications(user_id, phone, expires_at);

-- ═══════════════════════════════════════════════════════════════════════════
-- NOTES
-- ═══════════════════════════════════════════════════════════════════════════
--
-- OTP Lifecycle:
--   1. User requests phone change
--   2. System generates 6-digit OTP (e.g., "123456")
--   3. System sends OTP via SMS
--   4. System hashes OTP and stores in this table
--   5. User submits OTP code
--   6. System hashes submitted code and compares with stored hash
--   7. If match: mark verified_at, update user's phone
--   8. If no match: increment attempt_count
--   9. If attempt_count > 3: block OTP
--
-- Security Features:
--   - OTP codes are hashed (SHA-256)
--   - Rate limiting (max 3 attempts)
--   - Time-bound (10 minutes expiration)
--   - Single-use (verified_at prevents reuse)
--   - Tied to specific user + phone combination
--
-- Cleanup Strategy:
--   - Background job runs daily
--   - Deletes OTP records older than 24 hours
--   - Keeps database size manageable
--   - Preserves audit trail for recent verifications
--
-- ═══════════════════════════════════════════════════════════════════════════
