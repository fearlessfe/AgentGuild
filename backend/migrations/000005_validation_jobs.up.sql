CREATE TABLE validation_jobs (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    submission_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
    attempt INT NOT NULL DEFAULT 0,
    claimed_until TIMESTAMPTZ,
    claimed_by TEXT,
    config_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, submission_id)
);

CREATE INDEX idx_validation_jobs_pending ON validation_jobs (tenant_id, status, created_at)
    WHERE status IN ('pending', 'running');

CREATE TABLE validation_steps (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    job_id TEXT NOT NULL,
    step TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'skipped')),
    log_summary TEXT,
    resource_usage JSONB,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, job_id, step)
);

CREATE INDEX idx_validation_steps_job_id ON validation_steps (tenant_id, job_id);
