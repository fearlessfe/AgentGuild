#!/bin/sh
set -eu

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"

config=$(mktemp)
secure_config=$(mktemp)
env_file=$(mktemp)
empty_env_file=$(mktemp)
trap 'rm -f "$config" "$secure_config" "$env_file" "$empty_env_file"' EXIT HUP INT TERM

for variable in POSTGRES_PASSWORD CURSOR_SECRET SESSION_COOKIE_SECRET LOCAL_ADMIN_PASSWORD; do
  value=$(sed -n "s/^${variable}=//p" .env.example)
  [ -z "$value" ] || fail ".env.example must leave $variable empty"
done

cat >"$env_file" <<'EOF'
POSTGRES_PASSWORD=contract-database-password
CURSOR_SECRET=contract-cursor-secret-at-least-32-bytes
SESSION_COOKIE_SECRET=contract-session-secret-at-least-32-bytes
LOCAL_ADMIN_PASSWORD=contract-local-password
OAUTH_ISSUER=http://contract-issuer.local
OAUTH_AUDIENCE=contract-audience
APP_PORT=18080
EOF

OAUTH_ISSUER='http://ambient-issuer.invalid' \
OAUTH_AUDIENCE='ambient-audience' \
  env -i HOME="${HOME:-}" PATH="$PATH" \
  docker compose --env-file "$env_file" config --format json >"$config"

env -i HOME="${HOME:-}" PATH="$PATH" SESSION_COOKIE_SECURE=true \
  docker compose --env-file "$env_file" config --format json >"$secure_config"

cat >"$empty_env_file" <<'EOF'
POSTGRES_PASSWORD=
CURSOR_SECRET=
SESSION_COOKIE_SECRET=
LOCAL_ADMIN_PASSWORD=
EOF

if env -i HOME="${HOME:-}" PATH="$PATH" \
  docker compose --env-file "$empty_env_file" config --format json >/dev/null 2>&1; then
  fail "compose config must reject empty required secrets"
fi

services=$(jq -r '.services | keys | sort | join(",")' "$config")
[ "$services" = 'backend,frontend,migrate,postgres' ] || fail "unexpected services: $services"

published=$(jq '[.services[] | .ports // [] | .[]] | length' "$config")
[ "$published" -eq 0 ] || fail "expected no published host ports, got $published"
jq -e '.services.frontend.ports == null and (.services.frontend.expose | index("8080") != null)' "$config" >/dev/null \
  || fail "frontend must expose container port 8080 without publishing it"
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
jq -e '.services.backend.healthcheck.test | index("http://127.0.0.1:8080/healthz") != null' "$config" >/dev/null \
  || fail "backend healthcheck must use the database-aware /healthz endpoint"
jq -e '.services.frontend.depends_on.backend.condition == "service_healthy"' "$config" >/dev/null \
  || fail "frontend must wait for healthy backend"
jq -e '.services.frontend.healthcheck != null' "$config" >/dev/null \
  || fail "frontend must have a healthcheck"

jq -e '.services.backend.environment.DATABASE_URL == "postgres://agentguild:contract-database-password@postgres:5432/agentguild?sslmode=disable"' "$config" >/dev/null \
  || fail "backend DATABASE_URL is not wired to postgres"
jq -e '.services.backend.environment.OAUTH_ISSUER == "http://contract-issuer.local" and .services.backend.environment.OAUTH_AUDIENCE == "contract-audience"' "$config" >/dev/null \
  || fail "controlled OAuth issuer/audience are not preserved"
jq -e '.services.backend.environment.WEB_ENABLED == "true" and .services.backend.environment.MCP_ENABLED == "true"' "$config" >/dev/null \
  || fail "web and MCP transports must be enabled"
jq -e '.services.backend.environment.SESSION_COOKIE_SECURE == "false"' "$config" >/dev/null \
  || fail "SESSION_COOKIE_SECURE must default to false for localhost"
jq -e '.services.backend.environment.SESSION_COOKIE_SECURE == "true"' "$secure_config" >/dev/null \
  || fail "SESSION_COOKIE_SECURE=true must be passed through to backend"
jq -e '.services.backend.volumes[] | select(.target == "/var/lib/agentguild" and .source == "agentguild-keys" and .read_only != true)' "$config" >/dev/null \
  || fail "backend must have a writable agentguild-keys mount"

echo "PASS: compose contract (no host ports; frontend exposes 8080 internally)"
