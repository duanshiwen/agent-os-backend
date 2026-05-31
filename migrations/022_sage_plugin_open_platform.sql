-- M4 SAGE Plugin Open Platform control plane.

CREATE TABLE IF NOT EXISTS sage_plugins (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    plugin_key TEXT NOT NULL UNIQUE,
    developer_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    icon_object_id UUID,
    homepage_url TEXT NOT NULL DEFAULT '',
    manifest_url TEXT NOT NULL DEFAULT '',
    flow_url TEXT NOT NULL DEFAULT '',
    callback_url TEXT NOT NULL DEFAULT '',
    health_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    review_status TEXT NOT NULL DEFAULT 'pending',
    visibility TEXT NOT NULL DEFAULT 'private',
    risk_level TEXT NOT NULL DEFAULT 'unknown',
    trust_level TEXT NOT NULL DEFAULT 'unverified',
    latest_version_id UUID,
    approved_version_id UUID,
    CONSTRAINT chk_sage_plugins_status CHECK (status IN ('draft', 'submitted', 'published', 'suspended', 'archived', 'removed')),
    CONSTRAINT chk_sage_plugins_review_status CHECK (review_status IN ('pending', 'under_review', 'approved', 'rejected', 'request_changes', 'takedown')),
    CONSTRAINT chk_sage_plugins_visibility CHECK (visibility IN ('private', 'public', 'unlisted', 'organization'))
);
CREATE INDEX IF NOT EXISTS idx_sage_plugins_developer ON sage_plugins(developer_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugins_status_review ON sage_plugins(status, review_status);
CREATE INDEX IF NOT EXISTS idx_sage_plugins_category ON sage_plugins(category);

CREATE TABLE IF NOT EXISTS sage_plugin_versions (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    plugin_id UUID NOT NULL,
    version TEXT NOT NULL,
    sage_version TEXT NOT NULL DEFAULT '1.0',
    manifest_hash TEXT NOT NULL,
    manifest_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_manifest_object_id UUID,
    validation_status TEXT NOT NULL DEFAULT 'pending',
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    risk_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    permission_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'draft',
    submitted_at TIMESTAMPTZ,
    approved_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    CONSTRAINT chk_sage_plugin_versions_validation_status CHECK (validation_status IN ('pending', 'valid', 'invalid')),
    CONSTRAINT chk_sage_plugin_versions_status CHECK (status IN ('draft', 'submitted', 'approved', 'rejected', 'archived'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sage_plugin_versions_plugin_version ON sage_plugin_versions(plugin_id, version);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_versions_plugin_status ON sage_plugin_versions(plugin_id, status);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_versions_hash ON sage_plugin_versions(manifest_hash);

CREATE TABLE IF NOT EXISTS sage_plugin_reviews (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    plugin_id UUID NOT NULL,
    version_id UUID NOT NULL,
    reviewer_id UUID NOT NULL,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    security_findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    privacy_findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    policy_findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    CONSTRAINT chk_sage_plugin_reviews_decision CHECK (decision IN ('approved', 'rejected', 'request_changes', 'suspended', 'takedown'))
);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_reviews_plugin ON sage_plugin_reviews(plugin_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_reviews_version ON sage_plugin_reviews(version_id);

CREATE TABLE IF NOT EXISTS sage_plugin_installations (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL,
    plugin_id UUID NOT NULL,
    version_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    install_source TEXT NOT NULL DEFAULT 'catalog',
    track_mode TEXT NOT NULL DEFAULT 'latest_approved',
    installed_at TIMESTAMPTZ NOT NULL,
    disabled_at TIMESTAMPTZ,
    CONSTRAINT chk_sage_plugin_installations_status CHECK (status IN ('active', 'disabled', 'uninstalled')),
    CONSTRAINT chk_sage_plugin_installations_track_mode CHECK (track_mode IN ('latest_approved', 'pinned', 'manual_upgrade'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sage_plugin_installations_user_plugin ON sage_plugin_installations(user_id, plugin_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_installations_user_status ON sage_plugin_installations(user_id, status);

CREATE TABLE IF NOT EXISTS sage_plugin_permission_grants (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    installation_id UUID NOT NULL,
    user_id UUID NOT NULL,
    plugin_id UUID NOT NULL,
    permission_key TEXT NOT NULL,
    risk_level TEXT NOT NULL DEFAULT 'low',
    status TEXT NOT NULL DEFAULT 'active',
    grant_scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    requires_confirmation BOOLEAN NOT NULL DEFAULT false,
    confirmation_id UUID,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT chk_sage_plugin_grants_risk CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    CONSTRAINT chk_sage_plugin_grants_status CHECK (status IN ('active', 'revoked', 'expired'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sage_plugin_grants_install_permission ON sage_plugin_permission_grants(installation_id, permission_key);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_grants_user_plugin ON sage_plugin_permission_grants(user_id, plugin_id);

CREATE TABLE IF NOT EXISTS sage_plugin_invocations (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL,
    device_id TEXT NOT NULL,
    plugin_id UUID NOT NULL,
    installation_id UUID,
    client_request_id TEXT NOT NULL,
    user_intent TEXT NOT NULL DEFAULT '',
    flow_id TEXT NOT NULL DEFAULT '',
    flow_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'created',
    risk_level TEXT NOT NULL DEFAULT 'unknown',
    policy_decision JSONB NOT NULL DEFAULT '{}'::jsonb,
    permissions_used JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    CONSTRAINT chk_sage_plugin_invocations_status CHECK (status IN ('created', 'running', 'completed', 'failed', 'cancelled'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sage_plugin_invocations_client_request ON sage_plugin_invocations(user_id, plugin_id, client_request_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_invocations_plugin_status ON sage_plugin_invocations(plugin_id, status);

CREATE TABLE IF NOT EXISTS sage_plugin_execution_reports (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    invocation_id UUID NOT NULL,
    client_report_id TEXT NOT NULL,
    status TEXT NOT NULL,
    flow_id TEXT NOT NULL DEFAULT '',
    steps_completed JSONB NOT NULL DEFAULT '[]'::jsonb,
    step_summaries JSONB NOT NULL DEFAULT '{}'::jsonb,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    user_confirmations JSONB NOT NULL DEFAULT '[]'::jsonb,
    plugin_callbacks JSONB NOT NULL DEFAULT '[]'::jsonb,
    tokens_used BIGINT NOT NULL DEFAULT 0,
    metering JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sage_plugin_execution_reports_invocation_client ON sage_plugin_execution_reports(invocation_id, client_report_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_execution_reports_status ON sage_plugin_execution_reports(status);

CREATE TABLE IF NOT EXISTS sage_plugin_usage_ledgers (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    developer_id UUID NOT NULL,
    plugin_id UUID NOT NULL,
    user_id UUID NOT NULL,
    invocation_id UUID NOT NULL,
    report_id UUID,
    event_type TEXT NOT NULL,
    quantity BIGINT NOT NULL DEFAULT 1,
    amount BIGINT NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'CNY',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_usage_ledgers_developer ON sage_plugin_usage_ledgers(developer_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_usage_ledgers_plugin ON sage_plugin_usage_ledgers(plugin_id);
CREATE INDEX IF NOT EXISTS idx_sage_plugin_usage_ledgers_invocation ON sage_plugin_usage_ledgers(invocation_id);
