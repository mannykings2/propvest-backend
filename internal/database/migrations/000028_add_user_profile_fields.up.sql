-- Migration 000028: User profile fields (address / occupation / employer)
--
-- Profile.jsx has always sent these on PATCH /users/me, but the backend DTO
-- only persisted full_name — the edits were silently dropped. This adds the
-- columns and (with the matching Go changes) makes them stick.

ALTER TABLE users ADD COLUMN IF NOT EXISTS address TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS occupation VARCHAR(120);
ALTER TABLE users ADD COLUMN IF NOT EXISTS employer VARCHAR(120);
