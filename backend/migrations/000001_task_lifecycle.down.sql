DROP TABLE IF EXISTS execution_usage;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS task_events;
DROP TABLE IF EXISTS idempotency_records;
ALTER TABLE IF EXISTS tasks DROP CONSTRAINT IF EXISTS tasks_active_execution_fk;
DROP TABLE IF EXISTS executions;
DROP TABLE IF EXISTS tasks;
