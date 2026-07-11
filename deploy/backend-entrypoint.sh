#!/bin/sh
set -eu

: "${AGENT_RSA_PRIVATE_KEY_PATH:=/var/lib/agentguild/agent-rsa.pem}"
export AGENT_RSA_PRIVATE_KEY_PATH

if [ ! -f "$AGENT_RSA_PRIVATE_KEY_PATH" ]; then
  key_dir=$(dirname -- "$AGENT_RSA_PRIVATE_KEY_PATH")
  mkdir -p "$key_dir"
  umask 077
  key_tmp=$(mktemp "$key_dir/.agent-rsa.pem.tmp.XXXXXX")
  trap 'rm -f "$key_tmp"' EXIT HUP INT TERM
  openssl genpkey \
    -algorithm RSA \
    -pkeyopt rsa_keygen_bits:2048 \
    -out "$key_tmp"
  chmod 0600 "$key_tmp"

  if ln "$key_tmp" "$AGENT_RSA_PRIVATE_KEY_PATH" 2>/dev/null; then
    rm -f "$key_tmp"
  else
    rm -f "$key_tmp"
    if [ ! -f "$AGENT_RSA_PRIVATE_KEY_PATH" ]; then
      echo "failed to install Agent RSA private key" >&2
      exit 1
    fi
  fi
  trap - EXIT HUP INT TERM
fi

exec /usr/local/bin/agentguild-api
