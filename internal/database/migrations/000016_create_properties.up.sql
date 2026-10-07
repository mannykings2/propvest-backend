-- Migration 000016: Create properties table
-- This table stores real estate investment opportunities available on the PropVest platform.
-- Properties go through a lifecycle: draft → active → funded → completed
-- Only admins can create/manage properties; public users can only view active/funded/completed properties.

CREATE TABLE IF NOT EXISTS properties (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Basic Information
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(280) NOT NULL UNIQUE,
    description TEXT NOT NULL,

    -- Property Classification
    property_type VARCHAR(30) NOT NULL,

    -- Location Details
    address TEXT NOT NULL,
    city VARCHAR(100) NOT NULL,
    state VARCHAR(100) NOT NULL,
    country VARCHAR(100) NOT NULL DEFAULT 'Nigeria',

    -- Geolocation (optional)
    latitude NUMERIC(10,7),
    longitude NUMERIC(10,7),

    -- Financial Details (all amounts in kobo - minor currency units)
    target_amount BIGINT NOT NULL,
    raised_amount BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'NGN',

    -- Investment Returns
    roi_percent NUMERIC(10,2) NOT NULL,
    duration_months INT NOT NULL,

    -- Unit-based Investment
    total_units BIGINT NOT NULL,
    units_sold BIGINT NOT NULL DEFAULT 0,
    unit_price BIGINT NOT NULL,
    minimum_investment BIGINT NOT NULL,

    -- Statistics
    investor_count INT NOT NULL DEFAULT 0,

    -- Status & Flags
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    featured BOOLEAN NOT NULL DEFAULT FALSE,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    trending BOOLEAN NOT NULL DEFAULT FALSE,

    -- Media
    cover_image_url TEXT,

    -- Important Dates
    launch_date TIMESTAMP NULL,
    expected_completion_date TIMESTAMP NULL,

    -- Audit Fields
    created_by UUID NOT NULL REFERENCES users(id),
    updated_by UUID REFERENCES users(id),
    deleted_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- ═══════════════════════════════════════════════════════════════════════════
    -- CONSTRAINTS
    -- ═══════════════════════════════════════════════════════════════════════════

    -- Property type must be one of the allowed values
    CONSTRAINT chk_properties_type
        CHECK (property_type IN ('residential', 'commercial', 'land')),

    -- Status must be one of the lifecycle states
    CONSTRAINT chk_properties_status
        CHECK (status IN ('draft', 'active', 'funded', 'completed')),

    -- Financial invariants
    CONSTRAINT chk_properties_target_amount
        CHECK (target_amount > 0),

    CONSTRAINT chk_properties_raised_amount
        CHECK (raised_amount >= 0 AND raised_amount <= target_amount),

    CONSTRAINT chk_properties_roi
        CHECK (roi_percent > 0),

    CONSTRAINT chk_properties_duration
        CHECK (duration_months > 0),

    -- Unit-based investment invariants
    CONSTRAINT chk_properties_total_units
        CHECK (total_units > 0),

    CONSTRAINT chk_properties_units_sold
        CHECK (units_sold >= 0 AND units_sold <= total_units),

    CONSTRAINT chk_properties_unit_price
        CHECK (unit_price > 0),

    CONSTRAINT chk_properties_minimum_investment
        CHECK (minimum_investment > 0 AND minimum_investment <= target_amount),

    -- Statistics invariants
    CONSTRAINT chk_properties_investor_count
        CHECK (investor_count >= 0),

    -- Geolocation: both coordinates must be present or both must be null
    CONSTRAINT chk_properties_coordinates
        CHECK (
            (latitude IS NULL AND longitude IS NULL)
            OR
            (latitude IS NOT NULL AND longitude IS NOT NULL)
        ),

    -- Latitude range: -90 to 90
    CONSTRAINT chk_properties_latitude
        CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),

    -- Longitude range: -180 to 180
    CONSTRAINT chk_properties_longitude
        CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

-- Index on status for filtering (excludes soft-deleted)
CREATE INDEX IF NOT EXISTS idx_properties_status
    ON properties(status)
    WHERE deleted_at IS NULL;

-- Index on property type for filtering
CREATE INDEX IF NOT EXISTS idx_properties_type
    ON properties(property_type)
    WHERE deleted_at IS NULL;

-- Index on city for location-based searches
CREATE INDEX IF NOT EXISTS idx_properties_city
    ON properties(city)
    WHERE deleted_at IS NULL;

-- Index on state for location-based searches
CREATE INDEX IF NOT EXISTS idx_properties_state
    ON properties(state)
    WHERE deleted_at IS NULL;

-- Partial index for featured properties (excludes non-featured and deleted)
-- This is more efficient than a full index since most properties are not featured
CREATE INDEX IF NOT EXISTS idx_properties_featured
    ON properties(featured)
    WHERE deleted_at IS NULL AND status IN ('active', 'funded', 'completed');

-- Index on verified flag
CREATE INDEX IF NOT EXISTS idx_properties_verified
    ON properties(verified)
    WHERE deleted_at IS NULL;

-- Index on trending flag
CREATE INDEX IF NOT EXISTS idx_properties_trending
    ON properties(trending)
    WHERE deleted_at IS NULL;

-- Index on launch date for sorting
CREATE INDEX IF NOT EXISTS idx_properties_launch_date
    ON properties(launch_date)
    WHERE deleted_at IS NULL;

-- Index on created_at for default sorting (most recent first)
CREATE INDEX IF NOT EXISTS idx_properties_created_at
    ON properties(created_at DESC)
    WHERE deleted_at IS NULL;

-- Composite index optimized for public property listings
-- This covers the most common query: active/funded/completed properties sorted by creation date
CREATE INDEX IF NOT EXISTS idx_properties_public_listing
    ON properties(status, created_at DESC)
    WHERE deleted_at IS NULL
      AND status IN ('active', 'funded', 'completed');

-- ═══════════════════════════════════════════════════════════════════════════
-- COMMENTS
-- ═══════════════════════════════════════════════════════════════════════════

COMMENT ON TABLE properties IS
    'Real estate investment opportunities available on the PropVest platform. '
    'Properties go through lifecycle: draft → active → funded → completed.';

COMMENT ON COLUMN properties.target_amount IS
    'Total funding target in minor currency units (kobo for NGN). '
    'Example: ₦100,000,000 = 10000000000 kobo';

COMMENT ON COLUMN properties.raised_amount IS
    'Amount raised from investors in minor currency units. '
    'Updated by Investment Module when investments are made. '
    'Must never exceed target_amount.';

COMMENT ON COLUMN properties.roi_percent IS
    'Expected total ROI percentage over the investment duration. '
    'Example: 15.50 means 15.5% total return. '
    'NOT annualized unless specified in business logic.';

COMMENT ON COLUMN properties.unit_price IS
    'Price of one fractional investment unit in minor currency units. '
    'target_amount should equal unit_price × total_units for fractional investments.';

COMMENT ON COLUMN properties.investor_count IS
    'Number of unique investors (not number of investment transactions). '
    'Updated by Investment Module. NOT manually editable.';

COMMENT ON COLUMN properties.status IS
    'Property lifecycle status: draft (admin only), active (public), funded (fully funded), completed (investment period ended)';

COMMENT ON COLUMN properties.deleted_at IS
    'Soft deletion timestamp. Properties with investments should never be physically deleted.';
