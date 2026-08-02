-- ═══════════════════════════════════════════════════════════════════════════
-- PropVest Backend - Initial Schema Migration
-- ═══════════════════════════════════════════════════════════════════════════
-- Migration: 000001_init_schema
-- Description: Creates all core tables for the PropVest platform
-- Author: Principal Software Architect
-- Date: 2026-07-26
-- ═══════════════════════════════════════════════════════════════════════════

-- Enable UUID extension (PostgreSQL 13+ has gen_random_uuid() built-in)
-- This is idempotent - safe to run multiple times
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ───────────────────────────────────────────────────────────────────────────
-- USERS TABLE
-- ───────────────────────────────────────────────────────────────────────────
-- Represents all authenticated accounts on the platform
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_code VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    phone VARCHAR(20) UNIQUE,
    password_hash TEXT NOT NULL,
    full_name VARCHAR(255) NOT NULL,
    kyc_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    role VARCHAR(50) NOT NULL DEFAULT 'investor',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for users table
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at);

-- ───────────────────────────────────────────────────────────────────────────
-- WALLETS TABLE
-- ───────────────────────────────────────────────────────────────────────────
-- One wallet per user, stores investable funds
CREATE TABLE IF NOT EXISTS wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    main_balance BIGINT NOT NULL DEFAULT 0 CHECK (main_balance >= 0),
    earnings_balance BIGINT NOT NULL DEFAULT 0 CHECK (earnings_balance >= 0),
    virtual_acct_no VARCHAR(50),
    virtual_bank VARCHAR(100),
    currency VARCHAR(10) NOT NULL DEFAULT 'NGN',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for wallets table
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallets_user_id ON wallets(user_id);

-- ───────────────────────────────────────────────────────────────────────────
-- WALLET_TRANSACTIONS TABLE
-- ───────────────────────────────────────────────────────────────────────────
-- Immutable financial ledger - NEVER UPDATE OR DELETE ROWS
CREATE TABLE IF NOT EXISTS wallet_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id UUID NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    amount BIGINT NOT NULL,
    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    reference VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    status VARCHAR(50) NOT NULL DEFAULT 'completed',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for wallet_transactions table
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_wallet_id ON wallet_transactions(wallet_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_reference ON wallet_transactions(reference);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_type ON wallet_transactions(type);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_created_at ON wallet_transactions(created_at DESC);

-- ───────────────────────────────────────────────────────────────────────────
-- PROPERTIES TABLE
-- ───────────────────────────────────────────────────────────────────────────
-- Investment opportunities listed on the platform
CREATE TABLE IF NOT EXISTS properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prop_code VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    location VARCHAR(255) NOT NULL,
    state VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    income_type VARCHAR(50) NOT NULL,
    spv_name VARCHAR(255) NOT NULL,
    spv_cac_no VARCHAR(100),
    total_slots INTEGER NOT NULL CHECK (total_slots > 0),
    slot_price BIGINT NOT NULL CHECK (slot_price > 0),
    total_value BIGINT NOT NULL CHECK (total_value > 0),
    purchase_price BIGINT NOT NULL CHECK (purchase_price > 0),
    yield_pct NUMERIC(5,2),
    annual_rent BIGINT,
    monthly_rent BIGINT,
    funded_pct INTEGER NOT NULL DEFAULT 0 CHECK (funded_pct >= 0 AND funded_pct <= 100),
    slots_sold INTEGER NOT NULL DEFAULT 0 CHECK (slots_sold >= 0),
    hold_years INTEGER NOT NULL DEFAULT 3 CHECK (hold_years > 0),
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    developer_id UUID REFERENCES users(id) ON DELETE SET NULL,
    description TEXT,
    tag VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes for properties table
CREATE INDEX IF NOT EXISTS idx_properties_status ON properties(status);
CREATE INDEX IF NOT EXISTS idx_properties_location ON properties(location);
CREATE INDEX IF NOT EXISTS idx_properties_state ON properties(state);
CREATE INDEX IF NOT EXISTS idx_properties_developer_id ON properties(developer_id);
CREATE INDEX IF NOT EXISTS idx_properties_deleted_at ON properties(deleted_at);

-- ───────────────────────────────────────────────────────────────────────────
-- UPDATED_AT TRIGGER FUNCTION
-- ───────────────────────────────────────────────────────────────────────────
-- Automatically updates the updated_at column on every UPDATE
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply the trigger to all tables with updated_at
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_wallets_updated_at
    BEFORE UPDATE ON wallets
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_properties_updated_at
    BEFORE UPDATE ON properties
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ═══════════════════════════════════════════════════════════════════════════
-- COMMENTS (Documentation in the database)
-- ═══════════════════════════════════════════════════════════════════════════

COMMENT ON TABLE users IS 'All authenticated accounts (investors, developers, admins)';
COMMENT ON TABLE wallets IS 'Financial accounts - one per user';
COMMENT ON TABLE wallet_transactions IS 'Immutable financial ledger - NEVER update or delete rows';
COMMENT ON TABLE properties IS 'Real estate investment opportunities';

COMMENT ON COLUMN wallets.main_balance IS 'Spendable balance in kobo';
COMMENT ON COLUMN wallets.earnings_balance IS 'Investment returns in kobo';
COMMENT ON COLUMN wallet_transactions.reference IS 'Unique transaction identifier for idempotency';
COMMENT ON COLUMN properties.slot_price IS 'Price per investment slot in kobo';

-- ═══════════════════════════════════════════════════════════════════════════
-- END OF MIGRATION
-- ═══════════════════════════════════════════════════════════════════════════
