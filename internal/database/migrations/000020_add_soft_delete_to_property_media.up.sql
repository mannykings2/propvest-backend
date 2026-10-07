-- Migration: Add soft delete support to property_images and property_documents
-- This enables recoverability and audit trail for deleted media

-- Add deleted_at to property_images
ALTER TABLE property_images
ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;

-- Add deleted_at to property_documents
ALTER TABLE property_documents
ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;

-- Create indexes for soft delete queries (improves performance of NOT deleted queries)
CREATE INDEX idx_property_images_deleted_at ON property_images(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_property_documents_deleted_at ON property_documents(deleted_at) WHERE deleted_at IS NOT NULL;

-- Add comments
COMMENT ON COLUMN property_images.deleted_at IS 'Timestamp when image was soft deleted (NULL = active)';
COMMENT ON COLUMN property_documents.deleted_at IS 'Timestamp when document was soft deleted (NULL = active)';
