-- AgentOS Backend — Server Admission Policy

CREATE TABLE IF NOT EXISTS server_admissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id VARCHAR(128) UNIQUE NOT NULL,
    policy_type VARCHAR(32) NOT NULL,
    invitation_code_hash TEXT,
    admin_approval_required BOOLEAN DEFAULT FALSE,
    updated_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT chk_server_admissions_policy_type CHECK (policy_type IN ('protocol', 'invitation', 'approval'))
);

CREATE INDEX IF NOT EXISTS idx_server_admissions_server_id ON server_admissions(server_id);
