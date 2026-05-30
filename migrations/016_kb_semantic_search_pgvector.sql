-- AgentOS Backend — M3.5 KB semantic search with pgvector and durable embedding queue

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS kb_embedding_jobs (
    id UUID PRIMARY KEY,
    search_document_id UUID NOT NULL REFERENCES kb_search_documents(id) ON DELETE CASCADE,
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    snapshot_id UUID NOT NULL REFERENCES kb_snapshots(id) ON DELETE CASCADE,
    snapshot_entry_id UUID NOT NULL REFERENCES kb_snapshot_entries(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL DEFAULT 1024,
    content_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    priority INTEGER NOT NULL DEFAULT 100,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    last_error TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_kb_embedding_jobs_dimensions CHECK (dimensions > 0),
    CONSTRAINT chk_kb_embedding_jobs_priority CHECK (priority >= 0),
    CONSTRAINT chk_kb_embedding_jobs_attempts CHECK (attempts >= 0 AND max_attempts > 0),
    CONSTRAINT chk_kb_embedding_jobs_status CHECK (status IN ('pending', 'processing', 'ready', 'retrying', 'failed', 'cancelled', 'skipped')),
    CONSTRAINT uq_kb_embedding_jobs_doc_provider_model_hash UNIQUE (search_document_id, provider, model, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_kb_embedding_jobs_status_available ON kb_embedding_jobs(status, available_at, priority, created_at);
CREATE INDEX IF NOT EXISTS idx_kb_embedding_jobs_search_document_id ON kb_embedding_jobs(search_document_id);
CREATE INDEX IF NOT EXISTS idx_kb_embedding_jobs_collection_id ON kb_embedding_jobs(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_embedding_jobs_snapshot_id ON kb_embedding_jobs(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_kb_embedding_jobs_snapshot_entry_id ON kb_embedding_jobs(snapshot_entry_id);
CREATE INDEX IF NOT EXISTS idx_kb_embedding_jobs_locked_by ON kb_embedding_jobs(locked_by);
CREATE INDEX IF NOT EXISTS idx_kb_embedding_jobs_content_hash ON kb_embedding_jobs(content_hash);

CREATE TABLE IF NOT EXISTS kb_search_embeddings (
    id UUID PRIMARY KEY,
    search_document_id UUID NOT NULL REFERENCES kb_search_documents(id) ON DELETE CASCADE,
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    snapshot_id UUID NOT NULL REFERENCES kb_snapshots(id) ON DELETE CASCADE,
    snapshot_entry_id UUID NOT NULL REFERENCES kb_snapshot_entries(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL DEFAULT 1024,
    content_hash TEXT NOT NULL,
    embedding vector(1024) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_kb_search_embeddings_dimensions CHECK (dimensions = 1024),
    CONSTRAINT uq_kb_search_embeddings_doc_provider_model_hash UNIQUE (search_document_id, provider, model, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_kb_search_embeddings_search_document_id ON kb_search_embeddings(search_document_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_embeddings_collection_id ON kb_search_embeddings(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_embeddings_snapshot_id ON kb_search_embeddings(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_embeddings_snapshot_entry_id ON kb_search_embeddings(snapshot_entry_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_embeddings_content_hash ON kb_search_embeddings(content_hash);
CREATE INDEX IF NOT EXISTS idx_kb_search_embeddings_vector_cosine ON kb_search_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
