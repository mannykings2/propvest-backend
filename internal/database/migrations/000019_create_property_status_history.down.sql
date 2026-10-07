-- Migration 000019 Rollback: Drop property_status_history table
-- ON DELETE CASCADE on the foreign key means history is automatically deleted when property is deleted.

DROP TABLE IF EXISTS property_status_history;
