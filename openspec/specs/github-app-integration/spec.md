# GitHub App Integration

## Purpose

Allow each tenant to configure its own GitHub App for Agent submissions, credential issuance, and commit validation.
## Requirements

### ADDED Requirement: Tenant GitHub App configuration storage

The system SHALL persist per-tenant GitHub App configuration including provider, App ID, installation ID, private key, and base URL.

#### Scenario: Admin creates GitHub App config
- **GIVEN** an admin principal for tenant `tenant-1`
- **WHEN** calling `POST /v1/github-app` with `app_id`, `installation_id`, `private_key`
- **THEN** the configuration is persisted and a view without the private key is returned

#### Scenario: Admin reads GitHub App config
- **GIVEN** a tenant with an existing GitHub App config
- **WHEN** calling `GET /v1/github-app`
- **THEN** the system returns the public view including `configured: true` and no private key

#### Scenario: Admin deletes GitHub App config
- **GIVEN** a tenant with an existing GitHub App config
- **WHEN** calling `DELETE /v1/github-app`
- **THEN** the configuration is removed and the response indicates deletion

### ADDED Requirement: Per-tenant git driver resolution

The system SHALL resolve a `git.Driver` for a tenant from its persisted GitHub App configuration when issuing credentials or verifying commits.

#### Scenario: Credential service uses tenant driver
- **GIVEN** a tenant with GitHub App configured
- **WHEN** an Agent requests a credential for an execution
- **THEN** the system uses that tenant's App installation to issue the token

#### Scenario: Commit verifier uses tenant driver
- **GIVEN** a tenant with GitHub App configured
- **WHEN** verifying a submission commit
- **THEN** the system uses that tenant's App installation to query the repository

### ADDED Requirement: Missing configuration and onboarding resolution errors

Git operations (credential issuance, commit verification, issue sync) SHALL resolve runtime access through the tenant's onboarded-repository binding. When the target repository has not been onboarded for the tenant, the system SHALL return a `not_found` error without calling any external API. The legacy `not_configured` error remains only on edge paths that address the GitHub App configuration directly (for example connection tests or tenant-default issue-source resolution before any App is configured).

#### Scenario: Credential issue for a repository that is not onboarded
- **GIVEN** the target repository has no onboarded binding for the tenant
- **WHEN** an Agent requests a credential for that repository
- **THEN** the system returns `not_found` without calling any external API

#### Scenario: Direct GitHub App edge path without configuration
- **GIVEN** a tenant with no GitHub App configured
- **WHEN** a caller invokes an operation that addresses the GitHub App configuration directly
- **THEN** the system returns `not_configured` without calling any external API

### Requirement: GitHub App repository onboarding visibility
The system SHALL expose GitHub App installation repository visibility in a way that supports repository onboarding, including the tenant's GitHub App configured state and installation repository list.

#### Scenario: Admin views unconfigured GitHub App source
- **GIVEN** a tenant has no GitHub App configuration
- **WHEN** an admin opens repository onboarding
- **THEN** the system reports the GitHub App source as unconfigured
- **AND** the UI provides a connect or configure action

#### Scenario: Admin views configured GitHub App repositories
- **GIVEN** a tenant has a configured GitHub App
- **WHEN** an admin opens repository onboarding
- **THEN** the system returns the public GitHub App configuration state
- **AND** the system returns repositories visible to the installation

#### Scenario: App repository listing fails
- **GIVEN** a tenant has a configured GitHub App
- **WHEN** GitHub repository listing fails
- **THEN** the system preserves the GitHub App configured state
- **AND** the UI shows a repository-listing error without hiding the configuration action

#### Scenario: Admin reviews installed App details
- **GIVEN** a tenant has a configured GitHub App
- **WHEN** an admin opens repository onboarding
- **THEN** the UI shows the installed App's public details
- **AND** the UI does not expose stored private key material

#### Scenario: Admin installs an App after manifest creation
- **GIVEN** a tenant has created a GitHub App but has no installation id
- **WHEN** an admin opens GitHub App setup or repository onboarding
- **THEN** the UI shows the App as created but awaiting installation
- **AND** the UI provides an install action that redirects through a signed server endpoint

#### Scenario: GitHub returns installation details
- **GIVEN** a tenant has created a GitHub App through the manifest flow
- **WHEN** GitHub redirects back with an installation id
- **THEN** the system stores the installation id
- **AND** the system preserves the existing App private key and public App metadata

### Requirement: GitHub App manifest uses an explicit public origin
启用 Web transport 时，系统 MUST 要求配置 `GITHUB_APP_PUBLIC_BASE_URL`，并且该值 MUST 是不含 userinfo、query、fragment 或子路径的绝对 HTTP(S) origin。系统 SHALL 仅使用该配置生成 GitHub App manifest 的 callback 与 setup URL，不得从请求 Host 或转发头推导公网地址。

#### Scenario: Web service starts with a valid public origin
- **WHEN** `WEB_ENABLED=true` 且 `GITHUB_APP_PUBLIC_BASE_URL` 是合法 HTTP(S) origin
- **THEN** 配置加载成功
- **AND** manifest callback 与 setup URL 使用规范化后的 origin

#### Scenario: Web service starts without a public origin
- **WHEN** `WEB_ENABLED=true` 且 `GITHUB_APP_PUBLIC_BASE_URL` 为空
- **THEN** 配置加载失败并明确指出缺少该变量

#### Scenario: Web service starts with an invalid public origin
- **WHEN** `WEB_ENABLED=true` 且 `GITHUB_APP_PUBLIC_BASE_URL` 是相对地址、缺少 host、使用非 HTTP(S) scheme，或包含 userinfo、query、fragment、子路径
- **THEN** 配置加载失败并明确指出该变量无效

#### Scenario: Request headers disagree with configured origin
- **WHEN** manifest 请求的 `Host` 或任一 `X-Forwarded-*` 请求头与 `GITHUB_APP_PUBLIC_BASE_URL` 不同
- **THEN** 生成的 callback 与 setup URL 仍只使用 `GITHUB_APP_PUBLIC_BASE_URL`

## API

- `GET /v1/github-app` — return public GitHub App view
- `POST /v1/github-app` — create or replace config (admin only)
- `DELETE /v1/github-app` — remove config (admin only)
