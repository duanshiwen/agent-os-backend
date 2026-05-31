-- AgentOS Backend — Stage 2 KB Hub productionization

ALTER TABLE kb_collections
    ADD COLUMN IF NOT EXISTS review_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS review_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS takedown_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS takedown_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS source_declaration TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS copyright_declaration TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS moderation_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE kb_collections
SET review_status = CASE
    WHEN status = 'published' THEN 'approved'
    WHEN status = 'archived' THEN 'archived'
    ELSE COALESCE(NULLIF(review_status, ''), 'pending')
END;

DO $$ BEGIN
    ALTER TABLE kb_collections
        ADD CONSTRAINT chk_kb_collections_review_status
        CHECK (review_status IN ('pending', 'approved', 'rejected', 'takedown', 'archived'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_kb_collections_review_status ON kb_collections(review_status);
CREATE INDEX IF NOT EXISTS idx_kb_collections_reviewed_by ON kb_collections(reviewed_by);
CREATE INDEX IF NOT EXISTS idx_kb_collections_takedown_at ON kb_collections(takedown_at);

ALTER TABLE kb_snapshots
    ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

DO $$ BEGIN
    ALTER TABLE kb_snapshots
        ADD CONSTRAINT chk_kb_snapshots_status
        CHECK (status IN ('active', 'archived'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_kb_snapshots_status ON kb_snapshots(status);
CREATE INDEX IF NOT EXISTS idx_kb_snapshots_archived_at ON kb_snapshots(archived_at);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_expires_at ON kb_subscriptions(expires_at);

CREATE TABLE IF NOT EXISTS kb_moderation_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    reporter_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason VARCHAR(128) NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'open',
    resolved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ,
    resolution TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_kb_moderation_reports_status CHECK (status IN ('open', 'resolved', 'dismissed'))
);

CREATE INDEX IF NOT EXISTS idx_kb_moderation_reports_collection ON kb_moderation_reports(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_moderation_reports_reporter ON kb_moderation_reports(reporter_id);
CREATE INDEX IF NOT EXISTS idx_kb_moderation_reports_status ON kb_moderation_reports(status);
