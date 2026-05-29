-- AgentOS Backend — User agent settings sync object coverage

CREATE TABLE IF NOT EXISTS user_agent_settings (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    display_name TEXT DEFAULT '',
    config JSONB DEFAULT '{}'::jsonb,
    updated_by_device_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_user_agent_settings_user_agent UNIQUE (user_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_user_agent_settings_user_id ON user_agent_settings(user_id);
CREATE INDEX IF NOT EXISTS idx_user_agent_settings_agent_id ON user_agent_settings(agent_id);
CREATE INDEX IF NOT EXISTS idx_user_agent_settings_updated_by_device_id ON user_agent_settings(updated_by_device_id);
