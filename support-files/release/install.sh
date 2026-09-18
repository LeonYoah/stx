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

# 打印 STX 安装器的用法。/ Print STX installer usage.
usage() {
  cat <<'USAGE'
STX one-click installer

Usage:
  ./install.sh [options]

Options:
  --install-dir <path>    Install directory (default: /opt/stx)
  --force                 Backup existing install dir before reinstall
  --no-preserve-config    Do not keep existing config.yaml
  --no-start              Install only, do not auto start
  -h, --help              Show this help

Environment overrides when starting later:
  CONFIG_PATH                    Backend config path (default: <install-dir>/config.yaml)
  FRONTEND_PORT                  Frontend listen port (default: 80)
  FRONTEND_HOST                  Frontend listen host (default: 0.0.0.0)
  NEXT_PUBLIC_BACKEND_BASE_URL   Frontend API base URL (default: http://127.0.0.1:8000)
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-/opt/stx}"
CAPABILITY_PROXY_DEFAULT_VERSION="${CAPABILITY_PROXY_DEFAULT_VERSION:-2.3.13}"
FORCE=false
PRESERVE_CONFIG=true
AUTO_START=true

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    --force)
      FORCE=true
      shift
      ;;
    --no-preserve-config)
      PRESERVE_CONFIG=false
      shift
      ;;
    --no-start)
      AUTO_START=false
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[ERROR] unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$INSTALL_DIR" ]]; then
  echo "[ERROR] --install-dir cannot be empty"
  exit 1
fi

if [[ -x "$SCRIPT_DIR/stx" ]]; then
  SOURCE_DIR="$SCRIPT_DIR"
elif [[ -x "$SCRIPT_DIR/../stx" ]]; then
  SOURCE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
else
  echo "[ERROR] install.sh must run from package root (or bin/)."
  exit 1
fi

for f in stx bin/start.sh bin/stop.sh bin/status.sh config.example.yaml "lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar" scripts/stx-java-proxy.sh; do
  if [[ ! -e "$SOURCE_DIR/$f" ]]; then
    echo "[ERROR] package payload missing: $f"
    exit 1
  fi
done

if [[ ! -d "$INSTALL_DIR" ]]; then
  mkdir -p "$INSTALL_DIR" 2>/dev/null || {
    echo "[ERROR] cannot create $INSTALL_DIR, please retry with sudo"
    exit 1
  }
elif [[ ! -w "$INSTALL_DIR" ]]; then
  echo "[ERROR] no write permission on $INSTALL_DIR, please retry with sudo"
  exit 1
fi

BACKUP_CONFIG=""
if [[ "$PRESERVE_CONFIG" == "true" && -f "$INSTALL_DIR/config.yaml" ]]; then
  BACKUP_CONFIG="$(mktemp)"
  cp "$INSTALL_DIR/config.yaml" "$BACKUP_CONFIG"
fi

if [[ "$FORCE" == "true" && -d "$INSTALL_DIR" && "$(ls -A "$INSTALL_DIR" 2>/dev/null || true)" != "" ]]; then
  BACKUP_DIR="${INSTALL_DIR}.bak.$(date +%Y%m%d%H%M%S)"
  echo "[INFO] backup existing directory to: $BACKUP_DIR"
  mv "$INSTALL_DIR" "$BACKUP_DIR"
  mkdir -p "$INSTALL_DIR"
fi

echo "[INFO] installing STX to: $INSTALL_DIR"
tar -C "$SOURCE_DIR" \
  --exclude='run/*' \
  --exclude='logs/*' \
  --exclude='.DS_Store' \
  -cf - . | tar -C "$INSTALL_DIR" -xf -

if [[ -n "$BACKUP_CONFIG" && -f "$BACKUP_CONFIG" ]]; then
  cp "$BACKUP_CONFIG" "$INSTALL_DIR/config.yaml"
  rm -f "$BACKUP_CONFIG"
  echo "[INFO] existing config.yaml restored"
elif [[ ! -f "$INSTALL_DIR/config.yaml" ]]; then
  cp "$INSTALL_DIR/config.example.yaml" "$INSTALL_DIR/config.yaml"
  echo "[INFO] generated config.yaml from config.example.yaml"
fi

chmod +x \
  "$INSTALL_DIR/stx" \
  "$INSTALL_DIR/install.sh" \
  "$INSTALL_DIR/bin/start.sh" \
  "$INSTALL_DIR/bin/stop.sh" \
  "$INSTALL_DIR/bin/status.sh" \
  "$INSTALL_DIR/scripts/stx-java-proxy.sh"

for f in \
  "$INSTALL_DIR/deps/start-observability.sh" \
  "$INSTALL_DIR/deps/stop-observability.sh" \
  "$INSTALL_DIR/deps/status-observability.sh" \
  "$INSTALL_DIR/deps/init-observability-defaults.sh"; do
  if [[ -f "$f" ]]; then
    chmod +x "$f"
  fi
done

mkdir -p "$INSTALL_DIR/run" "$INSTALL_DIR/logs"

echo "[OK] install done."
echo "     start : $INSTALL_DIR/bin/start.sh"
echo "     stop  : $INSTALL_DIR/bin/stop.sh"
echo "     status: $INSTALL_DIR/bin/status.sh"
echo "     config: $INSTALL_DIR/config.yaml"
echo
echo "[INFO] port hints:"
echo "       backend http/grpc ports come from $INSTALL_DIR/config.yaml"
echo "       frontend port/host can be overridden with FRONTEND_PORT / FRONTEND_HOST"
echo "       example: CONFIG_PATH=$INSTALL_DIR/config.yaml FRONTEND_PORT=8080 NEXT_PUBLIC_BACKEND_BASE_URL=http://127.0.0.1:8000 $INSTALL_DIR/bin/start.sh"

if [[ "$AUTO_START" == "true" ]]; then
  echo "[INFO] auto starting ..."
  (cd "$INSTALL_DIR" && ./bin/start.sh)
else
  echo "[INFO] skip auto start (--no-start)"
fi
