## Context

The console already has GitHub-related building blocks:

- `/git-integration` exposes GitHub App configuration.
- `/sync` lists GitHub App installation repositories through `listRepositories()` and manages Issue sync rules.
- Public repositories can currently be typed into the sync-rule form, but that action creates a rule directly rather than registering a reusable repository source.
- The backend already stores per-tenant GitHub App configuration and can list installation repositories; public issue access exists for sync, but public repository onboarding inventory is not modeled as a first-class resource.

This change narrows the product surface to repository onboarding. Sync rules should consume onboarded repositories later, but this change should make repository sources visible and manageable on their own.

The intended product flow is explicitly two-step: first install or inspect the GitHub App, then add repositories either by selecting App-accessible repositories or by entering public GitHub repositories.

## Goals / Non-Goals

**Goals:**
- Provide a clear console entry for repository onboarding.
- Show GitHub App connection state and App installation repositories in one place.
- Separate GitHub App installation/inspection from repository addition/inspection.
- Let admins select App installation repositories for platform use.
- Let admins add public GitHub repositories and view them as onboarded repositories.
- Keep demo mode representative of both source types.

**Non-Goals:**
- Rebuild the Issue-to-Task sync engine.
- Replace GitHub App credential issuance, commit validation, or submission flows.
- Add non-GitHub providers.
- Add OAuth-style private repository access outside GitHub App installation access.

## Decisions

### 1. Introduce a repository onboarding module instead of extending the dashboard

Add a dedicated console route, likely `/repositories` or `/repository-onboarding`, and expose it through the primary rail or a visible settings/navigation entry. The dashboard may still link to it, but it must not be the only entry.

Alternative considered: keep the current dashboard button and improve its label. This does not solve the discoverability problem because admins still need to know to start from the overview.

### 1a. Use a two-step onboarding layout

The repository onboarding route should show two explicit steps:

1. GitHub App: install, inspect, test, or update the GitHub App connection.
2. Repositories: add repositories by selecting from App-accessible repositories or entering a public GitHub repository.

The same module should also expose scan-friendly summaries for installed Apps and added repositories so admins can return later and inspect current state without repeating setup.

Alternative considered: separate GitHub App and repository addition into two unrelated routes. That keeps each screen smaller, but it fragments a workflow whose success depends on understanding both the App connection and the resulting repository inventory.

### 2. Separate repository inventory from sync rules

Represent onboarded repositories as their own inventory with source metadata:

- `source_type`: `github_app` or `public_github`
- `full_name`: `owner/repo`
- `default_branch`
- `visibility`
- `selected` or equivalent inclusion state for App installation repositories

Sync rules can later reference this inventory, but onboarding a repository should not create a sync rule by itself.

Alternative considered: continue treating public repository input as "create public sync rule". That couples access setup to task automation too early and hides the repository list the user asked to inspect.

### 3. Reuse GitHub App APIs for discovery, add minimal persistence for selection/public repositories

GitHub App repository discovery should continue to use the existing installation repository listing. The new module should persist only the platform's selected repository inventory and explicitly added public repositories.

For public repositories, the backend should validate at least repository name shape and, where feasible, fetch metadata from the public GitHub API before persisting. If live validation is unavailable, the UI must show a recoverable error state rather than silently accepting ambiguous data.

Alternative considered: copy GitHub App repositories into storage on every page load. That risks stale data and duplicates GitHub's installation source of truth; selection state is the part that needs persistence.

### 4. Keep GitHub App configuration visible but not duplicated

The repository onboarding page should show the GitHub App state and provide a direct action to connect, update, or revisit `/git-integration`. It should not duplicate every secret/configuration field unless the existing GitHub integration page is folded into this route during implementation.

Alternative considered: merge all GitHub App configuration forms into repository onboarding immediately. That may be appropriate later, but the first improvement can be smaller: clear entry, state, and next actions.

## Risks / Trade-offs

- [Risk] Public repository inventory may need a new table and API surface. -> Mitigation: keep the data model narrow and source-specific, with tenant-scoped records and no sync-rule coupling.
- [Risk] App installation repository selection can drift if the GitHub App installation loses access later. -> Mitigation: display unavailable/error states and refresh discovery from GitHub rather than relying solely on stored names.
- [Risk] The existing `/sync` page may duplicate repository UI after this change. -> Mitigation: move onboarding concerns out of `/sync`; keep `/sync` focused on automation rules and link to the onboarding center.
- [Risk] Admin-only operations may accidentally become visible to non-admin users. -> Mitigation: enforce existing admin session checks on mutation endpoints and show read-only/forbidden states in the UI.

## Migration Plan

1. Add repository onboarding route and navigation entry.
2. Add or revise backend APIs for selected App repositories and public repository inventory.
3. Keep existing GitHub App configuration APIs and repository discovery behavior compatible.
4. Update `/sync` copy/actions so it points users to repository onboarding for access setup.
5. Add frontend demo data and tests before broadening sync-rule integration.
