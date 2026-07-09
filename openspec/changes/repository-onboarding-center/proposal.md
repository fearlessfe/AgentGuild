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
