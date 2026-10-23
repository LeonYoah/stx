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

# Build an offline install bundle on a networked machine.
# 在有网机器上组装离线安装包。

STX_GITHUB_OWNER="${STX_GITHUB_OWNER:-LeonYoah}"
STX_GITHUB_REPO="${STX_GITHUB_REPO:-stx}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# curl|bash 时仓库不在本地：从 GitHub raw 拉辅助脚本。/ When piped via curl|bash, fetch helpers from GitHub raw.
stx_bootstrap_root_if_needed() {
  if [[ -f "$ROOT_DIR/install/download-lib.sh" ]]; then
    return 0
  fi
  local boot ref prefix base path url
  boot="${TMPDIR:-/tmp}/stx-bootstrap-bundle-$$"
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
  # Agent 启动脚本：Release 资产缺失时（如 v1.2.0）作为兜底来源，失败不致命。
  # Agent launcher: fallback source when the release asset is missing (e.g. v1.2.0); non-fatal.
  path="scripts/stx-java-proxy.sh"
  url="${base}/${path}"
  [[ -n "$prefix" ]] && url="${prefix}/${base}/${path}"
  mkdir -p "$boot/scripts"
  if ! curl -fsSL --retry 3 --retry-delay 1 -o "$boot/$path" "$url"; then
    rm -f "$boot/$path"
    echo "[WARN] cannot bootstrap ${path} from ${ref}" >&2
  fi
  ROOT_DIR="$boot"
  export STX_BOOTSTRAP_ROOT="$boot"
}

# 前置依赖检查（不再需要 python3）。/ Upfront dependency check (python3 is no longer required).
for _stx_cmd in curl tar gzip awk sed grep; do
  if ! command -v "$_stx_cmd" >/dev/null 2>&1; then
    echo "[ERROR] missing required command: $_stx_cmd (please install it first, e.g. apt-get install -y $_stx_cmd)" >&2
    exit 1
  fi
done
unset _stx_cmd

stx_bootstrap_root_if_needed
# shellcheck source=../install/download-lib.sh
source "$ROOT_DIR/install/download-lib.sh"

usage() {
  cat <<'USAGE'
Build STX offline bundle

Usage:
  curl -fsSL <release-or-proxy-url>/download-bundle.sh | bash -s -- [options]
  install/download-bundle.sh [options]

Options:
  --version <tag>                 Release tag, e.g. v1.2.0 (default: latest; env STX_VERSION)
  --arch <amd64|arm64>            Default: auto-detect
  --node-variant <official|glibc217>  Default: auto by packaging host glibc (<2.27 => glibc217,
                                  amd64 only; arm64 always official)
  --without-node                  Do NOT bundle the Node runtime (default: always bundled).
                                  Only use when the OFFLINE TARGET already has Node ≥ 18.18;
                                  the packaging host's own Node is irrelevant.
  --without-observability         Skip observability deps package
  --with-agent                    Include stx-agent binary (default: true)
  --without-agent                 Skip stx-agent
  --deps-tag <tag>                Deps release tag (default: deps)
  --output-dir <path>             Output parent dir (default: ./dist/offline)
  --region <cn|global>            Force download region
  -h, --help
USAGE
}

VERSION="${STX_VERSION:-latest}"
ARCH=""
NODE_VARIANT=""
WITH_OBS=true
WITH_AGENT=true
WITHOUT_NODE=false
DEPS_TAG="${STX_DEPS_TAG:-deps}"
if [[ -n "${STX_BOOTSTRAP_ROOT:-}" ]]; then
  OUTPUT_PARENT="${OUTPUT_DIR:-$(pwd)/dist/offline}"
else
  OUTPUT_PARENT="${OUTPUT_DIR:-$ROOT_DIR/dist/offline}"
fi
REGION=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --arch) ARCH="${2:-}"; shift 2 ;;
    --node-variant) NODE_VARIANT="${2:-}"; shift 2 ;;
    --with-observability) WITH_OBS=true; shift ;;
    --without-observability) WITH_OBS=false; shift ;;
    --without-node) WITHOUT_NODE=true; shift ;;
    --with-agent) WITH_AGENT=true; shift ;;
    --without-agent) WITH_AGENT=false; shift ;;
    --deps-tag) DEPS_TAG="${2:-}"; shift 2 ;;
    --output-dir) OUTPUT_PARENT="${2:-}"; shift 2 ;;
    --region) REGION="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1"; usage; exit 1 ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  echo "[ERROR] --version is required"
  usage
  exit 1
fi
[[ -z "$ARCH" ]] && ARCH="$(stx_detect_arch)"
[[ -z "$NODE_VARIANT" ]] && NODE_VARIANT="$(stx_recommend_node_variant "$ARCH")"
[[ -n "$REGION" ]] && export STX_DOWNLOAD_REGION="$REGION"

region="$(stx_detect_download_region)"
stx_select_mirror_prefix "$region"

# Resolve latest like install-online. / 与在线安装一样解析 latest。
if [[ "$VERSION" == "latest" ]]; then
  api="https://api.github.com/repos/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/releases/latest"
  url="$api"
  if [[ -n "${STX_SELECTED_MIRROR_PREFIX:-}" ]]; then
    url="${STX_SELECTED_MIRROR_PREFIX%/}/https://api.github.com/repos/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/releases/latest"
  fi
  VERSION="$(curl -fsSL "$url" 2>/dev/null | stx_json_tag_name || true)"
  if [[ -z "$VERSION" ]]; then
    echo "[ERROR] cannot resolve latest release tag; pass --version explicitly" >&2
    exit 1
  fi
  echo "[INFO] resolved latest -> $VERSION"
fi

BUNDLE_NAME="stx-offline-bundle-${VERSION}-linux-${ARCH}"
BUNDLE_DIR="$OUTPUT_PARENT/$BUNDLE_NAME"
rm -rf "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR/packages" "$BUNDLE_DIR/bin"

download_checked() {
  local tag="$1" asset="$2" dest="$3"
  stx_download_release_asset "$tag" "$asset" "$dest"
  if stx_download_release_asset "$tag" "${asset}.sha256" "${dest}.sha256" 2>/dev/null; then
    stx_verify_sha256 "$dest" "${dest}.sha256"
  fi
}

echo "[INFO] building offline bundle: $BUNDLE_DIR"
download_checked "$VERSION" "stx-linux-${ARCH}" "$BUNDLE_DIR/packages/stx-linux-${ARCH}"

if stx_download_release_asset "$VERSION" "frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz" \
  "$BUNDLE_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz" 2>/dev/null; then
  stx_download_release_asset "$VERSION" "frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz.sha256" \
    "$BUNDLE_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz.sha256" 2>/dev/null || true
  if [[ -f "$BUNDLE_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz.sha256" ]]; then
    stx_verify_sha256 "$BUNDLE_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz" \
      "$BUNDLE_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz.sha256"
  fi
else
  download_checked "$VERSION" "frontend-standalone-linux-${ARCH}.tar.gz" \
    "$BUNDLE_DIR/packages/frontend-standalone-${VERSION}-linux-${ARCH}.tar.gz"
fi

if [[ "$WITH_AGENT" == "true" ]]; then
  download_checked "$VERSION" "stx-agent-linux-${ARCH}" "$BUNDLE_DIR/packages/stx-agent-linux-${ARCH}" || \
    echo "[WARN] stx-agent missing on release $VERSION"
fi

# 离线目标机的 Node 与打包机无关：默认总是组装 Node，仅 --without-node 时跳过。
# The offline target's Node has nothing to do with the packaging host: always bundle Node
# unless --without-node is given explicitly.
WITH_NODE=true
if [[ "$WITHOUT_NODE" == "true" ]]; then
  WITH_NODE=false
  echo "[INFO] --without-node: skip node package (offline target must provide Node >= 18.18)"
else
  node_ok=false
  for major in 22 18; do
    asset="node-${major}-${NODE_VARIANT}-linux-${ARCH}.tar.gz"
    if stx_download_release_asset "$DEPS_TAG" "$asset" "$BUNDLE_DIR/packages/$asset" 2>/dev/null; then
      if stx_download_release_asset "$DEPS_TAG" "${asset}.sha256" "$BUNDLE_DIR/packages/${asset}.sha256" 2>/dev/null; then
        stx_verify_sha256 "$BUNDLE_DIR/packages/$asset" "$BUNDLE_DIR/packages/${asset}.sha256"
      fi
      echo "[INFO] bundled node runtime: $asset"
      node_ok=true
      break
    fi
  done
  if [[ "$node_ok" != "true" ]]; then
    stx_node_package_missing_error "$NODE_VARIANT" "$ARCH" "$DEPS_TAG" \
      "or pass --without-node if the offline target already has Node >= 18.18 (otherwise it fails with 'node runtime not found')."
    exit 1
  fi
fi

if [[ "$WITH_OBS" == "true" ]]; then
  obs_asset="observability-prom3.9.1-am0.31.1-gf12.3.3-linux-${ARCH}.tar.gz"
  stx_download_release_asset "$DEPS_TAG" "$obs_asset" "$BUNDLE_DIR/packages/$obs_asset" 2>/dev/null \
    || echo "[WARN] observability package missing: $obs_asset"
fi

# 下载 capability proxy 代际 jar（v2 + v3）与启动脚本。
# Download capability proxy epoch jars (v2 + v3) and launcher script.
for epoch in v2 v3; do
  proxy_jar="stx-java-proxy-${epoch}.jar"
  download_checked "$VERSION" "$proxy_jar" "$BUNDLE_DIR/packages/$proxy_jar" || \
    echo "[WARN] capability proxy jar missing on release $VERSION: $proxy_jar"
done

mkdir -p "$BUNDLE_DIR/bin/lib" "$BUNDLE_DIR/scripts"
stx_stage_java_proxy_script "$VERSION" "$BUNDLE_DIR/scripts/stx-java-proxy.sh" "$ROOT_DIR" || true
cp "$ROOT_DIR/install/install.sh" "$BUNDLE_DIR/install.sh"
cp "$ROOT_DIR/install/download-lib.sh" "$BUNDLE_DIR/download-lib.sh"
cp "$ROOT_DIR/install/install-core.sh" "$BUNDLE_DIR/install-core.sh"
cp "$ROOT_DIR/install/bin/start.sh" "$BUNDLE_DIR/bin/start.sh"
cp "$ROOT_DIR/install/bin/stop.sh" "$BUNDLE_DIR/bin/stop.sh"
cp "$ROOT_DIR/install/bin/status.sh" "$BUNDLE_DIR/bin/status.sh"
cp "$ROOT_DIR/install/bin/lib/observability.sh" "$BUNDLE_DIR/bin/lib/observability.sh"
cp "$ROOT_DIR/config.example.yaml" "$BUNDLE_DIR/config.example.yaml"
chmod +x "$BUNDLE_DIR/install.sh" "$BUNDLE_DIR/bin/"*.sh

cat >"$BUNDLE_DIR/MANIFEST.json" <<EOF
{
  "version": "$VERSION",
  "arch": "$ARCH",
  "node_variant": "$NODE_VARIANT",
  "with_node": $WITH_NODE,
  "deps_tag": "$DEPS_TAG",
  "with_observability": $WITH_OBS,
  "with_agent": $WITH_AGENT,
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

(
  cd "$BUNDLE_DIR/packages"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum * >"../SHA256SUMS" 2>/dev/null || true
  else
    shasum -a 256 * >"../SHA256SUMS" 2>/dev/null || true
  fi
)

cat >"$BUNDLE_DIR/README-OFFLINE.md" <<EOF
# STX Offline Bundle ($VERSION / $ARCH)

1. Copy this directory (or the \`.tar.gz\` beside it) to the offline host.
2. Run:

\`\`\`bash
./install.sh --install-dir /opt/stx --offline
\`\`\`

Optional: \`--without-observability\` \`--no-start\` \`--no-systemd\`

Node runtime bundled: $WITH_NODE (when false, the bundle was built with \`--without-node\`
and the offline host must already provide Node >= 18.18 in PATH).

This bundle must not require network access during install.
EOF

tar -C "$OUTPUT_PARENT" -czf "${BUNDLE_DIR}.tar.gz" "$BUNDLE_NAME"
if [[ -n "${STX_BOOTSTRAP_ROOT:-}" ]]; then
  rm -rf "$STX_BOOTSTRAP_ROOT"
fi

echo "[OK] offline bundle ready:"
echo "     dir:  $BUNDLE_DIR"
echo "     tar:  ${BUNDLE_DIR}.tar.gz"
echo "     transfer to the offline host, then:"
echo "       tar -xzf ${BUNDLE_NAME}.tar.gz && cd ${BUNDLE_NAME} && ./install.sh --offline"
