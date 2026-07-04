CREATE TABLE reviews (
    tenant_id text NOT NULL,
    id text NOT NULL,
    submission_id text NOT NULL,
    reviewer_id text NOT NULL,
    rubric_version_id text NOT NULL,
    rubric_scores jsonb NOT NULL DEFAULT '[]'::jsonb,
    summary text,
    status text NOT NULL,
    final_decision text,
    submitted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT reviews_status_valid CHECK (status IN ('pending', 'submitted')),
    CONSTRAINT reviews_decision_valid CHECK (
        final_decision IS NULL OR final_decision IN ('accepted', 'rejected', 'revision_requested')
    )
);
CREATE UNIQUE INDEX reviews_one_per_submission ON reviews (tenant_id, submission_id);

CREATE TABLE line_comments (
    tenant_id text NOT NULL,
    id text NOT NULL,
    review_id text NOT NULL,
    submission_id text NOT NULL,
    file_path text NOT NULL,
    side text NOT NULL,
    line_number int NOT NULL,
    hunk_hash text NOT NULL,
    diff_fingerprint text NOT NULL,
    text text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id)
);
CREATE INDEX line_comments_review ON line_comments (tenant_id, review_id);

CREATE TABLE rubric_versions (
    tenant_id text NOT NULL,
    id text NOT NULL,
    version_number int NOT NULL,
    name text NOT NULL,
    dimensions jsonb NOT NULL,
    weights jsonb NOT NULL,
    algorithm_version text NOT NULL,
    is_active boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, version_number)
);
CREATE UNIQUE INDEX rubric_active_one_per_tenant ON rubric_versions (tenant_id) WHERE is_active;

CREATE TABLE reviewer_profiles (
    tenant_id text NOT NULL,
    id text NOT NULL,
    user_id text NOT NULL,
    capabilities text[] NOT NULL DEFAULT '{}',
    current_load int NOT NULL DEFAULT 0 CHECK (current_load >= 0),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, user_id)
);

CREATE TABLE reputation_projections (
    tenant_id text NOT NULL,
    id text NOT NULL,
    agent_version_id text NOT NULL,
    capability text NOT NULL,
    task_type text NOT NULL,
    total_reviews int NOT NULL DEFAULT 0,
    accepted_count int NOT NULL DEFAULT 0,
    rejected_count int NOT NULL DEFAULT 0,
    revision_requested_count int NOT NULL DEFAULT 0,
    pass_rate numeric(5,4),
    rework_rate numeric(5,4),
    avg_review_cost_cents bigint,
    avg_review_latency_ms bigint,
    sample_size_hint text NOT NULL DEFAULT 'low',
    algorithm_version text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, agent_version_id, capability, task_type)
);
