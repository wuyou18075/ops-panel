#!/bin/bash
set -euo pipefail

APP_DIR="/opt/ops-panel"
GO_VERSION="1.21.6"
MODULE_PATH="github.com/wuyou18075/ops-panel"
NODE_MIN_VERSION="22.13.0"

version_ge() { current="$1"; required="$2"; [ "$(printf '%s\n%s\n' "$required" "$current" | sort -V | head -n 1)" = "$required" ]; }

ensure_latest_node() {
  if ! command -v node &> /dev/null; then
    echo "未检测到 Node.js，准备安装最新版..."
    npm install -g n; n latest; export PATH="/usr/local/bin:$PATH"; hash -r; return
  fi
  current_node_version="$(node -v | sed 's/^v//')"
  if version_ge "$current_node_version" "$NODE_MIN_VERSION"; then echo "Node.js 版本满足要求: v$current_node_version"; return; fi
  echo "Node.js 版本过低: v$current_node_version，最低要求: v$NODE_MIN_VERSION，准备升级到最新版..."
  npm install -g n; n latest; export PATH="/usr/local/bin:$PATH"; hash -r
}

install_env() {
  echo "=== 开始安装环境与拉取项目 ==="
  apt-get update; apt-get install -y git wget curl tar nodejs npm
  if ! command -v go &> /dev/null; then
    echo "未检测到 Go，准备下载安装..."
    ARCH=$(uname -m)
    [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ] && GO_FILE="go${GO_VERSION}.linux-arm64.tar.gz" || GO_FILE="go${GO_VERSION}.linux-amd64.tar.gz"
    wget "https://golang.google.cn/dl/$GO_FILE"; rm -rf /usr/local/go; tar -C /usr/local -xzf "$GO_FILE"; rm "$GO_FILE"
    [ "$APP_DIR" != "/" ] && rm -rf "$APP_DIR" 2>/dev/null || true
    if ! grep -q "/usr/local/go/bin" ~/.profile 2>/dev/null; then
      echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.profile
    fi
    echo "Go 环境安装完成。"
  else
    echo "检测到 Go 已安装，跳过下载。"
  fi
  export PATH="$PATH:/usr/local/go/bin"
  ensure_latest_node; echo "Node.js 版本: $(node -v)"; echo "npm 版本: $(npm -v)"
  echo "正在从 Git 拉取项目文件..."
  if [ -d "$APP_DIR" ]; then
    echo "目录 $APP_DIR 已存在，正在清理旧目录..."; rm -rf "$APP_DIR"
  fi
  git clone https://github.com/wuyou18075/ops-panel.git "$APP_DIR"
  echo "开始初始化依赖并创建目录..."; cd "$APP_DIR"
  [ ! -f go.mod ] && go mod init "$MODULE_PATH"
  for pkg in github.com/gorilla/websocket github.com/shirou/gopsutil/v3/cpu github.com/shirou/gopsutil/v3/disk github.com/shirou/gopsutil/v3/host github.com/shirou/gopsutil/v3/load github.com/shirou/gopsutil/v3/mem github.com/shirou/gopsutil/v3/net gopkg.in/telebot.v3; do go get "$pkg"; done
  go mod tidy
  echo "开始构建前端资源..."; npm install -g pnpm; echo "pnpm 版本: $(pnpm -v)"
  cd "$APP_DIR/web"; pnpm install; pnpm build; cd "$APP_DIR"
  mkdir -p master/static agent
  echo "=== 环境安装与项目初始化完成！ ==="
}

start_master() {
  echo "=== 启动 Master 控制端 ==="
  export PATH="$PATH:/usr/local/go/bin"; cd "$APP_DIR"
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
  go run ./master
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
  if [ -d "$APP_DIR" ]; then cd "$APP_DIR"; git pull; echo "=== 代码更新完成 ==="
  else echo "错误: $APP_DIR 目录不存在，请先选择选项 1 安装环境。"; fi
}

delete_local_code() {
  echo "=== 开始删除本地代码 ==="
  [ -d "$APP_DIR" ] && { rm -rf "$APP_DIR"; echo "=== 本地代码目录 $APP_DIR 已成功删除 ==="; } || echo "提示: $APP_DIR 目录本身就不存在，无需删除。"
}

uninstall_all() {
  echo "=== 开始卸载所有环境与代码 ==="
  [ -d "$APP_DIR" ] && { rm -rf "$APP_DIR"; echo "已清除代码目录: $APP_DIR"; }
  [ -d "/usr/local/go" ] && { rm -rf /usr/local/go; echo "已清除 Go 环境目录: /usr/local/go"; }
  echo "=== 所有组件与环境已完全卸载 ==="
}

pause_and_return() { echo; read -r -p "按回车返回主菜单..."; }

show_menu() {
  clear
  echo "======================================"
  echo "      一键脚本面板管理工具            "
  echo "======================================"
  echo "  1. 安装环境 (Go, Git, 依赖与代码)   "
  echo "  2. 启动控制端 (Master)              "
  echo "  3. 注册受控端 (Agent)               "
  echo "  4. 拉取最新代码                    "
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
