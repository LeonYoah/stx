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

# Online one-click installer: detect region, download assets, invoke install.sh.
# 在线一键安装：判定地区、下载资产、调用 install.sh。

STX_GITHUB_OWNER="${STX_GITHUB_OWNER:-LeonYoah}"
STX_GITHUB_REPO="${STX_GITHUB_REPO:-stx}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# curl|bash 时仓库不在本地：从 GitHub raw 拉安装辅助脚本。/ When piped via curl|bash, fetch install helpers from GitHub raw.
stx_bootstrap_root_if_needed() {
  if [[ -f "$ROOT_DIR/install/download-lib.sh" ]]; then
    return 0
  fi
  local boot ref prefix base path url
  boot="${TMPDIR:-/tmp}/stx-bootstrap-$$"
  mkdir -p "$boot/install/bin/lib"
  ref="${STX_BOOTSTRAP_REF:-main}"
  prefix=""
  if [[ -n "${STX_DOWNLOAD_MIRROR_PREFIX:-}" ]]; then
    prefix="${STX_DOWNLOAD_MIRROR_PREFIX%/}"
  elif [[ "${STX_DOWNLOAD_REGION:-}" == "cn" ]]; then
    prefix="https://v4.gh-proxy.org"
  fi
  base="https://raw.githubusercontent.com/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/${ref}"
  echo "[INFO] standalone mode: bootstrap helpers from ${ref}" >&2
  for path in \
    install/download-lib.sh \
    install/install-core.sh \
    install/install.sh \
    install/bin/start.sh \
    install/bin/stop.sh \
    install/bin/status.sh \
    install/bin/lib/observability.sh \
    config.example.yaml
  do
    url="${base}/${path}"
    if [[ -n "$prefix" ]]; then
      url="${prefix}/${base}/${path}"
    fi
    mkdir -p "$(dirname "$boot/$path")"
    curl -fsSL --retry 3 --retry-delay 1 -o "$boot/$path" "$url"
  done
  ROOT_DIR="$boot"
  export STX_BOOTSTRAP_ROOT="$boot"
}

stx_bootstrap_root_if_needed
# shellcheck source=../install/download-lib.sh
source "$ROOT_DIR/install/download-lib.sh"
# shellcheck source=../install/install-core.sh
source "$ROOT_DIR/install/install-core.sh"

usage() {
  cat <<'USAGE'
STX online installer

Usage:
  curl -fsSL <release-or-proxy-url>/install-online.sh | bash
  install/install-online.sh [options]

Options:
  --version <tag>              Release tag (default: latest)
  --install-dir <path>         Install dir (default: /opt/stx)
  --arch <amd64|arm64>         Target arch (default: auto)
  --node-variant <official|glibc217>  Node runtime variant (default: auto by glibc)
  --without-node               Do not download Node package (use system Node ≥ 18.18)
  --without-observability      Skip bundled Prometheus/Alertmanager/Grafana
  --region <cn|global>         Force download region
  --no-start                   Pass through to install.sh
  --no-systemd                 Pass through to install.sh
  --deps-tag <tag>             Deps release tag (default: deps)
  -h, --help
USAGE
}

VERSION="${STX_VERSION:-latest}"
INSTALL_DIR="${STX_INSTALL_DIR:-/opt/stx}"
ARCH=""
NODE_VARIANT=""
WITH_OBS=true
WITHOUT_OBS=false
WITHOUT_NODE=false
REGION=""
DEPS_TAG="${STX_DEPS_TAG:-deps}"
NO_START=false
NO_SYSTEMD=false
WORK_DIR="${STX_DOWNLOAD_WORK_DIR:-${TMPDIR:-/tmp}/stx-online-$$}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --install-dir) INSTALL_DIR="${2:-}"; shift 2 ;;
    --arch) ARCH="${2:-}"; shift 2 ;;
    --node-variant) NODE_VARIANT="${2:-}"; shift 2 ;;
    --with-observability) WITH_OBS=true; shift ;;
    --without-observability) WITHOUT_OBS=true; WITH_OBS=false; shift ;;
    --without-node) WITHOUT_NODE=true; shift ;;
    --region) REGION="${2:-}"; shift 2 ;;
    --deps-tag) DEPS_TAG="${2:-}"; shift 2 ;;
    --no-start) NO_START=true; shift ;;
    --no-systemd) NO_SYSTEMD=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1"; usage; exit 1 ;;
  esac
done

if [[ -z "$ARCH" ]]; then
  ARCH="$(stx_detect_arch)"
fi
if [[ -z "$NODE_VARIANT" ]]; then
  NODE_VARIANT="$(stx_recommend_node_variant)"
fi
if [[ -n "$REGION" ]]; then
  export STX_DOWNLOAD_REGION="$REGION"
fi

region="$(stx_detect_download_region)"
echo "[INFO] region=$region arch=$ARCH node_variant=$NODE_VARIANT version=$VERSION"
stx_select_mirror_prefix "$region"

mkdir -p "$WORK_DIR/packages"
cleanup() {
  rm -rf "$WORK_DIR"
  if [[ -n "${STX_BOOTSTRAP_ROOT:-}" ]]; then
    rm -rf "$STX_BOOTSTRAP_ROOT"
  fi
}
trap cleanup EXIT

# Resolve "latest" via GitHub API when needed. / 需要时通过 GitHub API 解析 latest。
resolve_version() {
  local v="$1"
  if [[ "$v" != "latest" ]]; then
    echo "$v"
    return 0
  fi
  local api="https://api.github.com/repos/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/releases/latest"
  local prefix="${STX_SELECTED_MIRROR_PREFIX:-}"
  local url="$api"
  if [[ -n "$prefix" ]]; then
    url="${prefix%/}/https://api.github.com/repos/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/releases/latest"
  fi
  local tag
  tag="$(curl -fsSL "$url" 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("tag_name",""))' || true)"
  if [[ -z "$tag" ]]; then
    echo "[ERROR] cannot resolve latest release tag; pass --version explicitly" >&2
    exit 1
  fi
  echo "$tag"
}

VERSION="$(resolve_version "$VERSION")"
echo "[INFO] using release tag: $VERSION"

download_checked() {
  local tag="$1"
  local asset="$2"
  local dest="$3"
  stx_assert_online_allowed
  stx_download_release_asset "$tag" "$asset" "$dest"
  # checksum optional but preferred
  if stx_download_release_asset "$tag" "${asset}.sha256" "${dest}.sha256" 2>/dev/null; then
    stx_verify_sha256 "$dest" "${dest}.sha256"
  else
    echo "[WARN] no ${asset}.sha256; skip checksum"
  fi
}

download_checked "$VERSION" "stx-linux-${ARCH}" "$WORK_DIR/packages/stx-linux-${ARCH}"
if stx_download_release_asset "$VERSION" "frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz" \
  "$WORK_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz" 2>/dev/null; then
  if stx_download_release_asset "$VERSION" "frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz.sha256" \
    "$WORK_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz.sha256" 2>/dev/null; then
    stx_verify_sha256 "$WORK_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz" \
      "$WORK_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz.sha256"
  fi
else
  download_checked "$VERSION" "frontend-standalone-linux-${ARCH}.tar.gz" \
    "$WORK_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz"
fi

# Optional agent
if stx_download_release_asset "$VERSION" "stx-agent-linux-${ARCH}" "$WORK_DIR/packages/stx-agent-linux-${ARCH}" 2>/dev/null; then
  stx_download_release_asset "$VERSION" "stx-agent-linux-${ARCH}.sha256" \
    "$WORK_DIR/packages/stx-agent-linux-${ARCH}.sha256" 2>/dev/null || true
  if [[ -f "$WORK_DIR/packages/stx-agent-linux-${ARCH}.sha256" ]]; then
    stx_verify_sha256 "$WORK_DIR/packages/stx-agent-linux-${ARCH}" "$WORK_DIR/packages/stx-agent-linux-${ARCH}.sha256" || true
  fi
fi

# Node: skip when asked, or when local node is good enough. / 显式跳过，或本机 node 足够则跳过。
need_node=true
if [[ "$WITHOUT_NODE" == "true" ]]; then
  echo "[INFO] --without-node: skip node package"
  need_node=false
elif command -v node >/dev/null 2>&1; then
  if python3 - <<'PY'
import re, subprocess, sys
out = subprocess.check_output(["node", "-v"], text=True).strip().lstrip("v")
parts = [int(x) for x in re.split(r"[^\d]+", out) if x.isdigit()]
major, minor, patch = (parts + [0, 0, 0])[:3]
sys.exit(0 if (major > 18 or (major == 18 and minor >= 18)) else 1)
PY
  then
    echo "[INFO] local node $(node -v) is usable; skip node package"
    need_node=false
  fi
fi

if [[ "$need_node" == "true" ]]; then
  # Try common deps asset names. / 尝试常见 deps 资产名。
  node_asset=""
  for major in 22 18; do
    candidate="node-${major}-${NODE_VARIANT}-linux-${ARCH}.tar.gz"
    if stx_download_release_asset "$DEPS_TAG" "$candidate" "$WORK_DIR/packages/$candidate" 2>/dev/null; then
      node_asset="$candidate"
      break
    fi
  done
  if [[ -z "$node_asset" ]]; then
    echo "[WARN] node deps package not found on tag=$DEPS_TAG; install may require system node"
  else
    stx_download_release_asset "$DEPS_TAG" "${node_asset}.sha256" "$WORK_DIR/packages/${node_asset}.sha256" 2>/dev/null || true
  fi
fi

if [[ "$WITH_OBS" == "true" && "$WITHOUT_OBS" != "true" ]]; then
  obs_asset="$(curl -fsSL "$(stx_build_github_release_url "$DEPS_TAG" "MANIFEST.json" "${STX_SELECTED_MIRROR_PREFIX:-}")" 2>/dev/null \
    | python3 -c "import json,sys; m=json.load(sys.stdin); print(m.get('observability',{}).get('${ARCH}',''))" 2>/dev/null || true)"
  if [[ -z "$obs_asset" ]]; then
    # fallback glob-like name from known versions in package-release defaults
    obs_asset="observability-prom3.9.1-am0.31.1-gf12.3.3-linux-${ARCH}.tar.gz"
  fi
  stx_download_release_asset "$DEPS_TAG" "$obs_asset" "$WORK_DIR/packages/$obs_asset" 2>/dev/null \
    || echo "[WARN] observability package missing: $obs_asset"
fi

# Stage installer scripts into work dir. / 将安装脚本放入工作目录。
cp "$ROOT_DIR/install/install.sh" "$WORK_DIR/install.sh"
cp "$ROOT_DIR/install/download-lib.sh" "$WORK_DIR/download-lib.sh"
cp "$ROOT_DIR/install/install-core.sh" "$WORK_DIR/install-core.sh"
mkdir -p "$WORK_DIR/bin/lib"
cp "$ROOT_DIR/install/bin/start.sh" "$WORK_DIR/bin/start.sh"
cp "$ROOT_DIR/install/bin/stop.sh" "$WORK_DIR/bin/stop.sh"
cp "$ROOT_DIR/install/bin/status.sh" "$WORK_DIR/bin/status.sh"
cp "$ROOT_DIR/install/bin/lib/observability.sh" "$WORK_DIR/bin/lib/observability.sh"
cp "$ROOT_DIR/config.example.yaml" "$WORK_DIR/config.example.yaml"
chmod +x "$WORK_DIR/install.sh" "$WORK_DIR/bin/"*.sh

install_args=(--install-dir "$INSTALL_DIR" --offline)
if [[ "$WITH_OBS" == "true" && "$WITHOUT_OBS" != "true" ]]; then
  install_args+=(--with-observability)
fi
if [[ "$WITHOUT_OBS" == "true" ]]; then
  install_args+=(--without-observability)
fi
if [[ "$NO_START" == "true" ]]; then
  install_args+=(--no-start)
fi
if [[ "$NO_SYSTEMD" == "true" ]]; then
  install_args+=(--no-systemd)
fi

bash "$WORK_DIR/install.sh" "${install_args[@]}"
