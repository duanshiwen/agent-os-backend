CREATE TABLE IF NOT EXISTS capability_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT 'low',
    status TEXT NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_capability_definitions_risk_level ON capability_definitions(risk_level);
CREATE INDEX IF NOT EXISTS idx_capability_definitions_status ON capability_definitions(status);

CREATE TABLE IF NOT EXISTS policy_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    capability_key TEXT NOT NULL DEFAULT '',
    subject_type TEXT NOT NULL DEFAULT '',
    subject_id TEXT NOT NULL DEFAULT '',
    actor_user_id UUID,
    risk_level TEXT NOT NULL DEFAULT '',
    effect TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 100,
    status TEXT NOT NULL DEFAULT 'active',
    conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_policy_rules_capability_key ON policy_rules(capability_key);
CREATE INDEX IF NOT EXISTS idx_policy_rules_subject_type ON policy_rules(subject_type);
CREATE INDEX IF NOT EXISTS idx_policy_rules_subject_id ON policy_rules(subject_id);
CREATE INDEX IF NOT EXISTS idx_policy_rules_actor_user_id ON policy_rules(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_policy_rules_risk_level ON policy_rules(risk_level);
CREATE INDEX IF NOT EXISTS idx_policy_rules_effect ON policy_rules(effect);
CREATE INDEX IF NOT EXISTS idx_policy_rules_priority ON policy_rules(priority);
CREATE INDEX IF NOT EXISTS idx_policy_rules_status ON policy_rules(status);

CREATE TABLE IF NOT EXISTS kill_switches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ,
    created_by UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_kill_switches_scope_type ON kill_switches(scope_type);
CREATE INDEX IF NOT EXISTS idx_kill_switches_scope_id ON kill_switches(scope_id);
CREATE INDEX IF NOT EXISTS idx_kill_switches_status ON kill_switches(status);
CREATE INDEX IF NOT EXISTS idx_kill_switches_expires_at ON kill_switches(expires_at);

CREATE TABLE IF NOT EXISTS policy_decisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_user_id UUID,
    actor_device_id TEXT NOT NULL DEFAULT '',
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL DEFAULT '',
    capability_key TEXT NOT NULL,
    risk_level TEXT NOT NULL DEFAULT 'low',
    decision TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    policy_rule_id UUID,
    kill_switch_id UUID,
    context JSONB NOT NULL DEFAULT '{}'::jsonb,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_policy_decisions_actor_user_id ON policy_decisions(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_policy_decisions_subject_type ON policy_decisions(subject_type);
CREATE INDEX IF NOT EXISTS idx_policy_decisions_subject_id ON policy_decisions(subject_id);
CREATE INDEX IF NOT EXISTS idx_policy_decisions_capability_key ON policy_decisions(capability_key);
CREATE INDEX IF NOT EXISTS idx_policy_decisions_decision ON policy_decisions(decision);
CREATE INDEX IF NOT EXISTS idx_policy_decisions_decided_at ON policy_decisions(decided_at DESC);

CREATE TABLE IF NOT EXISTS approval_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    policy_decision_id UUID NOT NULL,
    actor_user_id UUID NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL DEFAULT '',
    capability_key TEXT NOT NULL,
    decision TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    consumed_by TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_approval_receipts_policy_decision_id ON approval_receipts(policy_decision_id);
CREATE INDEX IF NOT EXISTS idx_approval_receipts_actor_user_id ON approval_receipts(actor_user_id);
CREATE INDEX IF NOT EXISTS idx_approval_receipts_subject_type ON approval_receipts(subject_type);
CREATE INDEX IF NOT EXISTS idx_approval_receipts_subject_id ON approval_receipts(subject_id);
CREATE INDEX IF NOT EXISTS idx_approval_receipts_capability_key ON approval_receipts(capability_key);
CREATE INDEX IF NOT EXISTS idx_approval_receipts_status ON approval_receipts(status);
CREATE INDEX IF NOT EXISTS idx_approval_receipts_expires_at ON approval_receipts(expires_at);

CREATE TABLE IF NOT EXISTS governance_scan_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL DEFAULT '',
    scanner TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    status TEXT NOT NULL DEFAULT 'open',
    message TEXT NOT NULL DEFAULT '',
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_at TIMESTAMPTZ,
    resolved_by UUID
);

CREATE INDEX IF NOT EXISTS idx_governance_scan_results_subject_type ON governance_scan_results(subject_type);
CREATE INDEX IF NOT EXISTS idx_governance_scan_results_subject_id ON governance_scan_results(subject_id);
CREATE INDEX IF NOT EXISTS idx_governance_scan_results_scanner ON governance_scan_results(scanner);
CREATE INDEX IF NOT EXISTS idx_governance_scan_results_severity ON governance_scan_results(severity);
CREATE INDEX IF NOT EXISTS idx_governance_scan_results_status ON governance_scan_results(status);
