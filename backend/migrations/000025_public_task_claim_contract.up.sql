-- Public claims keep the legacy agent_version_id column as the actual
-- immutable version binding while adding the stable global Agent identity and
-- the immutable Task Specification Version used by the execution. The new
-- columns remain nullable for legacy tenant-scoped executions during rollout.

ALTER TABLE executions
    ADD COLUMN agent_id TEXT,
    ADD COLUMN task_specification_version_id TEXT;

ALTER TABLE executions
    ADD CONSTRAINT executions_global_agent_version_fk
        FOREIGN KEY (agent_id, agent_version_id)
        REFERENCES agent_identity_versions (agent_id, id),
    ADD CONSTRAINT executions_global_agent_id_nonempty
        CHECK (agent_id IS NULL OR agent_id <> ''),
    ADD CONSTRAINT executions_task_specification_nonempty
        CHECK (
            task_specification_version_id IS NULL
            OR task_specification_version_id <> ''
        );

CREATE INDEX executions_global_agent_created
    ON executions (agent_id, created_at DESC, id)
    WHERE agent_id IS NOT NULL;
