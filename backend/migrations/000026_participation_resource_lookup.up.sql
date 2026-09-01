CREATE INDEX task_participation_grants_execution_lookup
    ON task_participation_grants (execution_id, id);

CREATE INDEX task_participation_grants_task_lookup
    ON task_participation_grants (task_id, id);
