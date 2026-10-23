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

# 前置依赖检查：缺命令时直接给出清晰错误，而不是在中途静默失败（不需要 python3）。
# Upfront dependency check: fail early with a clear message instead of failing silently midway (no python3 needed).
stx_require_cmds tar gzip find awk sed grep mktemp cp mv || exit 1

# 返回目录下第一个子目录（按名称排序，含隐藏目录；不跟随软链接）。/ Print the first subdirectory (sorted, incl. dot dirs; symlinks skipped).
stx_first_subdir() {
  local parent="$1" d
  local -a cands=()
  shopt -s nullglob dotglob
  cands=("$parent"/*/)
  shopt -u nullglob dotglob
  for d in "${cands[@]}"; do
    d="${d%/}"
    if [[ -d "$d" && ! -L "$d" ]]; then
      echo "$d"
      return 0
    fi
  done
  echo ""
}

# 把指向构建机 .../frontend/.next/standalone/<x> 的绝对软链接改写为相对链接。
# Rewrite absolute symlinks pointing at the build host's .../frontend/.next/standalone/<x> into relative links.
stx_normalize_standalone_symlinks() {
  local root="$1" link target suffix dir rel up
  local marker="/frontend/.next/standalone/"
  [[ -d "$root" ]] || return 0
  while IFS= read -r -d '' link; do
    target="$(readlink "$link" 2>/dev/null)" || continue
    [[ "$target" == *"$marker"* ]] || continue
    suffix="${target#*"$marker"}"
    dir="$(dirname "$link")"
    rel="${dir#"$root"}"
    rel="${rel#/}"
    up=""
    if [[ -n "$rel" ]]; then
      up="$(printf '%s\n' "$rel" | awk -F/ '{ s = ""; for (i = 1; i <= NF; i++) s = s "../"; print s }')"
    fi
    rm -f "$link" && ln -s "${up}${suffix}" "$link" || true
  done < <(find "$root" -type l -print0 2>/dev/null)
}

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
    nested="$(stx_first_subdir "$STAGE_DIR/frontend")"
    if [[ -n "$nested" && -f "$nested/server.js" ]]; then
      shopt -s dotglob
      mv "$nested"/* "$STAGE_DIR/frontend/"
      shopt -u dotglob
      rmdir "$nested" 2>/dev/null || true
    fi
  fi

  # 规范化 pnpm/Next.js standalone 解压后可能包含的构建机绝对路径软链接（纯 shell，不依赖 python3）。
  # Normalize potential build-machine absolute symlinks after extracting Next.js standalone (pure shell, no python3).
  stx_normalize_standalone_symlinks "$STAGE_DIR/frontend"

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
      nested="$(stx_first_subdir "$STAGE_DIR/runtime")"
      if [[ -n "$nested" ]]; then
        rm -rf "$STAGE_DIR/runtime/node"
        mv "$nested" "$STAGE_DIR/runtime/node"
      fi
    fi
    if [[ ! -x "$STAGE_DIR/runtime/node/bin/node" ]]; then
      echo "[ERROR] bundled node package $node_tar does not contain bin/node" >&2
      exit 1
    fi
  else
    # 包内无 Node（bundle 以 --without-node 组装，或在线安装 --without-node）：仅当本机 Node ≥ 18.18 时复用，
    # 并建立符号链接，保证 systemd 等无 NVM 环境下亦可直接执行；否则立即报错，避免启动时才 'node runtime not found'。
    # No node package (bundle built with --without-node, or online --without-node): reuse local Node only when
    # it is >= 18.18 (symlinked so systemd without NVM in PATH works); otherwise fail now instead of at start.
    local_node="$(command -v node 2>/dev/null || true)"
    local_node_ver=""
    [[ -n "$local_node" ]] && local_node_ver="$("$local_node" -v 2>/dev/null || true)"
    if [[ -z "$local_node" ]] || ! stx_node_version_ok "$local_node_ver"; then
      echo "[ERROR] no node-*-linux-${arch}.tar.gz in $PACKAGES_DIR and no usable system Node (need >= 18.18, found: ${local_node_ver:-none})" >&2
      echo "        this package was likely built with --without-node. Either install Node >= 18.18 on this host" >&2
      echo "        (and make sure it is in PATH), or rebuild the offline bundle WITHOUT --without-node." >&2
      exit 1
    fi
    echo "[INFO] no bundled node package; using system node $local_node ($local_node_ver)"
    mkdir -p "$STAGE_DIR/runtime/node/bin"
    ln -sf "$local_node" "$STAGE_DIR/runtime/node/bin/node"
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
  # 同步全部代际 jar（v2/v3），供 2.x / 3.x 集群按版本选型。
  # Stage every epoch jar (v2/v3) so 2.x / 3.x clusters can pick the matching build.
  if compgen -G "$PACKAGES_DIR/stx-java-proxy-*.jar" >/dev/null; then
    cp "$PACKAGES_DIR"/stx-java-proxy-*.jar "$STAGE_DIR/lib/"
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
  if ! compgen -G "$SOURCE_DIR/lib/stx-java-proxy-*.jar" >/dev/null; then
    echo "[WARN] missing lib/stx-java-proxy-*.jar (Agent capability proxy epochs v2/v3)"
  elif [[ ! -e "$SOURCE_DIR/lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar" ]]; then
    echo "[WARN] missing default lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar (Agent capability proxy)"
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
# 复制源文件到安装目录，使用 --anchored 确保仅排除根目录下的临时文件/缓存包，避免误伤 frontend/.next/server/app/(main)/packages 等深层业务路由
# Copy source files to install directory, use --anchored to ensure only root-level temporary files/cached packages are excluded, avoiding unintended exclusion of deep routes like frontend/.next/server/app/(main)/packages
tar -C "$SOURCE_DIR" \
  --anchored \
  --exclude='./run/*' \
  --exclude='./logs/*' \
  --exclude='./packages' \
  --exclude='./packages/*' \
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

STX_SYSTEMD_MODE=""
if [[ "$SKIP_SYSTEMD" != "true" ]]; then
  stx_install_systemd "$INSTALL_DIR"
fi

stx_print_finish_tips "$INSTALL_DIR"

if [[ "$AUTO_START" == "true" ]]; then
  echo "[INFO] auto starting ..."
  # 已安装 systemd unit 时经 systemctl 启动（并 enable），这样首次安装后 systemctl status/restart stx 即可用；
  # --no-systemd 或无 systemd 时仍直接调用 start.sh。
  # unit 是 Type=oneshot + RemainAfterExit=yes：重装时若进程已被 stop.sh 停掉而 unit 仍是 active(exited)，
  # `systemctl start` 是空操作，所以用 restart，并用 status.sh 核实 backend 确实在跑。
  # When the systemd unit is installed, start through systemctl (enabled) so `systemctl status/restart stx`
  # works right after install; fall back to start.sh for --no-systemd or hosts without systemd.
  # The unit is Type=oneshot + RemainAfterExit=yes: on reinstall it may still be active(exited) while the
  # processes were stopped via stop.sh, making `systemctl start` a no-op. Use restart and verify via status.sh.
  if [[ -n "$STX_SYSTEMD_MODE" ]]; then
    sysctl_cmd=(systemctl)
    journal_hint="journalctl -u stx -n 200"
    if [[ "$STX_SYSTEMD_MODE" == "user" ]]; then
      sysctl_cmd=(systemctl --user)
      journal_hint="journalctl --user -u stx -n 200"
    fi
    "${sysctl_cmd[@]}" enable stx.service >/dev/null 2>&1 || true
    if ! "${sysctl_cmd[@]}" restart stx.service; then
      echo "[ERROR] ${sysctl_cmd[*]} restart stx failed; inspect: ${sysctl_cmd[*]} status stx / $journal_hint" >&2
      exit 1
    fi
    # 先取完整输出再匹配：pipefail 下 `status.sh | grep -q` 可能因 SIGPIPE 误判失败。
    # Capture first: with pipefail, `status.sh | grep -q` can fail spuriously on SIGPIPE.
    status_out="$(cd "$INSTALL_DIR" && ./bin/status.sh 2>/dev/null || true)"
    if grep -q 'backend: running' <<<"$status_out"; then
      echo "[OK] started via systemd: ${sysctl_cmd[*]} status stx"
    else
      echo "[ERROR] ${sysctl_cmd[*]} restart stx returned but backend is not running; inspect: $INSTALL_DIR/bin/status.sh / $journal_hint" >&2
      exit 1
    fi
  else
    (
      cd "$INSTALL_DIR"
      FRONTEND_PORT="$STX_FRONTEND_PORT" \
      NEXT_PUBLIC_BACKEND_BASE_URL="http://127.0.0.1:${STX_HTTP_PORT}" \
      CONFIG_PATH="$INSTALL_DIR/config.yaml" \
      ./bin/start.sh
    )
  fi
else
  echo "[INFO] skip auto start (--no-start)"
fi
