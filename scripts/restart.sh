#!/usr/bin/env bash
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# STX 构建/重启脚本：
# - 后端使用 PM2 启动（stx-api）
# - 前端默认使用 Next.js standalone 产物 + PM2 启动（stx-ui）
# - 支持统一的目标（前端/后端）与动作（构建/重启）
# - 后端构建后默认同步并重启已安装的本机 stx-agent
# - 启动前会检测并清理同名 PM2 进程，最后执行 pm2 save

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

# 打印构建与重启脚本的用法。/ Print build and restart usage.
print_help() {
  cat <<'EOF'
STX 构建/重启脚本

用法:
  ./scripts/restart.sh [选项]

选项:
  --build-only     仅构建（不重启前后端 PM2；本机已安装 Agent 时仍默认同步/重启）
  --restart-only   仅重启（不构建）
  --frontend-only  仅处理前端
  --backend-only   仅处理后端
  --no-build       兼容旧参数，等价于 --restart-only
  --no-frontend    兼容旧参数，跳过前端
  --no-backend     跳过后端
  --frontend-dev   前端用 pnpm run dev 启动（仅重启前端时生效）
  --no-local-agent-restart
                   后端构建后不同步/不重启本机 stx-agent（默认仅在本机已安装时同步并重启）
  --restart-java-proxy
                   构建/重启后，重启本机 stx-java-proxy
  --stop-frontend  仅停止前端 PM2 进程并退出
  -h, --help       显示本帮助

示例:
  ./scripts/restart.sh                               # 默认：构建并重启前后端；本机已安装 Agent 时同步/重启
  ./scripts/restart.sh --restart-only --frontend-only # 仅重启前端
  ./scripts/restart.sh --build-only --backend-only    # 构建后端；本机已安装 Agent 时同步/重启
  ./scripts/restart.sh --backend-only --no-local-agent-restart
  ./scripts/restart.sh --backend-only --restart-java-proxy
  ./scripts/restart.sh --build-only                   # 仅构建前后端；本机已安装 Agent 时同步/重启

环境变量:
  PM2_API                        后端 PM2 进程名，默认 stx-api
  PM2_UI                         前端 PM2 进程名，默认 stx-ui
  CONFIG_PATH                    后端配置文件路径，默认 ./config.yaml
  APP_EXTERNAL_URL               写入 config.yaml 的 app.external_url，默认 http://127.0.0.1:8000
  FRONTEND_PORT                  前端端口，默认 80
  NEXT_PUBLIC_BACKEND_BASE_URL   前端访问后端的基础地址，默认 http://127.0.0.1:8000
  LOCAL_AGENT_INSTALL_DIR        本机 Agent 二进制目录，默认 $HOME/.stx/agent/bin
  LOCAL_AGENT_BINARY             本机 Agent 二进制名，默认 stx-agent
  LOCAL_AGENT_SERVICE            本机 Agent systemd 服务名，默认 stx-agent
  LOCAL_AGENT_RESTART            本机已安装 Agent 时是否默认同步/重启，默认 true
  LOCAL_SEATUNNEL_HOME           本机 SeaTunnel 安装目录，默认 /opt/seatunnel-2.3.13-new
  LOCAL_JAVA_PROXY_PORT          本机 stx-java-proxy 端口，默认 18080
  CONTROL_PLANE_BASE_URL         控制面地址，默认 http://127.0.0.1:8000
  CONTROL_PLANE_USERNAME         登录用户名，默认 admin
  CONTROL_PLANE_PASSWORD         登录密码，默认 admin123
EOF
}

BUILD_ONLY=false
RESTART_ONLY=false
NO_BUILD=false
NO_FRONTEND=false
NO_BACKEND=false
FRONTEND_ONLY=false
BACKEND_ONLY=false
STOP_FRONTEND=false
FRONTEND_DEV=false
RESTART_JAVA_PROXY=false
case "${LOCAL_AGENT_RESTART:-true}" in
  true|TRUE|1|yes|YES|y|Y) RESTART_LOCAL_AGENT=true ;;
  false|FALSE|0|no|NO|n|N) RESTART_LOCAL_AGENT=false ;;
  *)
    echo "LOCAL_AGENT_RESTART 取值无效: ${LOCAL_AGENT_RESTART}（支持 true/false）"
    exit 1
    ;;
esac
while [[ $# -gt 0 ]]; do
  arg="$1"
  case "$arg" in
    -h|--help)
      print_help
      exit 0
      ;;
    --build-only) BUILD_ONLY=true ;;
    --restart-only) RESTART_ONLY=true ;;
    --frontend-only) FRONTEND_ONLY=true ;;
    --backend-only) BACKEND_ONLY=true ;;
    --no-build) NO_BUILD=true ;;
    --frontend-dev) FRONTEND_DEV=true ;;
    --no-frontend) NO_FRONTEND=true ;;
    --no-backend) NO_BACKEND=true ;;
    --stop-frontend) STOP_FRONTEND=true ;;
    --no-local-agent-restart) RESTART_LOCAL_AGENT=false ;;
    --restart-java-proxy) RESTART_JAVA_PROXY=true ;;
    *)
      echo "未知参数: $arg"
      echo
      print_help
      exit 1
      ;;
  esac
  shift
done

if $BUILD_ONLY && $RESTART_ONLY; then
  echo "参数冲突: --build-only 与 --restart-only 不能同时使用"
  exit 1
fi

if $FRONTEND_ONLY && $BACKEND_ONLY; then
  echo "参数冲突: --frontend-only 与 --backend-only 不能同时使用"
  exit 1
fi

DO_BUILD=true
DO_RESTART=true
if $BUILD_ONLY; then
  DO_RESTART=false
fi
if $RESTART_ONLY || $NO_BUILD; then
  DO_BUILD=false
fi

RUN_BACKEND=true
RUN_FRONTEND=true
if $FRONTEND_ONLY; then
  RUN_BACKEND=false
fi
if $BACKEND_ONLY; then
  RUN_FRONTEND=false
fi
if $NO_FRONTEND; then
  RUN_FRONTEND=false
fi
if $NO_BACKEND; then
  RUN_BACKEND=false
fi

if ! $RUN_BACKEND && ! $RUN_FRONTEND; then
  echo "没有可执行目标：前后端都被禁用了"
  exit 1
fi

PM2_API="${PM2_API:-stx-api}"
PM2_UI="${PM2_UI:-stx-ui}"
CONFIG_PATH="${CONFIG_PATH:-$PROJECT_ROOT/config.yaml}"
APP_EXTERNAL_URL="${APP_EXTERNAL_URL:-http://127.0.0.1:8000}"
FRONTEND_PORT="${FRONTEND_PORT:-80}"
NEXT_PUBLIC_BACKEND_BASE_URL="${NEXT_PUBLIC_BACKEND_BASE_URL:-http://127.0.0.1:8000}"
CAPABILITY_PROXY_DEFAULT_VERSION="${CAPABILITY_PROXY_DEFAULT_VERSION:-2.3.13}"
LOCAL_AGENT_INSTALL_DIR="${LOCAL_AGENT_INSTALL_DIR:-$HOME/.stx/agent/bin}"
LOCAL_AGENT_BINARY="${LOCAL_AGENT_BINARY:-stx-agent}"
LOCAL_AGENT_SERVICE="${LOCAL_AGENT_SERVICE:-stx-agent}"
AGENT_HOME="${AGENT_HOME:-$HOME/.stx/agent}"
AGENT_PROXY_LIB_DIR="${AGENT_PROXY_LIB_DIR:-$AGENT_HOME/lib}"
LOCAL_SEATUNNEL_HOME="${LOCAL_SEATUNNEL_HOME:-/opt/seatunnel-2.3.13-new}"
LOCAL_JAVA_PROXY_PORT="${LOCAL_JAVA_PROXY_PORT:-18080}"
CONTROL_PLANE_BASE_URL="${CONTROL_PLANE_BASE_URL:-http://127.0.0.1:8000}"
CONTROL_PLANE_USERNAME="${CONTROL_PLANE_USERNAME:-admin}"
CONTROL_PLANE_PASSWORD="${CONTROL_PLANE_PASSWORD:-admin123}"
FRONTEND_DIR="$PROJECT_ROOT/frontend"
FRONTEND_STANDALONE_DIR="$FRONTEND_DIR/dist-standalone"
FRONTEND_ENTRY=""
FRONTEND_RUNTIME_DIR="$FRONTEND_STANDALONE_DIR"

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "缺少命令: $cmd"
    exit 1
  fi
}

detect_agent_goos() {
  if [[ -n "${GOOS:-}" ]]; then
    echo "$GOOS"
    return 0
  fi
  go env GOOS
}

detect_agent_goarch() {
  if [[ -n "${GOARCH:-}" ]]; then
    echo "$GOARCH"
    return 0
  fi
  go env GOARCH
}

detect_host_goos() {
  go env GOHOSTOS 2>/dev/null || uname -s | tr '[:upper:]' '[:lower:]'
}

detect_host_goarch() {
  local host_arch=""
  host_arch="$(go env GOHOSTARCH 2>/dev/null || true)"
  if [[ -n "$host_arch" ]]; then
    echo "$host_arch"
    return 0
  fi
  case "$(uname -m)" in
    x86_64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) uname -m ;;
  esac
}

# 返回目标系统和架构对应的 STX Agent 文件名。/ Return the STX Agent file name for the target OS and architecture.
agent_binary_name_for_target() {
  local goos="$1"
  local goarch="$2"

  case "$goos" in
    linux|darwin) ;;
    *)
      echo "不支持同步的 Agent 目标系统: $goos（支持: linux, darwin）" >&2
      return 1
      ;;
  esac

  case "$goarch" in
    amd64|arm64) ;;
    *)
      echo "不支持同步的 Agent 目标架构: $goarch（支持: amd64, arm64）" >&2
      return 1
      ;;
  esac

  echo "stx-agent-${goos}-${goarch}"
}

sync_and_restart_local_agent() {
  local built_binary="$1"
  local target_goos="$2"
  local target_goarch="$3"
  local host_goos=""
  local host_goarch=""
  local target_path="${LOCAL_AGENT_INSTALL_DIR}/${LOCAL_AGENT_BINARY}"
  local temp_path="${target_path}.new"

  host_goos="$(detect_host_goos)"
  host_goarch="$(detect_host_goarch)"

  if [[ "$target_goos" != "$host_goos" || "$target_goarch" != "$host_goarch" ]]; then
    echo "      跳过本机 Agent 同步：构建目标 ${target_goos}/${target_goarch} 与本机 ${host_goos}/${host_goarch} 不一致."
    return 0
  fi

  if [[ "$target_goos" != "linux" ]]; then
    echo "      跳过本机 Agent 重启：当前脚本仅自动管理 Linux systemd 服务."
    return 0
  fi

  if ! command -v systemctl >/dev/null 2>&1; then
    echo "      未找到 systemctl，跳过本机 Agent 同步/重启."
    return 0
  fi

  if ! systemctl cat "$LOCAL_AGENT_SERVICE" >/dev/null 2>&1; then
    echo "      未找到本机 Agent systemd 服务 ${LOCAL_AGENT_SERVICE}，跳过同步/重启."
    return 0
  fi

  if [[ ! -e "$target_path" ]]; then
    echo "      未检测到本机 Agent 二进制 ${target_path}，跳过同步/重启."
    return 0
  fi

  if [[ ! -f "$built_binary" ]]; then
    echo "      未找到已构建的 Agent 二进制: $built_binary"
    return 1
  fi

  cp -f "$built_binary" "$temp_path"
  chmod +x "$temp_path"
  mv -f "$temp_path" "$target_path"
  echo "      已同步本机 Agent 到 ${target_path}."

  systemctl restart "$LOCAL_AGENT_SERVICE"
  if systemctl is-active --quiet "$LOCAL_AGENT_SERVICE"; then
    echo "      本机 Agent 服务已重启: ${LOCAL_AGENT_SERVICE}."
  else
    echo "      本机 Agent 服务重启后未处于 active 状态: ${LOCAL_AGENT_SERVICE}"
    return 1
  fi
}

# 判断进程是否为 STX Java Proxy，避免误杀端口占用者。/ Check whether a process is STX Java Proxy to avoid killing an unrelated listener.
is_java_proxy_pid() {
  local pid="$1"
  if [[ -z "$pid" ]]; then
    return 1
  fi
  local args=""
  args="$(ps -p "$pid" -o args= 2>/dev/null || true)"
  if [[ -z "$args" ]]; then
    return 1
  fi
  if [[ "$args" == *"StxJavaProxyApplication"* ]] || [[ "$args" == *"stx-java-proxy"* ]]; then
    return 0
  fi
  return 1
}

# 使用新名称的脚本与状态目录重启本机 STX Java Proxy。/ Restart the local STX Java Proxy with the renamed script and state directory.
restart_local_java_proxy() {
  local install_dir="$LOCAL_SEATUNNEL_HOME"
  local port="$LOCAL_JAVA_PROXY_PORT"
  local script_path=""
  local jar_path="$AGENT_PROXY_LIB_DIR/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar"
  local state_dir="$install_dir/.stx/stx-java-proxy"
  local log_path="$state_dir/service.log"
  local pid_path="$state_dir/service.pid"
  local port_path="$state_dir/service.port"
  local old_pid=""

  for candidate in \
    "$AGENT_HOME/scripts/stx-java-proxy.sh" \
    "$PROJECT_ROOT/scripts/stx-java-proxy.sh" \
    "$PROJECT_ROOT/tools/stx-java-proxy/bin/stx-java-proxy.sh"
  do
    if [[ -f "$candidate" ]]; then
      script_path="$candidate"
      break
    fi
  done

  if [[ -z "$script_path" ]]; then
    echo "      未找到本机 stx-java-proxy 启动脚本，跳过重启."
    return 1
  fi
  if [[ ! -f "$jar_path" ]]; then
    echo "      未找到本机 stx-java-proxy jar: $jar_path"
    return 1
  fi
  if [[ ! -f "$install_dir/starter/seatunnel-starter.jar" ]]; then
    echo "      未找到本机 SeaTunnel 安装目录: $install_dir"
    return 1
  fi

  mkdir -p "$state_dir"
  touch "$log_path"

  old_pid="$(ss -lntp 2>/dev/null | awk -v port=":$port" '$4 ~ port"$" {print $NF}' | sed -n 's/.*pid=\([0-9]\+\).*/\1/p' | head -n1 || true)"
  if [[ -n "$old_pid" ]]; then
    if ! is_java_proxy_pid "$old_pid"; then
      echo "      端口 $port 当前被非 stx-java-proxy 进程占用 (pid=$old_pid)，为避免误杀已跳过重启."
      return 1
    fi
    echo "[*] 停止本机 stx-java-proxy (pid=$old_pid, port=$port) ..."
    kill -TERM "$old_pid" 2>/dev/null || true
    for _ in {1..20}; do
      if kill -0 "$old_pid" 2>/dev/null || ss -lntp 2>/dev/null | rg -q ":$port"; then
        sleep 1
      else
        break
      fi
    done
    if kill -0 "$old_pid" 2>/dev/null || ss -lntp 2>/dev/null | rg -q ":$port"; then
      kill -KILL "$old_pid" 2>/dev/null || true
      sleep 1
    fi
  fi

  echo "[*] 启动本机 stx-java-proxy ..."
  local pid
  pid="$(
    SEATUNNEL_HOME="$install_dir" \
    STX_JAVA_PROXY_JAR="$jar_path" \
    STX_JAVA_PROXY_VERSION="$CAPABILITY_PROXY_DEFAULT_VERSION" \
    nohup bash "$script_path" -Dstx.java.proxy.port="$port" >>"$log_path" 2>&1 < /dev/null & echo $!
  )"
  echo "$pid" >"$pid_path"
  echo "$port" >"$port_path"

  local health_url="http://127.0.0.1:${port}/healthz"
  for _ in {1..30}; do
    if ! kill -0 "$pid" 2>/dev/null; then
      break
    fi
    if curl -fsS -o /dev/null "$health_url" 2>/dev/null; then
      echo "      本机 stx-java-proxy 已就绪: http://127.0.0.1:${port}"
      return 0
    fi
    sleep 1
  done

  echo "      本机 stx-java-proxy 启动后未通过健康检查，请检查 $log_path"
  return 1
}

pm2_name_count() {
  local name="$1"
  pm2 pid "$name" 2>/dev/null | awk '
    /^[[:space:]]*[0-9]+[[:space:]]*$/ && $1 != "0" { count++ }
    END { print count + 0 }
  '
}

pm2_delete_if_exists() {
  local name="$1"
  local count
  count="$(pm2_name_count "$name" 2>/dev/null || echo 0)"
  count="$(echo "$count" | tr -dc '0-9')"
  count="${count:-0}"
  if [[ "$count" -gt 0 ]]; then
    echo "      检测到 PM2 中已有 $name (${count} 个)，先清理..."
    pm2 delete "$name" >/dev/null 2>&1 || true
  fi
}

port_listener_pids() {
  local port="$1"
  ss -ltnp 2>/dev/null | awk -v port=":$port" '
    $4 ~ port"$" {
      while (match($0, /pid=[0-9]+/)) {
        pid = substr($0, RSTART + 4, RLENGTH - 4)
        print pid
        $0 = substr($0, RSTART + RLENGTH)
      }
    }
  ' | sort -u
}

kill_port_listeners_if_exists() {
  local port="$1"
  local pids=""
  pids="$(port_listener_pids "$port" || true)"
  if [[ -z "$pids" ]]; then
    return 0
  fi

  echo "      检测到端口 $port 已被占用，清理旧进程: $(echo "$pids" | xargs)"
  while read -r pid; do
    [[ -z "$pid" ]] && continue
    kill "$pid" >/dev/null 2>&1 || true
  done <<< "$pids"
  sleep 1
  while read -r pid; do
    [[ -z "$pid" ]] && continue
    if kill -0 "$pid" >/dev/null 2>&1; then
      kill -9 "$pid" >/dev/null 2>&1 || true
    fi
  done <<< "$pids"
}

ensure_config_external_url() {
  local config_path="$1"
  local external_url="$2"
  local temp_path=""

  if [[ ! -f "$config_path" ]]; then
    echo "未找到配置文件 $config_path，跳过 external_url 同步."
    return 0
  fi

  temp_path="$(mktemp)"
  awk -v external_url="$external_url" '
    function indent_len(line, matched) {
      match(line, /^[[:space:]]*/)
      return RLENGTH
    }
    function leading_indent(line) {
      match(line, /^[[:space:]]*/)
      return substr(line, 1, RLENGTH)
    }
    function ltrim(line) {
      sub(/^[[:space:]]+/, "", line)
      return line
    }
    BEGIN {
      app_found = 0
      external_updated = 0
      app_indent = ""
      app_indent_len = -1
    }
    {
      line = $0
      trimmed = ltrim(line)

      if (!app_found && line ~ /^[[:space:]]*app:[[:space:]]*$/) {
        app_found = 1
        app_indent = leading_indent(line)
        app_indent_len = indent_len(line)
        print line
        next
      }

      if (app_found && !external_updated) {
        current_indent_len = indent_len(line)

        if (trimmed ~ /^external_url:[[:space:]]*/) {
          print app_indent "  external_url: \"" external_url "\""
          external_updated = 1
          next
        }

        if (trimmed != "" && trimmed !~ /^#/ && current_indent_len <= app_indent_len) {
          print app_indent "  external_url: \"" external_url "\""
          external_updated = 1
        }
      }

      print line
    }
    END {
      if (app_found && !external_updated) {
        print app_indent "  external_url: \"" external_url "\""
      }
    }
  ' "$config_path" > "$temp_path"
  mv "$temp_path" "$config_path"

  echo "      已同步 app.external_url = $external_url"
}

prepare_frontend_standalone() {
  local should_build="${1:-false}"
  local next_standalone_dir="$FRONTEND_DIR/.next/standalone"
  local next_standalone_entry=""
  local entry_relative_path=""
  local runtime_relative_dir=""

  if [[ ! -f "$FRONTEND_DIR/package.json" ]]; then
    echo "未找到 frontend/package.json，跳过前端"
    return 1
  fi

  cd "$FRONTEND_DIR"

  if [[ "$should_build" == "true" ]]; then
    echo "      构建前端（next build）..."
    pnpm run build
  fi

  if [[ -f "$next_standalone_dir/server.js" ]]; then
    next_standalone_entry="$next_standalone_dir/server.js"
  else
    next_standalone_entry="$(find "$next_standalone_dir" -maxdepth 3 -type f -name 'server.js' | sort | head -n 1)"
  fi

  if [[ -z "$next_standalone_entry" || ! -f "$next_standalone_entry" ]]; then
    echo "未找到 .next/standalone 下的 server.js，请确认 next.config.ts 已配置 output: 'standalone'"
    return 1
  fi

  entry_relative_path="${next_standalone_entry#"$next_standalone_dir"/}"
  runtime_relative_dir="$(dirname "$entry_relative_path")"

  echo "      组装 standalone 运行目录 (entry: $entry_relative_path)..."
  rm -rf "$FRONTEND_STANDALONE_DIR"
  mkdir -p "$FRONTEND_STANDALONE_DIR"
  cp -a "$FRONTEND_DIR/.next/standalone/." "$FRONTEND_STANDALONE_DIR/"
  FRONTEND_ENTRY="$FRONTEND_STANDALONE_DIR/$entry_relative_path"
  if [[ "$runtime_relative_dir" == "." ]]; then
    FRONTEND_RUNTIME_DIR="$FRONTEND_STANDALONE_DIR"
  else
    FRONTEND_RUNTIME_DIR="$FRONTEND_STANDALONE_DIR/$runtime_relative_dir"
  fi
  if [[ -d "$FRONTEND_DIR/.next/static" ]]; then
    mkdir -p "$FRONTEND_RUNTIME_DIR/.next"
    cp -a "$FRONTEND_DIR/.next/static" "$FRONTEND_RUNTIME_DIR/.next/static"
  fi
  if [[ -d "$FRONTEND_DIR/public" ]]; then
    cp -a "$FRONTEND_DIR/public" "$FRONTEND_RUNTIME_DIR/public"
  fi
  cd "$PROJECT_ROOT"

  if [[ ! -f "$FRONTEND_ENTRY" ]]; then
    echo "standalone 产物不完整: $FRONTEND_ENTRY 不存在"
    return 1
  fi
  return 0
}

start_frontend_dev() {
  if [[ ! -f "$FRONTEND_DIR/package.json" ]]; then
    echo "未找到 frontend/package.json，跳过前端"
    return 1
  fi

  pm2_delete_if_exists "$PM2_UI"
  kill_port_listeners_if_exists "$FRONTEND_PORT"
  HOSTNAME="0.0.0.0" PORT="$FRONTEND_PORT" NEXT_PUBLIC_BACKEND_BASE_URL="$NEXT_PUBLIC_BACKEND_BASE_URL" \
    pm2 start pnpm --name "$PM2_UI" --cwd "$FRONTEND_DIR" --update-env -- exec next dev --turbopack --hostname 0.0.0.0 --port "$FRONTEND_PORT"
  echo "      前端开发模式已启动 (http://127.0.0.1:$FRONTEND_PORT, command: pnpm run dev)."
  return 0
}

require_cmd pm2
if $RESTART_JAVA_PROXY; then
  require_cmd curl
fi
if $RUN_BACKEND && $DO_BUILD; then
  require_cmd go
fi
if $RUN_FRONTEND && ($DO_BUILD || $DO_RESTART); then
  require_cmd pnpm
fi

if [[ ! -f go.mod ]]; then
  echo "未在项目根找到 go.mod，请于项目根目录执行: ./scripts/restart.sh"
  exit 1
fi

ensure_config_external_url "$CONFIG_PATH" "$APP_EXTERNAL_URL"

if $STOP_FRONTEND; then
  echo "停止前端 (PM2: $PM2_UI)..."
  pm2_delete_if_exists "$PM2_UI"
  pm2 save >/dev/null 2>&1 || true
  pm2 status
  echo "完成."
  exit 0
fi

if ! $DO_BUILD && ! $DO_RESTART; then
  echo "无操作可执行：构建与重启均已禁用"
  exit 0
fi

if $FRONTEND_DEV && ! $RUN_FRONTEND; then
  echo "警告: --frontend-dev 在当前参数下不会生效（前端已被禁用）"
fi
if $FRONTEND_DEV && ! $DO_RESTART; then
  echo "警告: --frontend-dev 在 --build-only 下不会生效"
fi

total=0
if $DO_BUILD && $RUN_BACKEND; then total=$((total + 3)); fi
if $DO_BUILD && $RUN_BACKEND && $RESTART_LOCAL_AGENT; then total=$((total + 1)); fi
if $DO_BUILD && $RUN_FRONTEND && ! ($FRONTEND_DEV && $DO_RESTART); then total=$((total + 1)); fi
if $DO_RESTART && $RUN_BACKEND; then total=$((total + 1)); fi
if $DO_RESTART && $RUN_FRONTEND; then total=$((total + 1)); fi

if [[ "$total" -eq 0 ]]; then
  echo "无操作可执行：请检查参数组合"
  exit 0
fi

step=0
FRONTEND_PREPARED=false

if $DO_BUILD && $RUN_BACKEND; then
  step=$((step + 1)); echo "[$step/$total] 构建 stx ..."
  go build -o stx .
  echo "      stx 构建完成."

  agent_goos="$(detect_agent_goos)"
  agent_goarch="$(detect_agent_goarch)"
  agent_binary_name="$(agent_binary_name_for_target "$agent_goos" "$agent_goarch")"

  step=$((step + 1)); echo "[$step/$total] 构建 stx-agent ..."
  (cd agent && GOOS="$agent_goos" GOARCH="$agent_goarch" go build -o stx-agent ./cmd)
  echo "      stx-agent 构建完成: ${agent_goos}/${agent_goarch}"

  if [[ -f agent/stx-agent ]]; then
    mkdir -p lib/agent
    cp -f agent/stx-agent "lib/agent/${agent_binary_name}"
    chmod +x "lib/agent/${agent_binary_name}"
    echo "      已同步 agent 到 lib/agent/${agent_binary_name}."
  fi

  step=$((step + 1)); echo "[$step/$total] 构建 stx-java-proxy 薄 jar ..."
  if command -v mvn >/dev/null 2>&1; then
    mvn -q -f tools/stx-java-proxy/pom.xml -DskipTests package
    proxy_jar="$(find tools/stx-java-proxy/target -maxdepth 1 -type f -name 'stx-java-proxy-*.jar' ! -name '*-bin.jar' | sort | head -n1)"
    if [[ -n "${proxy_jar:-}" && -f "${proxy_jar:-}" ]]; then
      mkdir -p lib
      cp -f "$proxy_jar" "lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar"
      echo "      已同步 stx-java-proxy jar 到 lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar."
      if [[ -d "$AGENT_PROXY_LIB_DIR" ]]; then
        cp -f "$proxy_jar" "$AGENT_PROXY_LIB_DIR/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar"
        echo "      已同步 stx-java-proxy jar 到 $AGENT_PROXY_LIB_DIR/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar."
      fi
      if [[ -d "$AGENT_HOME/scripts" && -f "$PROJECT_ROOT/scripts/stx-java-proxy.sh" ]]; then
        cp -f "$PROJECT_ROOT/scripts/stx-java-proxy.sh" "$AGENT_HOME/scripts/stx-java-proxy.sh"
        chmod +x "$AGENT_HOME/scripts/stx-java-proxy.sh"
        echo "      已同步 stx-java-proxy 启动脚本到 $AGENT_HOME/scripts/stx-java-proxy.sh."
      fi
    else
      echo "      未找到 stx-java-proxy 薄 jar，跳过同步."
    fi
  else
    echo "      未找到 mvn，跳过 stx-java-proxy 薄 jar 构建与同步."
  fi

  if $RESTART_LOCAL_AGENT; then
    step=$((step + 1)); echo "[$step/$total] 同步并重启本机 stx-agent ..."
    sync_and_restart_local_agent "agent/stx-agent" "$agent_goos" "$agent_goarch"
  fi
fi

if $DO_BUILD && $RUN_FRONTEND && ! ($FRONTEND_DEV && $DO_RESTART); then
  step=$((step + 1)); echo "[$step/$total] 构建前端 standalone 产物 ..."
  if prepare_frontend_standalone true; then
    FRONTEND_PREPARED=true
    echo "      前端 standalone 构建完成."
  else
    echo "前端构建失败，已退出."
    exit 1
  fi
fi

if $DO_RESTART && $RUN_BACKEND; then
  step=$((step + 1)); echo "[$step/$total] 启动后端 (PM2: $PM2_API) ..."
  if [[ ! -f "$PROJECT_ROOT/stx" ]]; then
    echo "未找到 $PROJECT_ROOT/stx，请先执行一次包含后端构建的命令"
    exit 1
  fi
  pm2_delete_if_exists "$PM2_API"
  # 兜底：清理非 PM2 拉起的旧后端进程
  pkill -f "$PROJECT_ROOT/stx api" >/dev/null 2>&1 || true
  CONFIG_PATH="$CONFIG_PATH" pm2 start "$PROJECT_ROOT/stx" --name "$PM2_API" --cwd "$PROJECT_ROOT" --interpreter none -- api
  echo "      后端已启动 (API: http://127.0.0.1:8000)."
fi

if $DO_RESTART && $RUN_FRONTEND; then
  step=$((step + 1))
  if $FRONTEND_DEV; then
    echo "[$step/$total] 启动前端开发模式 (PM2: $PM2_UI) ..."
    if ! start_frontend_dev; then
      echo "前端开发模式启动失败，已退出."
      exit 1
    fi
  else
    echo "[$step/$total] 启动前端 standalone (PM2: $PM2_UI) ..."
    if ! $FRONTEND_PREPARED; then
      if ! prepare_frontend_standalone false; then
        echo "前端 standalone 准备失败，已退出."
        exit 1
      fi
    fi
    pm2_delete_if_exists "$PM2_UI"
    kill_port_listeners_if_exists "$FRONTEND_PORT"
    HOSTNAME="0.0.0.0" PORT="$FRONTEND_PORT" NEXT_PUBLIC_BACKEND_BASE_URL="$NEXT_PUBLIC_BACKEND_BASE_URL" \
      pm2 start "$FRONTEND_ENTRY" --name "$PM2_UI" --cwd "$FRONTEND_RUNTIME_DIR" --update-env
    echo "      前端已启动 (http://127.0.0.1:$FRONTEND_PORT)."
  fi
fi

if $DO_RESTART; then
  echo "[*] 保存 PM2 进程列表 (pm2 save) ..."
  pm2 save
  pm2 status
else
  echo "[*] 构建完成（未执行重启）."
fi

if $RESTART_JAVA_PROXY; then
  restart_local_java_proxy
fi

echo "完成."
