---
comet_change: github-app-integration
role: technical-design
canonical_spec: openspec
---

# GitHub App Integration and Local Login Design

## Background

AgentGuild's git operations (credential issuance, commit verification, submission validation) currently rely on a single global GitHub App configured via environment variables. This design does not scale to multi-tenant deployments where each organization may want to use its own GitHub App installation. In addition, requiring a real OIDC provider for local development adds friction for contributors.

## Goals

- Allow each tenant to configure its own GitHub App via a REST API.
- Resolve the correct `git.Driver` for a tenant at operation time.
- Keep the change backward compatible in terms of API shape and error codes.
- Provide a development-only local admin login that creates an OIDC-equivalent session cookie.

## Non-Goals

- Frontend UI for GitHub App management.
- Automatic GitHub App creation or marketplace onboarding.
- Production use of local login.
- Non-GitHub git providers.

## Design

### GitHub App Configuration

A new `github_apps` table stores per-tenant configuration:

| Column | Purpose |
|--------|---------|
| tenant_id | Composite PK / partition key |
| provider | Provider name (default `github`) |
| app_id | GitHub App ID |
| installation_id | GitHub App installation ID |
| private_key | PEM private key (write-only via API) |
| base_url | API base URL (default `https://api.github.com`) |
| created_at / updated_at | Audit timestamps |

The repository interface `GitHubAppRepository` exposes Upsert, GetByTenant, and Delete.

### Application Services

`gitHubAppService` implements both `GitHubAppService` (driver resolution) and `GitHubAppManager` (admin CRUD). It is constructed from the repository and injected into:

- `CredentialService`: resolves driver per tenant before issuing a token.
- `CommitVerifier`: resolves driver per tenant before verifying commits.

`application.Tx` is extended with `GitHubApps() GitHubAppRepository` so the service can participate in transactions if needed.

### REST Endpoints

All GitHub App endpoints require a human session principal and are admin-only by virtue of being under `requireSession`:

- `GET /v1/github-apps` — public view of the tenant config.
- `PUT /v1/github-apps` — create or replace config.
- `DELETE /v1/github-apps` — remove config.

Local login:

- `POST /v1/auth/local-login` — accepts `{ "password": "..." }` and sets a session cookie when `LOCAL_ADMIN_ENABLED=true`.

### Configuration

New optional environment variables:

- `LOCAL_ADMIN_TENANT_ID` (default `local`)
- `LOCAL_ADMIN_OWNER_ID` (default `local-admin`)
- `LOCAL_ADMIN_OWNER_EMAIL` (default `admin@local`)
- `LOCAL_ADMIN_PASSWORD` (required to enable; min 12 characters)

Local admin is enabled only when `OIDC_TENANT_ID` is empty and `LOCAL_ADMIN_PASSWORD` is non-empty.

### Security

- Private key is never returned in API responses.
- Password comparison uses `subtle.ConstantTimeCompare`.
- Local login is disabled unless explicitly configured.

## Testing Strategy

- Unit tests for `gitHubAppService`.
- REST handler tests for `/v1/github-apps` and `/v1/auth/local-login`.
- Updated existing git tests to new signatures.
- Full backend test suite (`go test -race ./...`).

## Risks and Trade-offs

- [Risk] Per-tenant config removes the implicit global fallback.
  - Mitigation: Clear `not_configured` error and admin API.
- [Risk] Private key stored in database.
  - Mitigation: Column-level encryption can be added later; API never exposes it.
- [Risk] Local login misconfiguration in production.
  - Mitigation: Requires both empty OIDC tenant and explicit password; documented as dev-only.

## Spec Patch

Adds requirements to `openspec/specs/github-app-integration/spec.md` and `openspec/specs/local-login/spec.md`.
