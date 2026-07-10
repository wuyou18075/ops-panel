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
cd "$APP_DIR" && go build -o /usr/local/bin/ops-panel-agent ./agent
if ! command -v vnstat >/dev/null 2>&1; then
  echo "安装独立持久化流量统计 vnStat..."
  if command -v apt-get >/dev/null 2>&1; then apt-get update && apt-get install -y vnstat
  elif command -v dnf >/dev/null 2>&1; then dnf install -y vnstat
  elif command -v yum >/dev/null 2>&1; then yum install -y epel-release && yum install -y vnstat
  elif command -v apk >/dev/null 2>&1; then apk add vnstat
  else echo "无法识别包管理器，请手动安装 vnstat" >&2; exit 1; fi
fi
IFACE="$(ip route show default 2>/dev/null | awk 'NR==1{print $5}')"
[ -n "$IFACE" ] && vnstat --add -i "$IFACE" >/dev/null 2>&1 || true
systemctl enable --now vnstat
mkdir -p /etc/ops-panel
cat > /etc/ops-panel/agent.env <<EOF
AGENT_ID=$AGENT_ID
AGENT_SECRET=$AGENT_SECRET
MASTER_URL=$MASTER_URL
EOF
chmod 600 /etc/ops-panel/agent.env
cat > /etc/systemd/system/ops-panel-agent.service <<'EOF'
[Unit]
Description=Ops Panel monitoring agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/ops-panel/agent.env
ExecStart=/usr/local/bin/ops-panel-agent
Restart=no

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable ops-panel-agent
systemctl restart ops-panel-agent
echo "Agent 已设置为开机启动且连接失败不重试；流量由独立 vnStat 服务持续记录"
`

func handleAgentInstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(agentInstall))
}
