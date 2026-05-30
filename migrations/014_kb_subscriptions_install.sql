-- AgentOS Backend — KB Hub consumer install/subscription slice

CREATE TABLE IF NOT EXISTS kb_subscriptions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    collection_id UUID NOT NULL REFERENCES kb_collections(id) ON DELETE CASCADE,
    snapshot_id UUID REFERENCES kb_snapshots(id) ON DELETE SET NULL,
    track_mode TEXT NOT NULL DEFAULT 'latest',
    pinned_version INTEGER,
    status TEXT NOT NULL DEFAULT 'active',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_kb_subscriptions_user_collection UNIQUE (user_id, collection_id),
    CONSTRAINT chk_kb_subscriptions_track_mode CHECK (track_mode IN ('latest', 'pinned')),
    CONSTRAINT chk_kb_subscriptions_status CHECK (status IN ('active', 'cancelled', 'expired')),
    CONSTRAINT chk_kb_subscriptions_pinned_version CHECK (pinned_version IS NULL OR pinned_version > 0)
);

ALTER TABLE kb_subscriptions ADD COLUMN IF NOT EXISTS snapshot_id UUID REFERENCES kb_snapshots(id) ON DELETE SET NULL;
ALTER TABLE kb_subscriptions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';

DO $$ BEGIN
    ALTER TABLE kb_subscriptions ADD CONSTRAINT chk_kb_subscriptions_track_mode CHECK (track_mode IN ('latest', 'pinned'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE kb_subscriptions ADD CONSTRAINT chk_kb_subscriptions_status CHECK (status IN ('active', 'cancelled', 'expired'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE kb_subscriptions ADD CONSTRAINT chk_kb_subscriptions_pinned_version CHECK (pinned_version IS NULL OR pinned_version > 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_subscriptions_user_collection ON kb_subscriptions(user_id, collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_user_id ON kb_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_collection_id ON kb_subscriptions(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_snapshot_id ON kb_subscriptions(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_track_mode ON kb_subscriptions(track_mode);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_status ON kb_subscriptions(status);
