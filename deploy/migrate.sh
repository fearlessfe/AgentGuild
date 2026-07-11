#!/bin/sh
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"

migrations_dir=/migrations
files=$(find "$migrations_dir" -type f -name '*.up.sql' | sort)

if [ -z "$files" ]; then
  echo "no migrations found in $migrations_dir" >&2
  exit 1
fi

for file in $files; do
  name=$(basename "$file")
  matched=$(expr "$name" : '[0-9][0-9][0-9][0-9][0-9][0-9]_[^/][^/]*\.up\.sql$') || matched=0
  if [ "$matched" -ne "${#name}" ]; then
    echo "invalid migration filename: $name" >&2
    exit 1
  fi
done

prefixes=$(
  for file in $files; do
    basename "$file" | sed 's/_.*//'
  done
)
duplicates=$(printf '%s\n' "$prefixes" | sort | uniq -d)
if [ -n "$duplicates" ]; then
  echo "duplicate migration numeric prefix: $duplicates" >&2
  exit 1
fi

migration_connect_retries=${MIGRATION_CONNECT_RETRIES:-30}
migration_connect_retry_delay=${MIGRATION_CONNECT_RETRY_DELAY:-2}
case $migration_connect_retries in
  ''|*[!0-9]*|0) echo "MIGRATION_CONNECT_RETRIES must be a positive integer" >&2; exit 1 ;;
esac
case $migration_connect_retry_delay in
  ''|*[!0-9]*) echo "MIGRATION_CONNECT_RETRY_DELAY must be a non-negative integer" >&2; exit 1 ;;
esac

sql_file=$(mktemp)
trap 'rm -f "$sql_file"' EXIT HUP INT TERM

{
  cat <<'SQL'
SELECT pg_advisory_lock(hashtext('agentguild-schema-migrations'));
CREATE TABLE IF NOT EXISTS schema_migrations (
  version text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
SQL

  for file in $files; do
    name=$(basename "$file")
    version=${name%%_*}
    printf "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '%s') AS migration_applied \\gset\n" "$version"
    printf '\\if :migration_applied\n'
    printf '%s\n' "\\echo 'migration $version already applied'"
    printf '\\else\nBEGIN;\n'
    printf '\\i %s\n' "$file"
    printf "INSERT INTO schema_migrations (version) VALUES ('%s');\n" "$version"
    printf 'COMMIT;\n\\endif\n'
  done

  cat <<'SQL'
SELECT pg_advisory_unlock(hashtext('agentguild-schema-migrations'));
SQL
} >"$sql_file"

attempt=1
while :; do
  if psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <"$sql_file"; then
    exit 0
  else
    status=$?
  fi

  if [ "$status" -ne 2 ] || [ "$attempt" -ge "$migration_connect_retries" ]; then
    exit "$status"
  fi

  echo "database unavailable; retrying migration connection ($attempt/$migration_connect_retries)" >&2
  attempt=$((attempt + 1))
  sleep "$migration_connect_retry_delay"
done
