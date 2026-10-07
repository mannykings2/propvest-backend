-- Migration 000018 Rollback: Drop property_documents table
-- ON DELETE CASCADE on the foreign key means documents are automatically deleted when property is deleted.

DROP TABLE IF EXISTS property_documents;
