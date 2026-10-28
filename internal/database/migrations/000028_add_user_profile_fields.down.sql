-- Rollback Migration 000028: User profile fields

ALTER TABLE users DROP COLUMN IF EXISTS address;
ALTER TABLE users DROP COLUMN IF EXISTS occupation;
ALTER TABLE users DROP COLUMN IF EXISTS employer;
