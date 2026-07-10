#!/bin/bash
set -euo pipefail

APP_DIR="/opt/ops-panel"
GO_VERSION="1.25.0"
MODULE_PATH="github.com/wuyou18075/ops-panel"

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

install_env() {
  echo "=== 开始安装环境与拉取项目 ==="
  apt-get update; apt-get install -y git wget curl tar
  if ! command -v go &> /dev/null; then
    echo "未检测到 Go，准备下载安装..."
    ARCH=$(uname -m)
    [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ] && GO_FILE="go${GO_VERSION}.linux-arm64.tar.gz" || GO_FILE="go${GO_VERSION}.linux-amd64.tar.gz"
    wget "https://golang.google.cn/dl/$GO_FILE"; rm -rf /usr/local/go; tar -C /usr/local -xzf "$GO_FILE"; rm "$GO_FILE"
    if ! grep -q "/usr/local/go/bin" ~/.profile 2>/dev/null; then
      echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.profile
    fi
    echo "Go 环境安装完成。"
  else
    echo "检测到 Go 已安装，跳过下载。"
  fi
  export PATH="$PATH:/usr/local/go/bin"
  echo "正在从 Git 拉取项目文件..."
  if [ -d "$APP_DIR" ]; then echo "目录 $APP_DIR 已存在，正在清理旧目录..."; rm -rf "$APP_DIR"; fi
  git clone https://github.com/wuyou18075/ops-panel.git "$APP_DIR"
  echo "开始初始化依赖并创建目录..."; cd "$APP_DIR"
  mkdir -p agent
  go mod download
  # 小内存机器先建 swap，避免编译 modernc（SQLite）时 OOM
  ensure_swap
  # 前端已随仓库提交到 master/dist（go:embed 直接内嵌），服务器无需 Node/pnpm 构建。
  # -p=1 限制并行编译，降低峰值内存，适配小内存 VPS。
  echo "开始编译 Master（内嵌已构建前端，单并行以省内存）..."
  GOFLAGS=-p=1 go build -o "$APP_DIR/ops-panel-master" ./master
  echo "=== 环境安装与项目初始化完成！二进制: $APP_DIR/ops-panel-master ==="
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
  if [ ! -x "$APP_DIR/ops-panel-master" ]; then
    echo "未找到已编译二进制，正在编译（小内存机器请确保已有 swap）..."
    ensure_swap
    GOFLAGS=-p=1 go build -o "$APP_DIR/ops-panel-master" ./master
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
  if [ -d "$APP_DIR" ]; then
    export PATH="$PATH:/usr/local/go/bin"; cd "$APP_DIR"; git pull
    go mod download; ensure_swap
    echo "重新编译 Master..."; GOFLAGS=-p=1 go build -o "$APP_DIR/ops-panel-master" ./master
    echo "=== 代码更新并重新编译完成 ==="
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
