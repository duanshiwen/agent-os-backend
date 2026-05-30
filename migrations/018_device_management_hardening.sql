-- AgentOS Backend — Stage 1 device management hardening

ALTER TABLE devices ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'active';
ALTER TABLE devices ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS revoked_by UUID REFERENCES users(id) ON DELETE SET NULL;

UPDATE devices SET status = 'active' WHERE status IS NULL OR status = '';

ALTER TABLE devices DROP CONSTRAINT IF EXISTS chk_devices_status;
ALTER TABLE devices ADD CONSTRAINT chk_devices_status CHECK (status IN ('active', 'revoked'));

CREATE INDEX IF NOT EXISTS idx_devices_user_status ON devices(user_id, status);
CREATE INDEX IF NOT EXISTS idx_devices_revoked_at ON devices(revoked_at);
