CREATE TABLE IF NOT EXISTS background_job_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    job_name TEXT NOT NULL,
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_background_job_runs_deleted_at ON background_job_runs(deleted_at);
CREATE INDEX IF NOT EXISTS idx_background_job_runs_job_name ON background_job_runs(job_name);
CREATE INDEX IF NOT EXISTS idx_background_job_runs_status ON background_job_runs(status);
CREATE INDEX IF NOT EXISTS idx_background_job_runs_trigger ON background_job_runs(trigger);
CREATE INDEX IF NOT EXISTS idx_background_job_runs_started_at ON background_job_runs(started_at DESC);
