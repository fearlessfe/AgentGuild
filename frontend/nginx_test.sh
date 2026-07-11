#!/bin/sh
set -eu

config="$(dirname "$0")/nginx.conf"

fail() {
  echo "nginx contract failed: $1" >&2
  exit 1
}

[ -f "$config" ] || fail "nginx.conf does not exist"

grep -F 'try_files $uri $uri/ /index.html;' "$config" >/dev/null || fail "SPA fallback is missing"
grep -F 'location = /healthz' "$config" >/dev/null || fail "/healthz endpoint is missing"
grep -F 'add_header Cache-Control "no-store" always;' "$config" >/dev/null || fail "proxied responses are not marked no-store"

for header in \
  'proxy_set_header X-Request-ID $request_id;' \
  'proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;' \
  'proxy_set_header X-Forwarded-Host $host;' \
  'proxy_set_header X-Forwarded-Proto $scheme;' \
  'proxy_set_header Authorization $http_authorization;'
do
  grep -F "$header" "$config" >/dev/null || fail "missing forwarded header: $header"
done

grep -F 'location /api/' "$config" >/dev/null || fail "/api/ proxy is missing"
grep -F 'proxy_pass http://backend:8080/;' "$config" >/dev/null || fail "/api/ does not remove its prefix"
grep -F 'location /oauth/' "$config" >/dev/null || fail "/oauth/ proxy is missing"
grep -F 'location = /mcp' "$config" >/dev/null || fail "/mcp proxy is missing"
grep -F 'proxy_pass http://backend:8080;' "$config" >/dev/null || fail "direct backend proxy is missing"

listen_count="$(grep -Ec '^[[:space:]]*listen[[:space:]]+' "$config" || true)"
[ "$listen_count" -eq 1 ] || fail "expected exactly one listen directive"
grep -E '^[[:space:]]*listen[[:space:]]+8080;' "$config" >/dev/null || fail "container must listen on port 8080"

echo "nginx contract passed"
