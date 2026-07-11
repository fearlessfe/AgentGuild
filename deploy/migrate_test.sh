#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
migrations_dir="$repo_root/backend/migrations"

versions=$(
  find "$migrations_dir" -type f -name '*.up.sql' -exec basename {} \; |
    sed 's/_.*//' |
    sort
)

duplicates=$(printf '%s\n' "$versions" | uniq -d)
if [ -n "$duplicates" ]; then
  echo "duplicate migration numeric prefix: $duplicates" >&2
  exit 1
fi

expected=$(awk 'BEGIN { for (i = 1; i <= 12; i++) printf "000%03d\n", i }')
if [ "$versions" != "$expected" ]; then
  echo "migration versions must sort exactly from 000001 through 000012" >&2
  exit 1
fi

sh -n "$repo_root/deploy/backend-entrypoint.sh"
sh -n "$repo_root/deploy/migrate.sh"

test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
mkdir "$test_dir/bin"

cat >"$test_dir/bin/find" <<'EOF'
#!/bin/sh
printf '%s\n' "${TEST_MIGRATION_FILE:?}"
EOF
cat >"$test_dir/bin/psql" <<'EOF'
#!/bin/sh
attempts_file=${TEST_PSQL_ATTEMPTS_FILE:?}
attempts=0
[ ! -f "$attempts_file" ] || attempts=$(cat "$attempts_file")
attempts=$((attempts + 1))
printf '%s\n' "$attempts" >"$attempts_file"
if [ -n "${TEST_PSQL_EXIT_STATUS:-}" ]; then
  exit "$TEST_PSQL_EXIT_STATUS"
fi
if [ "$attempts" -lt "${TEST_PSQL_SUCCEEDS_ON_ATTEMPT:-1}" ]; then
  exit 2
fi
cat >"${TEST_PSQL_INPUT_FILE:?}"
EOF
chmod +x "$test_dir/bin/find" "$test_dir/bin/psql"

run_migrate() {
  rm -f "$test_dir/psql-attempts"
  TEST_MIGRATION_FILE=$1 TEST_PSQL_ATTEMPTS_FILE="$test_dir/psql-attempts" \
    TEST_PSQL_INPUT_FILE="$test_dir/psql-input" \
    TEST_PSQL_SUCCEEDS_ON_ATTEMPT=${TEST_PSQL_SUCCEEDS_ON_ATTEMPT:-1} \
    TEST_PSQL_EXIT_STATUS=${TEST_PSQL_EXIT_STATUS:-} \
    MIGRATION_CONNECT_ATTEMPTS=${MIGRATION_CONNECT_ATTEMPTS:-1} \
    MIGRATION_CONNECT_DELAY_SECONDS=${MIGRATION_CONNECT_DELAY_SECONDS:-0} \
    DATABASE_URL=postgres://contract-test \
    PATH="$test_dir/bin:$PATH" sh "$repo_root/deploy/migrate.sh"
}

run_migrate /migrations/000001_valid.up.sql
grep -q '^\\echo '\''migration 000001 already applied'\''' "$test_dir/psql-input" || {
  echo "migration runner did not emit a literal psql echo command" >&2
  exit 1
}

TEST_PSQL_SUCCEEDS_ON_ATTEMPT=3 MIGRATION_CONNECT_ATTEMPTS=3 \
  run_migrate /migrations/000001_valid.up.sql
[ "$(cat "$test_dir/psql-attempts")" -eq 3 ] || {
  echo "migration runner did not retry a transient database connection failure" >&2
  exit 1
}

TEST_PSQL_SUCCEEDS_ON_ATTEMPT=4 MIGRATION_CONNECT_ATTEMPTS=3 \
  run_migrate /migrations/000001_valid.up.sql >"$test_dir/output" 2>&1 && {
    echo "migration runner succeeded after exhausting exactly N attempts" >&2
    exit 1
  }
[ "$(cat "$test_dir/psql-attempts")" -eq 3 ] || {
  echo "migration runner did not exhaust exactly N attempts" >&2
  exit 1
}

for status in 1 3; do
  if TEST_PSQL_EXIT_STATUS=$status MIGRATION_CONNECT_ATTEMPTS=3 \
    run_migrate /migrations/000001_valid.up.sql >"$test_dir/output" 2>&1; then
    echo "migration runner swallowed psql exit status $status" >&2
    exit 1
  else
    actual=$?
  fi
  [ "$actual" -eq "$status" ] || {
    echo "migration runner changed psql exit status $status to $actual" >&2
    exit 1
  }
  [ "$(cat "$test_dir/psql-attempts")" -eq 1 ] || {
    echo "migration runner retried psql exit status $status" >&2
    exit 1
  }
done

for config in \
  'MIGRATION_CONNECT_ATTEMPTS=0' \
  'MIGRATION_CONNECT_ATTEMPTS=61' \
  'MIGRATION_CONNECT_ATTEMPTS=invalid' \
  'MIGRATION_CONNECT_ATTEMPTS=99999999999999999999999999999999999999999999999999' \
  'MIGRATION_CONNECT_DELAY_SECONDS=31' \
  'MIGRATION_CONNECT_DELAY_SECONDS=invalid' \
  'MIGRATION_CONNECT_DELAY_SECONDS=99999999999999999999999999999999999999999999999999'; do
  rm -f "$test_dir/psql-attempts"
  if env $config TEST_MIGRATION_FILE=/migrations/000001_valid.up.sql \
    TEST_PSQL_ATTEMPTS_FILE="$test_dir/psql-attempts" \
    TEST_PSQL_INPUT_FILE="$test_dir/psql-input" DATABASE_URL=postgres://contract-test \
    PATH="$test_dir/bin:$PATH" sh "$repo_root/deploy/migrate.sh" >"$test_dir/output" 2>&1; then
    echo "migration runner accepted invalid retry config: $config" >&2
    exit 1
  fi
  if ! grep -q 'must be an integer from' "$test_dir/output"; then
    echo "migration runner did not report invalid retry config before numeric evaluation: $config" >&2
    cat "$test_dir/output" >&2
    exit 1
  fi
  if [ -f "$test_dir/psql-attempts" ]; then
    echo "migration runner invoked psql for invalid retry config: $config" >&2
    exit 1
  fi
done

TEST_PSQL_SUCCEEDS_ON_ATTEMPT=1 TEST_PSQL_EXIT_STATUS= \
  MIGRATION_CONNECT_ATTEMPTS=60 MIGRATION_CONNECT_DELAY_SECONDS=30 \
  run_migrate /migrations/000001_valid.up.sql

for invalid in \
  00001_too-short.up.sql \
  0000001_too-long.up.sql \
  abcdef_not-numeric.up.sql \
  000001_.up.sql \
  '000001_has space.up.sql' \
  '000001_has"quote.up.sql' \
  '000001_has\backslash.up.sql' \
  000001_missing-suffix.sql; do
  if run_migrate "/migrations/$invalid" >"$test_dir/output" 2>&1; then
    echo "migration runner accepted invalid basename: $invalid" >&2
    exit 1
  fi
  if ! grep -q 'invalid migration filename' "$test_dir/output"; then
    echo "migration runner did not report invalid basename: $invalid" >&2
    cat "$test_dir/output" >&2
    exit 1
  fi
done

grep -q '^ENV AGENT_RSA_PRIVATE_KEY_PATH=/var/lib/agentguild/agent-rsa.pem$' \
  "$repo_root/backend/Dockerfile"
grep -q '^USER postgres$' "$repo_root/backend/Dockerfile"

echo "migration container contracts passed"
