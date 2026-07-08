-- GitHub App: columns for Manifest one-click install flow (nullable, additive)
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS webhook_secret TEXT;
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS client_id TEXT;
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS client_secret TEXT;
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS app_slug TEXT;

-- Issue -> Task sync rules (one-tenant-many-rules)
CREATE TABLE sync_rules (
    id TEXT NOT NULL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    repo TEXT NOT NULL,
    include_labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    exclude_labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    issue_state TEXT NOT NULL DEFAULT 'open',
    task_type TEXT NOT NULL,
    default_priority TEXT NOT NULL DEFAULT 'P2',
    dedupe_strategy TEXT NOT NULL DEFAULT 'update',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT sync_rules_issue_state_valid CHECK (issue_state IN ('open','closed','all')),
    CONSTRAINT sync_rules_dedupe_valid CHECK (dedupe_strategy IN ('update','skip'))
);
CREATE INDEX sync_rules_tenant_enabled_idx ON sync_rules (tenant_id, enabled);

-- Issue <-> Task mapping / dedupe (unique per tenant+repo+issue)
CREATE TABLE issue_task_map (
    tenant_id TEXT NOT NULL,
    repo TEXT NOT NULL,
    issue_number INTEGER NOT NULL,
    task_id TEXT NOT NULL,
    issue_state TEXT NOT NULL,
    issue_closed BOOLEAN NOT NULL DEFAULT FALSE,
    issue_url TEXT,
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, repo, issue_number)
);
CREATE INDEX issue_task_map_task_idx ON issue_task_map (tenant_id, task_id);
