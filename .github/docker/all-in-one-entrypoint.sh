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

BASE_DIR="/opt/stx"
LOG_DIR="$BASE_DIR/logs"
RUN_DIR="$BASE_DIR/run"

BACKEND_BIN="$BASE_DIR/stx"
FRONTEND_SERVER="$BASE_DIR/frontend/server.js"
CONFIG_PATH="${CONFIG_PATH:-$BASE_DIR/config.yaml}"

FRONTEND_ENABLE="${FRONTEND_ENABLE:-true}"
FRONTEND_PORT="${FRONTEND_PORT:-80}"
NEXT_PUBLIC_BACKEND_BASE_URL="${NEXT_PUBLIC_BACKEND_BASE_URL:-http://127.0.0.1:8000}"

mkdir -p "$LOG_DIR" "$RUN_DIR"

terminate() {
  echo "[entrypoint] stopping..."
  if [[ -n "${frontend_pid:-}" ]]; then
    kill "$frontend_pid" 2>/dev/null || true
  fi
  if [[ -n "${backend_pid:-}" ]]; then
    kill "$backend_pid" 2>/dev/null || true
  fi
  wait || true
}

trap terminate SIGINT SIGTERM

if [[ ! -x "$BACKEND_BIN" ]]; then
  echo "[entrypoint] backend binary not found: $BACKEND_BIN"
  exit 1
fi

CONFIG_PATH="$CONFIG_PATH" "$BACKEND_BIN" api >>"$LOG_DIR/backend.log" 2>&1 &
backend_pid=$!
echo "$backend_pid" >"$RUN_DIR/backend.pid"
echo "[entrypoint] backend started (pid=$backend_pid)"

if [[ "$FRONTEND_ENABLE" == "true" || "$FRONTEND_ENABLE" == "1" ]]; then
  if [[ ! -f "$FRONTEND_SERVER" ]]; then
    echo "[entrypoint] frontend server not found: $FRONTEND_SERVER"
    terminate
    exit 1
  fi

  PORT="$FRONTEND_PORT" \
  NEXT_PUBLIC_BACKEND_BASE_URL="$NEXT_PUBLIC_BACKEND_BASE_URL" \
  node "$FRONTEND_SERVER" >>"$LOG_DIR/frontend.log" 2>&1 &
  frontend_pid=$!
  echo "$frontend_pid" >"$RUN_DIR/frontend.pid"
  echo "[entrypoint] frontend started (pid=$frontend_pid, port=$FRONTEND_PORT)"
else
  echo "[entrypoint] frontend disabled by FRONTEND_ENABLE=$FRONTEND_ENABLE"
fi

set +e
if [[ -n "${frontend_pid:-}" ]]; then
  wait -n "$backend_pid" "$frontend_pid"
  exit_code=$?
else
  wait "$backend_pid"
  exit_code=$?
fi
set -e

echo "[entrypoint] one process exited (code=$exit_code), shutting down"
terminate
exit "$exit_code"
