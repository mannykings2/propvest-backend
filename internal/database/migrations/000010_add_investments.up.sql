-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000010: investments
-- ═══════════════════════════════════════════════════════════════════════════
-- The core product record: a user's purchase of slots in a property. Creating a
-- row is transactional with the wallet debit and property funding update.
-- ═══════════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS investments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    property_id UUID NOT NULL,
    slots INTEGER NOT NULL,
    amount_kobo BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    reference VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT fk_investments_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_investments_property FOREIGN KEY (property_id) REFERENCES properties(id) ON DELETE RESTRICT,
    CONSTRAINT chk_investments_slots_positive CHECK (slots > 0),
    CONSTRAINT chk_investments_amount_positive CHECK (amount_kobo > 0),
    CONSTRAINT chk_investments_status CHECK (status IN ('active', 'completed', 'cancelled', 'refunded'))
);

CREATE INDEX IF NOT EXISTS idx_investments_user_id ON investments(user_id);
CREATE INDEX IF NOT EXISTS idx_investments_property_id ON investments(property_id);
CREATE INDEX IF NOT EXISTS idx_investments_status ON investments(status);
CREATE INDEX IF NOT EXISTS idx_investments_deleted_at ON investments(deleted_at);

CREATE OR REPLACE TRIGGER update_investments_updated_at
    BEFORE UPDATE ON investments
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE investments IS 'User purchases of property slots; created atomically with wallet debit + property funding update';
