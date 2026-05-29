-- AgentOS Backend — Sync client idempotency
-- Allows clients to retry event creation safely without creating duplicate sync events.

ALTER TABLE sync_events
    ADD COLUMN IF NOT EXISTS client_event_id VARCHAR(128) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_events_user_client_event_id_unique
    ON sync_events(user_id, client_event_id)
    WHERE client_event_id <> '';
