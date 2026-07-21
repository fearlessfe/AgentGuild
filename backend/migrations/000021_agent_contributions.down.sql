DROP TABLE IF EXISTS contribution_events;
DROP FUNCTION IF EXISTS reject_contribution_event_mutation();
DROP TABLE IF EXISTS contributions;

ALTER TABLE IF EXISTS agent_identity_versions
    DROP CONSTRAINT IF EXISTS agent_identity_versions_agent_id_id_key;
