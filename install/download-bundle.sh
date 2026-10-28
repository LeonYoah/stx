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

# 在有网机器上下载完整离线包（测速选线、校验 sha256），拷到离线机后解压执行 install.sh。
# Download a whole offline bundle on a networked machine (line probing, sha256 verified); copy it to the
# offline host, extract and run install.sh.

STX_GITHUB_OWNER="${STX_GITHUB_OWNER:-LeonYoah}"
STX_GITHUB_REPO="${STX_GITHUB_REPO:-stx}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# 定位 download-lib.sh：仓库 install/、离线包根目录，或 curl | bash 时从 GitHub raw（依次尝试各加速线路）拉取。
# Locate download-lib.sh: repo install/, bundle root, or (curl | bash) fetch it from GitHub raw via each line in turn.
stx_bootstrap_lib() {
  local f
  for f in "$SCRIPT_DIR/download-lib.sh" "$ROOT_DIR/install/download-lib.sh"; do
    [[ -f "$f" ]] && { STX_LIB="$f"; return 0; }
  done
  local ref="${STX_BOOTSTRAP_REF:-main}" raw prefix boot
  raw="https://raw.githubusercontent.com/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/${ref}/install/download-lib.sh"
  boot="$(mktemp -d "${TMPDIR:-/tmp}/stx-bootstrap.XXXXXX")"
  STX_BOOTSTRAP_ROOT="$boot"
  local -a prefixes=()
  case "${STX_MIRROR:-}" in
    http://*|https://*) prefixes+=("${STX_MIRROR%/}") ;;
  esac
  [[ -n "${STX_DOWNLOAD_MIRROR_PREFIX:-}" ]] && prefixes+=("${STX_DOWNLOAD_MIRROR_PREFIX%/}")
  prefixes+=("" "https://v4.gh-proxy.org" "https://gh-proxy.org" "https://cdn.gh-proxy.org" "https://v6.gh-proxy.org" "https://axisnow.gh-proxy.org")
  for prefix in "${prefixes[@]}"; do
    if curl -fsSL --connect-timeout 4 -m 20 -o "$boot/download-lib.sh" "${prefix:+${prefix}/}${raw}" 2>/dev/null \
      && grep -q "stx_fetch_bundle" "$boot/download-lib.sh"; then
      STX_LIB="$boot/download-lib.sh"
      return 0
    fi
  done
  echo "[ERROR] cannot fetch install/download-lib.sh from GitHub (direct or gh-proxy); check network or set STX_MIRROR=<prefix>" >&2
  return 1
}

for _stx_cmd in curl tar gzip awk sed grep; do
  if ! command -v "$_stx_cmd" >/dev/null 2>&1; then
    echo "[ERROR] missing required command: $_stx_cmd (please install it first, e.g. yum/apt-get install -y $_stx_cmd)" >&2
    exit 1
  fi
done
unset _stx_cmd

STX_LIB=""
STX_BOOTSTRAP_ROOT=""
stx_bootstrap_lib || exit 1
# shellcheck source=download-lib.sh
source "$STX_LIB"

usage() {
  cat <<'USAGE'
下载 STX 离线安装包 / download the STX offline bundle

Usage:
  curl -fsSL https://github.com/LeonYoah/stx/releases/latest/download/download-bundle.sh | bash -s -- [options]
  install/download-bundle.sh [options]

Options:
  --version <tag>          Release 版本（默认 latest；env STX_VERSION）
  --variant <v>            amd64 | arm64 | centos7（默认按本机自动判断；给离线机下载时请显式指定）
  --arch <amd64|arm64>     只指定 CPU 架构，glibc 仍按本机判断
  --mirror <line>          1-6 / gh-proxy | v4 | v6 | cdn | axisnow | github / 自定义前缀（env STX_MIRROR）
  --bundle-name <name>     覆盖 Release 中的离线包文件名
  --output-dir <dir>       保存目录（默认当前目录）
  --print-urls             只打印 6 条线路的 wget 命令，不下载
  -y, --yes                不提问（自动选最快线路）
  -h, --help
USAGE
}

VERSION="${STX_VERSION:-latest}"
VARIANT="${STX_BUNDLE_VARIANT:-}"
ARCH=""
BUNDLE_NAME=""
OUTPUT_PARENT="${OUTPUT_DIR:-$(pwd)}"
PRINT_URLS=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --variant) VARIANT="${2:-}"; shift 2 ;;
    --arch) ARCH="${2:-}"; shift 2 ;;
    --mirror) STX_MIRROR="${2:-}"; export STX_MIRROR; shift 2 ;;
    --bundle-name) BUNDLE_NAME="${2:-}"; shift 2 ;;
    --output-dir) OUTPUT_PARENT="${2:-}"; shift 2 ;;
    --print-urls) PRINT_URLS=true; shift ;;
    -y|--yes) STX_ASSUME_YES=true; export STX_ASSUME_YES; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[ERROR] unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

on_exit() {
  local rc=$?
  stx_report_failure "$rc" "下载"
  [[ -n "$STX_BOOTSTRAP_ROOT" ]] && rm -rf "$STX_BOOTSTRAP_ROOT"
  return 0
}
trap on_exit EXIT

if [[ -z "$VARIANT" ]]; then
  [[ -z "$ARCH" ]] && ARCH="$(stx_detect_arch)"
  VARIANT="$(stx_detect_bundle_variant "$ARCH")"
fi
stx_bundle_platform "$VARIANT" >/dev/null || exit 1

if [[ "$PRINT_URLS" == "true" ]]; then
  url="$(stx_bundle_release_url "$VERSION" "$VARIANT" "$BUNDLE_NAME")"
  stx_print_wget_lines "$url"
  echo "# sha256: 把上面任意一条命令末尾加上 .sha256 下载校验文件，然后 sha256sum -c ${url##*/}.sha256"
  exit 0
fi

STX_STEP_TOTAL=4
stx_step "确认离线包 / resolving the bundle"
stx_info "版本 / version: $VERSION   变体 / variant: $VARIANT   保存到 / output: $OUTPUT_PARENT"
stx_step_ok "$(stx_bundle_release_url "$VERSION" "$VARIANT" "$BUNDLE_NAME" | sed 's|.*/||')"

mkdir -p "$OUTPUT_PARENT"
OUTPUT_PARENT="$(cd "$OUTPUT_PARENT" && pwd)"
stx_fetch_bundle "$VERSION" "$VARIANT" "$OUTPUT_PARENT" "$BUNDLE_NAME"

name="$(basename "$STX_BUNDLE_FILE")"
cat <<MSG

${STX_C_GREEN}${STX_C_BOLD}[OK] 离线包已就绪 / bundle ready${STX_C_RESET}
  $STX_BUNDLE_FILE
  ${STX_BUNDLE_FILE}.sha256

拷贝到离线机后执行 / on the offline host:
  sha256sum -c ${name}.sha256
  tar -xzf ${name}
  cd ${name%.tar.gz}/ 2>/dev/null || cd stx-*-offline/
  sudo ./install.sh
MSG
