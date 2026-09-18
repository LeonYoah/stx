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
FRONTEND_NODE_BIN="${FRONTEND_NODE_BIN:-$BASE_DIR/runtime/node/bin/node}"
FRONTEND_SERVER="$BASE_DIR/frontend/server.js"
CONFIG_PATH="${CONFIG_PATH:-$BASE_DIR/config.yaml}"

FRONTEND_ENABLE="${FRONTEND_ENABLE:-true}"
FRONTEND_PORT="${FRONTEND_PORT:-17880}"
FRONTEND_HOST="${FRONTEND_HOST:-0.0.0.0}"
NEXT_PUBLIC_BACKEND_BASE_URL="${NEXT_PUBLIC_BACKEND_BASE_URL:-http://127.0.0.1:17800}"
START_OBSERVABILITY="${START_OBSERVABILITY:-auto}"

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

  CONFIG_PATH="$CONFIG_PATH" nohup "$BACKEND_BIN" api >>"$LOG_DIR/backend.log" 2>&1 &
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

  HOSTNAME="$FRONTEND_HOST" \
  PORT="$FRONTEND_PORT" \
  NEXT_PUBLIC_BACKEND_BASE_URL="$NEXT_PUBLIC_BACKEND_BASE_URL" \
  nohup "$FRONTEND_NODE_BIN" "$FRONTEND_SERVER" >>"$LOG_DIR/frontend.log" 2>&1 &
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
  if [[ "$START_OBSERVABILITY" == "false" || "$START_OBSERVABILITY" == "0" ]]; then
    echo "observability start skipped by START_OBSERVABILITY=$START_OBSERVABILITY"
    return
  fi

  if [[ -x "$BASE_DIR/deps/start-observability.sh" ]]; then
    echo "starting bundled observability stack..."
    (cd "$BASE_DIR" && "$BASE_DIR/deps/start-observability.sh")
  else
    echo "bundled observability stack not found, skipping"
  fi
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
echo "  tips    : set FRONTEND_PORT / FRONTEND_HOST to override frontend binding"
