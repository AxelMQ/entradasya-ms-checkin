-- Migration: 001_create_checkins_table.sql
-- Table: checkins

CREATE TABLE IF NOT EXISTS checkins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_code VARCHAR(64) NOT NULL,
    ticket_id UUID,
    event_id UUID NOT NULL,
    sector_name VARCHAR(100),
    customer_name VARCHAR(100),
    porter_id UUID NOT NULL,
    porter_name VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('ACCEPTED', 'REJECTED')),
    rejection_reason VARCHAR(100),
    checked_in_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Partial unique index: Guarantees at the database engine level that a ticket code
-- can NEVER have more than one record in 'ACCEPTED' status.
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_accepted_ticket 
ON checkins(ticket_code) 
WHERE status = 'ACCEPTED';

CREATE INDEX IF NOT EXISTS idx_checkins_event_id ON checkins(event_id);
CREATE INDEX IF NOT EXISTS idx_checkins_ticket_code ON checkins(ticket_code);
