#!/bin/bash
set -euo pipefail

APP_DIR="/opt/ops-panel"
GO_VERSION="1.25.0"
REPO_URL="https://github.com/wuyou18075/ops-panel.git"
RELEASE_BASE="https://github.com/wuyou18075/ops-panel/releases/latest/download"

# ensure_swap 在小内存机器（<1.5G 且 swap 不足）上创建 2G swap，
# 避免编译 modernc.org/libc（SQLite 依赖）时内存耗尽触发 OOM 杀掉 sshd。
ensure_swap() {
  local mem_total swap_total
  mem_total=$(free -m | awk '/^Mem:/{print $2}')
  swap_total=$(free -m | awk '/^Swap:/{print $2}')
  if [ "${mem_total:-0}" -ge 1536 ]; then return; fi
  if [ "${swap_total:-0}" -ge 1024 ]; then echo "已有 swap ${swap_total}MB，跳过创建。"; return; fi
  if [ -f /swapfile ]; then swapon /swapfile 2>/dev/null || true; return; fi
  echo "内存仅 ${mem_total}MB 且 swap 不足，创建 2G swap 防止编译 OOM..."
  fallocate -l 2G /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=2048
  chmod 600 /swapfile; mkswap /swapfile; swapon /swapfile
  grep -q '/swapfile' /etc/fstab 2>/dev/null || echo '/swapfile none swap sw 0 0' >> /etc/fstab
  echo "swap 已启用。"
}

arch_tag() {
  case "$(uname -m)" in
    x86_64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *) echo "" ;;
  esac
}

install_go() {
  if command -v go &> /dev/null; then echo "检测到 Go 已安装，跳过下载。"; export PATH="$PATH:/usr/local/go/bin"; return; fi
  echo "未检测到 Go，准备下载安装 ${GO_VERSION}..."
  local arch GO_FILE
  arch=$(uname -m)
  { [ "$arch" = "aarch64" ] || [ "$arch" = "arm64" ]; } && GO_FILE="go${GO_VERSION}.linux-arm64.tar.gz" || GO_FILE="go${GO_VERSION}.linux-amd64.tar.gz"
  wget "https://golang.google.cn/dl/$GO_FILE"; rm -rf /usr/local/go; tar -C /usr/local -xzf "$GO_FILE"; rm "$GO_FILE"
  grep -q "/usr/local/go/bin" ~/.profile 2>/dev/null || echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.profile
  export PATH="$PATH:/usr/local/go/bin"
  echo "Go 环境安装完成。"
}

clone_repo() {
  echo "正在拉取项目文件..."
  if [ -d "$APP_DIR/.git" ]; then (cd "$APP_DIR" && git pull); else rm -rf "$APP_DIR"; git clone "$REPO_URL" "$APP_DIR"; fi
  mkdir -p "$APP_DIR/agent"
}

# 方式1：直接下载预编译静态二进制（无需 Go/Node，最省内存，适合 1G 小机）
install_from_binary() {
  echo "=== 方式1：使用预编译二进制 ==="
  local tag; tag=$(arch_tag)
  if [ -z "$tag" ]; then echo "架构 $(uname -m) 暂无预编译包，改用本地编译。"; install_from_source; return; fi
  clone_repo
  local url="${RELEASE_BASE}/ops-panel-master-linux-${tag}"
  echo "下载 $url ..."
  if wget -O "$APP_DIR/ops-panel-master" "$url"; then
    chmod +x "$APP_DIR/ops-panel-master"; echo "binary" > "$APP_DIR/.mode"
    echo "=== 二进制就绪：$APP_DIR/ops-panel-master ==="
  else
    echo "下载失败（网络问题或尚未发布该架构），回退到本地编译。"
    rm -f "$APP_DIR/ops-panel-master"; install_from_source
  fi
}

# 可选：重建前端（仅改过 web/ 源码才需要；占内存大）
build_frontend() {
  echo "安装 Node 并构建前端..."
  apt-get install -y nodejs npm; npm install -g pnpm
  cd "$APP_DIR/web"
  NODE_OPTIONS='--max-old-space-size=768' pnpm install
  NODE_OPTIONS='--max-old-space-size=768' pnpm build
  cd "$APP_DIR"
}

# 方式2：本地从源码编译（保留原有本机编译能力）
install_from_source() {
  echo "=== 方式2：本地从源码编译 ==="
  install_go
  clone_repo; cd "$APP_DIR"
  go mod download
  ensure_swap
  read -r -p "是否重新构建前端？(仅改过 web/ 源码才需要，占内存大) [y/N]: " rebuild_fe
  [ "${rebuild_fe,,}" = "y" ] && build_frontend
  echo "编译 Master（前端已内嵌 master/dist，单并行 -p=1 省内存）..."
  GOFLAGS=-p=1 go build -o "$APP_DIR/ops-panel-master" ./master
  echo "source" > "$APP_DIR/.mode"
  echo "=== 编译完成：$APP_DIR/ops-panel-master ==="
}

install_env() {
  echo "=== 开始安装环境与拉取项目 ==="
  apt-get update; apt-get install -y git wget curl tar
  echo ""
  echo "请选择安装方式："
  echo "  1. 直接使用预编译二进制（推荐，无需 Go/Node，最省内存，适合 1G 小机）"
  echo "  2. 本地从源码编译（需要 Go，会自动建 swap；可改代码后自行编译）"
  read -r -p "请输入 [1/2，默认 1]: " mode
  case "$mode" in
    2) install_from_source ;;
    *) install_from_binary ;;
  esac
}

start_master() {
  echo "=== 启动 Master 控制端 ==="
  cd "$APP_DIR" 2>/dev/null || { echo "错误：$APP_DIR 不存在，请先执行选项 1 安装。"; return; }
  read -r -p "请输入 TG 机器人 Token (留空则仅启动 Web 面板): " tg_token
  read -r -p "请输入服务端口 (留空默认 8080): " master_port
  read -r -p "请输入路径前缀 (留空随机生成, 如 /sbg): " master_path
  read -r -p "请输入运维用户名 (留空默认 admin): " op_user
  read -r -p "请输入运维密码 (留空随机生成 8 位): " op_pass
  echo "正在启动..."
  [ -n "$tg_token" ] && export TG_TOKEN="$tg_token"
  [ -n "$master_port" ] && export MASTER_PORT="$master_port"
  [ -n "$master_path" ] && export MASTER_PATH="$master_path"
  [ -n "$op_user" ] && export OPERATOR_USERNAME="$op_user"
  [ -n "$op_pass" ] && export OPERATOR_PASSWORD="$op_pass"
  if [ ! -x "$APP_DIR/ops-panel-master" ]; then
    echo "未找到已编译二进制，请先执行选项 1 安装。"; return
  fi
  exec "$APP_DIR/ops-panel-master"
}

start_agent() {
  echo "=== Agent 注册说明 ==="
  echo ""
  echo "Agent 注册已改为 Web 面板操作："
  echo "1. 启动 Master 后，打开 Web 面板"
  echo "2. 登录 -> 点击「添加节点」"
  echo "3. 填写备注、选择分组、设置偏好"
  echo "4. 生成注册命令，在目标 VPS 上执行"
}

pull_latest() {
  echo "=== 开始拉取最新代码 ==="
  if [ ! -d "$APP_DIR/.git" ]; then echo "错误: $APP_DIR 非 git 安装，请先选择选项 1 安装。"; return; fi
  cd "$APP_DIR"; git pull
  local mode; mode=$(cat "$APP_DIR/.mode" 2>/dev/null || echo source)
  if [ "$mode" = "binary" ]; then
    local tag; tag=$(arch_tag)
    echo "更新预编译二进制..."
    wget -O "$APP_DIR/ops-panel-master" "${RELEASE_BASE}/ops-panel-master-linux-${tag}" && chmod +x "$APP_DIR/ops-panel-master"
  else
    install_go; go mod download; ensure_swap
    echo "重新编译 Master..."; GOFLAGS=-p=1 go build -o "$APP_DIR/ops-panel-master" ./master
  fi
  echo "=== 代码更新完成 ==="
}

delete_local_code() {
  echo "=== 开始删除本地代码 ==="
  [ -d "$APP_DIR" ] && { rm -rf "$APP_DIR"; echo "=== 本地代码目录 $APP_DIR 已成功删除 ==="; } || echo "提示: $APP_DIR 目录本身就不存在，无需删除。"
}

uninstall_all() {
  echo "=== 开始卸载所有环境与代码 ==="
  [ -d "$APP_DIR" ] && { rm -rf "$APP_DIR"; echo "已清除代码目录: $APP_DIR"; }
  [ -d "/usr/local/go" ] && { rm -rf /usr/local/go; echo "已清除 Go 环境目录: /usr/local/go"; }
  if command -v pnpm &> /dev/null; then npm uninstall -g pnpm 2>/dev/null && echo "已卸载 pnpm" || true; fi
  PNPM_HOME="${PNPM_HOME:-$HOME/.local/share/pnpm}"
  [ -d "$PNPM_HOME" ] && { rm -rf "$PNPM_HOME"; echo "已清除 pnpm 数据"; }
  N_PREFIX="${N_PREFIX:-$HOME/n}"
  [ -d "$N_PREFIX" ] && { rm -rf "$N_PREFIX"; echo "已清除 n 数据"; }
  npm cache clean --force 2>/dev/null && echo "已清理 npm 缓存" || true
  [ -f ~/.profile ] && sed -i '/\/usr\/local\/go\/bin/d' ~/.profile 2>/dev/null && echo "已清理 ~/.profile 中的 Go 路径"
  echo "=== 所有组件与环境已完全卸载 ==="
}

pause_and_return() { echo; read -r -p "按回车返回主菜单..."; }

show_menu() {
  clear
  echo "======================================"
  echo "      一键脚本面板管理工具            "
  echo "======================================"
  echo "  1. 安装（可选：二进制 / 本地编译）  "
  echo "  2. 启动控制端 (Master)              "
  echo "  3. 注册受控端 (Agent)               "
  echo "  4. 拉取最新代码并更新              "
  echo "  60. 删除本地代码                   "
  echo "  99. 卸载所有                       "
  echo "  0. 退出                            "
  echo "======================================"
}

while true; do
  show_menu
  read -r -p "请输入序号选择对应的操作: " choice
  case $choice in
    1) install_env; pause_and_return ;;
    2) start_master; pause_and_return ;;
    3) start_agent; pause_and_return ;;
    4) pull_latest; pause_and_return ;;
    60) delete_local_code; pause_and_return ;;
    99) uninstall_all; pause_and_return ;;
    0) echo "脚本已退出。"; exit 0 ;;
    *) echo "输入无效，请重新选择。"; pause_and_return ;;
  esac
done
