ALTER TABLE agent_versions
    ADD COLUMN parent_version_id TEXT,
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN content_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN environment_digest TEXT NOT NULL DEFAULT '',
    ADD COLUMN prompt_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN skill_refs TEXT[] NOT NULL DEFAULT '{}'::text[],
    ADD COLUMN memory_ref TEXT NOT NULL DEFAULT '',
    ADD COLUMN tool_refs TEXT[] NOT NULL DEFAULT '{}'::text[],
    ADD COLUMN created_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN promoted_at TIMESTAMPTZ,
    ADD COLUMN retired_at TIMESTAMPTZ;

UPDATE agent_versions
SET status = 'active',
    parent_version_id = NULL,
    version_number = 1
WHERE status = 'active' OR status IS NULL;

ALTER TABLE agent_versions
    ADD CONSTRAINT agent_versions_status_valid CHECK (
        status IN ('draft', 'evaluating', 'eligible', 'active', 'retired', 'rejected')
    );

ALTER TABLE agent_versions
    ADD CONSTRAINT agent_versions_parent_fk
        FOREIGN KEY (tenant_id, parent_version_id)
        REFERENCES agent_versions (tenant_id, id);
