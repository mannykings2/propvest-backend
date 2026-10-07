-- Rollback: Remove soft delete support from property_images and property_documents

-- Drop indexes
DROP INDEX IF EXISTS idx_property_images_deleted_at;
DROP INDEX IF EXISTS idx_property_documents_deleted_at;

-- Remove deleted_at columns
ALTER TABLE property_images DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE property_documents DROP COLUMN IF EXISTS deleted_at;
