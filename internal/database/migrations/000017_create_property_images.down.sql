-- Migration 000017 Rollback: Drop property_images table
-- ON DELETE CASCADE on the foreign key means images are automatically deleted when property is deleted.

DROP TABLE IF NOT EXISTS property_images;
