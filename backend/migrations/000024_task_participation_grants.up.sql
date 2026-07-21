CREATE TABLE task_participation_grants (
    id TEXT PRIMARY KEY,
    resource_tenant_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL,
    scopes TEXT[] NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revocation_actor TEXT,
    revocation_reason TEXT,
    UNIQUE (resource_tenant_id, execution_id),
    CONSTRAINT task_participation_grants_task_fk
        FOREIGN KEY (resource_tenant_id, task_id) REFERENCES tasks (tenant_id, id),
    CONSTRAINT task_participation_grants_execution_fk
        FOREIGN KEY (resource_tenant_id, execution_id, task_id)
        REFERENCES executions (tenant_id, id, task_id),
    CONSTRAINT task_participation_grants_agent_version_fk
        FOREIGN KEY (agent_id, agent_version_id)
        REFERENCES agent_identity_versions (agent_id, id),
    CONSTRAINT task_participation_grants_status_valid CHECK (
        status IN ('active', 'revoked', 'expired')
    ),
    CONSTRAINT task_participation_grants_scopes_valid CHECK (
        cardinality(scopes) > 0
        AND scopes <@ ARRAY[
            'task:read', 'execution:read', 'execution:write',
            'submission:create', 'review:read', 'git:write'
        ]::TEXT[]
    ),
    CONSTRAINT task_participation_grants_expiry_valid CHECK (expires_at > created_at),
    CONSTRAINT task_participation_grants_revocation_valid CHECK (
        (status = 'active' AND revoked_at IS NULL)
        OR (status = 'expired' AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL
            AND NULLIF(revocation_actor, '') IS NOT NULL
            AND NULLIF(revocation_reason, '') IS NOT NULL)
    )
);

CREATE INDEX task_participation_grants_agent_active
    ON task_participation_grants (agent_id, agent_version_id, expires_at)
    WHERE status='active';

CREATE TABLE task_participation_grant_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    grant_id TEXT,
    event_type TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL,
    resource_tenant_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    scope TEXT,
    reason_code TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT task_participation_grant_events_grant_fk
        FOREIGN KEY (grant_id) REFERENCES task_participation_grants (id),
    CONSTRAINT task_participation_grant_events_type_valid CHECK (
        event_type IN ('created', 'allowed', 'denied', 'renewed', 'revoked', 'expired')
    ),
    CONSTRAINT task_participation_grant_events_actor_valid CHECK (
        actor_type IN ('agent', 'system', 'human')
    ),
    CONSTRAINT task_participation_grant_events_scope_valid CHECK (
        scope IS NULL OR scope IN (
            'task:read', 'execution:read', 'execution:write',
            'submission:create', 'review:read', 'git:write'
        )
    ),
    CONSTRAINT task_participation_grant_events_metadata_object CHECK (
        jsonb_typeof(metadata) = 'object'
    )
);

CREATE INDEX task_participation_grant_events_task_order
    ON task_participation_grant_events (resource_tenant_id, task_id, created_at, id);

CREATE INDEX task_participation_grant_events_grant_order
    ON task_participation_grant_events (grant_id, created_at, id);

CREATE FUNCTION reject_task_participation_grant_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'task_participation_grant_events are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER task_participation_grant_events_reject_update
    BEFORE UPDATE ON task_participation_grant_events
    FOR EACH ROW EXECUTE FUNCTION reject_task_participation_grant_event_mutation();

CREATE TRIGGER task_participation_grant_events_reject_delete
    BEFORE DELETE ON task_participation_grant_events
    FOR EACH ROW EXECUTE FUNCTION reject_task_participation_grant_event_mutation();
