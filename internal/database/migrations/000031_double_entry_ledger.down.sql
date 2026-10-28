-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000031 (down): double-entry ledger, fee rules, settlement
-- ═══════════════════════════════════════════════════════════════════════════
DROP TABLE IF EXISTS reconciliation_runs;
DROP TABLE IF EXISTS settlement_items;
DROP TABLE IF EXISTS settlement_batches;
DROP TABLE IF EXISTS fee_rules;

DROP TRIGGER IF EXISTS trg_ledger_entries_immutable ON ledger_entries;
DROP FUNCTION IF EXISTS ledger_entries_immutable();
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS ledger_accounts;

ALTER TABLE wallet_transactions
    DROP COLUMN IF EXISTS fee_kobo,
    DROP COLUMN IF EXISTS net_kobo;
