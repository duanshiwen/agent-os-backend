-- AgentOS Backend — User knowledge entries sync object coverage

CREATE TABLE IF NOT EXISTS user_knowledge_entries (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entry_id TEXT NOT NULL,
    title TEXT NOT NULL,
    content_markdown TEXT NOT NULL,
    summary TEXT DEFAULT '',
    tags JSONB DEFAULT '[]'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    source_uri TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    version BIGINT NOT NULL DEFAULT 1,
    content_hash TEXT DEFAULT '',
    deleted_at TIMESTAMPTZ,
    updated_by_device_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_user_knowledge_entries_user_entry UNIQUE (user_id, entry_id),
    CONSTRAINT chk_user_knowledge_entries_status CHECK (status IN ('active', 'deleted'))
);

CREATE INDEX IF NOT EXISTS idx_user_knowledge_entries_user_id ON user_knowledge_entries(user_id);
CREATE INDEX IF NOT EXISTS idx_user_knowledge_entries_entry_id ON user_knowledge_entries(entry_id);
CREATE INDEX IF NOT EXISTS idx_user_knowledge_entries_status ON user_knowledge_entries(status);
CREATE INDEX IF NOT EXISTS idx_user_knowledge_entries_content_hash ON user_knowledge_entries(content_hash);
CREATE INDEX IF NOT EXISTS idx_user_knowledge_entries_updated_by_device_id ON user_knowledge_entries(updated_by_device_id);
