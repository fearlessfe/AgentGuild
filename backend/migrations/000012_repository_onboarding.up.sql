CREATE TABLE onboarded_repositories (
    tenant_id text NOT NULL,
    id text NOT NULL,
    source_type text NOT NULL CHECK (source_type IN ('github_app', 'public_github')),
    full_name text NOT NULL,
    default_branch text NOT NULL,
    visibility text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, source_type, full_name)
);

CREATE INDEX onboarded_repositories_tenant_source_idx
    ON onboarded_repositories (tenant_id, source_type, full_name);
