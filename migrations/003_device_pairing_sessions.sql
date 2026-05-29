-- AgentOS Backend — QR-only device pairing sessions

CREATE TABLE IF NOT EXISTS device_pairing_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pairing_token_hash TEXT NOT NULL,
    qr_payload_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_by_device_id VARCHAR(255) NOT NULL,
    claimed_by_device_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_device_pairing_sessions_user_id ON device_pairing_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_device_pairing_sessions_expires_at ON device_pairing_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_device_pairing_sessions_created_by_device_id ON device_pairing_sessions(created_by_device_id);
CREATE INDEX IF NOT EXISTS idx_device_pairing_sessions_claimed_by_device_id ON device_pairing_sessions(claimed_by_device_id);
