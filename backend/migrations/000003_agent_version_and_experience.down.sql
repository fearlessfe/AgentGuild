DROP TABLE IF EXISTS evaluation_run_results;
DROP TABLE IF EXISTS evaluation_runs;
DROP TABLE IF EXISTS benchmark_set_tasks;
DROP TABLE IF EXISTS benchmark_sets;
DROP TABLE IF EXISTS experience_candidates;

ALTER TABLE IF EXISTS agent_versions
    DROP CONSTRAINT IF EXISTS agent_versions_parent_fk;

ALTER TABLE IF EXISTS agent_versions
    DROP CONSTRAINT IF EXISTS agent_versions_status_valid;

ALTER TABLE IF EXISTS agent_versions
    DROP COLUMN IF EXISTS parent_version_id,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS content_hash,
    DROP COLUMN IF EXISTS environment_digest,
    DROP COLUMN IF EXISTS prompt_ref,
    DROP COLUMN IF EXISTS skill_refs,
    DROP COLUMN IF EXISTS memory_ref,
    DROP COLUMN IF EXISTS tool_refs,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS promoted_at,
    DROP COLUMN IF EXISTS retired_at,
    DROP COLUMN IF EXISTS rejected_reason;
