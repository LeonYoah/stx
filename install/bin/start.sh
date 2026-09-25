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

set -euo pipefail

BASE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RUN_DIR="$BASE_DIR/run"
LOG_DIR="$BASE_DIR/logs"

BACKEND_BIN="$BASE_DIR/stx"
# 优先使用内置 Node，缺失时回退至系统 Node。
# Prefer bundled Node; fallback to system Node when omitted.
FRONTEND_NODE_BIN="${FRONTEND_NODE_BIN:-}"
if [[ -z "$FRONTEND_NODE_BIN" ]]; then
  if [[ -x "$BASE_DIR/runtime/node/bin/node" ]]; then
    FRONTEND_NODE_BIN="$BASE_DIR/runtime/node/bin/node"
  elif command -v node >/dev/null 2>&1; then
    FRONTEND_NODE_BIN="$(command -v node)"
  else
    FRONTEND_NODE_BIN="$BASE_DIR/runtime/node/bin/node"
  fi
fi
FRONTEND_SERVER="$BASE_DIR/frontend/server.js"
CONFIG_PATH="${CONFIG_PATH:-$BASE_DIR/config.yaml}"

FRONTEND_ENABLE="${FRONTEND_ENABLE:-true}"
FRONTEND_PORT="${FRONTEND_PORT:-17880}"
FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
NEXT_PUBLIC_BACKEND_BASE_URL="${NEXT_PUBLIC_BACKEND_BASE_URL:-http://127.0.0.1:17800}"

OBS_CLI=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --observability)
      OBS_CLI="${2:-}"
      shift 2
      ;;
    --observability=*)
      OBS_CLI="${1#*=}"
      shift
      ;;
    -h|--help)
      cat <<'USAGE'
Usage: start.sh [--observability auto|on|off]

Starts STX backend, frontend, and optionally the bundled observability stack.
Default: --observability auto (start local stack only if present and enabled in config).

Environment:
  START_OBSERVABILITY   Same as --observability (CLI wins)
  FRONTEND_PORT / FRONTEND_HOST / FRONTEND_ENABLE
  NEXT_PUBLIC_BACKEND_BASE_URL
  CONFIG_PATH
USAGE
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

# shellcheck source=lib/observability.sh
source "$BASE_DIR/bin/lib/observability.sh"

OBS_MODE="$(stx_obs_resolve_mode "$OBS_CLI" "${START_OBSERVABILITY:-}")"

mkdir -p "$RUN_DIR" "$LOG_DIR"

start_backend() {
  local pidfile="$RUN_DIR/backend.pid"
  if [[ -f "$pidfile" ]]; then
    local pid
    pid="$(cat "$pidfile" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      echo "backend already running (pid=$pid)"
      return
    fi
    rm -f "$pidfile"
  fi

  if [[ ! -x "$BACKEND_BIN" ]]; then
    echo "backend binary not found: $BACKEND_BIN"
    exit 1
  fi

  setsid sh -c "exec env CONFIG_PATH=\"$CONFIG_PATH\" \"$BACKEND_BIN\" server >>\"$LOG_DIR/backend.log\" 2>&1" < /dev/null &
  echo $! >"$pidfile"
  sleep 1
  if kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "backend started (pid=$(cat "$pidfile"))"
  else
    rm -f "$pidfile"
    echo "backend failed to start, check log: $LOG_DIR/backend.log"
    exit 1
  fi
}

start_frontend() {
  if [[ "$FRONTEND_ENABLE" != "true" && "$FRONTEND_ENABLE" != "1" ]]; then
    echo "frontend disabled by FRONTEND_ENABLE=$FRONTEND_ENABLE"
    return
  fi

  local pidfile="$RUN_DIR/frontend.pid"
  if [[ -f "$pidfile" ]]; then
    local pid
    pid="$(cat "$pidfile" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      echo "frontend already running (pid=$pid)"
      return
    fi
    rm -f "$pidfile"
  fi

  if [[ ! -x "$FRONTEND_NODE_BIN" ]]; then
    echo "node runtime not found: $FRONTEND_NODE_BIN"
    exit 1
  fi
  if [[ ! -f "$FRONTEND_SERVER" ]]; then
    echo "frontend standalone server not found: $FRONTEND_SERVER"
    exit 1
  fi

  setsid sh -c "exec env HOSTNAME=\"$FRONTEND_HOST\" PORT=\"$FRONTEND_PORT\" NEXT_PUBLIC_BACKEND_BASE_URL=\"$NEXT_PUBLIC_BACKEND_BASE_URL\" \"$FRONTEND_NODE_BIN\" \"$FRONTEND_SERVER\" >>\"$LOG_DIR/frontend.log\" 2>&1" < /dev/null &
  echo $! >"$pidfile"
  sleep 1
  if kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "frontend started (pid=$(cat "$pidfile"), host=$FRONTEND_HOST, port=$FRONTEND_PORT)"
  else
    rm -f "$pidfile"
    echo "frontend failed to start, check log: $LOG_DIR/frontend.log"
    exit 1
  fi
}

start_observability() {
  set +e
  local reason
  reason="$(stx_obs_should_start "$OBS_MODE" "$CONFIG_PATH")"
  local rc=$?
  set -e
  echo "$reason"
  case "$rc" in
    0) stx_obs_start_stack || echo "[WARN] observability failed to start; backend and frontend remain available" >&2 ;;
    1) return 0 ;;
    2) exit 1 ;;
  esac
}

start_backend
start_frontend
start_observability

echo
echo "done."
echo "  config  : $CONFIG_PATH"
echo "  backend : follow app.addr in config.yaml"
if [[ "$FRONTEND_ENABLE" == "true" || "$FRONTEND_ENABLE" == "1" ]]; then
  echo "  frontend: http://$FRONTEND_HOST:$FRONTEND_PORT"
fi
echo "  observability mode: $OBS_MODE"
echo "  tips    : start.sh --observability off|on|auto"
