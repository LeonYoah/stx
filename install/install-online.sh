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

# 一键在线安装：自动识别平台 → 测速选择下载线路 → 下载并校验完整离线包 → 解压 → 调用包内 install.sh。
# One-line online install: detect platform -> probe/choose a download line -> download + verify the whole
# offline bundle -> extract -> run the bundled install.sh.

STX_GITHUB_OWNER="${STX_GITHUB_OWNER:-LeonYoah}"
STX_GITHUB_REPO="${STX_GITHUB_REPO:-stx}"

# 发版时由 package-release.sh 写入本脚本所属的 Release tag（仓库里为空）。
# Set to this script's release tag by package-release.sh when publishing (empty in the repo).
STX_SCRIPT_RELEASE_TAG=""

# 是否以文件方式运行（而不是 curl | bash）。管道模式下 BASH_SOURCE 为空，不能信任当前目录里的同名文件。
# Running from a real file (not curl | bash)? In pipe mode BASH_SOURCE is empty, so files in cwd are not trusted.
STX_SCRIPT_FILE="${BASH_SOURCE[0]:-}"
if [[ -n "$STX_SCRIPT_FILE" && -f "$STX_SCRIPT_FILE" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "$STX_SCRIPT_FILE")" && pwd)"
  ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
else
  SCRIPT_DIR=""
  ROOT_DIR=""
fi

# 预读 --version / --mirror（只为选择 download-lib.sh 的来源，正式解析在后面）。
# Pre-read --version / --mirror (only to pick where download-lib.sh comes from; real parsing happens later).
stx_preparse_args() {
  STX_BOOT_VERSION="${STX_VERSION:-latest}"
  STX_BOOT_MIRROR="${STX_MIRROR:-}"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version) STX_BOOT_VERSION="${2:-latest}"; shift 2 || break ;;
      --version=*) STX_BOOT_VERSION="${1#*=}"; shift ;;
      --mirror) STX_BOOT_MIRROR="${2:-}"; shift 2 || break ;;
      --mirror=*) STX_BOOT_MIRROR="${1#*=}"; shift ;;
      *) shift ;;
    esac
  done
}

# 定位 download-lib.sh：从文件运行时用同目录 / 仓库里的；curl | bash 时从 GitHub raw 拉取与所请求版本一致的版本
# （--version 的 tag → 本脚本所属 Release 的 tag → main），依次尝试各加速线路。
# Locate download-lib.sh: next to this file / in the repo when run from a file; in curl | bash mode fetch the copy
# matching the requested version from GitHub raw (--version tag -> this script's release tag -> main), over each line.
stx_bootstrap_lib() {
  local f
  if [[ -n "$SCRIPT_DIR" ]]; then
    for f in "$SCRIPT_DIR/download-lib.sh" "$ROOT_DIR/install/download-lib.sh"; do
      [[ -f "$f" ]] && { STX_LIB="$f"; return 0; }
    done
  fi
  local -a refs=() prefixes=()
  [[ -n "${STX_BOOTSTRAP_REF:-}" ]] && refs+=("$STX_BOOTSTRAP_REF")
  [[ -n "$STX_BOOT_VERSION" && "$STX_BOOT_VERSION" != latest ]] && refs+=("$STX_BOOT_VERSION")
  [[ -n "$STX_SCRIPT_RELEASE_TAG" ]] && refs+=("$STX_SCRIPT_RELEASE_TAG")
  refs+=("main")
  case "$STX_BOOT_MIRROR" in
    http://*|https://*) prefixes+=("${STX_BOOT_MIRROR%/}") ;;
    gh-proxy|1) prefixes+=("https://gh-proxy.org") ;;
    v4|2) prefixes+=("https://v4.gh-proxy.org") ;;
    v6|3) prefixes+=("https://v6.gh-proxy.org") ;;
    cdn|4) prefixes+=("https://cdn.gh-proxy.org") ;;
    axisnow|5) prefixes+=("https://axisnow.gh-proxy.org") ;;
  esac
  [[ -n "${STX_DOWNLOAD_MIRROR_PREFIX:-}" ]] && prefixes+=("${STX_DOWNLOAD_MIRROR_PREFIX%/}")
  prefixes+=("" "https://v4.gh-proxy.org" "https://gh-proxy.org" "https://cdn.gh-proxy.org" "https://v6.gh-proxy.org" "https://axisnow.gh-proxy.org")
  local boot ref raw prefix
  boot="$(mktemp -d "${TMPDIR:-/tmp}/stx-bootstrap.XXXXXX")"
  STX_BOOTSTRAP_ROOT="$boot"
  for ref in "${refs[@]}"; do
    raw="https://raw.githubusercontent.com/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/${ref}/install/download-lib.sh"
    for prefix in "${prefixes[@]}"; do
      local code
      code="$(curl -sSL --connect-timeout 4 -m 20 -o "$boot/download-lib.sh" -w '%{http_code}' \
        "${prefix:+${prefix}/}${raw}" 2>/dev/null || true)"
      if [[ "$code" == 200 ]]; then
        if grep -q "stx_fetch_bundle" "$boot/download-lib.sh"; then
          STX_LIB="$boot/download-lib.sh"
          STX_BOOTSTRAP_LIB_REF="$ref"
          [[ "$ref" != "${refs[0]}" ]] && echo "[WARN] download-lib.sh for ${refs[0]} has no offline-bundle support; using ${ref}" >&2
          return 0
        fi
        break   # 该 ref 的 lib 太旧，换下一个 ref。/ This ref's lib is too old: next ref.
      fi
      [[ "$code" == 404 ]] && break   # 该 ref 不存在。/ No such ref.
    done
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
STX_BOOTSTRAP_LIB_REF=""
stx_preparse_args "$@"
stx_bootstrap_lib || exit 1
# shellcheck source=download-lib.sh
source "$STX_LIB"

usage() {
  cat <<'USAGE'
STX 一键安装 / online installer

Usage:
  curl -fsSL https://github.com/LeonYoah/stx/releases/latest/download/install-online.sh | sudo bash
  curl -fsSL https://v4.gh-proxy.org/https://github.com/LeonYoah/stx/releases/latest/download/install-online.sh | sudo bash
  ... | sudo bash -s -- [options]

下载 / download:
  --version <tag>          Release 版本，如 v1.3.0（默认 latest；env STX_VERSION）
  --variant <v>            amd64 | arm64 | centos7（默认自动：x86_64 且 glibc < 2.28 => centos7；env STX_BUNDLE_VARIANT）
  --arch <amd64|arm64>     只指定 CPU 架构，glibc 仍自动判断
  --mirror <line>          下载线路：1-6 / gh-proxy | v4 | v6 | cdn | axisnow | github / 自定义前缀 URL（env STX_MIRROR）
                           默认测速后自动选最快，交互时可按编号改选，失败自动换下一条
  --bundle-file <path>     使用本地已下载的离线包，不再下载
  --bundle-name <name>     覆盖 Release 中的离线包文件名（特殊预发包）
  --work-dir <dir>         下载与解压目录（默认 /tmp；env STX_WORK_DIR）
  --keep-bundle            安装后保留下载的离线包

安装（透传给 install.sh）/ install (passed to install.sh):
  -y, --yes                非交互，全部使用默认值 / no prompts
  --install-dir <path>     安装目录（默认 /opt/stx）
  --admin-password <pwd>   初始管理员密码（默认 admin123）
  --frontend-port <port>   Web 控制台端口（默认 17880）
  --http-port <port>       后端 API 端口（默认 17800）
  --grpc-port <port>       Agent gRPC 端口（默认 17890，Agent 回连）
  --external-ip <ip>       对外访问 IP/域名（默认自动探测）
  --db-type <sqlite|mysql> 及 --db-host/--db-port/--db-user/--db-password/--db-name
  --prometheus-port / --alertmanager-port / --grafana-port   三件套端口（只能用参数）
  --without-node           不用包内 Node（本机需 Node ≥ 18.18）
  --service-user <user>    systemd 服务运行用户（默认 root）
  --no-autostart           不设置开机自启
  --without-observability  不启用内置 Prometheus/Alertmanager/Grafana
  --force | --no-preserve-config | --no-start | --no-systemd
  -h, --help
USAGE
}

VERSION="${STX_VERSION:-latest}"
VARIANT="${STX_BUNDLE_VARIANT:-}"
ARCH=""
BUNDLE_FILE=""
BUNDLE_NAME=""
WORK_PARENT="${STX_WORK_DIR:-${TMPDIR:-/tmp}}"
KEEP_BUNDLE=false
INSTALL_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --variant) VARIANT="${2:-}"; shift 2 ;;
    --arch) ARCH="${2:-}"; shift 2 ;;
    --mirror) STX_MIRROR="${2:-}"; export STX_MIRROR; shift 2 ;;
    --bundle-file) BUNDLE_FILE="${2:-}"; shift 2 ;;
    --bundle-name) BUNDLE_NAME="${2:-}"; shift 2 ;;
    --work-dir) WORK_PARENT="${2:-}"; shift 2 ;;
    --keep-bundle) KEEP_BUNDLE=true; shift ;;
    -y|--yes) STX_ASSUME_YES=true; export STX_ASSUME_YES; INSTALL_ARGS+=("$1"); shift ;;
    --install-dir|--admin-password|--frontend-port|--http-port|--api-port|--grpc-port|--external-ip|--external-host|\
    --db-type|--db-host|--db-port|--db-user|--db-password|--db-name|--prometheus-port|--alertmanager-port|--grafana-port|\
    --service-user)
      INSTALL_ARGS+=("$1" "${2:-}"); shift 2 ;;
    --without-observability|--with-observability|--force|--no-preserve-config|--no-start|--no-systemd|--without-node|--no-autostart)
      INSTALL_ARGS+=("$1"); shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[ERROR] unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

stx_setup_log "${TMPDIR:-/tmp}/stx-install-$(date +%Y%m%d-%H%M%S).log"
WORK_DIR=""
on_exit() {
  local rc=$?
  stx_report_failure "$rc" "安装"
  if [[ -n "$WORK_DIR" && -d "$WORK_DIR" ]]; then
    if [[ "$KEEP_BUNDLE" == "true" || "$rc" -ne 0 ]]; then
      find "$WORK_DIR" -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} + 2>/dev/null || true
      [[ -n "${STX_BUNDLE_FILE:-}" && -f "$STX_BUNDLE_FILE" ]] && echo "  离线包保留在 / bundle kept at: $STX_BUNDLE_FILE" >&2
    else
      rm -rf "$WORK_DIR"
    fi
  fi
  [[ -n "$STX_BOOTSTRAP_ROOT" ]] && rm -rf "$STX_BOOTSTRAP_ROOT"
  return 0
}
trap on_exit EXIT

STX_STEP_PREFIX="下载 "
if [[ -n "$BUNDLE_FILE" ]]; then STX_STEP_TOTAL=2; else STX_STEP_TOTAL=5; fi
echo "${STX_C_BOLD}STX 一键安装 / online installer${STX_C_RESET}"
[[ -n "${STX_INSTALL_LOG:-}" ]] && echo "安装日志 / install log: $STX_INSTALL_LOG"

stx_step "识别平台 / detecting platform"
if [[ -z "$VARIANT" ]]; then
  [[ -z "$ARCH" ]] && ARCH="$(stx_detect_arch)"
  VARIANT="$(stx_detect_bundle_variant "$ARCH")"
fi
stx_bundle_platform "$VARIANT" >/dev/null || exit 1
glibc="$(stx_detect_glibc_version)"
stx_info "CPU: $(uname -m)   glibc: ${glibc:-unknown}   版本 / version: $VERSION"
if [[ "$VARIANT" == "arm64" ]] && stx_glibc_lt_228 "$glibc"; then
  stx_warn "arm64 只有官方 Node 构建（需要 glibc ≥ 2.28），本机 glibc ${glibc} 可能无法运行前端"
fi
stx_step_ok "离线包变体 / variant: $VARIANT"

if [[ -z "$BUNDLE_FILE" ]]; then
  WORK_DIR="$(mktemp -d "${WORK_PARENT%/}/stx-online.XXXXXX")"
  stx_fetch_bundle "$VERSION" "$VARIANT" "$WORK_DIR" "$BUNDLE_NAME"
  BUNDLE_FILE="$STX_BUNDLE_FILE"
else
  [[ -f "$BUNDLE_FILE" ]] || { stx_err "bundle file not found: $BUNDLE_FILE"; exit 1; }
  WORK_DIR="$(mktemp -d "${WORK_PARENT%/}/stx-online.XXXXXX")"
  KEEP_BUNDLE=true
fi

stx_step "解压离线包 / extracting the bundle"
tar -xzf "$BUNDLE_FILE" -C "$WORK_DIR"
bundle_dir="$(find "$WORK_DIR" -mindepth 1 -maxdepth 1 -type d -name 'stx-*' | head -n1)"
[[ -n "$bundle_dir" && -f "$bundle_dir/install.sh" ]] || { stx_err "install.sh not found in $BUNDLE_FILE"; exit 1; }
stx_step_ok "$bundle_dir"

echo
echo "${STX_C_BOLD}==> 运行安装程序 / running install.sh${STX_C_RESET}"
bash "$bundle_dir/install.sh" --offline ${INSTALL_ARGS[@]+"${INSTALL_ARGS[@]}"}
