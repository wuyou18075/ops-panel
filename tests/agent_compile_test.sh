#!/bin/bash

set -euo pipefail

SCRIPT_PATH="${1:-agent/main.go}"

if grep -Eq '^[[:space:]]*"log"[[:space:]]*$' "$SCRIPT_PATH"; then
  echo "agent must not import unused log package" >&2
  exit 1
fi
