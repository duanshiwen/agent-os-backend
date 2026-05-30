-- AgentOS Backend — KB Hub snapshot vertical slice

CREATE TABLE IF NOT EXISTS kb_collections (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    pricing_model TEXT DEFAULT '',
    monthly_price BIGINT NOT NULL DEFAULT 0,
    is_free BOOLEAN NOT NULL DEFAULT true,
    platform_min_price BIGINT NOT NULL DEFAULT 0,
    platform_max_price BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_kb_collections_status CHECK (status IN ('draft', 'published', 'archived'))
);

CREATE INDEX IF NOT EXISTS idx_kb_collections_owner_id ON kb_collections(owner_id);
CREATE INDEX IF NOT EXISTS idx_kb_collections_status ON kb_collections(status);

CREATE TABLE IF NOT EXISTS kb_snapshots (
    id UUID PRIMARY KEY,
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    entry_count INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    checksum TEXT NOT NULL,
    manifest_object_uri TEXT NOT NULL,
    archive_object_uri TEXT DEFAULT '',
    content_hash TEXT NOT NULL,
    content_size BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_kb_snapshots_collection_version UNIQUE (collection_id, version),
    CONSTRAINT chk_kb_snapshots_entry_count CHECK (entry_count >= 0),
    CONSTRAINT chk_kb_snapshots_total_tokens CHECK (total_tokens >= 0),
    CONSTRAINT chk_kb_snapshots_content_size CHECK (content_size >= 0)
);

CREATE INDEX IF NOT EXISTS idx_kb_snapshots_collection_id ON kb_snapshots(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_snapshots_published_at ON kb_snapshots(published_at);
CREATE INDEX IF NOT EXISTS idx_kb_snapshots_checksum ON kb_snapshots(checksum);
CREATE INDEX IF NOT EXISTS idx_kb_snapshots_content_hash ON kb_snapshots(content_hash);

CREATE TABLE IF NOT EXISTS kb_snapshot_entries (
    id UUID PRIMARY KEY,
    snapshot_id UUID NOT NULL REFERENCES kb_snapshots(id) ON DELETE CASCADE,
    entry_id TEXT NOT NULL,
    title TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    tags JSONB DEFAULT '[]'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    content_object_uri TEXT NOT NULL,
    embedding_path TEXT DEFAULT '',
    tokens INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_kb_snapshot_entries_tokens CHECK (tokens >= 0)
);

CREATE INDEX IF NOT EXISTS idx_kb_snapshot_entries_snapshot_id ON kb_snapshot_entries(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_kb_snapshot_entries_entry_id ON kb_snapshot_entries(entry_id);
