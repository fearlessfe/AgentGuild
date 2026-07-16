ALTER TABLE validation_steps
    DROP COLUMN hard_gate;

ALTER TABLE validation_jobs
    DROP CONSTRAINT validation_jobs_submission_fk;

ALTER TABLE validation_jobs
    DROP COLUMN execution_id;
