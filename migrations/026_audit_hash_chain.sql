ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS sequence BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS previous_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS event_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS hash_algorithm TEXT NOT NULL DEFAULT 'sha256';

CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_events_sequence ON audit_events(sequence) WHERE sequence > 0;
CREATE INDEX IF NOT EXISTS idx_audit_events_event_hash ON audit_events(event_hash);
CREATE INDEX IF NOT EXISTS idx_audit_events_previous_hash ON audit_events(previous_hash);
