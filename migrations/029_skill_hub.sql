-- Skill Hub MVP registry, immutable versions, install library, and publisher restriction.

CREATE TABLE IF NOT EXISTS skills (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    skill_key TEXT NOT NULL UNIQUE,
    publisher_id UUID NOT NULL,
    name TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    icon_object_id UUID,
    homepage_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    visibility TEXT NOT NULL DEFAULT 'private',
    latest_version_id UUID,
    published_version_id UUID,
    CONSTRAINT chk_skills_status CHECK (status IN ('draft', 'published', 'suspended', 'archived', 'removed')),
    CONSTRAINT chk_skills_visibility CHECK (visibility IN ('private', 'public', 'unlisted'))
);
CREATE INDEX IF NOT EXISTS idx_skills_publisher ON skills(publisher_id);
CREATE INDEX IF NOT EXISTS idx_skills_status_visibility ON skills(status, visibility);
CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category);

CREATE TABLE IF NOT EXISTS skill_versions (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    skill_id UUID NOT NULL,
    version TEXT NOT NULL,
    manifest_hash TEXT NOT NULL,
    manifest_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    package_object_id UUID,
    instructions_object_id UUID,
    examples_object_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation_status TEXT NOT NULL DEFAULT 'pending',
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'draft',
    published_at TIMESTAMPTZ,
    CONSTRAINT chk_skill_versions_validation_status CHECK (validation_status IN ('pending', 'valid', 'invalid')),
    CONSTRAINT chk_skill_versions_status CHECK (status IN ('draft', 'published', 'rejected', 'archived', 'suspended'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_versions_skill_version ON skill_versions(skill_id, version);
CREATE INDEX IF NOT EXISTS idx_skill_versions_skill_status ON skill_versions(skill_id, status);
CREATE INDEX IF NOT EXISTS idx_skill_versions_hash ON skill_versions(manifest_hash);

CREATE TABLE IF NOT EXISTS skill_installations (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    version_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    install_source TEXT NOT NULL DEFAULT 'catalog',
    track_mode TEXT NOT NULL DEFAULT 'latest',
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    installed_at TIMESTAMPTZ NOT NULL,
    disabled_at TIMESTAMPTZ,
    CONSTRAINT chk_skill_installations_status CHECK (status IN ('active', 'disabled', 'uninstalled')),
    CONSTRAINT chk_skill_installations_track_mode CHECK (track_mode IN ('latest', 'pinned'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skill_installations_user_skill ON skill_installations(user_id, skill_id);
CREATE INDEX IF NOT EXISTS idx_skill_installations_user_status ON skill_installations(user_id, status);

CREATE TABLE IF NOT EXISTS skill_publisher_restrictions (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    publisher_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    reason TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL,
    lifted_by UUID,
    lifted_at TIMESTAMPTZ,
    CONSTRAINT chk_skill_publisher_restrictions_status CHECK (status IN ('active', 'lifted'))
);
CREATE INDEX IF NOT EXISTS idx_skill_publisher_restrictions_publisher_status ON skill_publisher_restrictions(publisher_id, status);
