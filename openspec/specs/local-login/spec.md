# Local Login

## Purpose

Provide a development-only password-based login endpoint that creates an OIDC-equivalent session cookie when no OIDC provider is configured.

## Requirements

### ADDED Requirement: Local admin login endpoint

The system SHALL expose `POST /oauth/local/login` that accepts a password and, when enabled, sets a session cookie for a configured admin identity. Local admin login is enabled by a derived switch: the endpoint is registered only when Web transport is enabled, no OIDC tenant is configured (`OIDC_TENANT_ID` is empty), and `LOCAL_ADMIN_PASSWORD` is set. A configured password shorter than 12 characters MUST fail startup.

#### Scenario: Developer logs in locally
- **GIVEN** `OIDC_TENANT_ID` is empty and `LOCAL_ADMIN_PASSWORD` (at least 12 characters) plus an admin identity are configured
- **WHEN** calling `POST /oauth/local/login` with the correct password
- **THEN** the system sets a session cookie and returns success

#### Scenario: Wrong password rejected
- **GIVEN** local admin login is enabled
- **WHEN** calling `POST /oauth/local/login` with an incorrect password
- **THEN** the system returns 401 Unauthorized

### ADDED Requirement: Local login disabled by default

The system SHALL NOT expose or accept local login when an OIDC tenant is configured or `LOCAL_ADMIN_PASSWORD` is unset.

#### Scenario: Endpoint disabled
- **GIVEN** `OIDC_TENANT_ID` is configured or `LOCAL_ADMIN_PASSWORD` is unset
- **WHEN** calling `POST /oauth/local/login`
- **THEN** the system returns 404 Not Found or 401 Unauthorized

## Security Notes

- Local login is intended for development and testing only.
- Password comparison uses constant-time comparison to resist timing attacks.
- The endpoint is registered only when local admin configuration is enabled.
