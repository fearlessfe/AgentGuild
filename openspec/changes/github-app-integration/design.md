## Context

The platform supports git-based delivery and validation for Agent submissions. Until now the git driver was constructed once from a single set of `GITHUB_APP_ID` / `GITHUB_PRIVATE_KEY` / `GITHUB_INSTALLATION_ID` environment variables. This works for single-tenant deployments but prevents multiple tenants from using separate GitHub Apps. At the same time, developers have no easy way to run the Web console locally without configuring a real OIDC provider.

## Goals / Non-Goals

**Goals:**
- Store per-tenant GitHub App configuration securely (private key persisted, never exposed in API responses).
- Resolve the correct `git.Driver` per tenant when issuing credentials or verifying commits.
- Provide admin REST endpoints to Upsert/Get/Delete the tenant GitHub App config.
- Provide a local admin login endpoint for development environments that bypasses OIDC.
- Keep the change backward compatible where no tenant config exists (return `not_configured` errors clearly).

**Non-Goals:**
- Frontend UI for GitHub App management (console-only or API-only for now).
- Automatic GitHub App creation or marketplace flow.
- Production use of local login; it is gated behind explicit config and documented as dev-only.
- Support for non-GitHub providers in this change.

## Decisions

1. **GitHub App configuration lives in PostgreSQL, not environment variables.**
   - Rationale: Multi-tenancy requires isolation; env vars cannot be updated at runtime.
   - Alternative considered: keep env vars as global fallback. Rejected to enforce explicit per-tenant setup.

2. **`GitHubAppService` is injected into `CredentialService` and `CommitVerifier`.**
   - Rationale: Both operations need a tenant-scoped driver. Dependency injection keeps the services testable.
   - Alternative considered: pass driver factory at call site. Rejected because it leaks driver construction into handlers.

3. **Private key is write-only from the API.**
   - Rationale: Security; the private key must never be returned in `GET` responses.
   - `GitHubAppView` omits `PrivateKey` and exposes a `configured` flag.

4. **Local login is disabled unless `LOCAL_ADMIN_ENABLED=true`.**
   - Rationale: Prevents accidental exposure in production. The endpoint is a dev escape hatch.
   - Password is compared with `subtle.ConstantTimeCompare` to resist timing attacks.

5. **Use existing `github.NewDriver` for resolved configurations.**
   - Rationale: Reuses the existing GitHub driver implementation; only configuration sourcing changes.

## Risks / Trade-offs

- [Risk] Existing tests and callers had to be updated to the new `CredentialService`/`CommitVerifier` signatures.
  - Mitigation: Tests were updated as part of this change; all backend tests pass.
- [Risk] Private key stored in PostgreSQL increases attack surface.
  - Mitigation: Stored encrypted-at-rest by PostgreSQL; API never returns it; access limited to admin principals.
- [Risk] Local login could be misused in production.
  - Mitigation: Disabled by default; requires explicit env vars; documented as dev-only.
- [Risk] Tenants without GitHub App config see `not_configured` errors.
  - Mitigation: Clear error code and admin endpoint to set up configuration.

## Migration Plan

1. Apply migration `000009_github_apps.up.sql` to add the `github_apps` table.
2. Existing env-based GitHub App config is no longer used by `CredentialService`/`CommitVerifier`; operators must configure it per tenant via the new API.
3. Rollback: apply `000009_github_apps.down.sql`.

## Open Questions

- Should the global env-based GitHub App config remain as a fallback for backward compatibility?
- Should local login be restricted to non-production builds?
