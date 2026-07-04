CREATE TABLE git_credentials (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL,
    base_commit_sha TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, execution_id)
);
