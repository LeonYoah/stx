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
FRONTEND_PORT="${FRONTEND_PORT:-17880}"

status_one() {
  local name="$1"
  local pidfile="$2"
  if [[ ! -f "$pidfile" ]]; then
    echo "$name: stopped"
    return
  fi
  local pid
  pid="$(cat "$pidfile" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    echo "$name: running (pid=$pid)"
  else
    echo "$name: pidfile exists but process not running"
  fi
}

status_one "backend" "$RUN_DIR/backend.pid"
status_one "frontend" "$RUN_DIR/frontend.pid"

echo
echo "ports:"
ss -lntp | grep -E ":17800|:17890|:${FRONTEND_PORT}\\b|:9090|:9093|:3000" || true

if [[ -x "$BASE_DIR/deps/status-observability.sh" ]]; then
  echo
  echo "observability:"
  (cd "$BASE_DIR" && "$BASE_DIR/deps/status-observability.sh") || true
fi
