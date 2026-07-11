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
    MIGRATION_CONNECT_RETRIES=${MIGRATION_CONNECT_RETRIES:-1} \
    MIGRATION_CONNECT_RETRY_DELAY=0 DATABASE_URL=postgres://contract-test \
    PATH="$test_dir/bin:$PATH" sh "$repo_root/deploy/migrate.sh"
}

run_migrate /migrations/000001_valid.up.sql
grep -q '^\\echo '\''migration 000001 already applied'\''' "$test_dir/psql-input" || {
  echo "migration runner did not emit a literal psql echo command" >&2
  exit 1
}

TEST_PSQL_SUCCEEDS_ON_ATTEMPT=3 MIGRATION_CONNECT_RETRIES=3 \
  run_migrate /migrations/000001_valid.up.sql
[ "$(cat "$test_dir/psql-attempts")" -eq 3 ] || {
  echo "migration runner did not retry a transient database connection failure" >&2
  exit 1
}

for invalid in \
  00001_too-short.up.sql \
  0000001_too-long.up.sql \
  abcdef_not-numeric.up.sql \
  000001_.up.sql \
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
