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

# 组装 STX 完整离线包（CI 发版与本地通用）。
# Assemble complete STX offline bundles (shared by release CI and local builds).
#
# 输入 / Inputs:
#   --from-dir <dir>      本地构建产物（install/package-release.sh --layout split 的 dist/releases）
#                         Local build outputs (dist/releases from package-release.sh --layout split)
#   --from-release <tag>  或从已发布 Release 下载 stx / agent / frontend / java-proxy
#                         Or download stx / agent / frontend / java-proxy from a published release
#   Node 与三件套来自 --deps-dir 或 Release tag `deps`（自动下载并校验 sha256）。
#   Node and the observability stack come from --deps-dir or the `deps` release tag (sha256 verified).
#
# 输出 / Output (per variant):
#   <output>/stx-<ver>-linux-amd64-offline.tar.gz          + .sha256
#   <output>/stx-<ver>-linux-arm64-offline.tar.gz          + .sha256
#   <output>/stx-<ver>-linux-amd64-centos7-offline.tar.gz  + .sha256
#   --alias 时额外输出 latest 固定别名 stx-offline-linux-<...>.tar.gz（+ .sha256）。
#   With --alias also the stable latest aliases stx-offline-linux-<...>.tar.gz (+ .sha256).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=download-lib.sh
source "$SCRIPT_DIR/download-lib.sh"

usage() {
  cat <<'USAGE'
Build STX offline bundles

Usage:
  install/build-offline-bundle.sh --variant <amd64|arm64|centos7|all> [options]

Options:
  --variant <list>         amd64, arm64, centos7, comma separated, or all (default: all)
  --version <ver>          Version label in file names (default: git describe --tags --always)
  --from-dir <dir>         Local build outputs (default: dist/releases)
  --from-release <tag>     Download stx/agent/frontend/java-proxy from this release instead of --from-dir
  --deps-dir <dir>         Local dir with node-*.tar.gz / observability-*.tar.gz (default: download from deps tag)
  --deps-tag <tag>         Deps release tag (default: deps)
  --cache-dir <dir>        Download cache (default: .cache/offline-bundle)
  --output-dir <dir>       Output dir (default: dist/offline)
  --without-observability  Do not bundle Prometheus/Alertmanager/Grafana
  --alias                  Also write stable alias files for releases/latest/download links
  --keep-dir               Keep the unpacked bundle directory next to the tarball
  -h, --help

Variants:
  amd64    Linux x86_64, official Node 22 (glibc >= 2.28)
  arm64    Linux aarch64, official Node 22 (glibc >= 2.28)
  centos7  CentOS 7 / glibc 2.17 x86_64, Node 18 linux-x64-glibc217
USAGE
}

VARIANTS="all"
VERSION=""
FROM_DIR="$ROOT_DIR/dist/releases"
FROM_RELEASE=""
DEPS_DIR=""
DEPS_TAG="${STX_DEPS_TAG:-deps}"
CACHE_DIR="$ROOT_DIR/.cache/offline-bundle"
OUTPUT_DIR="$ROOT_DIR/dist/offline"
WITH_OBS=true
WITH_ALIAS=false
KEEP_DIR=false
MAX_ASSET_BYTES=$((2 * 1024 * 1024 * 1024 - 1))

while [[ $# -gt 0 ]]; do
  case "$1" in
    --variant) VARIANTS="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --from-dir) FROM_DIR="${2:-}"; shift 2 ;;
    --from-release) FROM_RELEASE="${2:-}"; shift 2 ;;
    --deps-dir) DEPS_DIR="${2:-}"; shift 2 ;;
    --deps-tag) DEPS_TAG="${2:-}"; shift 2 ;;
    --cache-dir) CACHE_DIR="${2:-}"; shift 2 ;;
    --output-dir) OUTPUT_DIR="${2:-}"; shift 2 ;;
    --without-observability) WITH_OBS=false; shift ;;
    --alias) WITH_ALIAS=true; shift ;;
    --keep-dir) KEEP_DIR=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[ERROR] unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

stx_require_cmds tar gzip awk sed grep curl || exit 1

[[ "$VARIANTS" == "all" ]] && VARIANTS="amd64,arm64,centos7"
IFS=',' read -r -a VARIANT_LIST <<<"$VARIANTS"
for v in "${VARIANT_LIST[@]}"; do
  stx_bundle_platform "$v" >/dev/null || exit 1
done

if [[ -z "$VERSION" ]]; then
  if [[ -n "$FROM_RELEASE" ]]; then
    VERSION="$FROM_RELEASE"
  else
    VERSION="$(git -C "$ROOT_DIR" describe --tags --always 2>/dev/null || echo dev)"
  fi
fi
GIT_COMMIT="${STX_GIT_COMMIT:-${GITHUB_SHA:-$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo unknown)}}"
mkdir -p "$OUTPUT_DIR" "$CACHE_DIR"
OUTPUT_DIR="$(cd "$OUTPUT_DIR" && pwd)"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# 从 Release 拉取一个资产（带 sha256 校验，缓存）。/ Fetch one release asset (sha256 verified, cached).
fetch_release_asset() {
  local tag="$1" asset="$2" dest="$3" required="${4:-true}"
  if [[ -f "$dest" && -f "${dest}.sha256" ]] && stx_verify_sha256 "$dest" "${dest}.sha256" 2>/dev/null; then
    return 0
  fi
  if ! stx_download_release_asset "$tag" "$asset" "$dest" 2>/dev/null; then
    rm -f "$dest"
    if [[ "$required" == "true" ]]; then
      echo "[ERROR] cannot download $asset from release $tag" >&2
      return 1
    fi
    return 1
  fi
  if stx_download_release_asset "$tag" "${asset}.sha256" "${dest}.sha256" 2>/dev/null; then
    stx_verify_sha256 "$dest" "${dest}.sha256" || { rm -f "$dest" "${dest}.sha256"; return 1; }
  else
    rm -f "${dest}.sha256"
  fi
}

# 找到 STX 自身产物（本地目录或 Release）。/ Locate STX's own artifacts (local dir or release).
SRC_DIR=""
if [[ -n "$FROM_RELEASE" ]]; then
  SRC_DIR="$CACHE_DIR/release-$FROM_RELEASE"
  mkdir -p "$SRC_DIR"
  echo "[INFO] downloading STX artifacts from release $FROM_RELEASE"
  for v in "${VARIANT_LIST[@]}"; do
    a="$(stx_bundle_variant_arch "$v")"
    [[ -f "$SRC_DIR/.done-$a" ]] && continue
    fetch_release_asset "$FROM_RELEASE" "stx-linux-$a" "$SRC_DIR/stx-linux-$a"
    fetch_release_asset "$FROM_RELEASE" "stx-agent-linux-$a" "$SRC_DIR/stx-agent-linux-$a"
    fetch_release_asset "$FROM_RELEASE" "frontend-standalone-${FROM_RELEASE}-linux-$a.tar.gz" \
      "$SRC_DIR/frontend-standalone-${FROM_RELEASE}-linux-$a.tar.gz" false \
      || fetch_release_asset "$FROM_RELEASE" "frontend-standalone-linux-$a.tar.gz" "$SRC_DIR/frontend-standalone-linux-$a.tar.gz"
    touch "$SRC_DIR/.done-$a"
  done
  for epoch in v2 v3; do
    fetch_release_asset "$FROM_RELEASE" "stx-java-proxy-${epoch}.jar" "$SRC_DIR/stx-java-proxy-${epoch}.jar"
  done
  fetch_release_asset "$FROM_RELEASE" "stx-java-proxy.sh" "$SRC_DIR/stx-java-proxy.sh" false || true
else
  [[ -d "$FROM_DIR" ]] || { echo "[ERROR] --from-dir $FROM_DIR not found (run install/package-release.sh first)" >&2; exit 1; }
  SRC_DIR="$(cd "$FROM_DIR" && pwd)"
fi

find_frontend() {
  local a="$1" f
  for f in "$SRC_DIR/frontend-standalone-${VERSION}-linux-$a.tar.gz" "$SRC_DIR/frontend-standalone-linux-$a.tar.gz"; do
    [[ -f "$f" ]] && { echo "$f"; return 0; }
  done
  f="$(find "$SRC_DIR" -maxdepth 1 -name "frontend-standalone-*-linux-$a.tar.gz" -print 2>/dev/null | sort | tail -n1)"
  [[ -n "$f" ]] && { echo "$f"; return 0; }
  return 1
}

# 依赖：Node 与三件套。/ Dependencies: Node runtime and observability stack.
node_asset_for() {
  case "$1" in
    amd64) echo "node-22-official-linux-amd64.tar.gz" ;;
    arm64) echo "node-22-official-linux-arm64.tar.gz" ;;
    centos7) echo "node-18-glibc217-linux-amd64.tar.gz" ;;
  esac
}

DEPS_MANIFEST=""
obs_asset_for() {
  local a="$1" name=""
  if [[ -n "$DEPS_DIR" ]]; then
    name="$(find "$DEPS_DIR" -maxdepth 1 -name "observability-*-linux-$a.tar.gz" -print 2>/dev/null | sort | tail -n1)"
    [[ -n "$name" ]] && { basename "$name"; return 0; }
  fi
  if [[ -z "$DEPS_MANIFEST" ]]; then
    DEPS_MANIFEST="$CACHE_DIR/deps-MANIFEST.json"
    stx_download_release_asset "$DEPS_TAG" "MANIFEST.json" "$DEPS_MANIFEST" 2>/dev/null || : >"$DEPS_MANIFEST"
  fi
  name="$(stx_manifest_obs_asset "$a" <"$DEPS_MANIFEST" || true)"
  echo "${name:-observability-prom3.9.1-am0.31.1-gf12.3.3-linux-$a.tar.gz}"
}

dep_file() {
  local asset="$1"
  if [[ -n "$DEPS_DIR" && -f "$DEPS_DIR/$asset" ]]; then
    if [[ -f "$DEPS_DIR/$asset.sha256" ]]; then
      stx_verify_sha256 "$DEPS_DIR/$asset" "$DEPS_DIR/$asset.sha256" >&2 || return 1
    fi
    echo "$DEPS_DIR/$asset"
    return 0
  fi
  if [[ -n "$DEPS_DIR" ]]; then
    echo "[ERROR] $asset not found in --deps-dir $DEPS_DIR" >&2
    return 1
  fi
  echo "[INFO] fetching $asset from release tag $DEPS_TAG" >&2
  fetch_release_asset "$DEPS_TAG" "$asset" "$CACHE_DIR/$asset" >&2 || return 1
  echo "$CACHE_DIR/$asset"
}

# Go 二进制必须静态链接（CentOS 7 glibc 2.17）。/ Go binaries must be static (CentOS 7 glibc 2.17).
assert_static() {
  local bin="$1"
  if command -v readelf >/dev/null 2>&1; then
    if readelf -l "$bin" 2>/dev/null | grep -q 'INTERP'; then
      echo "[ERROR] $bin is dynamically linked; build with CGO_ENABLED=0" >&2
      return 1
    fi
  elif grep -qa 'ld-linux' "$bin"; then
    echo "[ERROR] $bin looks dynamically linked (mentions ld-linux); build with CGO_ENABLED=0" >&2
    return 1
  fi
}

copy_or_link() { ln "$1" "$2" 2>/dev/null || cp "$1" "$2"; }

build_variant() {
  local variant="$1" arch plat name dir stx_bin agent_bin fe node_tar obs_tar node_asset obs_asset node_ver tarball size
  arch="$(stx_bundle_variant_arch "$variant")"
  plat="$(stx_bundle_platform "$variant")"
  name="stx-${VERSION}-${plat}-offline"
  dir="$OUTPUT_DIR/$name"
  tarball="$OUTPUT_DIR/${name}.tar.gz"
  echo
  echo "==> building $name"

  stx_bin="$SRC_DIR/stx-linux-$arch"
  agent_bin="$SRC_DIR/stx-agent-linux-$arch"
  [[ -f "$stx_bin" ]] || { echo "[ERROR] missing $stx_bin" >&2; return 1; }
  [[ -f "$agent_bin" ]] || { echo "[ERROR] missing $agent_bin" >&2; return 1; }
  fe="$(find_frontend "$arch")" || { echo "[ERROR] missing frontend-standalone-*-linux-$arch.tar.gz in $SRC_DIR" >&2; return 1; }
  for epoch in v2 v3; do
    [[ -f "$SRC_DIR/stx-java-proxy-${epoch}.jar" ]] || { echo "[ERROR] missing $SRC_DIR/stx-java-proxy-${epoch}.jar" >&2; return 1; }
  done
  assert_static "$stx_bin"
  assert_static "$agent_bin"

  node_asset="$(node_asset_for "$variant")"
  node_tar="$(dep_file "$node_asset")" || { echo "[ERROR] node runtime $node_asset unavailable" >&2; return 1; }
  obs_tar=""
  if [[ "$WITH_OBS" == "true" ]]; then
    obs_asset="$(obs_asset_for "$arch")"
    obs_tar="$(dep_file "$obs_asset")" || { echo "[ERROR] observability package $obs_asset unavailable" >&2; return 1; }
  fi
  node_ver="$( (tar -tzf "$node_tar" 2>/dev/null || true) | awk -F/ 'NR==1 {print $1; exit}')"
  node_ver="$(echo "$node_ver" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -n1 || true)"

  rm -rf "$dir"
  mkdir -p "$dir/packages" "$dir/bin/lib" "$dir/scripts"
  copy_or_link "$stx_bin" "$dir/packages/stx-linux-$arch"
  copy_or_link "$agent_bin" "$dir/packages/stx-agent-linux-$arch"
  copy_or_link "$fe" "$dir/packages/frontend-standalone-${VERSION}-linux-$arch.tar.gz"
  copy_or_link "$SRC_DIR/stx-java-proxy-v2.jar" "$dir/packages/stx-java-proxy-v2.jar"
  copy_or_link "$SRC_DIR/stx-java-proxy-v3.jar" "$dir/packages/stx-java-proxy-v3.jar"
  copy_or_link "$node_tar" "$dir/packages/$node_asset"
  [[ -n "$obs_tar" ]] && copy_or_link "$obs_tar" "$dir/packages/$(basename "$obs_tar")"
  chmod +x "$dir/packages/stx-linux-$arch" "$dir/packages/stx-agent-linux-$arch" 2>/dev/null || true

  if [[ -f "$SRC_DIR/stx-java-proxy.sh" ]]; then
    cp "$SRC_DIR/stx-java-proxy.sh" "$dir/scripts/stx-java-proxy.sh"
  else
    cp "$ROOT_DIR/scripts/stx-java-proxy.sh" "$dir/scripts/stx-java-proxy.sh"
  fi
  cp "$ROOT_DIR/install/install.sh" "$ROOT_DIR/install/install-core.sh" "$ROOT_DIR/install/download-lib.sh" "$dir/"
  cp "$ROOT_DIR/install/bin/start.sh" "$ROOT_DIR/install/bin/stop.sh" "$ROOT_DIR/install/bin/status.sh" "$dir/bin/"
  cp "$ROOT_DIR/install/bin/lib/observability.sh" "$dir/bin/lib/"
  cp "$ROOT_DIR/config.example.yaml" "$dir/config.example.yaml"
  chmod +x "$dir/install.sh" "$dir/bin/"*.sh "$dir/scripts/"*.sh

  local min_glibc="2.28" node_variant="official"
  if [[ "$variant" == "centos7" ]]; then min_glibc="2.17"; node_variant="glibc217"; fi
  cat >"$dir/MANIFEST.json" <<JSON
{
  "name": "$name",
  "version": "$VERSION",
  "git_commit": "$GIT_COMMIT",
  "variant": "$variant",
  "arch": "$arch",
  "min_glibc": "$min_glibc",
  "node_variant": "$node_variant",
  "node_version": "${node_ver:-unknown}",
  "node_package": "$node_asset",
  "with_node": true,
  "with_observability": $WITH_OBS,
  "with_agent": true,
  "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
JSON

  (cd "$dir/packages" && for f in *; do echo "$(sha256_of "$f")  $f"; done >"../SHA256SUMS")

  cat >"$dir/README-OFFLINE.md" <<MD
# STX 离线安装包 / Offline Bundle

- 版本 / version: \`$VERSION\` (commit \`$GIT_COMMIT\`)
- 平台 / platform: \`$plat\`（glibc ≥ $min_glibc）
- Node: \`${node_ver:-unknown}\` ($node_variant)，已内置 / bundled
- 三件套 / observability: $WITH_OBS

## 安装 / Install

\`\`\`bash
tar -xzf ${name}.tar.gz
cd ${name}/
sudo ./install.sh                 # 交互：管理员密码、端口（回车保持默认）
sudo ./install.sh -y              # 非交互，全部默认
\`\`\`

常用参数 / options: \`--install-dir /opt/stx\` \`--admin-password <pwd>\` \`--frontend-port 17880\`
\`--http-port 17800\` \`--grpc-port 17890\` \`--without-observability\` \`--no-start\` \`--no-systemd\`

安装过程不访问外网。SeaTunnel 节点需要自行安装 JDK。
Installation never touches the network. SeaTunnel nodes need a JDK installed separately.
MD

  echo "[INFO] packing $tarball"
  tar -C "$OUTPUT_DIR" -czf "$tarball" "$name"
  size="$(wc -c <"$tarball" | tr -d ' ')"
  if (( size > MAX_ASSET_BYTES )); then
    echo "[ERROR] $tarball is $size bytes, over GitHub's 2 GiB per-asset limit" >&2
    return 1
  fi
  local sum
  sum="$(sha256_of "$tarball")"
  echo "$sum  ${name}.tar.gz" >"${tarball}.sha256"
  if [[ "$WITH_ALIAS" == "true" ]]; then
    local alias
    alias="$(stx_bundle_alias_name "$variant")"
    rm -f "$OUTPUT_DIR/$alias"
    copy_or_link "$tarball" "$OUTPUT_DIR/$alias"
    echo "$sum  $alias" >"$OUTPUT_DIR/${alias}.sha256"
  fi
  [[ "$KEEP_DIR" == "true" ]] || rm -rf "$dir"
  echo "[OK] $tarball ($((size / 1024 / 1024)) MB, sha256 $sum, node ${node_ver:-unknown})"
}

for v in "${VARIANT_LIST[@]}"; do
  build_variant "$v"
done
echo
echo "[OK] offline bundles in $OUTPUT_DIR"
