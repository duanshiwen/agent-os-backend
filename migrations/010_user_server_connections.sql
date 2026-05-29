-- AgentOS Backend — User server connections sync object coverage

CREATE TABLE IF NOT EXISTS user_server_connections (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL,
    name TEXT DEFAULT '',
    base_url TEXT DEFAULT '',
    status TEXT DEFAULT 'active',
    config JSONB DEFAULT '{}'::jsonb,
    updated_by_device_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_user_server_connections_user_server UNIQUE (user_id, server_id)
);

CREATE INDEX IF NOT EXISTS idx_user_server_connections_user_id ON user_server_connections(user_id);
CREATE INDEX IF NOT EXISTS idx_user_server_connections_server_id ON user_server_connections(server_id);
CREATE INDEX IF NOT EXISTS idx_user_server_connections_updated_by_device_id ON user_server_connections(updated_by_device_id);
