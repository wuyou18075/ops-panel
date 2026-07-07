#!/bin/bash

set -euo pipefail

SCRIPT_PATH="${1:-install.sh}"

if grep -q $'\r' "$SCRIPT_PATH"; then
  echo "install.sh must use LF line endings, not CRLF" >&2
  exit 1
fi

bash -n "$SCRIPT_PATH"

if grep -q "go gopkg.in/telebot.v3" "$SCRIPT_PATH"; then
  echo "install.sh must install telebot with go get" >&2
  exit 1
fi
