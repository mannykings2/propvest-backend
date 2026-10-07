-- Migration 000017: Create property_images table
-- This table stores Cloudinary-backed property gallery images.
-- Each property can have multiple images for showcasing the investment opportunity.

CREATE TABLE IF NOT EXISTS property_images (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Foreign Key to properties
    property_id UUID NOT NULL
        REFERENCES properties(id)
        ON DELETE CASCADE,

    -- Cloudinary Image Details
    url TEXT NOT NULL,
    public_id VARCHAR(255) NOT NULL,

    -- Image Metadata
    alt_text VARCHAR(255),
    display_order INT NOT NULL DEFAULT 0,
    is_cover BOOLEAN NOT NULL DEFAULT FALSE,

    -- Image Dimensions (optional, set after upload)
    width INT,
    height INT,

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- ═══════════════════════════════════════════════════════════════════════════
    -- CONSTRAINTS
    -- ═══════════════════════════════════════════════════════════════════════════

    -- Display order must be non-negative
    CONSTRAINT chk_property_images_display_order
        CHECK (display_order >= 0),

    -- Image dimensions must both be present or both be null
    -- If present, both must be positive
    CONSTRAINT chk_property_images_dimensions
        CHECK (
            (width IS NULL AND height IS NULL)
            OR
            (width > 0 AND height > 0)
        )
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

-- Composite index for fetching property images ordered by display order
-- This is the most common query pattern
CREATE INDEX IF NOT EXISTS idx_property_images_property
    ON property_images(property_id, display_order);

-- Unique index on Cloudinary public_id
-- Prevents duplicate uploads and enables fast deletion
CREATE UNIQUE INDEX IF NOT EXISTS idx_property_images_public_id
    ON property_images(public_id);

-- Unique partial index to enforce only one cover image per property
-- This is a database-level guarantee that complements application logic
CREATE UNIQUE INDEX IF NOT EXISTS idx_property_images_cover
    ON property_images(property_id)
    WHERE is_cover = TRUE;

-- ═══════════════════════════════════════════════════════════════════════════
-- COMMENTS
-- ═══════════════════════════════════════════════════════════════════════════

COMMENT ON TABLE property_images IS
    'Cloudinary-backed property gallery images. '
    'Each property can have multiple images with one designated as cover.';

COMMENT ON COLUMN property_images.url IS
    'Full Cloudinary URL to the image. Stored for quick access.';

COMMENT ON COLUMN property_images.public_id IS
    'Cloudinary public_id used for deletion and transformations. Must be unique across all images.';

COMMENT ON COLUMN property_images.display_order IS
    'Order in which images appear in gallery. Lower numbers appear first. '
    'Example: 0=exterior, 1=living room, 2=bedroom, etc.';

COMMENT ON COLUMN property_images.is_cover IS
    'Whether this image is the property cover/thumbnail. '
    'Only one image per property can be marked as cover (enforced by unique partial index).';
