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

echo "migration container contracts passed"
