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
DEPS_ONLY=false
EMIT_OBSERVABILITY=true
EMIT_NODE=true

PROMETHEUS_VERSION="${PROMETHEUS_VERSION:-3.9.1}"
ALERTMANAGER_VERSION="${ALERTMANAGER_VERSION:-0.31.1}"
GRAFANA_VERSION="${GRAFANA_VERSION:-12.3.3}"
CAPABILITY_PROXY_DEFAULT_VERSION="${CAPABILITY_PROXY_DEFAULT_VERSION:-v2}"

# 打印发布打包脚本的用法。/ Print release packaging usage.
usage() {
  cat <<'EOF'
Usage: install/package-release.sh [options]

Options:
  --arch <amd64|arm64|all>          Target CPU arch for stx binary (default: amd64)
  --layout <split|legacy>           split=bare binaries+frontend tarball (default);
                                    legacy=old mega tar with optional bundled observability
  --emit-deps                       Also emit reusable node/observability deps tarballs (split layout)
  --deps-only                       Emit only node/observability deps; skip stx, agent, and frontend
  --no-observability                With --emit-deps/--deps-only, skip the observability tarball
  --no-node                         With --deps-only, skip the Node runtime tarball (observability only)
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
  install/package-release.sh --arch all --layout split --build-frontend
  install/package-release.sh --deps-only --arch all --node-major 22 --node-variant official
  install/package-release.sh --deps-only --no-node --arch all
  install/package-release.sh --arch amd64 --layout legacy --bundle-observability both --build-frontend
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
    --deps-only)
      # 只打可复用 deps，不编控制面。/ Emit reusable deps only; do not build the control plane.
      DEPS_ONLY=true
      EMIT_DEPS=true
      shift
      ;;
    --no-observability)
      EMIT_OBSERVABILITY=false
      shift
      ;;
    --no-node)
      # 只重打三件套，不碰 Node 包（release-deps components=observability）。
      # Rebuild observability only, leave Node packages alone (release-deps components=observability).
      EMIT_NODE=false
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

required_cmds=(tar curl python3)
if [[ "$DEPS_ONLY" != "true" ]]; then
  required_cmds+=(go mvn)
fi
for cmd in "${required_cmds[@]}"; do
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

case "$ARCH_OPTION" in
  amd64) ARCHES=("amd64") ;;
  arm64) ARCHES=("arm64") ;;
  all) ARCHES=("amd64" "arm64") ;;
  *)
    echo "invalid --arch: $ARCH_OPTION (supported: amd64, arm64, all)"
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

if [[ "$EMIT_NODE" != "true" ]]; then
  if [[ "$DEPS_ONLY" != "true" ]]; then
    echo "--no-node requires --deps-only (control plane packaging always needs Node)"
    exit 1
  fi
  if [[ "$EMIT_OBSERVABILITY" != "true" ]]; then
    echo "--no-node together with --no-observability leaves nothing to emit"
    exit 1
  fi
fi

if [[ "$DEPS_ONLY" == "true" && "$LAYOUT" != "split" ]]; then
  echo "--deps-only requires --layout split"
  exit 1
fi

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

NODE_VERSION=""
if [[ "$EMIT_NODE" == "true" ]]; then
  NODE_VERSION="$(resolve_latest_node_version)"
  echo "resolved Node runtime version: v$NODE_VERSION (major=$NODE_MAJOR, variant=$NODE_VARIANT)"
else
  echo "skip Node runtime (--no-node)"
fi

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

    # 前端只在宿主机构建一次（同一份 .next 输出），但要按架构分别打包 linux x64 / arm64。
    # 仅在发版构建期间临时声明 supportedArchitectures，让 pnpm 同时安装 x64/arm64 的 sharp 原生包；
    # 打包时再按架构裁剪，每个 tarball 只保留本架构 glibc 版 sharp 与 libvips（见 prepare_frontend_dist_for_arch）。
    # The frontend is built once on the host (one .next output) but packaged per arch for linux x64 / arm64.
    # Declare supportedArchitectures only for the release build so pnpm installs sharp for both arches;
    # packaging then prunes each tarball to its own glibc sharp + libvips (see prepare_frontend_dist_for_arch).
    local workspace_file="$ROOT_DIR/frontend/pnpm-workspace.yaml"
    if [[ -e "$workspace_file" ]]; then
      echo "refusing to overwrite existing $workspace_file (release build writes a temporary one)"
      exit 1
    fi
    trap 'rm -f "$workspace_file"' EXIT
    cat >"$workspace_file" <<'YAML'
# Temporary file written by install/package-release.sh; removed after the frontend build.
supportedArchitectures:
  os: [current, linux]
  cpu: [current, x64, arm64]
  libc: [current, glibc]
YAML

    pnpm install --no-frozen-lockfile
    pnpm run pack:standalone
  )
}

# 为指定架构准备前端 standalone 目录：复制同一份构建输出，只保留 linux-<cpu> glibc 版 sharp 与 libvips，
# 删除其它架构 / musl / 非 linux 的 sharp 原生包。输出目录路径打印到 stdout。
# Prepare the frontend standalone dir for one arch: copy the single build output and keep only the
# linux-<cpu> glibc sharp + libvips, pruning other arch / musl / non-linux sharp native packages.
# Prints the output directory on stdout.
prepare_frontend_dist_for_arch() {
  local arch="$1"
  local cpu
  cpu="$(node_arch "$arch")"
  local out="$STAGE_DIR/frontend-linux-${arch}"
  local keep_sharp="sharp-linux-${cpu}"
  local keep_vips="sharp-libvips-linux-${cpu}"

  rm -rf "$out"
  mkdir -p "$out"
  cp -a "$FRONTEND_DIST"/. "$out/"

  if [[ -d "$out/node_modules" ]]; then
    local entry name stem
    local -a entries=()
    # pnpm 布局：.pnpm/@img+sharp-*@<ver> 实体目录，以及各处 @img/sharp-* 符号链接或目录；先收集再删除。
    # pnpm layout: .pnpm/@img+sharp-*@<ver> package dirs plus @img/sharp-* symlinks or dirs; collect, then delete.
    while IFS= read -r -d '' entry; do
      entries+=("$entry")
    done < <(find "$out/node_modules" \( -name '@img+sharp-*' -o \( -path '*/@img/sharp-*' -a ! -path '*/@img/sharp-*/*' \) \) \
      \( -type d -o -type l \) -prune -print0)
    for entry in ${entries[@]+"${entries[@]}"}; do
      name="$(basename "$entry")"
      if [[ "$name" == @img+* ]]; then
        stem="${name#@img+}"
        stem="${stem%@*}"
      else
        stem="$name"
      fi
      if [[ "$stem" != "$keep_sharp" && "$stem" != "$keep_vips" ]]; then
        rm -rf "$entry"
      fi
    done
  fi

  local sharp_node vips_lib
  sharp_node="$(find "$out/node_modules" -path "*/@img/${keep_sharp}/lib/${keep_sharp}.node" -print -quit 2>/dev/null || true)"
  vips_lib="$(find "$out/node_modules" -path "*/@img/${keep_vips}/lib/libvips-cpp.so*" -print -quit 2>/dev/null || true)"
  if [[ -z "$sharp_node" || -z "$vips_lib" ]]; then
    local msg="frontend standalone for linux/${arch} lacks @img/${keep_sharp} or @img/${keep_vips}"
    if [[ "$BUILD_FRONTEND" == "true" ]]; then
      echo "$msg" >&2
      exit 1
    fi
    # 复用外部预构建的 dist-standalone 时只告警：next/image 会退化为返回原图。
    # With an externally prebuilt dist-standalone only warn: next/image falls back to serving originals.
    echo "warning: $msg; next/image optimization will fall back to original images" >&2
  fi
  local link
  while IFS= read -r -d '' link; do
    if [[ ! -e "$link" ]]; then
      echo "frontend standalone for linux/${arch} has a dangling symlink after pruning: ${link#"$out"/}" >&2
      exit 1
    fi
  done < <(find "$out/node_modules" -type l -path '*/@img/*' -print0 2>/dev/null)
  echo "$out"
}

FRONTEND_DIST="$ROOT_DIR/frontend/dist-standalone"
if [[ "$DEPS_ONLY" != "true" ]]; then
  if [[ "$BUILD_FRONTEND" == "true" ]]; then
    build_frontend_standalone
  fi
  if [[ ! -f "$FRONTEND_DIST/server.js" ]]; then
    echo "frontend standalone not found: $FRONTEND_DIST/server.js"
    echo "run with --build-frontend or build frontend manually."
    exit 1
  fi
fi

# 规范化产品版本（去掉 tag 的 v 前缀）供 ldflags 注入。
# Normalize product version (strip tag v prefix) for ldflags injection.
stx_product_version() {
  local raw="${1:-}"
  raw="${raw#v}"
  raw="${raw#V}"
  echo "$raw"
}

# 组装写入 internal/version 的 ldflags。
# Build ldflags that stamp internal/version variables.
stx_version_ldflags() {
  local product_version
  local git_commit
  local build_time
  product_version="$(stx_product_version "$APP_VERSION")"
  git_commit="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
  build_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf -- '-X github.com/LeonYoah/stx/internal/version.Version=%s -X github.com/LeonYoah/stx/internal/version.GitCommit=%s -X github.com/LeonYoah/stx/internal/version.BuildTime=%s' \
    "$product_version" "$git_commit" "$build_time"
}

# 构建指定架构的 STX 后端二进制。/ Build the STX backend binary for the requested architecture.
build_stx_binary() {
  local arch="$1"
  local out="$BUILD_DIR/stx-linux-${arch}"
  local ldflags
  ldflags="$(stx_version_ldflags)"
  echo "building stx for linux/$arch (version=$(stx_product_version "$APP_VERSION")) ..."
  (
    cd "$ROOT_DIR"
    # shellcheck disable=SC2086
    GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -ldflags "$ldflags" -o "$out" .
  )
}

# 构建发布包所需的双架构 STX Agent。/ Build both STX Agent architectures required by release packages.
build_agent_binaries() {
  local out_amd64="$BUILD_DIR/stx-agent-linux-amd64"
  local out_arm64="$BUILD_DIR/stx-agent-linux-arm64"
  local ldflags
  ldflags="$(stx_version_ldflags)"

  echo "building stx-agent for linux/amd64 (version=$(stx_product_version "$APP_VERSION")) ..."
  (
    cd "$ROOT_DIR/agent"
    # shellcheck disable=SC2086
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$ldflags" -o "$out_amd64" ./cmd
  )
  echo "building stx-agent for linux/arm64 (version=$(stx_product_version "$APP_VERSION")) ..."
  (
    cd "$ROOT_DIR/agent"
    # shellcheck disable=SC2086
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$ldflags" -o "$out_arm64" ./cmd
  )
}

# 按代际 profile 构建并暂存 STX Java Proxy 薄 JAR（v2 + v3）。
# Build and stage STX Java Proxy thin JARs for each epoch profile (v2 + v3).
build_stx_java_proxy_jar() {
  local proxy_project_dir="$ROOT_DIR/tools/stx-java-proxy"
  local proxy_target_dir="$proxy_project_dir/target"
  local profiles=("epoch-v2" "epoch-v3")
  local epochs=("v2" "v3")
  local i

  mkdir -p "$BUILD_DIR"
  for i in "${!profiles[@]}"; do
    local profile="${profiles[$i]}"
    local epoch="${epochs[$i]}"
    local proxy_out="$BUILD_DIR/stx-java-proxy-${epoch}.jar"

    echo "building stx-java-proxy thin jar (${profile} → stx-java-proxy-${epoch}.jar) ..."
    (
      cd "$ROOT_DIR"
      # v3 构建必须关闭默认 epoch-v2，避免同时解析 2.3 optional 与 3.0 shade 坐标。
      # v3 builds must disable default epoch-v2 to avoid resolving both 2.3 optional and 3.0 shade coords.
      if [[ "$epoch" == "v3" ]]; then
        mvn -q -f "$proxy_project_dir/pom.xml" -P'epoch-v3,!epoch-v2' -DskipTests package
      else
        mvn -q -f "$proxy_project_dir/pom.xml" -Pepoch-v2 -DskipTests package
      fi
    )

    local built_proxy_jar="$proxy_target_dir/stx-java-proxy-${epoch}.jar"
    if [[ ! -f "$built_proxy_jar" ]]; then
      built_proxy_jar="$(find "$proxy_target_dir" -maxdepth 1 -type f -name "stx-java-proxy-${epoch}.jar" ! -name '*-bin.jar' | sort | head -n1)"
    fi
    if [[ -z "$built_proxy_jar" || ! -f "$built_proxy_jar" ]]; then
      echo "failed to locate built stx-java-proxy-${epoch}.jar under $proxy_target_dir"
      exit 1
    fi

    cp "$built_proxy_jar" "$proxy_out"
  done

  CAPABILITY_PROXY_JAR="$BUILD_DIR/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar"
  if [[ ! -f "$CAPABILITY_PROXY_JAR" ]]; then
    echo "failed to locate default capability proxy jar: $CAPABILITY_PROXY_JAR"
    exit 1
  fi
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

  # 发布构建目录内全部代际 jar（v2/v3）。 / Stage every epoch jar produced in BUILD_DIR.
  find "$BUILD_DIR" -maxdepth 1 -type f -name 'stx-java-proxy-*.jar' -print0 | while IFS= read -r -d '' jar_path; do
    cp "$jar_path" "$destination_dir/"
  done
}

if [[ "$DEPS_ONLY" != "true" ]]; then
  build_agent_binaries
  build_stx_java_proxy_jar
  for arch in "${ARCHES[@]}"; do
    build_stx_binary "$arch"
  done
fi

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

  # Ship config templates + renderer; start/stop lives in bin/.
  # 只打配置模板与渲染脚本，启停由 bin 负责。
  cp -a "$ROOT_DIR/install/observability/prometheus_config" "$deps_dir/"
  cp -a "$ROOT_DIR/install/observability/alertmanager_config" "$deps_dir/"
  cp -a "$ROOT_DIR/install/observability/grafana_config" "$deps_dir/"
  cp "$ROOT_DIR/install/observability/render.sh" "$deps_dir/render.sh"
  chmod +x "$deps_dir/render.sh"
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
  if [[ "$EMIT_NODE" == "true" ]]; then
    local runtime_dir
    runtime_dir="$(prepare_node_runtime "$arch")"
    local node_name="node-${NODE_MAJOR}-${NODE_VARIANT}-linux-${arch}.tar.gz"
    local node_out="$OUTPUT_DIR/$node_name"
    echo "creating deps node package: $node_out"
    tar -C "$(dirname "$runtime_dir")" -czf "$node_out" "$(basename "$runtime_dir")"
    # Normalize inner folder name to node/ when extracted by installers that expect runtime/node.
    # 安装器期望 runtime/node；这里保持解压后可再命名。
    write_sha256 "$node_out"
  fi

  if [[ "$EMIT_OBSERVABILITY" == "true" ]]; then
    local obs_name="observability-prom${PROMETHEUS_VERSION}-am${ALERTMANAGER_VERSION}-gf${GRAFANA_VERSION}-linux-${arch}.tar.gz"
    local obs_out="$OUTPUT_DIR/$obs_name"
    local obs_stage="$STAGE_DIR/deps-${arch}"
    rm -rf "$obs_stage"
    prepare_observability_stack "$arch" "$obs_stage"
    echo "creating deps observability package: $obs_out"
    tar -C "$obs_stage" -czf "$obs_out" .
    write_sha256 "$obs_out"
  fi

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

if [[ "$LAYOUT" == "split" && "$DEPS_ONLY" == "true" ]]; then
  for arch in "${ARCHES[@]}"; do
    echo "emitting deps for linux/$arch"
    emit_deps_packages "$arch"
  done
  echo
  echo "all done (deps-only)."
  echo "output dir: $OUTPUT_DIR"
  exit 0
fi

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
    frontend_alias="frontend-standalone-linux-${arch}.tar.gz"
    echo "creating $frontend_name"
    frontend_arch_dist="$(prepare_frontend_dist_for_arch "$arch")"
    tar -C "$frontend_arch_dist" -czf "$OUTPUT_DIR/$frontend_name" .
    write_sha256 "$OUTPUT_DIR/$frontend_name"
    # latest/download 稳定文件名，方便 README 直链。/ Stable name for releases/latest/download links.
    cp "$OUTPUT_DIR/$frontend_name" "$OUTPUT_DIR/$frontend_alias"
    cp "$OUTPUT_DIR/${frontend_name}.sha256" "$OUTPUT_DIR/${frontend_alias}.sha256"

    if [[ "$EMIT_DEPS" == "true" ]]; then
      emit_deps_packages "$arch"
    fi
  done

  # 发布跨架构通用的 stx-java-proxy 薄 jar（v2/v3）与启动脚本。
  # Publish architecture-independent stx-java-proxy thin jars (v2/v3) and launcher script.
  for epoch in v2 v3; do
    epoch_jar="$BUILD_DIR/stx-java-proxy-${epoch}.jar"
    if [[ -f "$epoch_jar" ]]; then
      cp "$epoch_jar" "$OUTPUT_DIR/stx-java-proxy-${epoch}.jar"
      write_sha256 "$OUTPUT_DIR/stx-java-proxy-${epoch}.jar"
    fi
  done
  if [[ -f "$ROOT_DIR/scripts/stx-java-proxy.sh" ]]; then
    cp "$ROOT_DIR/scripts/stx-java-proxy.sh" "$OUTPUT_DIR/stx-java-proxy.sh"
    chmod +x "$OUTPUT_DIR/stx-java-proxy.sh"
    write_sha256 "$OUTPUT_DIR/stx-java-proxy.sh"
  fi

  # Also publish installer helpers for curl|bash consumers. / 同步发布安装辅助脚本供 curl|bash 使用。
  cp "$ROOT_DIR/install/install-online.sh" "$OUTPUT_DIR/install-online.sh"
  cp "$ROOT_DIR/install/download-bundle.sh" "$OUTPUT_DIR/download-bundle.sh"
  chmod +x "$OUTPUT_DIR/install-online.sh" "$OUTPUT_DIR/download-bundle.sh"

  # 手动安装辅助包（无仓库时用）。/ Helpers tarball for manual install without a git clone.
  helpers_stage="$STAGE_DIR/install-helpers"
  rm -rf "$helpers_stage"
  mkdir -p "$helpers_stage/bin/lib" "$helpers_stage/packages" "$helpers_stage/lib" "$helpers_stage/scripts"
  cp "$ROOT_DIR/install/install.sh" "$helpers_stage/install.sh"
  cp "$ROOT_DIR/install/download-lib.sh" "$helpers_stage/download-lib.sh"
  cp "$ROOT_DIR/install/install-core.sh" "$helpers_stage/install-core.sh"
  cp "$ROOT_DIR/install/bin/start.sh" "$helpers_stage/bin/start.sh"
  cp "$ROOT_DIR/install/bin/stop.sh" "$helpers_stage/bin/stop.sh"
  cp "$ROOT_DIR/install/bin/status.sh" "$helpers_stage/bin/status.sh"
  cp "$ROOT_DIR/install/bin/lib/observability.sh" "$helpers_stage/bin/lib/observability.sh"
  cp "$ROOT_DIR/config.example.yaml" "$helpers_stage/config.example.yaml"
  if [[ -f "$ROOT_DIR/scripts/stx-java-proxy.sh" ]]; then
    cp "$ROOT_DIR/scripts/stx-java-proxy.sh" "$helpers_stage/scripts/stx-java-proxy.sh"
    chmod +x "$helpers_stage/scripts/stx-java-proxy.sh"
  fi
  stage_stx_java_proxy_jars "$helpers_stage/lib"
  chmod +x "$helpers_stage/install.sh" "$helpers_stage/bin/"*.sh
  tar -C "$helpers_stage" -czf "$OUTPUT_DIR/stx-install-helpers.tar.gz" .
  write_sha256 "$OUTPUT_DIR/stx-install-helpers.tar.gz"

  # Docker Compose 全量包（无仓库时下载解压即可）。/ Full Docker Compose pack for users without a clone.
  # Docker Compose pack: render observability from single-source templates first.
  # Docker Compose 包：先从单源模板渲染可观测性配置。
  if [[ -d "$ROOT_DIR/deploy/docker" ]]; then
    "$ROOT_DIR/install/observability/render.sh" docker "$ROOT_DIR/deploy/docker/observability"
    tar -C "$ROOT_DIR/deploy" -czf "$OUTPUT_DIR/stx-docker-compose.tar.gz" docker
    write_sha256 "$OUTPUT_DIR/stx-docker-compose.tar.gz"
  fi

  # 发布自升级整合包（单架构独立全量包，不含重型外部运行时）。
  # Publish unified self-upgrade bundles (architecture-specific bundle excluding heavy external runtimes).
  for arch in "${ARCHES[@]}"; do
    upgrade_stage="$STAGE_DIR/upgrade-${arch}"
    rm -rf "$upgrade_stage"
    mkdir -p "$upgrade_stage/lib/agent" "$upgrade_stage/scripts" "$upgrade_stage/frontend"
    cp "$BUILD_DIR/stx-linux-${arch}" "$upgrade_stage/stx"
    chmod +x "$upgrade_stage/stx"
    cp "$BUILD_DIR/stx-agent-linux-${arch}" "$upgrade_stage/lib/agent/stx-agent-linux-${arch}"
    chmod +x "$upgrade_stage/lib/agent/stx-agent-linux-${arch}"
    stage_stx_java_proxy_jars "$upgrade_stage/lib"
    if [[ -f "$ROOT_DIR/scripts/stx-java-proxy.sh" ]]; then
      cp "$ROOT_DIR/scripts/stx-java-proxy.sh" "$upgrade_stage/scripts/stx-java-proxy.sh"
      chmod +x "$upgrade_stage/scripts/stx-java-proxy.sh"
    fi
    upgrade_frontend_dist="$(prepare_frontend_dist_for_arch "$arch")"
    cp -a "$upgrade_frontend_dist"/. "$upgrade_stage/frontend/"
    cat >"$upgrade_stage/BUILD_INFO" <<EOF
version=$APP_VERSION_SAFE
arch=$arch
created_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF
    upgrade_name="stx-upgrade-${APP_VERSION_SAFE}-linux-${arch}.tar.gz"
    upgrade_alias="stx-upgrade-linux-${arch}.tar.gz"
    echo "creating $upgrade_name"
    tar -C "$upgrade_stage" -czf "$OUTPUT_DIR/$upgrade_name" .
    write_sha256 "$OUTPUT_DIR/$upgrade_name"
    cp "$OUTPUT_DIR/$upgrade_name" "$OUTPUT_DIR/$upgrade_alias"
    cp "$OUTPUT_DIR/${upgrade_name}.sha256" "$OUTPUT_DIR/${upgrade_alias}.sha256"
  done

  (
    cd "$OUTPUT_DIR"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum stx-linux-* stx-agent-linux-* frontend-standalone-*.tar.gz stx-upgrade-*.tar.gz stx-java-proxy-*.jar stx-java-proxy.sh 2>/dev/null >SHA256SUMS || true
    else
      shasum -a 256 stx-linux-* stx-agent-linux-* frontend-standalone-*.tar.gz stx-upgrade-*.tar.gz stx-java-proxy-*.jar stx-java-proxy.sh 2>/dev/null >SHA256SUMS || true
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
    cp "$ROOT_DIR/install/install.sh" "$pkg_dir/install.sh"
    cp "$ROOT_DIR/install/download-lib.sh" "$pkg_dir/download-lib.sh"
    cp "$ROOT_DIR/install/install-core.sh" "$pkg_dir/install-core.sh"
    mkdir -p "$pkg_dir/lib"

    mkdir -p "$pkg_dir/lib/agent" "$pkg_dir/scripts"
    cp "$BUILD_DIR/stx-agent-linux-amd64" "$pkg_dir/lib/agent/stx-agent-linux-amd64"
    cp "$BUILD_DIR/stx-agent-linux-arm64" "$pkg_dir/lib/agent/stx-agent-linux-arm64"
    stage_stx_java_proxy_jars "$pkg_dir/lib"
    cp "$ROOT_DIR/scripts/stx-java-proxy.sh" "$pkg_dir/scripts/stx-java-proxy.sh"
    chmod +x "$pkg_dir/lib/agent/stx-agent-linux-amd64" "$pkg_dir/lib/agent/stx-agent-linux-arm64"
    chmod +x "$pkg_dir/scripts/stx-java-proxy.sh"

    mkdir -p "$pkg_dir/frontend"
    legacy_frontend_dist="$(prepare_frontend_dist_for_arch "$arch")"
    cp -a "$legacy_frontend_dist"/. "$pkg_dir/frontend/"

    local_node_runtime="$(prepare_node_runtime "$arch")"
    mkdir -p "$pkg_dir/runtime"
    cp -a "$local_node_runtime" "$pkg_dir/runtime/node"

    mkdir -p "$pkg_dir/bin/lib" "$pkg_dir/logs" "$pkg_dir/run"
    cp "$ROOT_DIR/install/bin/start.sh" "$pkg_dir/bin/start.sh"
    cp "$ROOT_DIR/install/bin/stop.sh" "$pkg_dir/bin/stop.sh"
    cp "$ROOT_DIR/install/bin/status.sh" "$pkg_dir/bin/status.sh"
    cp "$ROOT_DIR/install/bin/lib/observability.sh" "$pkg_dir/bin/lib/observability.sh"
    chmod +x "$pkg_dir/install.sh" "$pkg_dir/bin/start.sh" "$pkg_dir/bin/stop.sh" "$pkg_dir/bin/status.sh"

    if [[ "$obs" == "with" ]]; then
      # Legacy layout: still pack under observability/ (not deps/). / 旧包也落到 observability/
      prepare_observability_stack "$arch" "$pkg_dir/observability"
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
