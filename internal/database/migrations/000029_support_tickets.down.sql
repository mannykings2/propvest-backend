-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000029 (down): Support Tickets
-- ═══════════════════════════════════════════════════════════════════════════
DROP TABLE IF EXISTS support_replies;
DROP TABLE IF EXISTS support_tickets;
DROP SEQUENCE IF EXISTS support_ticket_seq;
