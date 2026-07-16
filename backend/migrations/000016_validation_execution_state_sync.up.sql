ALTER TABLE validation_jobs
    ADD COLUMN execution_state_synced BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE executions DROP CONSTRAINT executions_status_valid;
ALTER TABLE executions ADD CONSTRAINT executions_status_valid CHECK (
    status IN (
        'leased', 'running', 'submitted', 'validating', 'validation_failed', 'reviewing',
        'revision_requested', 'accepted', 'rejected', 'expired', 'cancelled'
    )
);

CREATE INDEX validation_jobs_execution_sync_idx
    ON validation_jobs (tenant_id, created_at)
    WHERE status IN ('succeeded', 'failed') AND execution_state_synced = FALSE;
