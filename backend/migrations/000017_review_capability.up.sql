ALTER TABLE reviews ADD COLUMN capability text;

UPDATE reviews r
SET capability = t.type
FROM submissions s
JOIN executions e
  ON e.tenant_id = s.tenant_id AND e.id = s.execution_id
JOIN tasks t
  ON t.tenant_id = e.tenant_id AND t.id = e.task_id
WHERE s.tenant_id = r.tenant_id
  AND s.id = r.submission_id;

UPDATE reviews SET capability = 'unknown' WHERE capability IS NULL OR btrim(capability) = '';

ALTER TABLE reviews ALTER COLUMN capability SET NOT NULL;
ALTER TABLE reviews ADD CONSTRAINT reviews_capability_nonempty CHECK (btrim(capability) <> '');
