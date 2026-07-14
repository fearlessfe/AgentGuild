ALTER TABLE github_apps ADD COLUMN id text;
UPDATE github_apps SET id = 'gha_' || md5(tenant_id || ':' || app_id::text);
ALTER TABLE github_apps ALTER COLUMN id SET NOT NULL;
ALTER TABLE github_apps ADD COLUMN installation_account_login text;
ALTER TABLE github_apps ADD COLUMN is_default boolean NOT NULL DEFAULT false;
UPDATE github_apps SET is_default = true;
ALTER TABLE github_apps DROP CONSTRAINT github_apps_pkey;
ALTER TABLE github_apps ADD PRIMARY KEY (tenant_id, id);
ALTER TABLE github_apps ADD CONSTRAINT github_apps_tenant_app_id_key UNIQUE (tenant_id, app_id);
CREATE UNIQUE INDEX github_apps_one_default_per_tenant
    ON github_apps (tenant_id) WHERE is_default;

ALTER TABLE onboarded_repositories ADD COLUMN github_app_id text;
UPDATE onboarded_repositories r
SET github_app_id = a.id
FROM github_apps a
WHERE r.tenant_id = a.tenant_id AND r.source_type = 'github_app';
ALTER TABLE onboarded_repositories
    ADD CONSTRAINT onboarded_repositories_github_app_fk
    FOREIGN KEY (tenant_id, github_app_id) REFERENCES github_apps (tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE onboarded_repositories
    ADD CONSTRAINT onboarded_repositories_source_binding_check CHECK (
      (source_type = 'github_app' AND github_app_id IS NOT NULL) OR
      (source_type = 'public_github' AND github_app_id IS NULL)
    );
ALTER TABLE onboarded_repositories DROP CONSTRAINT onboarded_repositories_tenant_id_source_type_full_name_key;
ALTER TABLE onboarded_repositories ADD CONSTRAINT onboarded_repositories_tenant_full_name_key UNIQUE (tenant_id, full_name);
