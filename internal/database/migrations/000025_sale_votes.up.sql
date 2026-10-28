-- Migration 000025: Sale vote campaigns (Investment governance)
--
-- When a co-owner's hold period elapses they may trigger a sale vote. All other
-- investors in the property become electors. A simple majority (>50% of
-- electors) in favour passes the campaign; otherwise it fails.
--
-- Also fixes model drift: hold_years has existed in the business rules since
-- migration 000001's schema intent, but was never exposed as a column.

ALTER TABLE properties ADD COLUMN IF NOT EXISTS hold_years INTEGER NOT NULL DEFAULT 3;

CREATE TABLE IF NOT EXISTS sale_vote_campaigns (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    property_id   UUID NOT NULL REFERENCES properties(id),
    status        VARCHAR(20) NOT NULL DEFAULT 'open'
                  CHECK (status IN ('open','passed','failed')),
    triggered_by  UUID NOT NULL REFERENCES users(id),
    electors      INT  NOT NULL,
    for_count     INT  NOT NULL DEFAULT 0,
    against_count INT  NOT NULL DEFAULT 0,
    result        VARCHAR(20),
    opened_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- At most one open campaign per property.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sale_votes_one_open
    ON sale_vote_campaigns(property_id) WHERE status = 'open';

CREATE INDEX IF NOT EXISTS idx_sale_votes_campaign_property
    ON sale_vote_campaigns(property_id, status);

CREATE TABLE IF NOT EXISTS sale_votes (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id   UUID NOT NULL REFERENCES sale_vote_campaigns(id) ON DELETE CASCADE,
    investment_id UUID NOT NULL REFERENCES investments(id),
    user_id       UUID NOT NULL REFERENCES users(id),
    property_id   UUID NOT NULL REFERENCES properties(id),
    vote          VARCHAR(10) NOT NULL CHECK (vote IN ('for','against')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (campaign_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_sale_votes_user ON sale_votes(user_id);
CREATE INDEX IF NOT EXISTS idx_sale_votes_property ON sale_votes(property_id);

COMMENT ON TABLE sale_vote_campaigns IS
    'Governance campaigns opened by a co-owner after the hold period. A majority (>50% of electors) in favour passes the campaign.';
COMMENT ON COLUMN sale_vote_campaigns.electors IS
    'Distinct investors in the property at trigger time. This is the voting denominator.';
COMMENT ON TABLE sale_votes IS
    'One vote per investor per campaign. Enforced by UNIQUE(campaign_id, user_id).';
