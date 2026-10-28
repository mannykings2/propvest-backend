-- ═══════════════════════════════════════════════════════════════════════════
-- PROPERTIES TABLE (canonical schema — owned here)
-- ═══════════════════════════════════════════════════════════════════════════
-- This migration creates the canonical `properties` table BEFORE `investments`
-- (which carries a foreign key REFERENCES properties(id)). Migration 000001
-- previously created a legacy, unrelated `properties` table (prop_code/
-- slot_price); that block is removed so this is the single source of truth.
-- The schema matches the GORM Property model and every repository.
CREATE TABLE IF NOT EXISTS properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Basic Information
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(280) NOT NULL UNIQUE,
    description TEXT NOT NULL,

    -- Classification
    property_type VARCHAR(30) NOT NULL,

    -- Location
    address TEXT NOT NULL,
    city VARCHAR(100) NOT NULL,
    state VARCHAR(100) NOT NULL,
    country VARCHAR(100) NOT NULL DEFAULT 'Nigeria',

    -- Geolocation (optional)
    latitude NUMERIC(10,7),
    longitude NUMERIC(10,7),

    -- Financials (kobo)
    target_amount BIGINT NOT NULL,
    raised_amount BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'NGN',

    -- Returns
    roi_percent NUMERIC(10,2) NOT NULL,
    duration_months INT NOT NULL,

    -- Unit-based investment
    total_units BIGINT NOT NULL,
    units_sold BIGINT NOT NULL DEFAULT 0,
    unit_price BIGINT NOT NULL,
    minimum_investment BIGINT NOT NULL,

    -- Statistics
    investor_count INT NOT NULL DEFAULT 0,

    -- Status & flags
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    featured BOOLEAN NOT NULL DEFAULT FALSE,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    trending BOOLEAN NOT NULL DEFAULT FALSE,

    -- Media
    cover_image_url TEXT,

    -- Dates
    launch_date TIMESTAMP NULL,
    expected_completion_date TIMESTAMP NULL,

    -- Audit
    created_by UUID NOT NULL REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    deleted_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT chk_properties_type
        CHECK (property_type IN ('residential', 'commercial', 'land')),
    CONSTRAINT chk_properties_status
        CHECK (status IN ('draft', 'active', 'funded', 'completed')),
    CONSTRAINT chk_properties_target_amount CHECK (target_amount > 0),
    CONSTRAINT chk_properties_raised_amount
        CHECK (raised_amount >= 0 AND raised_amount <= target_amount),
    CONSTRAINT chk_properties_roi CHECK (roi_percent > 0),
    CONSTRAINT chk_properties_duration CHECK (duration_months > 0),
    CONSTRAINT chk_properties_total_units CHECK (total_units > 0),
    CONSTRAINT chk_properties_units_sold
        CHECK (units_sold >= 0 AND units_sold <= total_units),
    CONSTRAINT chk_properties_unit_price CHECK (unit_price > 0),
    CONSTRAINT chk_properties_minimum_investment
        CHECK (minimum_investment > 0 AND minimum_investment <= target_amount),
    CONSTRAINT chk_properties_investor_count CHECK (investor_count >= 0),
    CONSTRAINT chk_properties_coordinates CHECK (
        (latitude IS NULL AND longitude IS NULL)
        OR (latitude IS NOT NULL AND longitude IS NOT NULL)
    ),
    CONSTRAINT chk_properties_latitude
        CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CONSTRAINT chk_properties_longitude
        CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);

CREATE INDEX IF NOT EXISTS idx_properties_status
    ON properties(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_type
    ON properties(property_type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_city
    ON properties(city) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_state
    ON properties(state) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_featured
    ON properties(featured)
    WHERE deleted_at IS NULL AND status IN ('active', 'funded', 'completed');
CREATE INDEX IF NOT EXISTS idx_properties_verified
    ON properties(verified) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_trending
    ON properties(trending) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_launch_date
    ON properties(launch_date) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_created_at
    ON properties(created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_properties_public_listing
    ON properties(status, created_at DESC)
    WHERE deleted_at IS NULL AND status IN ('active', 'funded', 'completed');

CREATE OR REPLACE TRIGGER update_properties_updated_at
    BEFORE UPDATE ON properties
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE properties IS
    'Real estate investment opportunities available on the PropVest platform.';

-- ═══════════════════════════════════════════════════════════════════════════
-- INVESTMENTS TABLE
-- ═══════════════════════════════════════════════════════════════════════════
-- The core product record: a user's purchase of slots in a property. Creating a
-- row is transactional with the wallet debit and property funding update.
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS investments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    property_id UUID NOT NULL,
    slots INTEGER NOT NULL,
    amount_kobo BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    reference VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk_investments_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_investments_property FOREIGN KEY (property_id) REFERENCES properties(id) ON DELETE RESTRICT,
    CONSTRAINT chk_investments_slots_positive CHECK (slots > 0),
    CONSTRAINT chk_investments_amount_positive CHECK (amount_kobo > 0),
    CONSTRAINT chk_investments_status CHECK (status IN ('active', 'completed', 'cancelled', 'refunded'))
);

CREATE INDEX IF NOT EXISTS idx_investments_user_id ON investments(user_id);
CREATE INDEX IF NOT EXISTS idx_investments_property_id ON investments(property_id);
CREATE INDEX IF NOT EXISTS idx_investments_status ON investments(status);
CREATE INDEX IF NOT EXISTS idx_investments_deleted_at ON investments(deleted_at);

CREATE OR REPLACE TRIGGER update_investments_updated_at
    BEFORE UPDATE ON investments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE investments IS 'User purchases of property slots; created atomically with wallet debit + property funding update';
