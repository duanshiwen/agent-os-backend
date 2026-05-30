-- AgentOS Backend — Full M3 KB Hub usage, billing, contributor earnings, and search index

CREATE TABLE IF NOT EXISTS kb_usage_records (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    snapshot_id UUID NOT NULL REFERENCES kb_snapshots(id) ON DELETE CASCADE,
    snapshot_entry_id UUID REFERENCES kb_snapshot_entries(id) ON DELETE SET NULL,
    operation_type TEXT NOT NULL,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    unit_price BIGINT NOT NULL DEFAULT 0,
    amount BIGINT NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'CNY',
    billed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_kb_usage_tokens CHECK (tokens_used >= 0),
    CONSTRAINT chk_kb_usage_unit_price CHECK (unit_price >= 0),
    CONSTRAINT chk_kb_usage_amount CHECK (amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_kb_usage_records_user_id ON kb_usage_records(user_id);
CREATE INDEX IF NOT EXISTS idx_kb_usage_records_collection_id ON kb_usage_records(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_usage_records_snapshot_id ON kb_usage_records(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_kb_usage_records_snapshot_entry_id ON kb_usage_records(snapshot_entry_id);
CREATE INDEX IF NOT EXISTS idx_kb_usage_records_operation_type ON kb_usage_records(operation_type);
CREATE INDEX IF NOT EXISTS idx_kb_usage_records_billed_at ON kb_usage_records(billed_at);

CREATE TABLE IF NOT EXISTS billing_accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    balance BIGINT NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'CNY',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS billing_transactions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    amount BIGINT NOT NULL DEFAULT 0,
    description TEXT DEFAULT '',
    related_id TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_billing_transactions_user_id ON billing_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_billing_transactions_type ON billing_transactions(type);
CREATE INDEX IF NOT EXISTS idx_billing_transactions_related_id ON billing_transactions(related_id);

CREATE TABLE IF NOT EXISTS contributor_earnings (
    id UUID PRIMARY KEY,
    contributor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    period TEXT NOT NULL,
    gross_amount BIGINT NOT NULL DEFAULT 0,
    platform_fee BIGINT NOT NULL DEFAULT 0,
    net_amount BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_contributor_earnings_period UNIQUE (contributor_id, collection_id, period),
    CONSTRAINT chk_contributor_earnings_amounts CHECK (gross_amount >= 0 AND platform_fee >= 0 AND net_amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_contributor_earnings_contributor_id ON contributor_earnings(contributor_id);
CREATE INDEX IF NOT EXISTS idx_contributor_earnings_collection_id ON contributor_earnings(collection_id);
CREATE INDEX IF NOT EXISTS idx_contributor_earnings_period ON contributor_earnings(period);

CREATE TABLE IF NOT EXISTS kb_search_documents (
    id UUID PRIMARY KEY,
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    snapshot_id UUID NOT NULL REFERENCES kb_snapshots(id) ON DELETE CASCADE,
    snapshot_entry_id UUID NOT NULL REFERENCES kb_snapshot_entries(id) ON DELETE CASCADE,
    entry_id TEXT NOT NULL,
    title TEXT DEFAULT '',
    summary TEXT DEFAULT '',
    tags JSONB DEFAULT '[]'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    content_text TEXT DEFAULT '',
    content_hash TEXT DEFAULT '',
    tokens INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    indexed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_kb_search_documents_tokens CHECK (tokens >= 0),
    CONSTRAINT chk_kb_search_documents_status CHECK (status IN ('active', 'deleted'))
);

CREATE INDEX IF NOT EXISTS idx_kb_search_documents_collection_id ON kb_search_documents(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_documents_snapshot_id ON kb_search_documents(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_documents_snapshot_entry_id ON kb_search_documents(snapshot_entry_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_documents_entry_id ON kb_search_documents(entry_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_documents_content_hash ON kb_search_documents(content_hash);
CREATE INDEX IF NOT EXISTS idx_kb_search_documents_status ON kb_search_documents(status);
CREATE INDEX IF NOT EXISTS idx_kb_search_documents_indexed_at ON kb_search_documents(indexed_at);
CREATE INDEX IF NOT EXISTS idx_kb_search_documents_text ON kb_search_documents USING GIN (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(content_text, '')));
