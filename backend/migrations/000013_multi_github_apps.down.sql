DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM github_apps
        GROUP BY tenant_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot downgrade multi GitHub App schema: at least one tenant has more than one App';
    END IF;
END
$$;

ALTER TABLE onboarded_repositories DROP CONSTRAINT onboarded_repositories_source_binding_check;
ALTER TABLE onboarded_repositories DROP CONSTRAINT onboarded_repositories_github_app_fk;
ALTER TABLE onboarded_repositories DROP CONSTRAINT onboarded_repositories_tenant_full_name_key;
ALTER TABLE onboarded_repositories
    ADD CONSTRAINT onboarded_repositories_tenant_id_source_type_full_name_key
    UNIQUE (tenant_id, source_type, full_name);
ALTER TABLE onboarded_repositories DROP COLUMN github_app_id;

DROP INDEX github_apps_one_default_per_tenant;
ALTER TABLE github_apps DROP CONSTRAINT github_apps_tenant_app_id_key;
ALTER TABLE github_apps DROP CONSTRAINT github_apps_pkey;
ALTER TABLE github_apps DROP COLUMN installation_account_login;
ALTER TABLE github_apps DROP COLUMN is_default;
ALTER TABLE github_apps DROP COLUMN id;
ALTER TABLE github_apps ADD PRIMARY KEY (tenant_id);
