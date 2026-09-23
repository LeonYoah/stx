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
Usage: stop.sh [--observability auto|on|off]

Stops STX frontend/backend and optionally the bundled observability stack.
Default: --observability auto (stop local stack processes if present).
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

OBS_MODE="$(stx_obs_resolve_mode "$OBS_CLI" "${STOP_OBSERVABILITY:-auto}")"

stop_one() {
  local name="$1"
  local pidfile="$2"
  if [[ ! -f "$pidfile" ]]; then
    echo "$name not running (no pidfile)"
    return
  fi
  local pid
  pid="$(cat "$pidfile" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" || true
    sleep 1
    if kill -0 "$pid" 2>/dev/null; then
      kill -9 "$pid" || true
    fi
    echo "stopped $name (pid=$pid)"
  else
    echo "$name already stopped"
  fi
  rm -f "$pidfile"
}

stop_one "frontend" "$RUN_DIR/frontend.pid"
stop_one "backend" "$RUN_DIR/backend.pid"

case "$OBS_MODE" in
  off)
    echo "observability: leave running (--observability off)"
    ;;
  on|auto)
    stx_obs_stop_stack || true
    ;;
esac
