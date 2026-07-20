# repository-onboarding-center Specification

## Purpose
Define the console repository onboarding center: a discoverable entry point, visible repository sources, and a two-step flow for onboarding repositories via the tenant's GitHub App installation or as public GitHub repositories.
## Requirements
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

