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
