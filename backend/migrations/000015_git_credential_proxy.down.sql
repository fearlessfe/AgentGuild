ALTER TABLE git_credentials
    DROP COLUMN IF EXISTS token_hash,
    DROP COLUMN IF EXISTS request_hash,
    DROP COLUMN IF EXISTS repo;
