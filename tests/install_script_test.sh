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

if ! grep -q "while true" "$SCRIPT_PATH"; then
  echo "install.sh must keep the menu open after actions" >&2
  exit 1
fi

if ! grep -q "按回车返回主菜单" "$SCRIPT_PATH"; then
  echo "install.sh must prompt Enter before returning to the menu" >&2
  exit 1
fi

if ! grep -q 'NODE_MIN_VERSION="22.13.0"' "$SCRIPT_PATH"; then
  echo "install.sh must define the minimum Node.js version required by latest pnpm" >&2
  exit 1
fi

if ! grep -q "version_ge()" "$SCRIPT_PATH"; then
  echo "install.sh must compare Node.js versions before upgrading" >&2
  exit 1
fi

if ! grep -q "ensure_latest_node" "$SCRIPT_PATH"; then
  echo "install.sh must upgrade Node.js only when the local version is too old" >&2
  exit 1
fi

if ! grep -q "npm install -g pnpm" "$SCRIPT_PATH"; then
  echo "install.sh must install the latest pnpm after upgrading Node.js" >&2
  exit 1
fi
