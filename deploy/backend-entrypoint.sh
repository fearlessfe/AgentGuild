#!/bin/sh
set -eu

if [ -n "${AGENT_RSA_PRIVATE_KEY_PATH:-}" ] && [ ! -f "$AGENT_RSA_PRIVATE_KEY_PATH" ]; then
  key_dir=$(dirname -- "$AGENT_RSA_PRIVATE_KEY_PATH")
  mkdir -p "$key_dir"
  umask 077
  openssl genpkey \
    -algorithm RSA \
    -pkeyopt rsa_keygen_bits:2048 \
    -out "$AGENT_RSA_PRIVATE_KEY_PATH"
  chmod 0600 "$AGENT_RSA_PRIVATE_KEY_PATH"
fi

exec /usr/local/bin/agentguild-api
