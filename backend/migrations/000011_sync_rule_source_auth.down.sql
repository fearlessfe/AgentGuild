ALTER TABLE sync_rules
    DROP CONSTRAINT IF EXISTS sync_rules_source_auth_valid;

ALTER TABLE sync_rules
    DROP COLUMN IF EXISTS source_auth;
