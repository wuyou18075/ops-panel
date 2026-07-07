#!/bin/bash

set -euo pipefail

SCRIPT_PATH="${1:-master/main.go}"

if grep -q "http://0.0.0.0:8080" "$SCRIPT_PATH"; then
  echo "master startup log must not show 0.0.0.0 as the Web panel address" >&2
  exit 1
fi

if ! grep -q "publicIPv4" "$SCRIPT_PATH"; then
  echo "master startup log must resolve the public IPv4 address" >&2
  exit 1
fi
