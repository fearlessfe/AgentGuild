CREATE TABLE agents (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    owner_email TEXT NOT NULL,
    team TEXT,
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL,
    current_version_id TEXT,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    repo_scope TEXT[] NOT NULL DEFAULT '{}',
    budget_cents BIGINT NOT NULL DEFAULT 0 CHECK (budget_cents >= 0),
    budget_currency TEXT,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT agents_status_valid CHECK (
        status IN ('pending_activation', 'active', 'suspended', 'revoked')
    )
);

CREATE TABLE agent_versions (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    version_number INT NOT NULL CHECK (version_number >= 1),
    runtime TEXT NOT NULL,
    model TEXT NOT NULL,
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    config_fingerprint TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, agent_id, version_number),
    CONSTRAINT agent_versions_agent_fk
        FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id)
);

ALTER TABLE agents
    ADD CONSTRAINT agents_current_version_fk
    FOREIGN KEY (tenant_id, current_version_id)
    REFERENCES agent_versions (tenant_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE activation_credentials (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    hash BYTEA NOT NULL,
    status TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, agent_id),
    CONSTRAINT activation_credentials_agent_fk
        FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id),
    CONSTRAINT activation_credentials_status_valid CHECK (
        status IN ('pending', 'consumed', 'expired')
    )
);

CREATE TABLE identity_events (
    id BIGSERIAL NOT NULL,
    tenant_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    intent TEXT NOT NULL,
    from_state TEXT,
    to_state TEXT,
    reason TEXT,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT identity_events_agent_fk
        FOREIGN KEY (tenant_id, agent_id) REFERENCES agents (tenant_id, id),
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX idx_identity_events_agent
    ON identity_events (tenant_id, agent_id, created_at DESC);
