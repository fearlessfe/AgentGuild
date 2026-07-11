#!/bin/sh
set -eu

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"

config=$(mktemp)
trap 'rm -f "$config"' EXIT HUP INT TERM

POSTGRES_PASSWORD='contract-database-password' \
CURSOR_SECRET='contract-cursor-secret-at-least-32-bytes' \
SESSION_COOKIE_SECRET='contract-session-secret-at-least-32-bytes' \
LOCAL_ADMIN_PASSWORD='contract-local-password' \
APP_PORT=18080 \
  docker compose config --format json >"$config"

services=$(jq -r '.services | keys | sort | join(",")' "$config")
[ "$services" = 'backend,frontend,migrate,postgres' ] || fail "unexpected services: $services"

published=$(jq '[.services[] | .ports // [] | .[]] | length' "$config")
[ "$published" -eq 1 ] || fail "expected exactly one published port, got $published"
jq -e '.services.frontend.ports | length == 1 and .[0].target == 8080 and .[0].published == "18080"' "$config" >/dev/null \
  || fail "frontend must publish APP_PORT to container port 8080"
jq -e '.services.postgres.ports == null and .services.backend.ports == null and .services.migrate.ports == null' "$config" >/dev/null \
  || fail "postgres, migrate, and backend must not publish ports"

jq -e '.services.postgres.healthcheck != null' "$config" >/dev/null \
  || fail "postgres must have a healthcheck"
jq -e '.services.migrate.depends_on.postgres.condition == "service_healthy"' "$config" >/dev/null \
  || fail "migrate must wait for healthy postgres"
jq -e '.services.backend.depends_on.migrate.condition == "service_completed_successfully"' "$config" >/dev/null \
  || fail "backend must wait for successful migration"
jq -e '.services.backend.healthcheck != null' "$config" >/dev/null \
  || fail "backend must have a healthcheck"
jq -e '.services.frontend.depends_on.backend.condition == "service_healthy"' "$config" >/dev/null \
  || fail "frontend must wait for healthy backend"
jq -e '.services.frontend.healthcheck != null' "$config" >/dev/null \
  || fail "frontend must have a healthcheck"

jq -e '.services.backend.environment.DATABASE_URL == "postgres://agentguild:contract-database-password@postgres:5432/agentguild?sslmode=disable"' "$config" >/dev/null \
  || fail "backend DATABASE_URL is not wired to postgres"
jq -e '.services.backend.environment.OAUTH_ISSUER == "http://agentguild.local" and .services.backend.environment.OAUTH_AUDIENCE == "agentguild"' "$config" >/dev/null \
  || fail "local OAuth issuer/audience defaults are missing"
jq -e '.services.backend.environment.WEB_ENABLED == "true" and .services.backend.environment.MCP_ENABLED == "true"' "$config" >/dev/null \
  || fail "web and MCP transports must be enabled"
jq -e '.services.backend.volumes[] | select(.target == "/var/lib/agentguild" and .source == "agentguild-keys" and .read_only != true)' "$config" >/dev/null \
  || fail "backend must have a writable agentguild-keys mount"

echo "PASS: compose contract (one published port, owned by frontend)"
