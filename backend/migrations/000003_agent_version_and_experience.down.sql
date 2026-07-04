ALTER TABLE agent_versions
    DROP CONSTRAINT IF EXISTS agent_versions_parent_fk;

ALTER TABLE agent_versions
    DROP CONSTRAINT IF EXISTS agent_versions_status_valid;

ALTER TABLE agent_versions
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
    DROP COLUMN IF EXISTS retired_at;
