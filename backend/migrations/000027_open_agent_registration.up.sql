CREATE TABLE agent_registration_challenges (
    id TEXT PRIMARY KEY,
    public_key BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    status TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    registered_agent_id TEXT,
    registered_version_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT agent_registration_challenges_status_valid CHECK (
        status IN ('pending', 'consumed')
    ),
    CONSTRAINT agent_registration_challenges_agent_fk
        FOREIGN KEY (registered_agent_id) REFERENCES agent_identities (id),
    CONSTRAINT agent_registration_challenges_version_fk
        FOREIGN KEY (registered_version_id) REFERENCES agent_identity_versions (id),
    CONSTRAINT agent_registration_challenges_result_valid CHECK (
        (status = 'pending' AND registered_agent_id IS NULL AND registered_version_id IS NULL)
        OR (status = 'consumed' AND registered_agent_id IS NOT NULL AND registered_version_id IS NOT NULL)
    )
);

CREATE INDEX idx_agent_registration_challenges_expires
    ON agent_registration_challenges (expires_at, status);

CREATE TABLE agent_identity_keys (
    agent_id TEXT NOT NULL,
    key_id TEXT NOT NULL,
    algorithm TEXT NOT NULL,
    thumbprint TEXT NOT NULL UNIQUE,
    public_key BYTEA NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY (agent_id, key_id),
    CONSTRAINT agent_identity_keys_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT agent_identity_keys_algorithm_valid CHECK (algorithm = 'Ed25519'),
    CONSTRAINT agent_identity_keys_status_valid CHECK (status IN ('active', 'revoked')),
    CONSTRAINT agent_identity_keys_revocation_valid CHECK (
        (status = 'active' AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX idx_agent_identity_keys_agent_status
    ON agent_identity_keys (agent_id, status);

CREATE TABLE agent_identity_events (
    id BIGSERIAL PRIMARY KEY,
    agent_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    intent TEXT NOT NULL,
    from_state TEXT,
    to_state TEXT,
    reason TEXT,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT agent_identity_events_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id)
);

CREATE INDEX idx_agent_identity_events_agent
    ON agent_identity_events (agent_id, created_at DESC);
