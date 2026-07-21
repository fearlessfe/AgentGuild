CREATE TABLE agent_contribution_projections (
    agent_id TEXT NOT NULL,
    algorithm_version TEXT NOT NULL,
    attempt_count INT NOT NULL CHECK (attempt_count >= 0),
    ci_passed_count INT NOT NULL CHECK (ci_passed_count >= 0),
    ci_failed_count INT NOT NULL CHECK (ci_failed_count >= 0),
    reviewed_count INT NOT NULL CHECK (reviewed_count >= 0),
    changes_requested_count INT NOT NULL CHECK (changes_requested_count >= 0),
    approved_count INT NOT NULL CHECK (approved_count >= 0),
    merged_count INT NOT NULL CHECK (merged_count >= 0),
    closed_count INT NOT NULL CHECK (closed_count >= 0),
    reverted_count INT NOT NULL CHECK (reverted_count >= 0),
    issue_reopened_count INT NOT NULL CHECK (issue_reopened_count >= 0),
    quality_sample_size INT NOT NULL CHECK (quality_sample_size >= 0),
    sample_size_hint TEXT NOT NULL,
    stable_merge_rate DOUBLE PRECISION NOT NULL CHECK (stable_merge_rate BETWEEN 0 AND 1),
    ci_pass_rate DOUBLE PRECISION NOT NULL CHECK (ci_pass_rate BETWEEN 0 AND 1),
    approval_rate DOUBLE PRECISION NOT NULL CHECK (approval_rate BETWEEN 0 AND 1),
    repository_distribution JSONB NOT NULL DEFAULT '{}'::jsonb,
    version_distribution JSONB NOT NULL DEFAULT '{}'::jsonb,
    duplicate_task_attempts INT NOT NULL CHECK (duplicate_task_attempts >= 0),
    self_owned_repository_count INT NOT NULL CHECK (self_owned_repository_count >= 0),
    without_independent_feedback INT NOT NULL CHECK (without_independent_feedback >= 0),
    reverted_after_merge INT NOT NULL CHECK (reverted_after_merge >= 0),
    issue_reopened_after_merge INT NOT NULL CHECK (issue_reopened_after_merge >= 0),
    dominant_repository_share DOUBLE PRECISION NOT NULL CHECK (dominant_repository_share BETWEEN 0 AND 1),
    latest_event_id BIGINT NOT NULL CHECK (latest_event_id >= 0),
    calculated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (agent_id, algorithm_version),
    CONSTRAINT agent_contribution_projections_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT agent_contribution_projections_sample_hint_valid CHECK (
        sample_size_hint IN ('unverified', 'low', 'medium', 'high')
    ),
    CONSTRAINT agent_contribution_projections_distribution_objects CHECK (
        jsonb_typeof(repository_distribution) = 'object'
        AND jsonb_typeof(version_distribution) = 'object'
    )
);

CREATE TABLE agent_version_performance_projections (
    agent_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL,
    algorithm_version TEXT NOT NULL,
    attempt_count INT NOT NULL CHECK (attempt_count >= 0),
    ci_passed_count INT NOT NULL CHECK (ci_passed_count >= 0),
    ci_failed_count INT NOT NULL CHECK (ci_failed_count >= 0),
    reviewed_count INT NOT NULL CHECK (reviewed_count >= 0),
    changes_requested_count INT NOT NULL CHECK (changes_requested_count >= 0),
    approved_count INT NOT NULL CHECK (approved_count >= 0),
    merged_count INT NOT NULL CHECK (merged_count >= 0),
    closed_count INT NOT NULL CHECK (closed_count >= 0),
    reverted_count INT NOT NULL CHECK (reverted_count >= 0),
    issue_reopened_count INT NOT NULL CHECK (issue_reopened_count >= 0),
    quality_sample_size INT NOT NULL CHECK (quality_sample_size >= 0),
    sample_size_hint TEXT NOT NULL,
    stable_merge_rate DOUBLE PRECISION NOT NULL CHECK (stable_merge_rate BETWEEN 0 AND 1),
    ci_pass_rate DOUBLE PRECISION NOT NULL CHECK (ci_pass_rate BETWEEN 0 AND 1),
    approval_rate DOUBLE PRECISION NOT NULL CHECK (approval_rate BETWEEN 0 AND 1),
    repository_distribution JSONB NOT NULL DEFAULT '{}'::jsonb,
    duplicate_task_attempts INT NOT NULL CHECK (duplicate_task_attempts >= 0),
    self_owned_repository_count INT NOT NULL CHECK (self_owned_repository_count >= 0),
    without_independent_feedback INT NOT NULL CHECK (without_independent_feedback >= 0),
    reverted_after_merge INT NOT NULL CHECK (reverted_after_merge >= 0),
    issue_reopened_after_merge INT NOT NULL CHECK (issue_reopened_after_merge >= 0),
    dominant_repository_share DOUBLE PRECISION NOT NULL CHECK (dominant_repository_share BETWEEN 0 AND 1),
    latest_event_id BIGINT NOT NULL CHECK (latest_event_id >= 0),
    calculated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (agent_version_id, algorithm_version),
    CONSTRAINT agent_version_performance_projections_version_fk
        FOREIGN KEY (agent_id, agent_version_id)
        REFERENCES agent_identity_versions (agent_id, id),
    CONSTRAINT agent_version_performance_projections_sample_hint_valid CHECK (
        sample_size_hint IN ('unverified', 'low', 'medium', 'high')
    ),
    CONSTRAINT agent_version_performance_projections_distribution_object CHECK (
        jsonb_typeof(repository_distribution) = 'object'
    )
);

CREATE INDEX agent_version_performance_agent_algorithm
    ON agent_version_performance_projections (agent_id, algorithm_version, agent_version_id);
