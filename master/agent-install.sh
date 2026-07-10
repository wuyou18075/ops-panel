#!/bin/bash
# Ops-Panel Agent 安装脚本
# 使用方法: curl -fsSL http://HOST:PORT/PATH/agent-install.sh | bash

set -euo pipefail

# 从环境变量读取（由 buildInstallCmd 生成的管道命令传入）
AGENT_ID="${AGENT_ID:?未设置 AGENT_ID}"
AGENT_SECRET="${AGENT_SECRET:?未设置 AGENT_SECRET}"
MASTER_URL="${MASTER_URL:-${MASTER:?未设置 MASTER 或 MASTER_URL}}"
echo "[Agent] ID: $AGENT_ID  Master: $MASTER_URL"

# Go 环境检查
if ! command -v go &>/dev/null; then
  echo "[Agent] 未检测到 Go，下载安装..."
  GO_VERSION="1.21.6"
  ARCH=$(uname -m)
  [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ] && GO_FILE="go${GO_VERSION}.linux-arm64.tar.gz" || GO_FILE="go${GO_VERSION}.linux-amd64.tar.gz"
  wget -q "https://golang.google.cn/dl/$GO_FILE" -O /tmp/go.tar.gz
  tar -C /usr/local -xzf /tmp/go.tar.gz
  rm /tmp/go.tar.gz
  export PATH="$PATH:/usr/local/go/bin"
fi

# 下载项目代码
APP_DIR="/opt/ops-panel"
if [ -d "$APP_DIR" ]; then
  echo "[Agent] 更新已有项目代码..."
  cd "$APP_DIR" && git pull
else
  echo "[Agent] 克隆项目代码..."
  git clone https://github.com/wuyou18075/ops-panel.git "$APP_DIR"
fi
cd "$APP_DIR"

# 注入 Agent 凭证
export AGENT_ID="$AGENT_ID"
export AGENT_SECRET="$AGENT_SECRET"
export MASTER_URL="$MASTER_URL"

# 安装为常驻服务；Master 离线时仍在节点本地累计流量。
echo "[Agent] 安装 systemd 服务..."
go build -o /usr/local/bin/ops-panel-agent ./agent
if ! command -v vnstat >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then apt-get update && apt-get install -y vnstat
  elif command -v dnf >/dev/null 2>&1; then dnf install -y vnstat
  elif command -v yum >/dev/null 2>&1; then yum install -y epel-release && yum install -y vnstat
  elif command -v apk >/dev/null 2>&1; then apk add vnstat
  else echo "请手动安装 vnstat" >&2; exit 1; fi
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
EnvironmentFile=/etc/ops-panel/agent.env
ExecStart=/usr/local/bin/ops-panel-agent
Restart=no
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable ops-panel-agent
systemctl restart ops-panel-agent
echo "[Agent] 已设置开机启动；连接失败不重试，流量由 vnStat 独立持续记录"
