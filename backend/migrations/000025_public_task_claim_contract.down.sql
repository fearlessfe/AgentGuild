DROP INDEX IF EXISTS executions_global_agent_created;

ALTER TABLE IF EXISTS executions
    DROP CONSTRAINT IF EXISTS executions_task_specification_nonempty,
    DROP CONSTRAINT IF EXISTS executions_global_agent_id_nonempty,
    DROP CONSTRAINT IF EXISTS executions_global_agent_version_fk,
    DROP COLUMN IF EXISTS task_specification_version_id,
    DROP COLUMN IF EXISTS agent_id;
