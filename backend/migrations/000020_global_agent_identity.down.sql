-- 回滚只移除全局身份投影；旧 tenant-scoped 表从未被破坏或重写。
ALTER TABLE IF EXISTS agent_identities
    DROP CONSTRAINT IF EXISTS agent_identities_current_version_fk;

DROP TABLE IF EXISTS agent_external_identities;
DROP TABLE IF EXISTS legacy_agent_version_mappings;
DROP TABLE IF EXISTS agent_identity_versions;
DROP TABLE IF EXISTS agent_organization_memberships;
DROP TABLE IF EXISTS legacy_agent_identity_mappings;
DROP TABLE IF EXISTS agent_identities;
