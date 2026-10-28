-- Rollback Migration 000025: Sale vote campaigns

DROP TABLE IF EXISTS sale_votes;
DROP TABLE IF EXISTS sale_vote_campaigns;
ALTER TABLE properties DROP COLUMN IF EXISTS hold_years;
