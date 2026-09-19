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

# macOS 本机开发入口：复用 restart.sh，并固定本机 launchd / Agent 路径。
# macOS local entry: wraps restart.sh with this machine's launchd / Agent paths.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "restart-mac.sh 仅用于 macOS。其他系统请直接运行: ./scripts/restart.sh"
  exit 1
fi

# 本机已安装的 Agent / launchd 约定
# Local Agent / launchd conventions on this Mac
export AGENT_HOME="${AGENT_HOME:-$HOME/.stx/agent}"
export LOCAL_AGENT_INSTALL_DIR="${LOCAL_AGENT_INSTALL_DIR:-$AGENT_HOME/bin}"
export LOCAL_AGENT_BINARY="${LOCAL_AGENT_BINARY:-stx-agent}"
export LOCAL_AGENT_LAUNCHD_LABEL="${LOCAL_AGENT_LAUNCHD_LABEL:-org.apache.stx.stx-agent}"
export LOCAL_AGENT_LAUNCHD_PLIST="${LOCAL_AGENT_LAUNCHD_PLIST:-$HOME/Library/LaunchAgents/${LOCAL_AGENT_LAUNCHD_LABEL}.plist}"
export LOCAL_AGENT_START_WRAPPER="${LOCAL_AGENT_START_WRAPPER:-$LOCAL_AGENT_INSTALL_DIR/stx-agent-start.sh}"
export LOCAL_AGENT_RESTART="${LOCAL_AGENT_RESTART:-true}"

# 控制面本机端口约定
# Local control-plane ports
export APP_EXTERNAL_URL="${APP_EXTERNAL_URL:-http://127.0.0.1:17800}"
export NEXT_PUBLIC_BACKEND_BASE_URL="${NEXT_PUBLIC_BACKEND_BASE_URL:-http://127.0.0.1:17800}"
export FRONTEND_PORT="${FRONTEND_PORT:-17880}"
export CONTROL_PLANE_BASE_URL="${CONTROL_PLANE_BASE_URL:-http://127.0.0.1:17800}"

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  exec "$SCRIPT_DIR/restart.sh" --help
fi

if ! command -v pm2 >/dev/null 2>&1; then
  echo "本机尚未安装 pm2，前后端无法用 PM2 托管。"
  echo "请先执行: npm i -g pm2"
  echo "然后再运行: ./scripts/restart-mac.sh"
  exit 1
fi

echo "[mac] Agent home: ${AGENT_HOME}"
echo "[mac] launchd: ${LOCAL_AGENT_LAUNCHD_LABEL}"
if [[ -f "$LOCAL_AGENT_LAUNCHD_PLIST" ]]; then
  echo "[mac] plist: ${LOCAL_AGENT_LAUNCHD_PLIST}"
else
  echo "[mac] 警告: 未找到 ${LOCAL_AGENT_LAUNCHD_PLIST}（Agent 同步后可能无法自动重启）"
fi

cd "$PROJECT_ROOT"
exec "$SCRIPT_DIR/restart.sh" "$@"
