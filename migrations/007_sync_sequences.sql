-- AgentOS Backend — Sync sequence allocation hardening
-- Allocates per-user sync event sequences without relying on MAX(sequence)+1.

CREATE TABLE IF NOT EXISTS sync_sequences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    next_sequence BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Seed sequence cursors for existing users that already have sync events.
INSERT INTO sync_sequences(user_id, next_sequence, updated_at)
SELECT user_id, COALESCE(MAX(sequence), 0) + 1, NOW()
FROM sync_events
GROUP BY user_id
ON CONFLICT (user_id) DO UPDATE
SET
    next_sequence = GREATEST(sync_sequences.next_sequence, EXCLUDED.next_sequence),
    updated_at = NOW();

-- Defend the client contract: sequence must be unique and ordered per user.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_events_user_sequence_unique ON sync_events(user_id, sequence);
