-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Add Wallet Currency Column
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000005_add_wallet_currency
-- Description: Adds currency column to wallets table for multi-currency support
-- Author: Backend Engineering Team
-- Date: 2026-08-04
-- Reason: Currency field was added to code model but missing in deployed database
-- ═══════════════════════════════════════════════════════════════════════════

-- Add currency column to wallets table
-- Uses IF NOT EXISTS for idempotency (safe if column already exists)
-- Default value is 'NGN' (Nigerian Naira)
-- NOT NULL constraint ensures every wallet has a currency
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'wallets' 
        AND column_name = 'currency'
    ) THEN
        ALTER TABLE wallets 
        ADD COLUMN currency VARCHAR(10) NOT NULL DEFAULT 'NGN';
    END IF;
END $$;

-- Add comment for documentation
COMMENT ON COLUMN wallets.currency IS 'Wallet currency code (NGN, USD, etc.) - enables future multi-currency support';

-- ═══════════════════════════════════════════════════════════════════════════
-- EXPLANATION FOR JUNIOR DEVELOPERS
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Q: Why do we need this migration if currency is already in 000001_init_schema.up.sql?
-- A: Because some databases were created BEFORE the currency field was added to 
--    the init schema. This migration handles existing databases.
--
-- Q: What does "IF NOT EXISTS" do?
-- A: It checks if the column already exists before adding it. This makes the 
--    migration idempotent (safe to run multiple times).
--
-- Q: Why VARCHAR(10)?
-- A: Currency codes are typically 3 characters (ISO 4217: NGN, USD, EUR).
--    We use 10 to allow for potential future extensions or custom codes.
--
-- Q: Why NOT NULL?
-- A: Every wallet MUST have a currency. Without it, we don't know how to 
--    display or calculate the balance.
--
-- Q: Why DEFAULT 'NGN'?
-- A: PropVest is a Nigerian platform, so NGN (Nigerian Naira) is the default.
--    For existing rows, this ensures they get NGN automatically.
--
-- Q: What is DO $$ ... END $$?
-- A: It's a PL/pgSQL anonymous code block. Think of it as a mini-program that
--    runs during the migration. We use it to check if the column exists first.
--
-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
