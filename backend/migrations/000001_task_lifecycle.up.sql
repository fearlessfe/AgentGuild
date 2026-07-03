CREATE TABLE tasks (
    tenant_id text NOT NULL,
    id text NOT NULL,
    publisher_agent_version_id text NOT NULL,
    type text NOT NULL,
    title text NOT NULL,
    problem text NOT NULL,
    constraints jsonb NOT NULL DEFAULT '[]'::jsonb,
    requirements jsonb NOT NULL DEFAULT '[]'::jsonb,
    deadline timestamptz NOT NULL,
    status text NOT NULL,
    state_version bigint NOT NULL DEFAULT 0 CHECK (state_version >= 0),
    active_execution_id text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT tasks_status_valid CHECK (
        status IN ('draft', 'open', 'claimed', 'in_progress', 'completed', 'cancelled', 'expired')
    )
);

CREATE TABLE executions (
    tenant_id text NOT NULL,
    id text NOT NULL,
    task_id text NOT NULL,
    agent_version_id text NOT NULL,
    status text NOT NULL,
    state_version bigint NOT NULL DEFAULT 0 CHECK (state_version >= 0),
    lease_secret_hash bytea,
    lease_generation bigint NOT NULL CHECK (lease_generation >= 0),
    lease_soft_expires_at timestamptz,
    lease_hard_expires_at timestamptz,
    stage text,
    progress double precision CHECK (progress IS NULL OR progress BETWEEN 0 AND 1),
    last_heartbeat_at timestamptz,
    claimed_at timestamptz,
    started_at timestamptz,
    submitted_at timestamptz,
    expired_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT executions_tenant_id_task_id_key
        UNIQUE (tenant_id, id, task_id),
    CONSTRAINT executions_task_fk
        FOREIGN KEY (tenant_id, task_id) REFERENCES tasks (tenant_id, id),
    CONSTRAINT executions_status_valid CHECK (
        status IN (
            'leased', 'running', 'submitted', 'validating', 'reviewing',
            'revision_requested', 'accepted', 'rejected', 'expired', 'cancelled'
        )
    ),
    CONSTRAINT executions_lease_expiry_order CHECK (
        lease_soft_expires_at IS NULL
        OR lease_hard_expires_at IS NULL
        OR lease_soft_expires_at <= lease_hard_expires_at
    )
);

CREATE UNIQUE INDEX executions_one_active_per_task
    ON executions (tenant_id, task_id)
    WHERE status IN (
        'leased', 'running', 'submitted', 'validating', 'reviewing', 'revision_requested'
    );

ALTER TABLE tasks
    ADD CONSTRAINT tasks_active_execution_fk
    FOREIGN KEY (tenant_id, active_execution_id, id)
    REFERENCES executions (tenant_id, id, task_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE idempotency_records (
    tenant_id text NOT NULL,
    actor_id text NOT NULL,
    operation text NOT NULL,
    request_id text NOT NULL,
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    owner_token text NOT NULL,
    response_code integer,
    response_body bytea,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE UNIQUE INDEX idempotency_scope_key
    ON idempotency_records (tenant_id, actor_id, operation, request_id);

CREATE TABLE task_events (
    id bigint GENERATED ALWAYS AS IDENTITY,
    tenant_id text NOT NULL,
    task_id text NOT NULL,
    execution_id text,
    actor_type text NOT NULL,
    actor_id text NOT NULL,
    intent text NOT NULL,
    from_state text NOT NULL,
    to_state text NOT NULL,
    reason text,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT task_events_task_fk
        FOREIGN KEY (tenant_id, task_id) REFERENCES tasks (tenant_id, id),
    CONSTRAINT task_events_execution_fk
        FOREIGN KEY (tenant_id, execution_id, task_id)
        REFERENCES executions (tenant_id, id, task_id)
);

CREATE INDEX task_events_tenant_task_created
    ON task_events (tenant_id, task_id, created_at, id);

CREATE TABLE outbox_events (
    tenant_id text NOT NULL,
    id text NOT NULL,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    payload jsonb NOT NULL,
    available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    claimed_until timestamptz,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX outbox_events_ready
    ON outbox_events (available_at, created_at)
    WHERE published_at IS NULL;

CREATE TABLE execution_usage (
    tenant_id text NOT NULL,
    task_id text NOT NULL,
    execution_id text NOT NULL,
    agent_version_id text NOT NULL,
    observed_cost numeric(20, 8),
    self_reported_cost numeric(20, 8),
    coverage text NOT NULL,
    provider text NOT NULL,
    source_cursor text NOT NULL,
    observed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, execution_id, provider, source_cursor),
    CONSTRAINT execution_usage_task_fk
        FOREIGN KEY (tenant_id, task_id) REFERENCES tasks (tenant_id, id),
    CONSTRAINT execution_usage_execution_fk
        FOREIGN KEY (tenant_id, execution_id, task_id)
        REFERENCES executions (tenant_id, id, task_id),
    CONSTRAINT execution_usage_coverage_valid
        CHECK (coverage IN ('complete', 'partial', 'unavailable')),
    CONSTRAINT execution_usage_cost_nonnegative
        CHECK (
            (observed_cost IS NULL OR observed_cost >= 0)
            AND (self_reported_cost IS NULL OR self_reported_cost >= 0)
        )
);
