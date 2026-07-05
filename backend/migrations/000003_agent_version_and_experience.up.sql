ALTER TABLE agent_versions
    ADD COLUMN parent_version_id TEXT,
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN environment_digest TEXT NOT NULL DEFAULT '',
    ADD COLUMN prompt_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN skill_refs TEXT[] NOT NULL DEFAULT '{}'::text[],
    ADD COLUMN memory_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN tool_refs TEXT[] NOT NULL DEFAULT '{}'::text[],
    ADD COLUMN created_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN promoted_at TIMESTAMPTZ,
    ADD COLUMN retired_at TIMESTAMPTZ,
    ADD COLUMN rejected_reason TEXT;

UPDATE agent_versions
SET status = 'active',
    parent_version_id = NULL,
    version_number = 1
WHERE status = 'active' OR status IS NULL;

ALTER TABLE agent_versions
    ADD CONSTRAINT agent_versions_status_valid CHECK (
        status IN ('draft', 'evaluating', 'eligible', 'active', 'retired', 'rejected')
    );

ALTER TABLE agent_versions
    ADD CONSTRAINT agent_versions_parent_fk
        FOREIGN KEY (tenant_id, parent_version_id)
        REFERENCES agent_versions (tenant_id, id);

CREATE TABLE benchmark_sets (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    version_number INT NOT NULL CHECK (version_number >= 1),
    name TEXT NOT NULL,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT false,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, version_number)
);

CREATE TABLE benchmark_set_tasks (
    tenant_id TEXT NOT NULL,
    benchmark_set_id TEXT NOT NULL,
    task_ref TEXT NOT NULL,
    ordering INT NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, benchmark_set_id, task_ref),
    CONSTRAINT benchmark_set_tasks_fk
        FOREIGN KEY (tenant_id, benchmark_set_id) REFERENCES benchmark_sets (tenant_id, id)
);

CREATE INDEX idx_benchmark_set_tasks_set
    ON benchmark_set_tasks (tenant_id, benchmark_set_id);

CREATE TABLE evaluation_runs (
    id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL,
    benchmark_set_id TEXT NOT NULL,
    status TEXT NOT NULL,
    environment_digest TEXT NOT NULL DEFAULT '',
    scoring_rule_version TEXT NOT NULL DEFAULT '',
    threshold_results JSONB NOT NULL DEFAULT '{}',
    summary JSONB NOT NULL DEFAULT '{}',
    started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT evaluation_runs_status_valid CHECK (
        status IN ('running', 'passed', 'failed')
    ),
    CONSTRAINT evaluation_runs_version_fk
        FOREIGN KEY (tenant_id, agent_version_id) REFERENCES agent_versions (tenant_id, id),
    CONSTRAINT evaluation_runs_benchmark_fk
        FOREIGN KEY (tenant_id, benchmark_set_id) REFERENCES benchmark_sets (tenant_id, id)
);

CREATE INDEX idx_evaluation_runs_version
    ON evaluation_runs (tenant_id, agent_version_id, started_at DESC);

CREATE INDEX idx_evaluation_runs_status
    ON evaluation_runs (tenant_id, agent_version_id, status, started_at DESC);

CREATE TABLE evaluation_run_results (
    tenant_id TEXT NOT NULL,
    evaluation_run_id TEXT NOT NULL,
    task_ref TEXT NOT NULL,
    score DOUBLE PRECISION,
    passed BOOLEAN NOT NULL,
    details JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (tenant_id, evaluation_run_id, task_ref),
    CONSTRAINT evaluation_run_results_run_fk
        FOREIGN KEY (tenant_id, evaluation_run_id) REFERENCES evaluation_runs (tenant_id, id)
);

CREATE INDEX idx_evaluation_run_results_run
    ON evaluation_run_results (tenant_id, evaluation_run_id);
