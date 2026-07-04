CREATE TABLE submissions (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    base_commit_sha TEXT NOT NULL,
    summary TEXT NOT NULL,
    test_declaration TEXT,
    evidence JSONB,
    diff_fingerprint TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending_verification', 'validated', 'validation_failed', 'invalid')),
    validation_job_id TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX idx_submissions_execution_id ON submissions (tenant_id, execution_id);
CREATE INDEX idx_submissions_task_id ON submissions (tenant_id, task_id);
