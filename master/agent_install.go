package main

import (
	"net/http"
)

const agentInstallScript = `#!/bin/bash
set -euo pipefail
AGENT_ID="${AGENT_ID:?未设置 AGENT_ID}"
AGENT_SECRET="${AGENT_SECRET:?未设置 AGENT_SECRET}"
MASTER_URL="${MASTER_URL:-${MASTER:?未设置 MASTER_URL}}"
echo "[Agent] ID: $AGENT_ID  Master: $MASTER_URL"
if ! command -v go &>/dev/null; then
  echo "[Agent] 下载 Go..."
  GO_VERSION="1.21.6"
  ARCH=$(uname -m)
  [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ] && GF="go${GO_VERSION}.linux-arm64.tar.gz" || GF="go${GO_VERSION}.linux-amd64.tar.gz"
  wget -q "https://golang.google.cn/dl/$GF" -O /tmp/go.tar.gz
  tar -C /usr/local -xzf /tmp/go.tar.gz; rm /tmp/go.tar.gz
  export PATH="$PATH:/usr/local/go/bin"
fi
APP_DIR="/opt/ops-panel"
if [ -d "$APP_DIR" ]; then cd "$APP_DIR" && git pull; else git clone https://github.com/wuyou18075/ops-panel.git "$APP_DIR" && cd "$APP_DIR"; fi
export AGENT_ID AGENT_SECRET MASTER_URL
go run ./agent
`

func handleAgentInstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(agentInstallScript))
}