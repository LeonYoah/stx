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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/dist/releases}"
CACHE_DIR="${CACHE_DIR:-$ROOT_DIR/.cache/release}"

ARCH_OPTION="amd64"
BUNDLE_OBSERVABILITY="without" # with | without | both — legacy layout only
NODE_MAJOR="${NODE_MAJOR:-18}" # 18 | 22
NODE_VARIANT="${NODE_VARIANT:-official}" # official | glibc217
BUILD_FRONTEND=false
APP_VERSION="${APP_VERSION:-}"
LAYOUT="split" # split | legacy
EMIT_DEPS=false

PROMETHEUS_VERSION="${PROMETHEUS_VERSION:-3.9.1}"
ALERTMANAGER_VERSION="${ALERTMANAGER_VERSION:-0.31.1}"
GRAFANA_VERSION="${GRAFANA_VERSION:-12.3.3}"
CAPABILITY_PROXY_DEFAULT_VERSION="${CAPABILITY_PROXY_DEFAULT_VERSION:-2.3.13}"

# 打印发布打包脚本的用法。/ Print release packaging usage.
usage() {
  cat <<'EOF'
Usage: scripts/package-release.sh [options]

Options:
  --arch <amd64|arm64|all>          Target CPU arch for stx binary (default: amd64)
  --layout <split|legacy>           split=bare binaries+frontend tarball (default);
                                    legacy=old mega tar with optional bundled observability
  --emit-deps                       Also emit reusable node/observability deps tarballs (split layout)
  --bundle-observability <with|without|both>
                                     Legacy layout only (default: without)
  --node-major <18|22>               Node major for bundled/emitted runtime (default: 18)
  --node-variant <official|glibc217> Node binary source variant (default: official)
  --build-frontend                   Build frontend standalone before packaging
  --version <string>                 Package version label (default: git describe --tags --always --dirty)
  --output-dir <path>                Output directory (default: dist/releases)
  --cache-dir <path>                 Download/build cache dir (default: .cache/release)
  --help                             Show this help

Examples:
  scripts/package-release.sh --arch all --layout split --build-frontend --emit-deps
  scripts/package-release.sh --arch amd64 --layout legacy --bundle-observability both --build-frontend
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)
      ARCH_OPTION="${2:-}"
      shift 2
      ;;
    --bundle-observability)
      BUNDLE_OBSERVABILITY="${2:-}"
      shift 2
      ;;
    --layout)
      LAYOUT="${2:-}"
      shift 2
      ;;
    --emit-deps)
      EMIT_DEPS=true
      shift
      ;;
    --node-major)
      NODE_MAJOR="${2:-}"
      shift 2
      ;;
    --node-variant)
      NODE_VARIANT="${2:-}"
      shift 2
      ;;
    --build-frontend)
      BUILD_FRONTEND=true
      shift
      ;;
    --version)
      APP_VERSION="${2:-}"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --cache-dir)
      CACHE_DIR="${2:-}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "required command not found: $cmd"
    exit 1
  fi
}

for cmd in go tar curl python3 mvn; do
  require_cmd "$cmd"
done

case "$NODE_MAJOR" in
  18|22) ;;
  *)
    echo "invalid --node-major: $NODE_MAJOR (supported: 18, 22)"
    exit 1
    ;;
esac

case "$NODE_VARIANT" in
  official|glibc217) ;;
  *)
    echo "invalid --node-variant: $NODE_VARIANT (supported: official, glibc217)"
    exit 1
    ;;
esac

if [[ "$NODE_VARIANT" == "glibc217" ]]; then
  for arch in "${ARCHES[@]}"; do
    if [[ "$arch" != "amd64" ]]; then
      echo "glibc217 node builds are x64-only; drop --arch arm64/all or use official node"
      exit 1
    fi
  done
fi

case "$ARCH_OPTION" in
  amd64) ARCHES=("amd64") ;;
  arm64) ARCHES=("arm64") ;;
  all) ARCHES=("amd64" "arm64") ;;
  *)
    echo "invalid --arch: $ARCH_OPTION (supported: amd64, arm64, all)"
    exit 1
    ;;
esac

case "$LAYOUT" in
  split|legacy) ;;
  *)
    echo "invalid --layout: $LAYOUT (supported: split, legacy)"
    exit 1
    ;;
esac

case "$BUNDLE_OBSERVABILITY" in
  with) OBS_VARIANTS=("with") ;;
  without) OBS_VARIANTS=("without") ;;
  both) OBS_VARIANTS=("with" "without") ;;
  *)
    echo "invalid --bundle-observability: $BUNDLE_OBSERVABILITY (supported: with, without, both)"
    exit 1
    ;;
esac

if [[ "$LAYOUT" == "split" ]]; then
  # Split layout does not embed observability into the main package.
  # split 布局不再把可观测性打进主包。
  OBS_VARIANTS=("without")
fi

if [[ -z "$APP_VERSION" ]]; then
  APP_VERSION="$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || date +%Y%m%d%H%M%S)"
fi
APP_VERSION_SAFE="$(echo "$APP_VERSION" | tr '/ ' '__')"

DOWNLOAD_DIR="$CACHE_DIR/downloads"
BUILD_DIR="$CACHE_DIR/build"
STAGE_DIR="$CACHE_DIR/stage"

mkdir -p "$OUTPUT_DIR" "$DOWNLOAD_DIR" "$BUILD_DIR" "$STAGE_DIR"

node_arch() {
  case "$1" in
    amd64) echo "x64" ;;
    arm64) echo "arm64" ;;
    *)
      echo "unsupported arch: $1" >&2
      exit 1
      ;;
  esac
}

resolve_host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "unsupported host arch: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

download_if_missing() {
  local url="$1"
  local output="$2"
  if [[ -f "$output" ]]; then
    return
  fi
  echo "downloading: $url" >&2
  curl -fL --retry 3 --retry-delay 1 "$url" -o "$output"
}

resolve_latest_node_version() {
  python3 - "$NODE_MAJOR" "$NODE_VARIANT" <<'PY'
import json
import sys
import urllib.request

major = sys.argv[1]
variant = sys.argv[2]
index_url = "https://nodejs.org/dist/index.json"
if variant == "glibc217":
    index_url = "https://unofficial-builds.nodejs.org/download/release/index.json"

data = json.load(urllib.request.urlopen(index_url, timeout=15))
for item in data:
    version = item["version"].lstrip("v")
    if version.split(".")[0] == major:
        print(version)
        break
else:
    raise SystemExit(
        f"cannot resolve latest node version for major={major}, variant={variant}"
    )
PY
}

NODE_VERSION="$(resolve_latest_node_version)"
echo "resolved Node runtime version: v$NODE_VERSION (major=$NODE_MAJOR, variant=$NODE_VARIANT)"

prepare_node_runtime() {
  local arch="$1"
  local node_arch_value
  node_arch_value="$(node_arch "$arch")"

  local cache_runtime_dir="$BUILD_DIR/node-runtime/v${NODE_VERSION}-${node_arch_value}-${NODE_VARIANT}"
  if [[ -x "$cache_runtime_dir/bin/node" ]]; then
    echo "$cache_runtime_dir"
    return
  fi

  mkdir -p "$cache_runtime_dir"
  local archive_name
  local url
  if [[ "$NODE_VARIANT" == "official" ]]; then
    archive_name="node-v${NODE_VERSION}-linux-${node_arch_value}.tar.xz"
    url="https://nodejs.org/dist/v${NODE_VERSION}/${archive_name}"
  else
    archive_name="node-v${NODE_VERSION}-linux-${node_arch_value}-glibc-217.tar.xz"
    url="https://unofficial-builds.nodejs.org/download/release/v${NODE_VERSION}/${archive_name}"
  fi

  local archive_path="$DOWNLOAD_DIR/$archive_name"
  download_if_missing "$url" "$archive_path"

  local tmp_extract="$BUILD_DIR/tmp-node-${node_arch_value}-${NODE_VARIANT}"
  rm -rf "$tmp_extract"
  mkdir -p "$tmp_extract"
  tar -xJf "$archive_path" -C "$tmp_extract"
  local extracted
  extracted="$(find "$tmp_extract" -mindepth 1 -maxdepth 1 -type d | head -n1)"
  if [[ -z "$extracted" ]]; then
    echo "failed to extract node runtime: $archive_path"
    exit 1
  fi
  rm -rf "$cache_runtime_dir"
  mv "$extracted" "$cache_runtime_dir"
  rm -rf "$tmp_extract"

  if [[ ! -x "$cache_runtime_dir/bin/node" ]]; then
    echo "node binary missing after extraction: $cache_runtime_dir/bin/node"
    exit 1
  fi
  echo "$cache_runtime_dir"
}

build_frontend_standalone() {
  local host_arch
  host_arch="$(resolve_host_arch)"
  local runtime_dir
  runtime_dir="$(prepare_node_runtime "$host_arch")"

  echo "building frontend standalone with Node v$NODE_VERSION ..."
  (
    cd "$ROOT_DIR/frontend"
    export PATH="$runtime_dir/bin:$PATH"
    export COREPACK_ENABLE_DOWNLOAD_PROMPT=0
    corepack enable >/dev/null 2>&1 || true
    corepack prepare pnpm@10.10.0 --activate >/dev/null 2>&1 || true
    pnpm install --no-frozen-lockfile
    pnpm run pack:standalone
  )
}

FRONTEND_DIST="$ROOT_DIR/frontend/dist-standalone"
if [[ "$BUILD_FRONTEND" == "true" ]]; then
  build_frontend_standalone
fi
if [[ ! -f "$FRONTEND_DIST/server.js" ]]; then
  echo "frontend standalone not found: $FRONTEND_DIST/server.js"
  echo "run with --build-frontend or build frontend manually."
  exit 1
fi

# 构建指定架构的 STX 后端二进制。/ Build the STX backend binary for the requested architecture.
build_stx_binary() {
  local arch="$1"
  local out="$BUILD_DIR/stx-linux-${arch}"
  echo "building stx for linux/$arch ..."
  (
    cd "$ROOT_DIR"
    GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -o "$out" .
  )
}

# 构建发布包所需的双架构 STX Agent。/ Build both STX Agent architectures required by release packages.
build_agent_binaries() {
  local out_amd64="$BUILD_DIR/stx-agent-linux-amd64"
  local out_arm64="$BUILD_DIR/stx-agent-linux-arm64"

  echo "building stx-agent for linux/amd64 ..."
  (
    cd "$ROOT_DIR/agent"
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$out_amd64" ./cmd
  )
  echo "building stx-agent for linux/arm64 ..."
  (
    cd "$ROOT_DIR/agent"
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out_arm64" ./cmd
  )
}

# 构建并暂存 STX Java Proxy 薄 JAR。/ Build and stage the STX Java Proxy thin JAR.
build_stx_java_proxy_jar() {
  local proxy_project_dir="$ROOT_DIR/tools/stx-java-proxy"
  local proxy_target_dir="$proxy_project_dir/target"
  local proxy_out="$BUILD_DIR/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar"

  echo "building stx-java-proxy thin jar ..."
  (
    cd "$ROOT_DIR"
    mvn -q -f "$proxy_project_dir/pom.xml" -DskipTests package
  )

  local built_proxy_jar
  built_proxy_jar="$(find "$proxy_target_dir" -maxdepth 1 -type f -name 'stx-java-proxy-*.jar' ! -name '*-bin.jar' | sort | head -n1)"
  if [[ -z "$built_proxy_jar" || ! -f "$built_proxy_jar" ]]; then
    echo "failed to locate built stx-java-proxy thin jar under $proxy_target_dir"
    exit 1
  fi

  cp "$built_proxy_jar" "$proxy_out"
  CAPABILITY_PROXY_JAR="$proxy_out"
}

# 将可用的 STX Java Proxy JAR 放入发布目录。/ Stage available STX Java Proxy JARs in the release directory.
stage_stx_java_proxy_jars() {
  local destination_dir="$1"
  mkdir -p "$destination_dir"

  if [[ -d "$ROOT_DIR/lib" ]]; then
    find "$ROOT_DIR/lib" -maxdepth 1 -type f -name 'stx-java-proxy-*.jar' -print0 | while IFS= read -r -d '' jar_path; do
      cp "$jar_path" "$destination_dir/"
    done
  fi

  cp "$CAPABILITY_PROXY_JAR" "$destination_dir/$(basename "$CAPABILITY_PROXY_JAR")"
}

build_agent_binaries
build_stx_java_proxy_jar
for arch in "${ARCHES[@]}"; do
  build_stx_binary "$arch"
done

prepare_observability_stack() {
  local arch="$1"
  local deps_dir="$2"
  mkdir -p "$deps_dir"

  local prom_archive="prometheus-${PROMETHEUS_VERSION}.linux-${arch}.tar.gz"
  local prom_url="https://github.com/prometheus/prometheus/releases/download/v${PROMETHEUS_VERSION}/${prom_archive}"
  local prom_path="$DOWNLOAD_DIR/$prom_archive"
  download_if_missing "$prom_url" "$prom_path"

  local alert_archive="alertmanager-${ALERTMANAGER_VERSION}.linux-${arch}.tar.gz"
  local alert_url="https://github.com/prometheus/alertmanager/releases/download/v${ALERTMANAGER_VERSION}/${alert_archive}"
  local alert_path="$DOWNLOAD_DIR/$alert_archive"
  download_if_missing "$alert_url" "$alert_path"

  local grafana_archive="grafana-${GRAFANA_VERSION}.linux-${arch}.tar.gz"
  local grafana_url="https://dl.grafana.com/oss/release/${grafana_archive}"
  local grafana_path="$DOWNLOAD_DIR/$grafana_archive"
  download_if_missing "$grafana_url" "$grafana_path"

  local tmp="$BUILD_DIR/tmp-observability-${arch}"
  rm -rf "$tmp"
  mkdir -p "$tmp"

  tar -xzf "$prom_path" -C "$tmp"
  local prom_dir
  prom_dir="$(find "$tmp" -maxdepth 1 -type d -name "prometheus-*" | head -n1)"
  rm -rf "$deps_dir/prometheus"
  mkdir -p "$deps_dir/prometheus"
  cp -a "$prom_dir"/. "$deps_dir/prometheus/"
  rm -rf "$tmp"/*

  tar -xzf "$alert_path" -C "$tmp"
  local alert_dir
  alert_dir="$(find "$tmp" -maxdepth 1 -type d -name "alertmanager-*" | head -n1)"
  rm -rf "$deps_dir/alertmanager"
  mkdir -p "$deps_dir/alertmanager"
  cp -a "$alert_dir"/. "$deps_dir/alertmanager/"
  rm -rf "$tmp"/*

  tar -xzf "$grafana_path" -C "$tmp"
  local grafana_dir
  grafana_dir="$(find "$tmp" -maxdepth 1 -type d -name "grafana-*" | head -n1)"
  rm -rf "$deps_dir/grafana"
  mkdir -p "$deps_dir/grafana"
  cp -a "$grafana_dir"/. "$deps_dir/grafana/"
  rm -rf "$tmp"

  cp "$ROOT_DIR/deps/init-observability-defaults.sh" "$deps_dir/"
  cp "$ROOT_DIR/deps/start-observability.sh" "$deps_dir/"
  cp "$ROOT_DIR/deps/stop-observability.sh" "$deps_dir/"
  cp "$ROOT_DIR/deps/status-observability.sh" "$deps_dir/"
  chmod +x \
    "$deps_dir/init-observability-defaults.sh" \
    "$deps_dir/start-observability.sh" \
    "$deps_dir/stop-observability.sh" \
    "$deps_dir/status-observability.sh"
}

# 写入文件 sha256 sidecar。/ Write sha256 sidecar for a file.
write_sha256() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}' >"${file}.sha256"
  else
    shasum -a 256 "$file" | awk '{print $1}' >"${file}.sha256"
  fi
}

# Emit reusable deps tarballs (node + optional observability). / 产出可复用 deps 包。
emit_deps_packages() {
  local arch="$1"
  local runtime_dir
  runtime_dir="$(prepare_node_runtime "$arch")"
  local node_name="node-${NODE_MAJOR}-${NODE_VARIANT}-linux-${arch}.tar.gz"
  local node_out="$OUTPUT_DIR/$node_name"
  echo "creating deps node package: $node_out"
  tar -C "$(dirname "$runtime_dir")" -czf "$node_out" "$(basename "$runtime_dir")"
  # Normalize inner folder name to node/ when extracted by installers that expect runtime/node.
  # 安装器期望 runtime/node；这里保持解压后可再命名。
  write_sha256 "$node_out"

  local obs_name="observability-prom${PROMETHEUS_VERSION}-am${ALERTMANAGER_VERSION}-gf${GRAFANA_VERSION}-linux-${arch}.tar.gz"
  local obs_out="$OUTPUT_DIR/$obs_name"
  local obs_stage="$STAGE_DIR/deps-${arch}"
  rm -rf "$obs_stage"
  prepare_observability_stack "$arch" "$obs_stage"
  echo "creating deps observability package: $obs_out"
  tar -C "$obs_stage" -czf "$obs_out" .
  write_sha256 "$obs_out"

  cat >"$OUTPUT_DIR/MANIFEST.json" <<EOF
{
  "node_major": "$NODE_MAJOR",
  "node_variant": "$NODE_VARIANT",
  "node_version": "$NODE_VERSION",
  "prometheus": "$PROMETHEUS_VERSION",
  "alertmanager": "$ALERTMANAGER_VERSION",
  "grafana": "$GRAFANA_VERSION",
  "observability": {
    "amd64": "observability-prom${PROMETHEUS_VERSION}-am${ALERTMANAGER_VERSION}-gf${GRAFANA_VERSION}-linux-amd64.tar.gz",
    "arm64": "observability-prom${PROMETHEUS_VERSION}-am${ALERTMANAGER_VERSION}-gf${GRAFANA_VERSION}-linux-arm64.tar.gz"
  },
  "build_time": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
}

if [[ "$LAYOUT" == "split" ]]; then
  for arch in "${ARCHES[@]}"; do
    echo "staging split assets for linux/$arch"

    cp "$BUILD_DIR/stx-linux-${arch}" "$OUTPUT_DIR/stx-linux-${arch}"
    chmod +x "$OUTPUT_DIR/stx-linux-${arch}"
    write_sha256 "$OUTPUT_DIR/stx-linux-${arch}"

    cp "$BUILD_DIR/stx-agent-linux-${arch}" "$OUTPUT_DIR/stx-agent-linux-${arch}"
    chmod +x "$OUTPUT_DIR/stx-agent-linux-${arch}"
    write_sha256 "$OUTPUT_DIR/stx-agent-linux-${arch}"

    frontend_name="frontend-standalone-${APP_VERSION_SAFE}-linux-${arch}.tar.gz"
    echo "creating $frontend_name"
    tar -C "$FRONTEND_DIST" -czf "$OUTPUT_DIR/$frontend_name" .
    write_sha256 "$OUTPUT_DIR/$frontend_name"

    if [[ "$EMIT_DEPS" == "true" ]]; then
      emit_deps_packages "$arch"
    fi
  done

  # Also publish installer helpers for curl|bash consumers. / 同步发布安装辅助脚本供 curl|bash 使用。
  cp "$ROOT_DIR/scripts/install-online.sh" "$OUTPUT_DIR/install-online.sh"
  cp "$ROOT_DIR/scripts/download-bundle.sh" "$OUTPUT_DIR/download-bundle.sh"
  chmod +x "$OUTPUT_DIR/install-online.sh" "$OUTPUT_DIR/download-bundle.sh"

  (
    cd "$OUTPUT_DIR"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum stx-linux-* stx-agent-linux-* frontend-standalone-*.tar.gz 2>/dev/null >SHA256SUMS || true
    else
      shasum -a 256 stx-linux-* stx-agent-linux-* frontend-standalone-*.tar.gz 2>/dev/null >SHA256SUMS || true
    fi
  )

  echo
  echo "all done (layout=split)."
  echo "output dir: $OUTPUT_DIR"
  exit 0
fi

for arch in "${ARCHES[@]}"; do
  for obs in "${OBS_VARIANTS[@]}"; do
    pkg_name="stx-${APP_VERSION_SAFE}-linux-${arch}-node${NODE_MAJOR}-${NODE_VARIANT}-${obs}-observability"
    pkg_dir="$STAGE_DIR/$pkg_name"
    rm -rf "$pkg_dir"
    mkdir -p "$pkg_dir"

    echo "staging legacy package: $pkg_name"

    cp "$BUILD_DIR/stx-linux-${arch}" "$pkg_dir/stx"
    chmod +x "$pkg_dir/stx"

    cp "$ROOT_DIR/README.md" "$pkg_dir/"
    cp "$ROOT_DIR/LICENSE" "$pkg_dir/"
    cp "$ROOT_DIR/NOTICE" "$pkg_dir/"
    cp "$ROOT_DIR/config.example.yaml" "$pkg_dir/config.example.yaml"
    cp "$ROOT_DIR/support-files/release/install.sh" "$pkg_dir/install.sh"
    cp "$ROOT_DIR/support-files/release/download-lib.sh" "$pkg_dir/download-lib.sh"
    cp "$ROOT_DIR/support-files/release/install-core.sh" "$pkg_dir/install-core.sh"
    mkdir -p "$pkg_dir/lib"

    mkdir -p "$pkg_dir/lib/agent" "$pkg_dir/scripts"
    cp "$BUILD_DIR/stx-agent-linux-amd64" "$pkg_dir/lib/agent/stx-agent-linux-amd64"
    cp "$BUILD_DIR/stx-agent-linux-arm64" "$pkg_dir/lib/agent/stx-agent-linux-arm64"
    stage_stx_java_proxy_jars "$pkg_dir/lib"
    cp "$ROOT_DIR/scripts/stx-java-proxy.sh" "$pkg_dir/scripts/stx-java-proxy.sh"
    chmod +x "$pkg_dir/lib/agent/stx-agent-linux-amd64" "$pkg_dir/lib/agent/stx-agent-linux-arm64"
    chmod +x "$pkg_dir/scripts/stx-java-proxy.sh"

    mkdir -p "$pkg_dir/frontend"
    cp -a "$FRONTEND_DIST"/. "$pkg_dir/frontend/"

    local_node_runtime="$(prepare_node_runtime "$arch")"
    mkdir -p "$pkg_dir/runtime"
    cp -a "$local_node_runtime" "$pkg_dir/runtime/node"

    mkdir -p "$pkg_dir/bin" "$pkg_dir/logs" "$pkg_dir/run"
    cp "$ROOT_DIR/support-files/release/start.sh" "$pkg_dir/bin/start.sh"
    cp "$ROOT_DIR/support-files/release/stop.sh" "$pkg_dir/bin/stop.sh"
    cp "$ROOT_DIR/support-files/release/status.sh" "$pkg_dir/bin/status.sh"
    chmod +x "$pkg_dir/install.sh" "$pkg_dir/bin/start.sh" "$pkg_dir/bin/stop.sh" "$pkg_dir/bin/status.sh"

    if [[ "$obs" == "with" ]]; then
      prepare_observability_stack "$arch" "$pkg_dir/deps"
    fi

    cat >"$pkg_dir/BUILD_INFO" <<EOF
version=$APP_VERSION_SAFE
arch=$arch
node_major=$NODE_MAJOR
node_version=$NODE_VERSION
node_variant=$NODE_VARIANT
observability=$obs
layout=legacy
build_time=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

    tarball="$OUTPUT_DIR/${pkg_name}.tar.gz"
    tar -C "$STAGE_DIR" -czf "$tarball" "$pkg_name"
    echo "created: $tarball"
  done
done

echo
echo "all done (layout=legacy)."
echo "output dir: $OUTPUT_DIR"
