-- Migration 000027: Off-plan projects and subscriptions
--
-- The Off-Plan page previously ran entirely on static frontend constants.
-- This module gives it a real backend: projects seeded from the frontend's own
-- OFF_PLAN data (so the UI shape is unchanged), plus subscription rows for
-- instalment purchases.
--
-- JSONB columns store plans/finish_types/renders/features verbatim — no lossy
-- flattening, no frontend shape changes.

CREATE TABLE IF NOT EXISTS offplan_projects (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(255) NOT NULL,
    location      VARCHAR(255) NOT NULL,
    property_type VARCHAR(100) NOT NULL,
    price_kobo    BIGINT NOT NULL,
    stage         VARCHAR(50) NOT NULL,
    completion    VARCHAR(50) NOT NULL,
    developer     VARCHAR(255) NOT NULL,
    down_pct      INT NOT NULL,
    img           VARCHAR(10) NOT NULL DEFAULT '🏡',
    features      JSONB NOT NULL DEFAULT '[]',
    renders       JSONB NOT NULL DEFAULT '[]',
    finish_types  JSONB NOT NULL DEFAULT '[]',
    plans         JSONB NOT NULL DEFAULT '[]',
    status        VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ,
    CONSTRAINT chk_offplan_projects_price CHECK (price_kobo > 0),
    CONSTRAINT chk_offplan_projects_down_pct CHECK (down_pct >= 0 AND down_pct <= 100)
);

CREATE INDEX IF NOT EXISTS idx_offplan_projects_status
    ON offplan_projects(status) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS offplan_subscriptions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id),
    project_id        UUID NOT NULL REFERENCES offplan_projects(id),
    plan_id           VARCHAR(20) NOT NULL,
    finish_type_id    VARCHAR(20) NOT NULL,
    price_kobo        BIGINT NOT NULL,
    down_payment_kobo BIGINT NOT NULL,
    fee_kobo          BIGINT NOT NULL DEFAULT 0,
    reference         VARCHAR(60) UNIQUE NOT NULL,
    status            VARCHAR(20) NOT NULL DEFAULT 'active'
                      CHECK (status IN ('pending','active','completed','cancelled')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_offplan_subs_user ON offplan_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_offplan_subs_project ON offplan_subscriptions(project_id);

COMMENT ON COLUMN offplan_subscriptions.fee_kobo IS
    '2% subscription fee charged alongside the down payment.';

-- ═════════════════════════════════════════════════════════════════════════
-- SEED DATA — the three projects from the frontend's OFF_PLAN constants.
-- Amounts are Naira in the frontend, stored here as kobo (×100).
-- JSONB arrays are copied verbatim so the UI renders with zero shape changes.
-- ═════════════════════════════════════════════════════════════════════════
INSERT INTO offplan_projects
    (name, location, property_type, price_kobo, stage, completion, developer,
     down_pct, img, features, renders, finish_types, plans)
VALUES
(
    'Katampe Extension Villa', 'FCT Abuja', '4-Bed Detached', 4500000000,
    'Foundation', 'Q4 2027', 'Sunrise Properties Ltd', 20, '🏡',
    '["Smart Home","Solar Panels","BQ Quarters","2 Parking","Estate Security"]'::jsonb,
    '[{"view":"Exterior","label":"Front Elevation","desc":"4-bed detached with double garage, large compound and BQ quarters","col":"#A0522D","accent":"#C4956A","shape":"detached"},{"view":"Interior","label":"Living Room","desc":"Open plan living area, 18ft ceiling, imported marble tiles","col":"#7A3B1E","accent":"#B8AFA6","shape":"interior"},{"view":"Floor Plan","label":"Ground Floor","desc":"Living, dining, kitchen, guest room, 2 en-suite masters on first floor","col":"#1A2A1A","accent":"#52B788","shape":"floorplan"}]'::jsonb,
    '[{"id":"carcass","label":"Carcass","price":45000000,"desc":"Structural shell only. Walls, roof, windows & doors. No interior finishes, tiling or fittings.","includes":["Structural walls & columns","Roof & fascia boards","Electrical & plumbing rough-in"]},{"id":"semi","label":"Semi-Finished","price":52000000,"desc":"Carcass plus internal plastering, electrical wiring and plumbing piping.","includes":["All Carcass inclusions","Internal plastering","External doors (steel)","Window frames & louvres","Outside painting","Electrical wiring (conduits & cables)","Plumbing piping (water supply & drainage)"]},{"id":"full","label":"Fully Finished","price":62000000,"desc":"Turnkey, move-in ready. All premium fittings, kitchen cabinets, wardrobes and painting included.","includes":["All Semi-Finished inclusions","Electrical fittings (sockets, switches & fixtures)","Plumbing fittings (WC, basin, taps & shower)","Floor & wall tiling (premium)","POP ceiling","Premium kitchen cabinets & countertop","Built-in wardrobes","Skim coat & painting (2 coats)","Landscaping & compound paving"]}]'::jsonb,
    '[{"id":"6m","label":"6 Months","months":6,"rate":0},{"id":"12m","label":"12 Months","months":12,"rate":3},{"id":"24m","label":"24 Months","months":24,"rate":6},{"id":"36m","label":"36 Months","months":36,"rate":9}]'::jsonb
),
(
    'Lekki Phase 2 Apartments', 'Lekki, Lagos', '2-Bed Apartment', 2200000000,
    'Superstructure', 'Q2 2027', 'Coastline Homes Ltd', 15, '🏢',
    '["Swimming Pool","Gym","24hr Power","CCTV","Concierge"]'::jsonb,
    '[{"view":"Exterior","label":"Building Facade","desc":"Modern apartment block with glass balconies, swimming pool on podium","col":"#0F4C5C","accent":"#22D3EE","shape":"apartment"},{"view":"Interior","label":"Sample Unit","desc":"2-bed apartment with open kitchen, floor-to-ceiling windows","col":"#1A1A2E","accent":"#A5B4FC","shape":"interior"},{"view":"Floor Plan","label":"Unit Layout","desc":"Open plan living, 2 bedrooms, 2 bathrooms, balcony","col":"#1A2A1A","accent":"#52B788","shape":"floorplan"}]'::jsonb,
    '[{"id":"carcass","label":"Carcass","price":22000000,"desc":"Structural shell with windows, external doors and rough-in services only.","includes":["Structural walls","Roof slab","Electrical & plumbing rough-in"]},{"id":"semi","label":"Semi-Finished","price":26500000,"desc":"Carcass plus internal plastering, electrical wiring and plumbing piping.","includes":["All Carcass inclusions","Internal plastering","External doors (steel)","Window frames & louvres","Outside painting","Plumbing piping (water supply & drainage)"]},{"id":"full","label":"Fully Finished","price":32000000,"desc":"Turnkey apartment ready for immediate occupation with all premium fittings.","includes":["All Semi-Finished inclusions","Electrical fittings (sockets, switches & fixtures)","Plumbing fittings (WC, basin, taps & shower)","Floor & wall tiling (premium)","POP ceiling","Kitchen cabinets & countertop","Full painting"]}]'::jsonb,
    '[{"id":"6m","label":"6 Months","months":6,"rate":0},{"id":"12m","label":"12 Months","months":12,"rate":2.5},{"id":"18m","label":"18 Months","months":18,"rate":5},{"id":"24m","label":"24 Months","months":24,"rate":7}]'::jsonb
),
(
    'Asokoro Terraces', 'FCT Abuja', '3-Bed Terrace', 3200000000,
    'Roofing', 'Q1 2027', 'Capitol Developers Ltd', 25, '🏘',
    '["Serviced Estate","Fitted Kitchen","BQ","Green Areas","CCTV"]'::jsonb,
    '[{"view":"Exterior","label":"Street View","desc":"Row of 3-bed terraces with gated estate, green areas and CCTV","col":"#3A2A1A","accent":"#C4956A","shape":"terrace"},{"view":"Interior","label":"Master Bedroom","desc":"En-suite master with fitted wardrobe, large windows","col":"#1A1A1A","accent":"#B8AFA6","shape":"interior"},{"view":"Floor Plan","label":"Floor Layout","desc":"3 beds, 3 baths, study, open plan ground floor, BQ","col":"#1A2A1A","accent":"#52B788","shape":"floorplan"}]'::jsonb,
    '[{"id":"carcass","label":"Carcass","price":32000000,"desc":"Shell structure including walls, roof, windows and rough mechanical installations.","includes":["Structural walls & columns","Roof structure & tiles","Electrical & plumbing rough-in"]},{"id":"semi","label":"Semi-Finished","price":38000000,"desc":"Carcass plus internal plastering, electrical wiring and plumbing piping.","includes":["All Carcass inclusions","Internal plastering","External doors (steel)","Window frames & louvres","Outside painting","Plumbing piping (water supply & drainage)","BQ finishing"]},{"id":"full","label":"Fully Finished","price":46500000,"desc":"Move-in ready with kitchen fittings, wardrobes, full painting and all appliance connections.","includes":["All Semi-Finished inclusions","Electrical fittings (sockets, switches & fixtures)","Plumbing fittings (WC, basin, taps & shower)","Floor & wall tiling (premium)","POP ceiling","Kitchen cabinets & fitted appliances","Built-in wardrobes (all beds)","Full interior painting","Paved compound & landscaping"]}]'::jsonb,
    '[{"id":"6m","label":"6 Months","months":6,"rate":0},{"id":"12m","label":"12 Months","months":12,"rate":3},{"id":"18m","label":"18 Months","months":18,"rate":5.5}]'::jsonb
)
ON CONFLICT DO NOTHING;
