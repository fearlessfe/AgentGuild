DROP INDEX IF EXISTS validation_jobs_execution_sync_idx;
ALTER TABLE validation_jobs DROP COLUMN IF EXISTS execution_state_synced;
UPDATE executions SET status='rejected' WHERE status='validation_failed';
ALTER TABLE executions DROP CONSTRAINT executions_status_valid;
ALTER TABLE executions ADD CONSTRAINT executions_status_valid CHECK (
    status IN (
        'leased', 'running', 'submitted', 'validating', 'reviewing',
        'revision_requested', 'accepted', 'rejected', 'expired', 'cancelled'
    )
);
