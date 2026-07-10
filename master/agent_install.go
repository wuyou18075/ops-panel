package main

import (
	"net/http"
)

const agentInstall = `#!/bin/bash
set -euo pipefail
AGENT_ID="${AGENT_ID:?未设置 AGENT_ID}"
AGENT_SECRET="${AGENT_SECRET:?未设置 AGENT_SECRET}"
MASTER_URL="${MASTER_URL:-${MASTER:?未设置 MASTER_URL}}"
if ! command -v go &>/dev/null; then
  GF="go1.25.0.$(uname -m|sed 's/x86_64/linux-amd64/;s/aarch64/linux-arm64/').tar.gz"
  wget -q "https://golang.google.cn/dl/$GF" -O /tmp/go.tar.gz
  tar -C /usr/local -xzf /tmp/go.tar.gz; rm /tmp/go.tar.gz
  export PATH="$PATH:/usr/local/go/bin"
fi
APP_DIR="/opt/ops-panel"
[ -d "$APP_DIR" ] && (cd "$APP_DIR" && git pull) || git clone https://github.com/wuyou18075/ops-panel.git "$APP_DIR"
export AGENT_ID AGENT_SECRET MASTER_URL
cd "$APP_DIR" && go run ./agent
`

func handleAgentInstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(agentInstall))
}
