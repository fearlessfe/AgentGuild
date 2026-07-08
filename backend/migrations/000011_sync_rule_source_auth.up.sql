ALTER TABLE sync_rules
    ADD COLUMN IF NOT EXISTS source_auth TEXT NOT NULL DEFAULT 'app';

ALTER TABLE sync_rules
    ADD CONSTRAINT sync_rules_source_auth_valid CHECK (source_auth IN ('app','public'));
