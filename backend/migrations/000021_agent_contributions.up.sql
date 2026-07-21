-- Contribution 固化全局 Agent/Version 与 sponsor-owned Task/Execution 的归属；
-- provider 事实进入 append-only event log，供后续算法版本化投影重放。

ALTER TABLE agent_identity_versions
    ADD CONSTRAINT agent_identity_versions_agent_id_id_key
    UNIQUE (agent_id, id);

CREATE TABLE contributions (
    id TEXT PRIMARY KEY,
    resource_tenant_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    task_specification_version_id TEXT NOT NULL,
    canonical_repository TEXT NOT NULL,
    self_owned_repository BOOLEAN NOT NULL DEFAULT FALSE,
    issue_number BIGINT NOT NULL CHECK (issue_number > 0),
    issue_url TEXT NOT NULL,
    provider TEXT NOT NULL,
    pull_request_number BIGINT NOT NULL CHECK (pull_request_number > 0),
    pull_request_url TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    attribution_status TEXT NOT NULL,
    outcome TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT contributions_agent_version_fk
        FOREIGN KEY (agent_id, agent_version_id)
        REFERENCES agent_identity_versions (agent_id, id),
    CONSTRAINT contributions_task_fk
        FOREIGN KEY (resource_tenant_id, task_id)
        REFERENCES tasks (tenant_id, id),
    CONSTRAINT contributions_execution_fk
        FOREIGN KEY (resource_tenant_id, execution_id, task_id)
        REFERENCES executions (tenant_id, id, task_id),
    CONSTRAINT contributions_id_provider_key UNIQUE (id, provider),
    CONSTRAINT contributions_pull_request_key
        UNIQUE (provider, canonical_repository, pull_request_number),
    CONSTRAINT contributions_provider_valid CHECK (
        provider IN ('github', 'gitlab', 'gitee')
    ),
    CONSTRAINT contributions_attribution_status_valid CHECK (
        attribution_status IN ('pending_verification', 'verified', 'rejected')
    ),
    CONSTRAINT contributions_outcome_valid CHECK (
        outcome IN (
            'attempt', 'ci_passed', 'ci_failed', 'reviewed',
            'changes_requested', 'approved', 'merged', 'closed',
            'reverted', 'issue_reopened'
        )
    ),
    CONSTRAINT contributions_commit_sha_valid CHECK (
        commit_sha ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'
    ),
    CONSTRAINT contributions_references_nonempty CHECK (
        canonical_repository <> ''
        AND issue_url <> ''
        AND pull_request_url <> ''
        AND task_specification_version_id <> ''
    )
);

CREATE INDEX contributions_agent_created
    ON contributions (agent_id, created_at DESC, id);

CREATE INDEX contributions_agent_version_created
    ON contributions (agent_version_id, created_at DESC, id);

CREATE INDEX contributions_resource_execution
    ON contributions (resource_tenant_id, execution_id, created_at, id);

-- task_specification_versions is introduced by the analysis/specification
-- migration. This non-null binding is intentionally created first; that later
-- migration adds the referential constraint once its table exists.

CREATE TABLE contribution_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    contribution_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_delivery_id TEXT,
    object_version TEXT,
    event_type TEXT NOT NULL,
    outcome TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT contribution_events_contribution_provider_fk
        FOREIGN KEY (contribution_id, provider)
        REFERENCES contributions (id, provider),
    CONSTRAINT contribution_events_provider_valid CHECK (
        provider IN ('github', 'gitlab', 'gitee')
    ),
    CONSTRAINT contribution_events_type_valid CHECK (
        event_type IN (
            'pr_opened', 'pr_synchronized', 'commit', 'ci', 'review',
            'changes_requested', 'approved', 'merged', 'closed',
            'reverted', 'issue_reopened'
        )
    ),
    CONSTRAINT contribution_events_outcome_valid CHECK (
        outcome IN (
            'attempt', 'ci_passed', 'ci_failed', 'reviewed',
            'changes_requested', 'approved', 'merged', 'closed',
            'reverted', 'issue_reopened'
        )
    ),
    CONSTRAINT contribution_events_commit_sha_valid CHECK (
        commit_sha ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'
    ),
    CONSTRAINT contribution_events_payload_object CHECK (
        jsonb_typeof(payload) = 'object'
    ),
    CONSTRAINT contribution_events_idempotency_source CHECK (
        NULLIF(provider_delivery_id, '') IS NOT NULL
        OR NULLIF(object_version, '') IS NOT NULL
    )
);

CREATE UNIQUE INDEX contribution_events_delivery_key
    ON contribution_events (provider, provider_delivery_id)
    WHERE provider_delivery_id IS NOT NULL AND provider_delivery_id <> '';

CREATE UNIQUE INDEX contribution_events_object_version_key
    ON contribution_events (contribution_id, provider, event_type, object_version)
    WHERE object_version IS NOT NULL AND object_version <> '';

CREATE INDEX contribution_events_contribution_order
    ON contribution_events (contribution_id, occurred_at, id);

CREATE FUNCTION reject_contribution_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'contribution_events are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER contribution_events_reject_update
    BEFORE UPDATE ON contribution_events
    FOR EACH ROW EXECUTE FUNCTION reject_contribution_event_mutation();

CREATE TRIGGER contribution_events_reject_delete
    BEFORE DELETE ON contribution_events
    FOR EACH ROW EXECUTE FUNCTION reject_contribution_event_mutation();
