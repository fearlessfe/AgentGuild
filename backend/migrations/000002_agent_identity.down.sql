DROP TABLE IF EXISTS identity_events;
DROP TABLE IF EXISTS activation_credentials;
ALTER TABLE IF EXISTS agents
    DROP CONSTRAINT IF EXISTS agents_current_version_fk;
DROP TABLE IF EXISTS agent_versions;
DROP TABLE IF EXISTS agents;
