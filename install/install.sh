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
  --install-dir <path>     Install directory (default: /opt/stx)
  --offline                Offline mode: use ./packages only, forbid network
  --with-observability     Enable bundled observability when observability/ present
  --without-observability  Force observability.enabled=false
  --force                  Backup existing install dir before reinstall
  --no-preserve-config     Do not keep existing config.yaml
  --no-start               Install only, do not auto start
  --no-systemd             Skip systemd unit installation
  -h, --help               Show this help

Environment:
  STX_SKIP_SYSTEMD=true            Same as --no-systemd
  FRONTEND_PORT / FRONTEND_HOST    Frontend bind overrides for start.sh
  NEXT_PUBLIC_BACKEND_BASE_URL     Frontend -> API base URL
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=download-lib.sh
source "$SCRIPT_DIR/download-lib.sh"
# shellcheck source=install-core.sh
source "$SCRIPT_DIR/install-core.sh"

INSTALL_DIR="${STX_INSTALL_DIR:-${INSTALL_DIR:-/opt/stx}}"
CAPABILITY_PROXY_DEFAULT_VERSION="${CAPABILITY_PROXY_DEFAULT_VERSION:-v2}"
FORCE=false
PRESERVE_CONFIG=true
AUTO_START=true
OFFLINE=false
WITH_OBS="auto"
SKIP_SYSTEMD=false

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
    --offline)
      OFFLINE=true
      STX_OFFLINE=true
      shift
      ;;
    --with-observability)
      WITH_OBS=true
      shift
      ;;
    --without-observability)
      WITH_OBS=false
      shift
      ;;
    --no-systemd)
      SKIP_SYSTEMD=true
      STX_SKIP_SYSTEMD=true
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

# Resolve package root: classic layout, offline bundle, or bin/.
# 解析安装包根：经典布局、离线 bundle、或 bin/。
SOURCE_DIR=""
PACKAGES_DIR=""
if [[ -x "$SCRIPT_DIR/stx" ]]; then
  SOURCE_DIR="$SCRIPT_DIR"
elif [[ -x "$SCRIPT_DIR/../stx" ]]; then
  SOURCE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
elif [[ -d "$SCRIPT_DIR/packages" ]]; then
  SOURCE_DIR="$SCRIPT_DIR"
  PACKAGES_DIR="$SCRIPT_DIR/packages"
elif [[ -d "$SCRIPT_DIR/../packages" ]]; then
  SOURCE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
  PACKAGES_DIR="$SOURCE_DIR/packages"
else
  echo "[ERROR] install.sh must run from package root, offline bundle, or bin/."
  exit 1
fi

if [[ "$OFFLINE" == "true" ]]; then
  if [[ -z "$PACKAGES_DIR" && -d "$SOURCE_DIR/packages" ]]; then
    PACKAGES_DIR="$SOURCE_DIR/packages"
  fi
  if [[ -z "$PACKAGES_DIR" ]]; then
    echo "[ERROR] --offline requires packages/ directory"
    exit 1
  fi
fi

# Offline: assemble a staging tree from packages/*. / 离线：从 packages 组装暂存树。
STAGE_DIR=""
cleanup_stage() {
  if [[ -n "$STAGE_DIR" && -d "$STAGE_DIR" ]]; then
    rm -rf "$STAGE_DIR"
  fi
}
trap cleanup_stage EXIT

if [[ -n "$PACKAGES_DIR" ]]; then
  echo "[INFO] assembling install tree from $PACKAGES_DIR"
  STAGE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/stx-install.XXXXXX")"
  arch="$(stx_detect_arch)"

  stx_bin="$PACKAGES_DIR/stx-linux-${arch}"
  [[ -f "$stx_bin" ]] || { echo "[ERROR] missing $stx_bin"; exit 1; }
  if [[ -f "${stx_bin}.sha256" ]]; then
    stx_verify_sha256 "$stx_bin" "${stx_bin}.sha256"
  fi
  cp "$stx_bin" "$STAGE_DIR/stx"
  chmod +x "$STAGE_DIR/stx"

  frontend_tar="$(ls "$PACKAGES_DIR"/frontend-standalone-*-linux-${arch}.tar.gz 2>/dev/null | head -n1 || true)"
  if [[ -z "$frontend_tar" ]]; then
    echo "[ERROR] missing frontend-standalone-*-linux-${arch}.tar.gz in packages/"
    exit 1
  fi
  if [[ -f "${frontend_tar}.sha256" ]]; then
    stx_verify_sha256 "$frontend_tar" "${frontend_tar}.sha256"
  fi
  mkdir -p "$STAGE_DIR/frontend"
  tar -xzf "$frontend_tar" -C "$STAGE_DIR/frontend"
  # Support tarball with a single top-level directory. / 支持单层顶栏目录的 tar。
  if [[ ! -f "$STAGE_DIR/frontend/server.js" ]]; then
    nested="$(find "$STAGE_DIR/frontend" -mindepth 1 -maxdepth 1 -type d | head -n1 || true)"
    if [[ -n "$nested" && -f "$nested/server.js" ]]; then
      shopt -s dotglob
      mv "$nested"/* "$STAGE_DIR/frontend/"
      shopt -u dotglob
      rmdir "$nested" 2>/dev/null || true
    fi
  fi

  # 规范化 pnpm/Next.js standalone 解压后可能包含的构建机绝对路径软链接。
  # Normalize potential build-machine absolute symlinks after extracting Next.js standalone.
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "
import os
root = '$STAGE_DIR/frontend'
for dirpath, dirnames, filenames in os.walk(root):
    for f in filenames + dirnames:
        p = os.path.join(dirpath, f)
        if os.path.islink(p):
            target = os.readlink(p)
            if '/frontend/.next/standalone/' in target:
                rel_suffix = target.split('/frontend/.next/standalone/')[1]
                real_target = os.path.join(root, rel_suffix)
                rel_target = os.path.relpath(real_target, dirpath)
                try:
                    os.unlink(p)
                    os.symlink(rel_target, p)
                except OSError:
                    pass
" 2>/dev/null || true
  fi

  mkdir -p "$STAGE_DIR/bin/lib" "$STAGE_DIR/lib" "$STAGE_DIR/scripts" "$STAGE_DIR/lib/agent"
  for f in start.sh stop.sh status.sh; do
    if [[ -f "$SOURCE_DIR/bin/$f" ]]; then
      cp "$SOURCE_DIR/bin/$f" "$STAGE_DIR/bin/$f"
    elif [[ -f "$SCRIPT_DIR/bin/$f" ]]; then
      cp "$SCRIPT_DIR/bin/$f" "$STAGE_DIR/bin/$f"
    fi
  done
  if [[ -f "$SCRIPT_DIR/bin/lib/observability.sh" ]]; then
    cp "$SCRIPT_DIR/bin/lib/observability.sh" "$STAGE_DIR/bin/lib/observability.sh"
  elif [[ -f "$SOURCE_DIR/bin/lib/observability.sh" ]]; then
    cp "$SOURCE_DIR/bin/lib/observability.sh" "$STAGE_DIR/bin/lib/observability.sh"
  fi

  cp "$SCRIPT_DIR/install.sh" "$STAGE_DIR/install.sh"
  if [[ -d "$SCRIPT_DIR/lib" ]]; then
    cp -a "$SCRIPT_DIR/lib/." "$STAGE_DIR/lib/"
  fi

  if [[ -f "$SOURCE_DIR/config.example.yaml" ]]; then
    cp "$SOURCE_DIR/config.example.yaml" "$STAGE_DIR/config.example.yaml"
  elif [[ -f "$SCRIPT_DIR/../config.example.yaml" ]]; then
    cp "$SCRIPT_DIR/../config.example.yaml" "$STAGE_DIR/config.example.yaml"
  fi

  # Optional node / observability / agent / java-proxy from packages.
  node_tar="$(ls "$PACKAGES_DIR"/node-*-linux-${arch}.tar.gz 2>/dev/null | head -n1 || true)"
  if [[ -n "$node_tar" ]]; then
    mkdir -p "$STAGE_DIR/runtime"
    tar -xzf "$node_tar" -C "$STAGE_DIR/runtime"
    if [[ ! -x "$STAGE_DIR/runtime/node/bin/node" ]]; then
      nested="$(find "$STAGE_DIR/runtime" -mindepth 1 -maxdepth 1 -type d | head -n1 || true)"
      if [[ -n "$nested" ]]; then
        rm -rf "$STAGE_DIR/runtime/node"
        mv "$nested" "$STAGE_DIR/runtime/node"
      fi
    fi
  elif command -v node >/dev/null 2>&1; then
    # 复用本机系统 Node 时建立符号链接，保证 systemd 等无 NVM 环境下亦可直接执行。
    # Link local system Node so systemd service without NVM in PATH can execute directly.
    mkdir -p "$STAGE_DIR/runtime/node/bin"
    ln -sf "$(command -v node)" "$STAGE_DIR/runtime/node/bin/node"
  fi

  obs_tar="$(ls "$PACKAGES_DIR"/observability-*-linux-${arch}.tar.gz 2>/dev/null | head -n1 || true)"
  if [[ -n "$obs_tar" ]]; then
    mkdir -p "$STAGE_DIR/observability"
    tar -xzf "$obs_tar" -C "$STAGE_DIR/observability"
  fi

  agent_bin="$PACKAGES_DIR/stx-agent-linux-${arch}"
  if [[ -f "$agent_bin" ]]; then
    cp "$agent_bin" "$STAGE_DIR/lib/agent/stx-agent-linux-${arch}"
    chmod +x "$STAGE_DIR/lib/agent/stx-agent-linux-${arch}"
  fi

  # Soft requirement: java-proxy may arrive later via control plane. / java-proxy 可后续由控制面下发。
  if [[ -f "$PACKAGES_DIR/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar" ]]; then
    cp "$PACKAGES_DIR/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar" "$STAGE_DIR/lib/"
  fi
  if [[ -f "$SOURCE_DIR/scripts/stx-java-proxy.sh" ]]; then
    cp "$SOURCE_DIR/scripts/stx-java-proxy.sh" "$STAGE_DIR/scripts/"
  fi

  SOURCE_DIR="$STAGE_DIR"
fi

# Classic package validation when not building from packages-only tree.
# 非 packages 组装时校验经典包内容。
if [[ -z "$PACKAGES_DIR" ]]; then
  for f in stx bin/start.sh bin/stop.sh bin/status.sh config.example.yaml; do
    if [[ ! -e "$SOURCE_DIR/$f" ]]; then
      echo "[ERROR] package payload missing: $f"
      exit 1
    fi
  done
  if [[ ! -e "$SOURCE_DIR/lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar" ]]; then
    echo "[WARN] missing lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar (Agent capability proxy)"
  fi
fi

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
  --exclude='packages' \
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
  "$INSTALL_DIR/bin/status.sh" 2>/dev/null || true
if [[ -f "$INSTALL_DIR/scripts/stx-java-proxy.sh" ]]; then
  chmod +x "$INSTALL_DIR/scripts/stx-java-proxy.sh"
fi

mkdir -p "$INSTALL_DIR/run" "$INSTALL_DIR/logs" "$INSTALL_DIR/data"

stx_probe_default_ports

obs_flag=false
if [[ "$WITH_OBS" == "true" ]]; then
  obs_flag=true
elif [[ "$WITH_OBS" == "auto" && -x "$INSTALL_DIR/observability/prometheus/prometheus" ]]; then
  obs_flag=true
fi
if [[ "$WITH_OBS" == "false" ]]; then
  obs_flag=false
  stx_yaml_set "$INSTALL_DIR/config.yaml" "observability.enabled" "false" || true
fi

stx_configure_defaults "$INSTALL_DIR" "$STX_HTTP_PORT" "$STX_GRPC_PORT" "$STX_FRONTEND_PORT" "$obs_flag"

if [[ "$SKIP_SYSTEMD" != "true" ]]; then
  stx_install_systemd "$INSTALL_DIR"
fi

stx_print_finish_tips "$INSTALL_DIR"

if [[ "$AUTO_START" == "true" ]]; then
  echo "[INFO] auto starting ..."
  (
    cd "$INSTALL_DIR"
    FRONTEND_PORT="$STX_FRONTEND_PORT" \
    NEXT_PUBLIC_BACKEND_BASE_URL="http://127.0.0.1:${STX_HTTP_PORT}" \
    CONFIG_PATH="$INSTALL_DIR/config.yaml" \
    ./bin/start.sh
  )
else
  echo "[INFO] skip auto start (--no-start)"
fi
