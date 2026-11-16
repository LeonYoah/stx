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

while [[ $# -gt 0 ]]; do
  case "$1" in
    # 仅为兼容旧 systemd 单元而保留并忽略。/ Accepted and ignored only for old systemd units.
    --observability)
      shift 2 || shift $#
      ;;
    --observability=*)
      shift
      ;;
    -h|--help)
      cat <<'USAGE'
Usage: stop.sh

Stops STX frontend and backend. Leftover processes of the legacy external monitoring stack
(from older releases) are stopped too when their pid files are still present.
USAGE
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

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

# 升级自旧版本时，外部监控组件可能仍在运行：有 pid 文件就停掉（数据目录保留不删）。
# After upgrading from an older release the external monitoring processes may still run: stop
# them when their pid files exist (data directories are kept).
for legacy in grafana/grafana prometheus/prometheus alertmanager/alertmanager; do
  if [[ -f "$BASE_DIR/observability/$legacy.pid" ]]; then
    stop_one "legacy ${legacy%%/*}" "$BASE_DIR/observability/$legacy.pid"
  fi
done
