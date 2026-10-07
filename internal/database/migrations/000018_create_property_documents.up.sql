-- Migration 000018: Create property_documents table
-- This table stores legal, valuation, certification, and supporting documents for properties.
-- Documents can be public (visible to all users) or private (admin-only).

CREATE TABLE IF NOT EXISTS property_documents (
    -- Primary Key
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Foreign Key to properties
    property_id UUID NOT NULL
        REFERENCES properties(id)
        ON DELETE CASCADE,

    -- Document Metadata
    name VARCHAR(255) NOT NULL,
    document_type VARCHAR(50) NOT NULL,

    -- Cloudinary Document Details
    url TEXT NOT NULL,
    public_id VARCHAR(255) NOT NULL,

    -- File Information
    mime_type VARCHAR(100) NOT NULL,
    file_size BIGINT NOT NULL,

    -- Access Control
    is_public BOOLEAN NOT NULL DEFAULT FALSE,

    -- Audit
    uploaded_by UUID NOT NULL
        REFERENCES users(id),

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

    -- ═══════════════════════════════════════════════════════════════════════════
    -- CONSTRAINTS
    -- ═══════════════════════════════════════════════════════════════════════════

    -- File size must be positive
    CONSTRAINT chk_property_documents_file_size
        CHECK (file_size > 0),

    -- Document type must be one of the allowed values
    CONSTRAINT chk_property_documents_type
        CHECK (
            document_type IN (
                'title_document',
                'survey_plan',
                'building_approval',
                'certificate',
                'legal_document',
                'valuation',
                'other'
            )
        )
);

-- ═══════════════════════════════════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════════════════════════════════

-- Index for fetching all documents for a property
CREATE INDEX IF NOT EXISTS idx_property_documents_property
    ON property_documents(property_id);

-- Composite index for filtering by property and document type
CREATE INDEX IF NOT EXISTS idx_property_documents_type
    ON property_documents(property_id, document_type);

-- Composite index for fetching public documents
-- Used by public API to show only documents marked as public
CREATE INDEX IF NOT EXISTS idx_property_documents_public
    ON property_documents(property_id, is_public);

-- Unique index on Cloudinary public_id
-- Prevents duplicate uploads and enables fast deletion
CREATE UNIQUE INDEX IF NOT EXISTS idx_property_documents_public_id
    ON property_documents(public_id);

-- ═══════════════════════════════════════════════════════════════════════════
-- COMMENTS
-- ═══════════════════════════════════════════════════════════════════════════

COMMENT ON TABLE property_documents IS
    'Legal, valuation, certification, and supporting documents associated with properties. '
    'Documents can be marked as public (visible to all) or private (admin-only).';

COMMENT ON COLUMN property_documents.document_type IS
    'Type of document: title_document, survey_plan, building_approval, certificate, legal_document, valuation, or other';

COMMENT ON COLUMN property_documents.is_public IS
    'Whether this document is visible to public users. '
    'Private documents (FALSE) are only visible to admins. '
    'Use with caution for sensitive legal/financial documents.';

COMMENT ON COLUMN property_documents.mime_type IS
    'MIME type of the document (e.g., application/pdf, image/jpeg). '
    'Used for validation and proper file handling.';

COMMENT ON COLUMN property_documents.file_size IS
    'File size in bytes. Used for storage tracking and validation.';
