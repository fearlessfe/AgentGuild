-- 平台级 Agent Identity 与现有 tenant-scoped Agent 并行存在于迁移窗口。
-- 旧表继续服务现有调用方；确定性 mapping 允许后续安全双写、核对与回滚。

CREATE TABLE IF NOT EXISTS agent_identities (
    id TEXT PRIMARY KEY,
    handle TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    current_version_id TEXT,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT agent_identities_status_valid CHECK (
        status IN ('pending_activation', 'active', 'suspended', 'revoked')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_identities_handle_ci
    ON agent_identities (lower(handle));

CREATE TABLE IF NOT EXISTS legacy_agent_identity_mappings (
    tenant_id TEXT NOT NULL,
    legacy_agent_id TEXT NOT NULL,
    agent_id TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, legacy_agent_id),
    CONSTRAINT legacy_agent_identity_mapping_legacy_fk
        FOREIGN KEY (tenant_id, legacy_agent_id) REFERENCES agents (tenant_id, id),
    CONSTRAINT legacy_agent_identity_mapping_global_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id)
);

CREATE TABLE IF NOT EXISTS agent_organization_memberships (
    agent_id TEXT NOT NULL,
    organization_id TEXT NOT NULL,
    operator_id TEXT NOT NULL,
    operator_email TEXT NOT NULL,
    team TEXT,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    budget_cents BIGINT NOT NULL DEFAULT 0 CHECK (budget_cents >= 0),
    budget_currency TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    revoked_at TIMESTAMPTZ,
    revocation_actor TEXT,
    revocation_reason TEXT,
    PRIMARY KEY (agent_id, organization_id),
    CONSTRAINT agent_organization_memberships_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT agent_organization_memberships_status_valid CHECK (
        status IN ('active', 'revoked')
    ),
    CONSTRAINT agent_organization_memberships_revocation_valid CHECK (
        (status = 'active' AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_organization_memberships_organization
    ON agent_organization_memberships (organization_id, status, agent_id);

CREATE TABLE IF NOT EXISTS agent_identity_versions (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    version_number INT NOT NULL CHECK (version_number >= 1),
    parent_version_id TEXT,
    status TEXT NOT NULL,
    runtime TEXT NOT NULL,
    model TEXT NOT NULL,
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    config_fingerprint TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    environment_digest TEXT NOT NULL DEFAULT '',
    prompt_ref TEXT NOT NULL DEFAULT '',
    skill_refs TEXT[] NOT NULL DEFAULT '{}',
    memory_ref TEXT NOT NULL DEFAULT '',
    tool_refs TEXT[] NOT NULL DEFAULT '{}',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    promoted_at TIMESTAMPTZ,
    promoted_by TEXT,
    retired_at TIMESTAMPTZ,
    rejected_reason TEXT,
    UNIQUE (agent_id, version_number),
    CONSTRAINT agent_identity_versions_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT agent_identity_versions_parent_fk
        FOREIGN KEY (parent_version_id) REFERENCES agent_identity_versions (id),
    CONSTRAINT agent_identity_versions_status_valid CHECK (
        status IN ('draft', 'evaluating', 'eligible', 'active', 'retired', 'rejected')
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_identity_versions_agent_status
    ON agent_identity_versions (agent_id, status, version_number DESC);

CREATE TABLE IF NOT EXISTS legacy_agent_version_mappings (
    tenant_id TEXT NOT NULL,
    legacy_version_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, legacy_version_id),
    CONSTRAINT legacy_agent_version_mapping_legacy_fk
        FOREIGN KEY (tenant_id, legacy_version_id) REFERENCES agent_versions (tenant_id, id),
    CONSTRAINT legacy_agent_version_mapping_global_fk
        FOREIGN KEY (agent_version_id) REFERENCES agent_identity_versions (id)
);

CREATE TABLE IF NOT EXISTS agent_external_identities (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_subject_id TEXT NOT NULL,
    login TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    verified_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (provider, provider_subject_id),
    CONSTRAINT agent_external_identities_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT agent_external_identities_provider_valid CHECK (
        provider IN ('github', 'gitlab', 'gitee')
    ),
    CONSTRAINT agent_external_identities_status_valid CHECK (
        status IN ('active', 'revoked')
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_external_identities_agent
    ON agent_external_identities (agent_id, provider, status);

-- length-prefix 保证 legacy tenant/id 组合可确定性映射且不依赖扩展。
INSERT INTO agent_identities (
    id, handle, display_name, description, status, last_seen_at, created_at, updated_at
)
SELECT
    'ag:legacy:' || char_length(a.tenant_id)::text || ':' || a.tenant_id || ':' || a.id,
    'legacy-' || md5(a.tenant_id || chr(31) || a.id),
    a.name,
    COALESCE(a.description, ''),
    a.status,
    a.last_seen_at,
    a.created_at,
    a.updated_at
FROM agents a
ON CONFLICT (id) DO NOTHING;

INSERT INTO legacy_agent_identity_mappings (tenant_id, legacy_agent_id, agent_id)
SELECT
    a.tenant_id,
    a.id,
    'ag:legacy:' || char_length(a.tenant_id)::text || ':' || a.tenant_id || ':' || a.id
FROM agents a
ON CONFLICT (tenant_id, legacy_agent_id) DO NOTHING;

INSERT INTO agent_organization_memberships (
    agent_id, organization_id, operator_id, operator_email, team, scopes,
    budget_cents, budget_currency, status, created_at, updated_at
)
SELECT
    m.agent_id,
    a.tenant_id,
    a.owner_id,
    a.owner_email,
    a.team,
    a.scopes,
    a.budget_cents,
    a.budget_currency,
    'active',
    a.created_at,
    a.updated_at
FROM agents a
JOIN legacy_agent_identity_mappings m
  ON m.tenant_id = a.tenant_id AND m.legacy_agent_id = a.id
ON CONFLICT (agent_id, organization_id) DO NOTHING;

INSERT INTO agent_identity_versions (
    id, agent_id, version_number, status, runtime, model, capabilities,
    config_fingerprint, content_hash, environment_digest, prompt_ref, skill_refs,
    memory_ref, tool_refs, created_by, created_at, promoted_at, promoted_by,
    retired_at, rejected_reason
)
SELECT
    'agv:legacy:' || char_length(v.tenant_id)::text || ':' || v.tenant_id || ':' || v.id,
    m.agent_id,
    v.version_number,
    v.status,
    v.runtime,
    v.model,
    v.capabilities,
    COALESCE(v.config_fingerprint, ''),
    v.content_hash,
    v.environment_digest,
    v.prompt_ref,
    v.skill_refs,
    v.memory_ref,
    v.tool_refs,
    v.created_by,
    v.created_at,
    v.promoted_at,
    v.promoted_by,
    v.retired_at,
    v.rejected_reason
FROM agent_versions v
JOIN legacy_agent_identity_mappings m
  ON m.tenant_id = v.tenant_id AND m.legacy_agent_id = v.agent_id
ON CONFLICT (id) DO NOTHING;

INSERT INTO legacy_agent_version_mappings (tenant_id, legacy_version_id, agent_version_id)
SELECT
    v.tenant_id,
    v.id,
    'agv:legacy:' || char_length(v.tenant_id)::text || ':' || v.tenant_id || ':' || v.id
FROM agent_versions v
ON CONFLICT (tenant_id, legacy_version_id) DO NOTHING;

UPDATE agent_identity_versions gv
SET parent_version_id = parent_map.agent_version_id
FROM legacy_agent_version_mappings self_map
JOIN agent_versions legacy
  ON legacy.tenant_id = self_map.tenant_id AND legacy.id = self_map.legacy_version_id
JOIN legacy_agent_version_mappings parent_map
  ON parent_map.tenant_id = legacy.tenant_id AND parent_map.legacy_version_id = legacy.parent_version_id
WHERE gv.id = self_map.agent_version_id
  AND gv.parent_version_id IS DISTINCT FROM parent_map.agent_version_id;

UPDATE agent_identities gi
SET current_version_id = version_map.agent_version_id
FROM legacy_agent_identity_mappings agent_map
JOIN agents legacy
  ON legacy.tenant_id = agent_map.tenant_id AND legacy.id = agent_map.legacy_agent_id
JOIN legacy_agent_version_mappings version_map
  ON version_map.tenant_id = legacy.tenant_id AND version_map.legacy_version_id = legacy.current_version_id
WHERE gi.id = agent_map.agent_id
  AND gi.current_version_id IS DISTINCT FROM version_map.agent_version_id;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'agent_identities_current_version_fk'
    ) THEN
        ALTER TABLE agent_identities
            ADD CONSTRAINT agent_identities_current_version_fk
            FOREIGN KEY (current_version_id) REFERENCES agent_identity_versions (id)
            DEFERRABLE INITIALLY DEFERRED;
    END IF;
END $$;
