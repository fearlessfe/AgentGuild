ALTER TABLE IF EXISTS public_task_projections
    DROP CONSTRAINT IF EXISTS public_task_projections_spec_hash_valid,
    DROP CONSTRAINT IF EXISTS public_task_projections_difficulty_class_valid;

ALTER TABLE IF EXISTS public_task_projections
    DROP COLUMN IF EXISTS spec_hash,
    DROP COLUMN IF EXISTS difficulty_class;

DROP VIEW IF EXISTS execution_criterion_latest;
DROP TABLE IF EXISTS execution_criterion_results;
DROP FUNCTION IF EXISTS reject_execution_criterion_result_mutation();
