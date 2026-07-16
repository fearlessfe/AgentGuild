ALTER TABLE validation_jobs
    ADD COLUMN execution_id TEXT;

UPDATE validation_jobs v
SET execution_id = s.execution_id
FROM submissions s
WHERE s.tenant_id = v.tenant_id
  AND s.id = v.submission_id;

ALTER TABLE validation_jobs
    ALTER COLUMN execution_id SET NOT NULL;

ALTER TABLE validation_jobs
    ADD CONSTRAINT validation_jobs_submission_fk
    FOREIGN KEY (tenant_id, submission_id)
    REFERENCES submissions (tenant_id, id);

ALTER TABLE validation_steps
    ADD COLUMN hard_gate BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE validation_steps
SET hard_gate = TRUE
WHERE step IN ('build', 'public_tests', 'hidden_tests', 'security_scan');
