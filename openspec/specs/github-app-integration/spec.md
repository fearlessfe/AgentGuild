# GitHub App Integration

## Overview

Allow each tenant to configure its own GitHub App for Agent submissions, credential issuance, and commit validation.

## Requirements

### ADDED Requirement: Tenant GitHub App configuration storage

The system SHALL persist per-tenant GitHub App configuration including provider, App ID, installation ID, private key, and base URL.

#### Scenario: Admin creates GitHub App config
- **GIVEN** an admin principal for tenant `tenant-1`
- **WHEN** calling `PUT /v1/github-apps` with `app_id`, `installation_id`, `private_key`
- **THEN** the configuration is persisted and a view without the private key is returned

#### Scenario: Admin reads GitHub App config
- **GIVEN** a tenant with an existing GitHub App config
- **WHEN** calling `GET /v1/github-apps`
- **THEN** the system returns the public view including `configured: true` and no private key

#### Scenario: Admin deletes GitHub App config
- **GIVEN** a tenant with an existing GitHub App config
- **WHEN** calling `DELETE /v1/github-apps`
- **THEN** the configuration is removed and the response indicates deletion

### ADDED Requirement: Per-tenant git driver resolution

The system SHALL resolve a `git.Driver` for a tenant from its persisted GitHub App configuration when issuing credentials or verifying commits.

#### Scenario: Credential service uses tenant driver
- **GIVEN** a tenant with GitHub App configured
- **WHEN** an Agent requests a credential for an execution
- **THEN** the system uses that tenant's App installation to issue the token

#### Scenario: Commit verifier uses tenant driver
- **GIVEN** a tenant with GitHub App configured
- **WHEN** verifying a submission commit
- **THEN** the system uses that tenant's App installation to query the repository

### ADDED Requirement: Missing configuration error

The system SHALL return a `not_configured` error when a git operation requires a tenant GitHub App but none is configured.

#### Scenario: Credential issue without config
- **GIVEN** a tenant with no GitHub App configured
- **WHEN** an Agent requests a credential
- **THEN** the system returns `not_configured` without calling any external API

## API

- `GET /v1/github-apps` — return public GitHub App view
- `PUT /v1/github-apps` — create or replace config (admin only)
- `DELETE /v1/github-apps` — remove config (admin only)
