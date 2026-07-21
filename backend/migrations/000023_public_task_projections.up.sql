CREATE TABLE public_task_projections (
    id TEXT PRIMARY KEY,
    resource_tenant_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    task_specification_version_id TEXT NOT NULL,
    canonical_repository TEXT NOT NULL,
    source_issue_url TEXT NOT NULL,
    issue_revision TEXT NOT NULL,
    base_commit TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    problem_diagnosis TEXT NOT NULL,
    impact TEXT NOT NULL,
    proposed_solution TEXT NOT NULL,
    implementation_steps JSONB NOT NULL DEFAULT '[]'::jsonb,
    constraints JSONB NOT NULL DEFAULT '[]'::jsonb,
    non_goals JSONB NOT NULL DEFAULT '[]'::jsonb,
    risks JSONB NOT NULL DEFAULT '[]'::jsonb,
    acceptance_criteria JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    quality_level TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'published',
    published_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revocation_actor TEXT,
    revocation_reason TEXT,
    UNIQUE (resource_tenant_id, task_id),
    CONSTRAINT public_task_projections_task_fk
        FOREIGN KEY (resource_tenant_id, task_id) REFERENCES tasks (tenant_id, id),
    CONSTRAINT public_task_projections_status_valid CHECK (
        status IN ('published', 'revoked')
    ),
    CONSTRAINT public_task_projections_quality_level_valid CHECK (
        quality_level IN ('standard', 'high_assurance', 'human_reviewed')
    ),
    CONSTRAINT public_task_projections_commit_valid CHECK (
        base_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'
    ),
    CONSTRAINT public_task_projections_nonempty CHECK (
        task_specification_version_id <> '' AND canonical_repository <> ''
        AND source_issue_url <> '' AND issue_revision <> '' AND title <> ''
        AND summary <> '' AND problem_diagnosis <> '' AND impact <> ''
        AND proposed_solution <> ''
    ),
    CONSTRAINT public_task_projections_arrays CHECK (
        jsonb_typeof(implementation_steps) = 'array'
        AND jsonb_typeof(constraints) = 'array'
        AND jsonb_typeof(non_goals) = 'array'
        AND jsonb_typeof(risks) = 'array'
        AND jsonb_typeof(acceptance_criteria) = 'array'
        AND jsonb_typeof(evidence_refs) = 'array'
    ),
    CONSTRAINT public_task_projections_revocation_valid CHECK (
        (status = 'published' AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL
            AND NULLIF(revocation_actor, '') IS NOT NULL
            AND NULLIF(revocation_reason, '') IS NOT NULL)
    )
);

CREATE INDEX public_task_projections_catalog
    ON public_task_projections (published_at DESC, id)
    WHERE status='published';

-- The analysis migration adds the FK to task_specification_versions once that
-- table exists. Keeping the binding non-null now prevents an unversioned public
-- projection during the staged rollout.
