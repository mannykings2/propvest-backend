-- Migration 000016 Rollback: Drop properties table
-- This migration should only be used during controlled rollback in development/staging.
-- NEVER run this in production if properties have associated investments.

DROP TABLE IF EXISTS properties;
