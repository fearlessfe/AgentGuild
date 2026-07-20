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
