-- AgentOS Backend — Stage 1 sensitive operation confirmation foundation

CREATE TABLE IF NOT EXISTS sensitive_operation_confirmations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id VARCHAR(128) NOT NULL,
    operation VARCHAR(160) NOT NULL,
    token_hash TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    consumed_by VARCHAR(160),
    remote_metadata TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sensitive_confirmations_user_operation ON sensitive_operation_confirmations(user_id, operation);
CREATE INDEX IF NOT EXISTS idx_sensitive_confirmations_device ON sensitive_operation_confirmations(device_id);
CREATE INDEX IF NOT EXISTS idx_sensitive_confirmations_expires_at ON sensitive_operation_confirmations(expires_at);
CREATE INDEX IF NOT EXISTS idx_sensitive_confirmations_used_at ON sensitive_operation_confirmations(used_at);
