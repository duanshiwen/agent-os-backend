-- AgentOS Backend — Phase 1 deployability hardening
-- Adds production sync tables and DB-level safeguards for identity/device pairing flows.

-- Sync foundation
CREATE TABLE IF NOT EXISTS sync_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id VARCHAR(128),
    event_type VARCHAR(128) NOT NULL,
    payload JSONB,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sequence BIGINT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sync_events_user_sequence ON sync_events(user_id, sequence);
CREATE INDEX IF NOT EXISTS idx_sync_events_user_device ON sync_events(user_id, device_id);
CREATE INDEX IF NOT EXISTS idx_sync_events_user_event_type ON sync_events(user_id, event_type);
CREATE INDEX IF NOT EXISTS idx_sync_events_timestamp ON sync_events(timestamp);

CREATE TABLE IF NOT EXISTS sync_cursors (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id VARCHAR(128) NOT NULL,
    last_synced_sequence BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, device_id)
);

-- DB-level safeguards for Phase 1 identity and QR pairing boundaries.
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_device_id_unique ON devices(device_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_device_pairing_sessions_qr_payload_hash_unique ON device_pairing_sessions(qr_payload_hash);
CREATE INDEX IF NOT EXISTS idx_auth_challenges_device_nonce ON auth_challenges(device_id, nonce);
