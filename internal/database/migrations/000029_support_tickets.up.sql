-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 000029: Support Tickets
-- ═══════════════════════════════════════════════════════════════════════════
-- Purpose: Real support-ticket storage for the Support page (Phase 4.2).
-- Previously Support.jsx kept tickets in React component state — nothing was
-- persisted. Now:
-- - support_tickets: one row per ticket, human-friendly display number
--   (TKT-000123, allocated from a sequence so numbers are unique and ordered)
-- - support_replies: the conversation thread (user messages + agent replies)
-- ═══════════════════════════════════════════════════════════════════════════

CREATE SEQUENCE support_ticket_seq START 1;

-- ═══════════════════════════════════════════════════════════════════════════
-- Table: support_tickets
-- ═══════════════════════════════════════════════════════════════════════════
CREATE TABLE support_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Human-friendly reference shown to users and quoted in correspondence.
    ticket_number VARCHAR(20) NOT NULL UNIQUE,

    subject VARCHAR(255) NOT NULL,
    category VARCHAR(100) NOT NULL,

    -- open → in_progress → resolved → closed (agents move it; a user reply on
    -- a resolved ticket reopens it to open, closed tickets accept no replies).
    status VARCHAR(20) NOT NULL DEFAULT 'open',
    priority VARCHAR(20) NOT NULL DEFAULT 'medium',

    last_reply_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_support_tickets_status
        CHECK (status IN ('open', 'in_progress', 'resolved', 'closed')),
    CONSTRAINT chk_support_tickets_priority
        CHECK (priority IN ('low', 'medium', 'high'))
);

CREATE INDEX idx_support_tickets_user ON support_tickets (user_id, created_at DESC);
CREATE INDEX idx_support_tickets_status ON support_tickets (status, created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════════
-- Table: support_replies
-- ═══════════════════════════════════════════════════════════════════════════
CREATE TABLE support_replies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- 'user' = the ticket owner, 'agent' = staff/admin working the queue.
    author_role VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_support_replies_author_role
        CHECK (author_role IN ('user', 'agent'))
);

CREATE INDEX idx_support_replies_ticket ON support_replies (ticket_id, created_at);
