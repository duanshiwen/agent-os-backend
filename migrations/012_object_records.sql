-- AgentOS Backend — Object storage records

CREATE TABLE IF NOT EXISTS object_records (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    bucket TEXT NOT NULL,
    object_key TEXT NOT NULL UNIQUE,
    object_uri TEXT NOT NULL UNIQUE,
    filename TEXT DEFAULT '',
    content_type TEXT DEFAULT '',
    content_hash TEXT NOT NULL,
    content_size BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    ref_count INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_object_records_status CHECK (status IN ('pending', 'active', 'deleted')),
    CONSTRAINT chk_object_records_content_size CHECK (content_size >= 0),
    CONSTRAINT chk_object_records_ref_count CHECK (ref_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_object_records_owner_id ON object_records(owner_id);
CREATE INDEX IF NOT EXISTS idx_object_records_scope ON object_records(scope);
CREATE INDEX IF NOT EXISTS idx_object_records_status ON object_records(status);
CREATE INDEX IF NOT EXISTS idx_object_records_content_hash ON object_records(content_hash);
