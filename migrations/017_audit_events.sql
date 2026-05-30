-- AgentOS Backend — Stage 1 platform audit event foundation

CREATE TABLE IF NOT EXISTS audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_device_id VARCHAR(128),
    action VARCHAR(160) NOT NULL,
    resource_type VARCHAR(120) NOT NULL,
    resource_id TEXT,
    outcome VARCHAR(32) NOT NULL DEFAULT 'success',
    ip_address TEXT,
    user_agent TEXT,
    metadata JSONB,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_audit_events_outcome CHECK (outcome IN ('success', 'failure', 'denied'))
);

CREATE INDEX IF NOT EXISTS idx_audit_events_actor_user_time ON audit_events(actor_user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor_device_time ON audit_events(actor_device_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_action_time ON audit_events(action, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_resource ON audit_events(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_outcome_time ON audit_events(outcome, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_occurred_at ON audit_events(occurred_at DESC);
