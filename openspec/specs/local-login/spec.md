# Local Login

## Overview

Provide a development-only password-based login endpoint that creates an OIDC-equivalent session cookie when no OIDC provider is configured.

## Requirements

### ADDED Requirement: Local admin login endpoint

The system SHALL expose `POST /v1/oauth/local/login` that accepts a password and, when enabled, sets a session cookie for a configured admin identity.

#### Scenario: Developer logs in locally
- **GIVEN** `LOCAL_ADMIN_ENABLED=true` and a configured password/admin identity
- **WHEN** calling `POST /v1/oauth/local/login` with the correct password
- **THEN** the system sets a session cookie and returns success

#### Scenario: Wrong password rejected
- **GIVEN** local admin login is enabled
- **WHEN** calling `POST /v1/oauth/local/login` with an incorrect password
- **THEN** the system returns 401 Unauthorized

### ADDED Requirement: Local login disabled by default

The system SHALL NOT expose or accept local login when `LOCAL_ADMIN_ENABLED` is false or unset.

#### Scenario: Endpoint disabled
- **GIVEN** `LOCAL_ADMIN_ENABLED=false`
- **WHEN** calling `POST /v1/oauth/local/login`
- **THEN** the system returns 404 Not Found or 401 Unauthorized

## Security Notes

- Local login is intended for development and testing only.
- Password comparison uses constant-time comparison to resist timing attacks.
- The endpoint is registered only when local admin configuration is enabled.
