# Comet Design Handoff

- Change: repository-onboarding-center
- Phase: design
- Mode: compact
- Context hash: 385f7723729d5665c3e8d86cea14c02fc5b6650947b8735dda57a1883f7ecf83

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/repository-onboarding-center/proposal.md

- Source: openspec/changes/repository-onboarding-center/proposal.md
- Lines: 1-29
- SHA256: f4084ae4e524f72edcde184c9914fb8248378e052461c0504e1c1dcfec0ac32d

```md
## Why

GitHub repository access is currently discoverable only through a dashboard action, so admins cannot quickly understand what repository sources are connected or which repositories are available. A dedicated repository onboarding center makes GitHub App and public repository access visible, inspectable, and ready for later task-sync workflows.

## What Changes

- Add a first-class console entry for repository onboarding and source inspection.
- Add a repository onboarding page that separates GitHub App repositories from public GitHub repositories.
- Show GitHub App configuration state and the repositories available through the configured installation.
- Allow admins to select GitHub App installation repositories for platform use.
- Allow admins to add public GitHub repositories and view them in the repository list.
- Keep Issue sync rule creation out of this change except where existing screens need to link to the repository onboarding center.

## Capabilities

### New Capabilities
- `repository-onboarding-center`: Covers repository source onboarding, repository source visibility, and selected/added repository inventory in the console.

### Modified Capabilities
- `github-app-integration`: GitHub App integration must expose installation repository visibility for admin onboarding, not only app configuration for git operations.
- `console-ui-ux`: The authenticated console must expose repository onboarding as a clear primary navigation or settings entry rather than a dashboard-only shortcut.

## Impact

- Frontend navigation, dashboard affordances, and a new or revised repository onboarding feature page.
- Frontend API client and demo data for GitHub App repositories and public repositories.
- Backend REST/application/storage changes if public repository inventory is not already persisted.
- OpenAPI and tests for any new repository onboarding endpoints.
- Existing GitHub App configuration and installation repository APIs remain the foundation for App-based repository discovery.
```

## openspec/changes/repository-onboarding-center/design.md

- Source: openspec/changes/repository-onboarding-center/design.md
- Lines: 1-90
- SHA256: 2bf868c5b69808ecf90bb9e4ecb1c0217a6982c732ded090a253c5316705aa69

[TRUNCATED]

```md
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
```

Full source: openspec/changes/repository-onboarding-center/design.md

## openspec/changes/repository-onboarding-center/tasks.md

- Source: openspec/changes/repository-onboarding-center/tasks.md
- Lines: 1-26
- SHA256: 754a068988d58ae7c351e4997da20033d0fdade6ab456c063a52aafb3a7eae0f

```md
## 1. Backend Repository Inventory

- [ ] 1.1 Design and add tenant-scoped storage for onboarded repositories, including source type, full name, default branch, visibility, and selection state.
- [ ] 1.2 Add application service commands/queries for listing onboarded repositories, selecting GitHub App installation repositories, adding public GitHub repositories, and removing onboarded repositories.
- [ ] 1.3 Add REST endpoints and OpenAPI definitions for repository onboarding inventory operations.
- [ ] 1.4 Reuse existing GitHub App repository discovery for App source listing and keep configuration errors distinguishable from repository listing errors.

## 2. Frontend Repository Onboarding

- [ ] 2.1 Add a persistent navigation entry and route for the repository onboarding module.
- [ ] 2.2 Build the repository onboarding page with separate GitHub App and public GitHub repository sections.
- [ ] 2.3 Show GitHub App configured/unconfigured/error states with a clear action back to GitHub integration setup.
- [ ] 2.4 Support selecting GitHub App installation repositories and adding public GitHub repositories without creating sync rules.
- [ ] 2.5 Update demo mode data and client helpers so both source types render without a backend.

## 3. Existing Flow Integration

- [ ] 3.1 Update dashboard or overview actions to point to the repository onboarding module.
- [ ] 3.2 Refocus the sync rules page on automation rules and link repository setup to onboarding.
- [ ] 3.3 Keep existing GitHub App configuration behavior compatible with the new onboarding entry.

## 4. Verification

- [ ] 4.1 Add backend tests for repository onboarding service and REST behavior, including tenant isolation and admin-only mutations.
- [ ] 4.2 Add frontend tests for navigation discoverability, App repository selection, and public repository addition.
- [ ] 4.3 Run backend build/tests and frontend build/tests relevant to the changed surface.
```

## openspec/changes/repository-onboarding-center/specs/console-ui-ux/spec.md

- Source: openspec/changes/repository-onboarding-center/specs/console-ui-ux/spec.md
- Lines: 1-24
- SHA256: 006d8656bdb3be9a856ed3625afedb2e5c6efb016b26f897cfc38025ad7ec316

```md
## ADDED Requirements

### Requirement: Repository onboarding is discoverable
The console SHALL make repository onboarding discoverable from persistent authenticated navigation or an equivalently prominent settings entry.

#### Scenario: User scans authenticated navigation
- **WHEN** an authenticated user views the application shell
- **THEN** the repository onboarding entry is visible without first opening the overview/dashboard page

#### Scenario: User identifies current module
- **WHEN** an authenticated user opens the repository onboarding route
- **THEN** the top-level module label and page header identify the repository onboarding context

### Requirement: Repository onboarding separates setup from automation
The console SHALL present repository access setup separately from Issue sync rule automation.

#### Scenario: User opens sync rules
- **WHEN** a user opens the sync rule page
- **THEN** the page focuses on automation rules
- **AND** repository access setup is linked to the repository onboarding module rather than embedded as the primary workflow

#### Scenario: User opens repository onboarding
- **WHEN** a user opens the repository onboarding module
- **THEN** adding or selecting a repository does not automatically create an Issue sync rule
```

## openspec/changes/repository-onboarding-center/specs/github-app-integration/spec.md

- Source: openspec/changes/repository-onboarding-center/specs/github-app-integration/spec.md
- Lines: 1-28
- SHA256: b5b04833bf72e07235ee6494c1211433ceaad906d4a66b61377d05ef68a374d6

```md
## ADDED Requirements

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
```

## openspec/changes/repository-onboarding-center/specs/repository-onboarding-center/spec.md

- Source: openspec/changes/repository-onboarding-center/specs/repository-onboarding-center/spec.md
- Lines: 1-74
- SHA256: e9e768ba846dd901b7b7af5b5c41a1101fec37aa09d4d814e4be9c3760394943

```md
## ADDED Requirements

### Requirement: Repository onboarding entry point
The console SHALL expose a direct repository onboarding entry point for authenticated users without requiring navigation through the overview/dashboard page.

#### Scenario: Admin finds repository onboarding from primary navigation
- **WHEN** an admin opens the authenticated console
- **THEN** the navigation exposes a repository onboarding entry
- **AND** activating that entry opens the repository onboarding module

#### Scenario: Existing overview action remains compatible
- **WHEN** an admin uses an existing overview or dashboard action for repository setup
- **THEN** the action navigates to the repository onboarding module

### Requirement: Repository sources are visible
The repository onboarding module SHALL show repository sources separately for GitHub App installation repositories and public GitHub repositories.

#### Scenario: Admin views repository source sections
- **WHEN** an admin opens the repository onboarding module
- **THEN** the page shows a GitHub App repository section
- **AND** the page shows a public GitHub repository section

#### Scenario: Repository source is distinguishable
- **WHEN** a repository appears in the onboarding inventory
- **THEN** the UI identifies whether it came from GitHub App access or public GitHub access

### Requirement: Repository onboarding uses a two-step flow
The repository onboarding module SHALL present GitHub App installation or inspection as a separate first step from repository addition or inspection.

#### Scenario: Admin starts repository onboarding
- **WHEN** an admin opens the repository onboarding module
- **THEN** the first step is GitHub App installation or inspection
- **AND** the second step is repository addition or inspection

#### Scenario: Admin reviews current onboarding state
- **WHEN** an admin opens the repository onboarding module after prior setup
- **THEN** the module shows installed GitHub App information
- **AND** the module shows the repositories already added to the platform

### Requirement: GitHub App repositories can be selected
The repository onboarding module SHALL allow admins to select repositories from the configured GitHub App installation for platform use.

#### Scenario: Admin selects an App installation repository
- **GIVEN** the tenant has a configured GitHub App with visible installation repositories
- **WHEN** an admin selects an unselected repository
- **THEN** the repository is marked as selected for platform use
- **AND** it remains visible as a GitHub App repository after refresh

#### Scenario: No App repositories are available
- **GIVEN** the tenant has a configured GitHub App but the installation returns no repositories
- **WHEN** an admin opens the repository onboarding module
- **THEN** the module shows an empty state for App repositories
- **AND** the empty state provides a recovery action or explanation

### Requirement: Public GitHub repositories can be added
The repository onboarding module SHALL allow admins to add public GitHub repositories by `owner/repo` full name and view them in the repository inventory.

#### Scenario: Admin adds a public repository
- **WHEN** an admin submits a valid public repository full name
- **THEN** the repository is persisted for the tenant
- **AND** it appears in the public GitHub repository section

#### Scenario: Public repository input is invalid
- **WHEN** an admin submits an invalid repository full name
- **THEN** the system rejects the request
- **AND** the UI shows a recoverable validation error

### Requirement: Repository onboarding demo mode
The frontend demo mode SHALL include representative GitHub App and public GitHub repository data for the repository onboarding module.

#### Scenario: User opens onboarding in demo mode
- **WHEN** demo mode is enabled and the user opens repository onboarding
- **THEN** the page shows at least one GitHub App repository
- **AND** the page shows at least one public GitHub repository
```

