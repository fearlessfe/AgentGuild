---
comet_change: repository-onboarding-center
role: technical-design
canonical_spec: openspec
---

# Repository Onboarding Center Design

## Context

AgentGuild already has pieces of GitHub integration, but they are split across the console in a way that makes repository access hard to understand. `/git-integration` handles GitHub App configuration, while `/sync` lists App installation repositories and also creates Issue sync rules. Public repositories can currently be typed into the sync screen, but that creates a sync rule directly rather than registering a repository as an onboarding asset.

The desired product shape is a dedicated repository onboarding module. It should make two steps obvious: first install or inspect the GitHub App, then add repositories either from the App installation or from public GitHub addresses. It should also let admins come back later and quickly answer: which App is installed, and which repositories have been added to the platform?

## Goals

- Add a direct authenticated console entry for repository onboarding.
- Present GitHub App installation/inspection as Step 1.
- Present repository addition/inspection as Step 2.
- Support adding repositories from GitHub App installation access and from public GitHub addresses.
- Persist a tenant-scoped repository inventory that is separate from Issue sync rules.
- Keep `/sync` focused on automation rules and link back to onboarding for repository setup.

## Non-Goals

- Rebuild the Issue-to-Task sync engine.
- Add non-GitHub providers.
- Add private repository access without GitHub App installation authorization.
- Automatically create sync rules when a repository is added.

## Recommended Architecture

Use a small repository onboarding module with a narrow backend inventory and a dedicated frontend page.

### Backend

Add a tenant-scoped repository inventory table, for example `onboarded_repositories`, with fields:

- `tenant_id`
- `id`
- `source_type`: `github_app` or `public_github`
- `full_name`: normalized `owner/repo`
- `default_branch`
- `visibility`
- `created_at`
- `updated_at`

The backend should expose a repository onboarding query that returns three pieces of state together:

- GitHub App public configuration state.
- GitHub App installation repositories discovered through the existing GitHub App manager.
- Tenant onboarded repositories from the new inventory table.

Mutations should remain admin-only:

- Add/select a GitHub App repository from the discovered installation repositories.
- Add a public GitHub repository by `owner/repo` or a GitHub URL.
- Remove an onboarded repository.

For public repositories, validate the input shape locally and, when feasible, fetch public metadata through the existing public GitHub client path before persisting. If live GitHub metadata lookup fails, return a recoverable error rather than silently adding incomplete data.

### Frontend

Add a persistent rail item and route, preferably `/repositories`, labelled `仓库接入`.

Create `RepositoryOnboardingScreen` with four visible regions:

1. `Step 1: GitHub App`
   - Shows configured/unconfigured/error state.
   - Shows public App details when configured: App slug, App ID, Installation ID, base URL, accessible repository count.
   - Provides actions to install/configure, test connection, or open the existing Git integration screen.

2. `Step 2: 添加仓库`
   - Shows App-accessible repositories with add/select actions.
   - Provides a public repository input accepting `owner/repo` and GitHub URL forms.
   - Adding a repository updates the repository inventory only; it does not create a sync rule.

3. `已安装 App`
   - A compact, scan-friendly summary of installed GitHub App state.

4. `已添加仓库`
   - A table or dense list of onboarded repositories.
   - Shows repository full name, source type, default branch, visibility, and management actions.

Update `/sync` so repository access setup is no longer the primary workflow there. `/sync` should manage automation rules and provide a link to `/repositories` when no repository is available.

## Data Flow

```text
Admin opens /repositories
  -> frontend calls repository onboarding summary endpoint
  -> backend loads GitHub App view
  -> backend attempts App installation repository discovery if configured
  -> backend loads onboarded repository inventory
  -> frontend renders Step 1, Step 2, installed App summary, added repository list

Admin adds App repository
  -> frontend posts selected full_name
  -> backend verifies it is visible through current App installation
  -> backend persists source_type=github_app
  -> frontend refreshes inventory

Admin adds public repository
  -> frontend posts owner/repo or URL
  -> backend normalizes and validates
  -> backend fetches public metadata when available
  -> backend persists source_type=public_github
  -> frontend refreshes inventory
```

## Key Trade-offs

- Persist selected repositories, not every App-discovered repository. GitHub remains the source of truth for installation access; AgentGuild stores what the tenant chose to onboard.
- Keep GitHub App setup reachable from onboarding but avoid duplicating secret handling. The existing Git integration screen can remain the configuration detail surface.
- Do not reuse sync rules as repository inventory. It would be faster, but it would preserve the confusing behavior where adding a public repository also creates automation.

## Risks and Mitigations

- GitHub App discovery can fail even when the App is configured. Show App configuration state separately from repository listing errors so the user still understands what is installed.
- Public repository metadata lookup can be rate-limited or fail. Return explicit validation/retry errors and keep the form state recoverable.
- Added App repositories can become unavailable if App installation access changes. Mark or surface stale/unavailable repositories during summary refresh instead of deleting silently.
- This change introduces a new table and API surface. Keep the model small and tenant-scoped, with admin-only mutations and straightforward tests.

## Testing Strategy

Backend tests:

- Migration applies and rolls back.
- Repository inventory store enforces tenant isolation.
- Add App repository verifies the repo is visible through the GitHub App installation source.
- Add public repository validates input and persists normalized names.
- REST mutations require admin principal.
- Summary endpoint distinguishes GitHub App unconfigured, configured with repositories, and configured with listing failure.

Frontend tests:

- Rail exposes `仓库接入` and route renders the onboarding module.
- Unconfigured GitHub App state shows the install/configure action.
- Configured GitHub App state shows installed App details and App repository choices.
- Adding an App repository updates the added repository list.
- Adding a public repository updates the added repository list without creating a sync rule.
- `/sync` links repository setup to `/repositories`.
