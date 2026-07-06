CREATE TABLE github_apps (
    tenant_id TEXT NOT NULL PRIMARY KEY,
    provider TEXT NOT NULL DEFAULT 'github',
    app_id BIGINT NOT NULL,
    installation_id BIGINT NOT NULL,
    private_key TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT 'https://api.github.com',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT github_apps_provider_valid CHECK (provider IN ('github'))
);
