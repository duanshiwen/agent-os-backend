-- AgentOS Backend — User skill settings sync object coverage

CREATE TABLE IF NOT EXISTS user_skill_settings (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    skill_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    config JSONB DEFAULT '{}'::jsonb,
    updated_by_device_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_user_skill_settings_user_skill UNIQUE (user_id, skill_id)
);

CREATE INDEX IF NOT EXISTS idx_user_skill_settings_user_id ON user_skill_settings(user_id);
CREATE INDEX IF NOT EXISTS idx_user_skill_settings_skill_id ON user_skill_settings(skill_id);
CREATE INDEX IF NOT EXISTS idx_user_skill_settings_updated_by_device_id ON user_skill_settings(updated_by_device_id);
