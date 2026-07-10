package main

import (
	"net/http"
)

const agentInstall = `#!/bin/bash
set -euo pipefail
AGENT_ID="${AGENT_ID:?未设置 AGENT_ID}"
AGENT_SECRET="${AGENT_SECRET:?未设置 AGENT_SECRET}"
MASTER_URL="${MASTER_URL:-${MASTER:?未设置 MASTER_URL}}"
export PATH="$PATH:/usr/local/go/bin"
# 需要 Go 1.25+；缺失/过低/损坏（混装）时先 rm -rf 再干净重装，避免 runtime 文件冲突
if ! go version 2>/dev/null | grep -qE 'go1\.(2[5-9]|[3-9][0-9])'; then
  echo "安装/修复 Go 1.25.0..."
  GF="go1.25.0.$(uname -m|sed 's/x86_64/linux-amd64/;s/aarch64/linux-arm64/').tar.gz"
  wget -q "https://golang.google.cn/dl/$GF" -O /tmp/go.tar.gz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tar.gz; rm /tmp/go.tar.gz
  export PATH="/usr/local/go/bin:$PATH"
fi
APP_DIR="/opt/ops-panel"
[ -d "$APP_DIR/.git" ] && (cd "$APP_DIR" && git pull) || { rm -rf "$APP_DIR"; git clone https://github.com/wuyou18075/ops-panel.git "$APP_DIR"; }
export AGENT_ID AGENT_SECRET MASTER_URL
cd "$APP_DIR" && go build -o /tmp/ops-agent ./agent && exec /tmp/ops-agent
`

func handleAgentInstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(agentInstall))
}
