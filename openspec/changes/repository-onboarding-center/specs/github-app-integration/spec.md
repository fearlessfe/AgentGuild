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
