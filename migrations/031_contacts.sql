-- User-scoped contact book with sync tombstones.
CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id VARCHAR(128) NOT NULL,
    linked_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    agentos_pubkey VARCHAR(128),
    display_name TEXT NOT NULL,
    alias TEXT,
    avatar_url TEXT,
    phones JSONB NOT NULL DEFAULT '[]'::jsonb,
    emails JSONB NOT NULL DEFAULT '[]'::jsonb,
    labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    notes TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    version BIGINT NOT NULL DEFAULT 1,
    updated_by_device_id VARCHAR(128),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_contacts_user_contact UNIQUE (user_id, contact_id)
);
CREATE INDEX IF NOT EXISTS idx_contacts_user_status ON contacts(user_id, status);
CREATE INDEX IF NOT EXISTS idx_contacts_user_linked_user ON contacts(user_id, linked_user_id);
CREATE INDEX IF NOT EXISTS idx_contacts_user_agentos_pubkey ON contacts(user_id, agentos_pubkey);
CREATE INDEX IF NOT EXISTS idx_contacts_user_display_name ON contacts(user_id, display_name);
