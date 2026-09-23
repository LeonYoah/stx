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
CONFIG_PATH="${CONFIG_PATH:-$BASE_DIR/config.yaml}"
FRONTEND_PORT="${FRONTEND_PORT:-17880}"

# shellcheck source=lib/observability.sh
source "$BASE_DIR/bin/lib/observability.sh"

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

echo "backend / frontend:"
status_one "backend" "$RUN_DIR/backend.pid"
status_one "frontend" "$RUN_DIR/frontend.pid"

echo
echo "observability:"
stx_obs_status_stack

echo
echo "ports:"
ss -lntp 2>/dev/null | grep -E ":17800|:17890|:${FRONTEND_PORT}\\b|:9090|:9093|:3000" || true
