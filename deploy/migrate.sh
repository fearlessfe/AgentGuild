#!/bin/sh
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"

migrations_dir=/migrations
files=$(find "$migrations_dir" -type f -name '*.up.sql' | sort)

if [ -z "$files" ]; then
  echo "no migrations found in $migrations_dir" >&2
  exit 1
fi

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
    printf "\\echo 'migration %s already applied'\n" "$version"
    printf '\\else\nBEGIN;\n'
    printf '\\i %s\n' "$file"
    printf "INSERT INTO schema_migrations (version) VALUES ('%s');\n" "$version"
    printf 'COMMIT;\n\\endif\n'
  done

  cat <<'SQL'
SELECT pg_advisory_unlock(hashtext('agentguild-schema-migrations'));
SQL
} | psql "$DATABASE_URL" -v ON_ERROR_STOP=1
