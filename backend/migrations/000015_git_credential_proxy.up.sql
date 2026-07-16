ALTER TABLE git_credentials
    ADD COLUMN repo TEXT,
    ADD COLUMN request_hash BYTEA,
    ADD COLUMN token_hash BYTEA;

UPDATE git_credentials
SET repo = '',
    request_hash = ''::bytea,
    token_hash = ''::bytea,
    revoked_at = COALESCE(revoked_at, clock_timestamp()),
    status = 'revoked'
WHERE repo IS NULL OR request_hash IS NULL OR token_hash IS NULL;

ALTER TABLE git_credentials
    ALTER COLUMN repo SET NOT NULL,
    ALTER COLUMN request_hash SET NOT NULL,
    ALTER COLUMN token_hash SET NOT NULL;
