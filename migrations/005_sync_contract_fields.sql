-- AgentOS Backend — Sync Contract Foundation
-- Adds stable envelope fields to sync_events while preserving the existing event_type/payload contract.

ALTER TABLE sync_events
    ADD COLUMN IF NOT EXISTS schema_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS object_type VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS object_id VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS operation VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_device_id VARCHAR(128) NOT NULL DEFAULT '';

-- Backfill existing rows from legacy event_type values such as "message.created".
UPDATE sync_events
SET
    schema_version = COALESCE(NULLIF(schema_version, 0), 1),
    object_type = CASE
        WHEN object_type = '' AND position('.' in event_type) > 0 THEN split_part(event_type, '.', 1)
        ELSE object_type
    END,
    operation = CASE
        WHEN operation = '' AND position('.' in event_type) > 0 THEN split_part(event_type, '.', 2)
        ELSE operation
    END,
    source_device_id = CASE
        WHEN source_device_id = '' THEN COALESCE(device_id, '')
        ELSE source_device_id
    END;

CREATE INDEX IF NOT EXISTS idx_sync_events_user_object ON sync_events(user_id, object_type, object_id);
CREATE INDEX IF NOT EXISTS idx_sync_events_user_operation ON sync_events(user_id, object_type, operation);
CREATE INDEX IF NOT EXISTS idx_sync_events_source_device ON sync_events(user_id, source_device_id);
