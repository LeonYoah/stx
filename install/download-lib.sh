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

# STX release download helpers: region detect, gh-proxy speed test, URL build, sha256.
# STX 发布下载辅助：地区判定、gh-proxy 测速、URL 拼装、sha256 校验。
# shellcheck shell=bash

: "${STX_GITHUB_OWNER:=LeonYoah}"
: "${STX_GITHUB_REPO:=stx}"
: "${STX_DOWNLOAD_CONNECT_TIMEOUT:=3}"
: "${STX_DOWNLOAD_MAX_TIME:=5}"

STX_GH_PROXY_DEFAULT_NODES=(
  "https://gh-proxy.org"
  "https://v4.gh-proxy.org"
  "https://v6.gh-proxy.org"
  "https://cdn.gh-proxy.org"
  "https://axisnow.gh-proxy.org"
)

# 解析本机 CPU 架构为 amd64/arm64。/ Resolve host CPU arch to amd64/arm64.
stx_detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *)
      echo "unsupported arch: $(uname -m)" >&2
      return 1
      ;;
  esac
}

# 读取 glibc 主.次版本；无法判定时返回空。/ Read glibc major.minor; empty when unknown.
stx_detect_glibc_version() {
  if command -v ldd >/dev/null 2>&1; then
    ldd --version 2>&1 | head -n1 | sed -n 's/.* \([0-9][0-9]*\.[0-9][0-9]*\).*/\1/p'
    return 0
  fi
  echo ""
}

# 根据 glibc 选择 node variant：<2.27 用 glibc217；glibc217 只有 amd64 构建，arm64 总是 official。
# Pick node variant from glibc (<2.27 => glibc217). glibc217 builds are amd64-only, so arm64 is always official.
# 用法 / Usage: stx_recommend_node_variant [arch]
stx_recommend_node_variant() {
  local arch="${1:-}" ver
  if [[ "$arch" == "arm64" ]]; then
    echo "official"
    return 0
  fi
  ver="$(stx_detect_glibc_version)"
  if [[ -z "$ver" ]]; then
    echo "official"
    return 0
  fi
  local major minor
  major="${ver%%.*}"
  minor="${ver#*.}"
  minor="${minor%%.*}"
  if [[ "$major" -lt 2 ]] || { [[ "$major" -eq 2 ]] && [[ "${minor:-0}" -lt 27 ]]; }; then
    echo "glibc217"
  else
    echo "official"
  fi
}

# 判定下载地区 cn|global。/ Detect download region cn|global.
stx_detect_download_region() {
  if [[ -n "${STX_DOWNLOAD_REGION:-}" ]]; then
    echo "$STX_DOWNLOAD_REGION"
    return 0
  fi

  # Strong signal: GitHub HTTPS reachability. / 强信号：直连 GitHub 是否可达。
  if ! curl -fsI --connect-timeout "$STX_DOWNLOAD_CONNECT_TIMEOUT" --max-time "$STX_DOWNLOAD_MAX_TIME" \
    "https://github.com/" >/dev/null 2>&1; then
    echo "cn"
    return 0
  fi

  local tz=""
  if command -v timedatectl >/dev/null 2>&1; then
    tz="$(timedatectl show -p Timezone --value 2>/dev/null || true)"
  fi
  if [[ -z "$tz" && -L /etc/localtime ]]; then
    tz="$(readlink /etc/localtime 2>/dev/null | sed 's|.*/zoneinfo/||')"
  fi
  case "${tz:-}" in
    Asia/Shanghai|Asia/Chongqing|Asia/Harbin|Asia/Urumqi|Asia/Hong_Kong|Asia/Taipei|PRC)
      echo "cn"
      return 0
      ;;
  esac
  case "${LANG:-}${LC_ALL:-}" in
    *zh_CN*|*zh_Hans*)
      echo "cn"
      return 0
      ;;
  esac
  echo "global"
}

# 组装 GitHub Release 资产下载 URL（可带镜像前缀）。/ Build GitHub release asset URL with optional mirror prefix.
stx_build_github_release_url() {
  local tag="$1"
  local asset="$2"
  local prefix="${3:-}"
  local raw
  # GitHub 的 latest 是特殊路径 releases/latest/download/<asset>；releases/download/latest/... 会 404。
  # "latest" is a special GitHub path (releases/latest/download/<asset>); releases/download/latest/... 404s.
  if [[ "$tag" == "latest" ]]; then
    raw="https://github.com/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/releases/latest/download/${asset}"
  else
    raw="https://github.com/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/releases/download/${tag}/${asset}"
  fi
  if [[ -n "$prefix" ]]; then
    prefix="${prefix%/}"
    echo "${prefix}/${raw}"
  else
    echo "$raw"
  fi
}

# 返回可用 gh-proxy 节点列表。/ Return gh-proxy node list.
stx_gh_proxy_nodes() {
  if [[ -n "${STX_GH_PROXY_NODES:-}" ]]; then
    local IFS=','
    # shellcheck disable=SC2206
    local nodes=($STX_GH_PROXY_NODES)
    printf '%s\n' "${nodes[@]}"
    return 0
  fi
  printf '%s\n' "${STX_GH_PROXY_DEFAULT_NODES[@]}"
}

# 当前毫秒时间戳；不支持 %N 的 date（如 busybox/BSD）退化为秒*1000，不依赖 python3。
# Current epoch milliseconds; falls back to seconds*1000 when date lacks %N (busybox/BSD). No python3 needed.
stx_now_ms() {
  local ms
  ms="$(date +%s%3N 2>/dev/null || true)"
  if [[ "$ms" =~ ^[0-9]{13,}$ ]]; then
    echo "$ms"
  else
    echo "$(( $(date +%s) * 1000 ))"
  fi
}

# 从 GitHub releases API JSON（stdin）提取 tag_name，仅用 grep/sed，不依赖 python3。
# Extract tag_name from GitHub releases API JSON on stdin using grep/sed only (no python3).
stx_json_tag_name() {
  tr -d '\r\n' | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -n1 \
    | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)"/\1/'
}

# 从 deps MANIFEST.json（stdin）读取 observability.<arch> 资产名，仅用 tr/grep/sed。
# Read observability.<arch> asset name from deps MANIFEST.json on stdin using tr/grep/sed only.
stx_manifest_obs_asset() {
  local arch="$1"
  tr -d '\r\n' | grep -o '"observability"[[:space:]]*:[[:space:]]*{[^}]*}' | head -n1 \
    | grep -o "\"${arch}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -n1 \
    | sed 's/.*:[[:space:]]*"\([^"]*\)"/\1/'
}

# 版本号 >= 18.18（node -v 形如 v20.11.1）。/ True when a node version string is >= 18.18.
stx_node_version_ok() {
  local v="${1#v}" major minor
  major="${v%%.*}"
  # 无小版本号（如 v18）按 minor=0 处理。/ No minor part (e.g. v18) => minor=0.
  if [[ "$v" == *.* ]]; then
    minor="${v#*.}"
    minor="${minor%%.*}"
  else
    minor=0
  fi
  [[ "$major" =~ ^[0-9]+$ ]] || return 1
  [[ "$minor" =~ ^[0-9]+$ ]] || minor=0
  if (( major > 18 )) || { (( major == 18 )) && (( minor >= 18 )); }; then
    return 0
  fi
  return 1
}

# 探测可用的系统 Node（>= 18.18）：设置 STX_SYSTEM_NODE / STX_SYSTEM_NODE_VER，不合格返回 1。
# Probe a usable system Node (>= 18.18): sets STX_SYSTEM_NODE / STX_SYSTEM_NODE_VER; returns 1 when unusable.
stx_probe_system_node() {
  STX_SYSTEM_NODE="$(command -v node 2>/dev/null || true)"
  STX_SYSTEM_NODE_VER=""
  [[ -n "$STX_SYSTEM_NODE" ]] && STX_SYSTEM_NODE_VER="$("$STX_SYSTEM_NODE" -v 2>/dev/null || true)"
  [[ -n "$STX_SYSTEM_NODE" ]] && stx_node_version_ok "$STX_SYSTEM_NODE_VER"
}

# 系统 Node 缺失/过旧时的统一报错（install.sh 与 install-online.sh 共用）。
# Shared error when the system Node is missing/too old (used by install.sh and install-online.sh).
# 用法 / Usage: stx_system_node_error <context line> [hint line...]
stx_system_node_error() {
  local context="$1" line
  shift
  echo "[ERROR] ${context}, and no usable system Node (need >= 18.18, found: ${STX_SYSTEM_NODE_VER:-none})" >&2
  for line in "$@"; do
    echo "        $line" >&2
  done
}

# Node deps 包缺失时的提示（glibc217 只有 amd64；非 official 时提示 --node-variant official）。
# Hint when the Node deps package is missing (glibc217 is amd64-only; suggest --node-variant official).
# 用法 / Usage: stx_node_package_missing_error <variant> <arch> <deps_tag> <without-node hint>
stx_node_package_missing_error() {
  local variant="$1" arch="$2" tag="$3" hint="$4"
  echo "[ERROR] node package node-{22,18}-${variant}-linux-${arch}.tar.gz not found on deps tag=${tag}" >&2
  if [[ "$variant" != "official" ]]; then
    echo "        variant '${variant}' may not exist for ${arch} (glibc217 is amd64-only); retry with --node-variant official" >&2
  fi
  echo "        or check --deps-tag/--arch; ${hint}" >&2
}

# 检查必需命令是否存在，缺失时给出清晰错误。/ Fail with a clear message when required commands are missing.
stx_require_cmds() {
  local missing=() c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || missing+=("$c")
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "[ERROR] missing required command(s): ${missing[*]}" >&2
    echo "        please install them first, e.g. apt-get install -y ${missing[*]}  /  yum install -y ${missing[*]}" >&2
    return 1
  fi
}

# 对国内节点测速，回写最快前缀到 STX_SELECTED_MIRROR_PREFIX。/ Speed-test CN nodes; set STX_SELECTED_MIRROR_PREFIX.
stx_select_mirror_prefix() {
  local region="$1"
  STX_SELECTED_MIRROR_PREFIX=""

  if [[ -n "${STX_DOWNLOAD_MIRROR_PREFIX:-}" ]]; then
    STX_SELECTED_MIRROR_PREFIX="${STX_DOWNLOAD_MIRROR_PREFIX%/}"
    echo "[INFO] using STX_DOWNLOAD_MIRROR_PREFIX=$STX_SELECTED_MIRROR_PREFIX" >&2
    return 0
  fi

  if [[ "$region" != "cn" ]]; then
    STX_SELECTED_MIRROR_PREFIX=""
    return 0
  fi

  local best="" best_ms=999999 node probe_url start end elapsed code
  local probe_tag="${STX_SPEEDTEST_TAG:-latest}"
  local probe_asset="${STX_SPEEDTEST_ASSET:-SHA256SUMS}"

  while IFS= read -r node; do
    [[ -z "$node" ]] && continue
    node="${node%/}"
    probe_url="$(stx_build_github_release_url "$probe_tag" "$probe_asset" "$node")"
    start="$(stx_now_ms)"
    code="$(curl -o /dev/null -s -w '%{http_code}' --connect-timeout "$STX_DOWNLOAD_CONNECT_TIMEOUT" \
      --max-time "$STX_DOWNLOAD_MAX_TIME" -I "$probe_url" 2>/dev/null || echo "000")"
    end="$(stx_now_ms)"
    elapsed=$((end - start))
    if [[ "$code" =~ ^[23] ]]; then
      echo "[INFO] mirror ok: $node (${elapsed}ms, http=$code)" >&2
      if [[ "$elapsed" -lt "$best_ms" ]]; then
        best_ms="$elapsed"
        best="$node"
      fi
    else
      echo "[WARN] mirror fail: $node (http=$code)" >&2
    fi
  done < <(stx_gh_proxy_nodes)

  if [[ -n "$best" ]]; then
    STX_SELECTED_MIRROR_PREFIX="$best"
    echo "[INFO] selected mirror: $STX_SELECTED_MIRROR_PREFIX (${best_ms}ms)" >&2
  else
    echo "[WARN] all gh-proxy nodes failed; falling back to direct GitHub" >&2
    STX_SELECTED_MIRROR_PREFIX=""
  fi
}

# 下载文件到目标路径。/ Download file to destination path.
stx_download_file() {
  local url="$1"
  local dest="$2"
  mkdir -p "$(dirname "$dest")"
  echo "[INFO] downloading: $url" >&2
  curl -fL --retry 3 --retry-delay 1 --connect-timeout 10 --max-time 600 -o "$dest" "$url"
}

# 校验文件 sha256；期望可为「纯哈希」或「sha256 文件路径」。/ Verify sha256; expect hex or checksum file path.
stx_verify_sha256() {
  local file="$1"
  local expect="$2"
  local actual want
  if [[ ! -f "$file" ]]; then
    echo "[ERROR] missing file for checksum: $file" >&2
    return 1
  fi
  if [[ -f "$expect" ]]; then
    want="$(awk 'NF{print $1; exit}' "$expect")"
  else
    want="$(echo "$expect" | awk 'NF{print $1; exit}')"
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$file" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "$file" | awk '{print $1}')"
  fi
  if [[ "$actual" != "$want" ]]; then
    echo "[ERROR] sha256 mismatch for $file" >&2
    echo "        expected: $want" >&2
    echo "        actual  : $actual" >&2
    return 1
  fi
  echo "[OK] sha256 verified: $(basename "$file")" >&2
}

# 下载 GitHub Release 资产（自动加镜像前缀）。/ Download GitHub release asset with selected mirror.
stx_download_release_asset() {
  local tag="$1"
  local asset="$2"
  local dest="$3"
  local prefix="${STX_SELECTED_MIRROR_PREFIX:-${STX_DOWNLOAD_MIRROR_PREFIX:-}}"
  local url
  url="$(stx_build_github_release_url "$tag" "$asset" "$prefix")"
  if ! stx_download_file "$url" "$dest"; then
    if [[ -n "$prefix" ]]; then
      echo "[WARN] mirrored download failed, retry direct GitHub" >&2
      url="$(stx_build_github_release_url "$tag" "$asset" "")"
      stx_download_file "$url" "$dest"
    else
      return 1
    fi
  fi
}

# 从仓库 raw 拉取单个文件（可带镜像前缀）。/ Fetch one repo file from GitHub raw (optional mirror prefix).
stx_fetch_repo_file() {
  local ref="$1"
  local path="$2"
  local dest="$3"
  local prefix="${STX_SELECTED_MIRROR_PREFIX:-${STX_DOWNLOAD_MIRROR_PREFIX:-}}"
  local raw="https://raw.githubusercontent.com/${STX_GITHUB_OWNER}/${STX_GITHUB_REPO}/${ref}/${path}"
  mkdir -p "$(dirname "$dest")"
  if [[ -n "$prefix" ]] && curl -fsSL --retry 2 --retry-delay 1 --connect-timeout 10 --max-time 120 \
    -o "$dest" "${prefix%/}/${raw}" 2>/dev/null; then
    return 0
  fi
  curl -fsSL --retry 2 --retry-delay 1 --connect-timeout 10 --max-time 120 -o "$dest" "$raw" 2>/dev/null || {
    rm -f "$dest"
    return 1
  }
}

# 准备 Agent 启动脚本 stx-java-proxy.sh：Release 资产 → 仓库同 tag 的 raw 文件 → 本地仓库/bootstrap 副本
# （bootstrap 默认取 main，放最后，保证旧版本安装拿到与版本匹配的脚本）。
# 保证安装目录 scripts/stx-java-proxy.sh 始终存在（控制面经 /api/v1/agent/assets/stx-java-proxy.sh 下发给 Agent）。
# Stage the agent launcher stx-java-proxy.sh: release asset -> raw file at the same tag -> local repo/bootstrap copy
# (bootstrap defaults to main, so it goes last to keep older-version installs on the matching script).
# Keeps <install>/scripts/stx-java-proxy.sh populated (served to agents via /api/v1/agent/assets/stx-java-proxy.sh).
stx_stage_java_proxy_script() {
  local tag="$1"
  local dest="$2"
  local root_dir="${3:-}"
  local from_release=false
  mkdir -p "$(dirname "$dest")"
  if stx_download_release_asset "$tag" "stx-java-proxy.sh" "$dest" 2>/dev/null; then
    from_release=true
    if stx_download_release_asset "$tag" "stx-java-proxy.sh.sha256" "${dest}.sha256" 2>/dev/null; then
      stx_verify_sha256 "$dest" "${dest}.sha256" || { from_release=false; rm -f "$dest"; }
    fi
    rm -f "${dest}.sha256"
  fi
  if [[ "$from_release" == "true" ]]; then
    echo "[INFO] stx-java-proxy.sh: from release $tag" >&2
  elif stx_fetch_repo_file "$tag" "scripts/stx-java-proxy.sh" "$dest"; then
    echo "[WARN] stx-java-proxy.sh not on release $tag; fetched scripts/stx-java-proxy.sh@$tag" >&2
  elif [[ -n "$root_dir" && -f "$root_dir/scripts/stx-java-proxy.sh" ]]; then
    cp "$root_dir/scripts/stx-java-proxy.sh" "$dest"
    echo "[WARN] stx-java-proxy.sh not on release $tag nor at raw @$tag; using bundled copy from $root_dir" >&2
  else
    echo "[WARN] stx-java-proxy.sh unavailable (release asset and repo fallback failed); agents cannot fetch it" >&2
    return 1
  fi
  chmod +x "$dest"
}

# 离线模式禁止外网：检测后直接失败。/ Fail fast when offline mode would hit the network.
stx_assert_online_allowed() {
  if [[ "${STX_OFFLINE:-false}" == "true" || "${STX_OFFLINE:-0}" == "1" ]]; then
    echo "[ERROR] offline mode forbids network download" >&2
    return 1
  fi
}

# ============================================================================
# 安装器 UX：分步日志、交互（/dev/tty）、下载线路测速与选择、离线整包。
# Installer UX: numbered steps, /dev/tty prompts, mirror probing/selection, whole offline bundles.
# ============================================================================

: "${STX_STEP_TOTAL:=0}"
: "${STX_STEP_INDEX:=0}"
STX_STEP_NAME="${STX_STEP_NAME:-}"
STX_STEP_PREFIX="${STX_STEP_PREFIX:-}"

# 颜色仅在终端且未设置 NO_COLOR 时启用。/ Colors only on a terminal and when NO_COLOR is unset.
stx_init_colors() {
  if [[ -z "${NO_COLOR:-}" && -t 2 ]]; then
    STX_C_BOLD=$'\033[1m'; STX_C_GREEN=$'\033[32m'; STX_C_RED=$'\033[31m'
    STX_C_YELLOW=$'\033[33m'; STX_C_CYAN=$'\033[36m'; STX_C_RESET=$'\033[0m'
  else
    STX_C_BOLD=""; STX_C_GREEN=""; STX_C_RED=""; STX_C_YELLOW=""; STX_C_CYAN=""; STX_C_RESET=""
  fi
  export STX_C_BOLD STX_C_GREEN STX_C_RED STX_C_YELLOW STX_C_CYAN STX_C_RESET
}
[[ -n "${STX_C_RESET+x}" ]] || stx_init_colors

# 把本脚本后续输出同时写入日志文件（父进程已 tee 时不重复）。
# Tee all further output into a log file (skipped when a parent already tees).
# 用法 / Usage: stx_setup_log <default-log-path>
stx_setup_log() {
  if [[ "${STX_LOG_TEED:-}" == "1" && -n "${STX_INSTALL_LOG:-}" ]]; then
    return 0
  fi
  STX_INSTALL_LOG="${STX_INSTALL_LOG:-$1}"
  mkdir -p "$(dirname "$STX_INSTALL_LOG")" 2>/dev/null || true
  if ! : >>"$STX_INSTALL_LOG" 2>/dev/null; then
    STX_INSTALL_LOG="${TMPDIR:-/tmp}/$(basename "$STX_INSTALL_LOG")"
    : >>"$STX_INSTALL_LOG" 2>/dev/null || { STX_INSTALL_LOG=""; return 0; }
  fi
  export STX_INSTALL_LOG
  export STX_LOG_TEED=1
  exec > >(tee -a "$STX_INSTALL_LOG") 2>&1
}

# 开始一个编号步骤：[3/8] 解压安装包...  / Start a numbered step.
stx_step() {
  STX_STEP_INDEX=$((STX_STEP_INDEX + 1))
  STX_STEP_NAME="$1"
  echo
  echo "${STX_C_BOLD}${STX_C_CYAN}${STX_STEP_PREFIX}[${STX_STEP_INDEX}/${STX_STEP_TOTAL}] ${STX_STEP_NAME}...${STX_C_RESET}"
}

# 当前步骤成功。/ Mark the current step as done.
stx_step_ok() {
  echo "${STX_C_GREEN}  ✔ ${STX_STEP_PREFIX}[${STX_STEP_INDEX}/${STX_STEP_TOTAL}] ${STX_STEP_NAME}${1:+：$1}${STX_C_RESET}"
}

stx_info() { echo "  $*"; }
stx_warn() { echo "${STX_C_YELLOW}  [WARN] $*${STX_C_RESET}" >&2; }
stx_err() { echo "${STX_C_RED}  [ERROR] $*${STX_C_RESET}" >&2; }

# EXIT trap 里调用：非 0 退出时说明哪一步失败、日志在哪。/ From an EXIT trap: report the failed step and log.
stx_report_failure() {
  local rc="$1" what="${2:-安装}"
  [[ "$rc" -eq 0 ]] && return 0
  echo >&2
  if [[ "$STX_STEP_INDEX" -gt 0 ]]; then
    echo "${STX_C_RED}${STX_C_BOLD}✘ ${what}失败：第 ${STX_STEP_INDEX}/${STX_STEP_TOTAL} 步「${STX_STEP_NAME}」出错（退出码 ${rc}）${STX_C_RESET}" >&2
  else
    echo "${STX_C_RED}${STX_C_BOLD}✘ ${what}失败（退出码 ${rc}）${STX_C_RESET}" >&2
  fi
  if [[ -n "${STX_INSTALL_LOG:-}" ]]; then
    echo "  完整日志 / full log: ${STX_INSTALL_LOG}" >&2
  fi
}

# 路径（或其最近的已存在上级目录）当前用户可写。/ The path, or its nearest existing parent, is writable by us.
stx_path_writable() {
  local p="$1"
  [[ "$p" == /* ]] || p="$(pwd)/$p"
  while [[ ! -e "$p" && "$p" != / ]]; do p="$(dirname "$p")"; done
  [[ -d "$p" && -w "$p" && -x "$p" ]]
}

# 非 root 是否允许安装：必须显式指定可写的安装目录且 --no-systemd（注册 systemd 服务、默认 /opt/stx、
# --service-user 的 chown 都需要 root）。
# Non-root install is allowed only with an explicit writable install dir AND --no-systemd (the systemd unit,
# the default /opt/stx and the --service-user chown all need root).
# 用法 / Usage: stx_nonroot_install_ok <dir-explicit:true|false> <install-dir> <no-systemd:true|false> <service-user>
stx_nonroot_install_ok() {
  [[ "$1" == true && "$3" == true && -z "$4" ]] && stx_path_writable "$2"
}

# 打印带 sudo 的重跑命令：原参数逐个按 shell 规则加引号，密码值一律替换为 ******。
# Print the sudo re-run command: original args are shell-quoted one by one, password values become ******.
# 用法 / Usage: stx_sudo_rerun_cmd <command prefix, e.g. "sudo ./install.sh"> [original args...]
stx_sudo_rerun_cmd() {
  local out="$1" a mask=0
  shift
  for a in "$@"; do
    if [[ "$mask" == 1 ]]; then out="$out ******"; mask=0; continue; fi
    case "$a" in
      --admin-password|--db-password) out="$out $a"; mask=1 ;;
      --admin-password=*|--db-password=*) out="$out ${a%%=*}=******" ;;
      *) out="$out $(printf '%q' "$a")" ;;
    esac
  done
  printf '%s\n' "$out"
}

# 安装需要 root：不是 root 且不满足非 root 条件时，在任何提问 / 检查之前给出 sudo 命令并退出 1。
# Installs need root: unless the non-root conditions hold, print the sudo command and exit 1 before any prompt.
# 用法 / Usage: stx_require_root <command prefix> <dir-explicit> <install-dir> <no-systemd> <service-user> [args...]
stx_require_root() {
  local prefix="$1" explicit="$2" dir="$3" nosd="$4" svc="$5" cmd
  shift 5
  [[ "$(id -u)" -eq 0 ]] && return 0
  stx_nonroot_install_ok "$explicit" "$dir" "$nosd" "$svc" && return 0
  cmd="$(stx_sudo_rerun_cmd "$prefix" "$@")"
  {
    echo "${STX_C_RED}  [ERROR] 安装需要 root 权限 / installation requires root${STX_C_RESET}"
    echo "  请用 sudo 执行：${cmd}"
    [[ "$cmd" == *"******"* ]] && echo "  （命令中的密码已隐藏，请把 ****** 换成实际密码 / passwords are hidden: replace ******）"
    echo "  不用 root 安装：同时指定当前用户可写的 --install-dir <目录> 与 --no-systemd（不注册系统服务）"
    echo "  non-root: pass a writable --install-dir <dir> together with --no-systemd (no system service)"
  } >&2
  exit 1
}

# 是否可以交互：未设置 -y/STX_ASSUME_YES，且 /dev/tty 可读写。
# Interactive only without -y/STX_ASSUME_YES and with a readable /dev/tty (works under curl | bash).
# 终端读到 EOF 后的标记文件（$$ 在子 shell 中不变，命令替换里也能看到）。
# Marker set once /dev/tty hits EOF ($$ is stable in subshells, so command substitutions see it too).
stx_tty_eof_flag() { echo "${TMPDIR:-/tmp}/.stx-tty-eof-$$"; }

stx_is_interactive() {
  [[ "${STX_ASSUME_YES:-false}" == "true" || "${STX_ASSUME_YES:-0}" == "1" ]] && return 1
  [[ "${STX_NONINTERACTIVE:-}" == "1" || "${STX_NONINTERACTIVE:-}" == "true" ]] && return 1
  # CI 环境（GitHub Actions 等设置 CI=true）一律不提问。/ CI environments never prompt.
  [[ -n "${CI:-}" && "${CI:-}" != "false" && "${CI:-}" != "0" ]] && return 1
  [[ -e "$(stx_tty_eof_flag)" ]] && return 1
  { : </dev/tty >/dev/tty; } 2>/dev/null
}

# 终端已关闭（EOF）：记下标记，之后按非交互处理。/ The terminal hit EOF: remember it and behave non-interactively.
stx_mark_tty_eof() {
  : >"$(stx_tty_eof_flag)" 2>/dev/null || true
  { echo; echo "  [WARN] 终端输入已结束，其余问题使用默认值 / terminal input closed, using defaults for the rest"; } >/dev/tty 2>/dev/null || true
}

# 从 /dev/tty 读一行（带提示），回车返回默认值。/ Read a line from /dev/tty; Enter returns the default.
# 用法 / Usage: v="$(stx_prompt "提示" "默认值" [timeout-seconds])"
stx_prompt() {
  local label="$1" def="${2:-}" timeout="${3:-0}" ans=""
  if ! stx_is_interactive; then
    echo "$def"
    return 0
  fi
  printf '%s' "${STX_C_BOLD}  ? ${label}${STX_C_RESET} " >/dev/tty
  local rc=0
  if [[ "$timeout" -gt 0 ]]; then
    IFS= read -r -t "$timeout" ans </dev/tty || rc=$?
  else
    IFS= read -r ans </dev/tty || rc=$?
  fi
  if [[ "$rc" -gt 128 ]]; then
    echo >/dev/tty; ans=""          # 超时：用默认值。/ Timeout: use the default.
  elif [[ "$rc" -ne 0 ]]; then
    [[ -z "$ans" ]] && stx_mark_tty_eof   # EOF：用默认值并切换为非交互。/ EOF: default, then non-interactive.
  fi
  ans="${ans#"${ans%%[![:space:]]*}"}"
  ans="${ans%"${ans##*[![:space:]]}"}"
  [[ -z "$ans" ]] && ans="$def"
  echo "$ans"
}

# 从 /dev/tty 读密码（不回显）。/ Read a secret from /dev/tty without echo.
stx_prompt_secret() {
  local label="$1" ans=""
  if ! stx_is_interactive; then
    return 0
  fi
  printf '%s' "${STX_C_BOLD}  ? ${label}${STX_C_RESET} " >/dev/tty
  if ! IFS= read -r -s ans </dev/tty && [[ -z "$ans" ]]; then
    stx_mark_tty_eof
    return 0
  fi
  echo >/dev/tty
  printf '%s' "$ans"
}

# ---------------------------------------------------------------------------
# 下载线路（gh-proxy 加速 + GitHub 直连）。/ Download lines (gh-proxy accelerators + direct GitHub).
# 每行：id|前缀|说明。/ Each entry: id|prefix|label.
# ---------------------------------------------------------------------------
STX_MIRROR_LINES=(
  "gh-proxy|https://gh-proxy.org|Cloudflare (v4) 主站加速"
  "v4|https://v4.gh-proxy.org|Cloudflare (v4) 优选，推荐，仅 IPv4"
  "v6|https://v6.gh-proxy.org|Cloudflare (v4/v6) 优选，支持 IPv6"
  "cdn|https://cdn.gh-proxy.org|Fastly (v4) CDN"
  "axisnow|https://axisnow.gh-proxy.org|AxisNow (v4) 三网优选，仅 IPv4"
  "github||GitHub 直连"
)

stx_mirror_field() { # <index0> <1=id|2=prefix|3=label>
  local entry="${STX_MIRROR_LINES[$1]}"
  echo "$entry" | cut -d'|' -f"$2"
}

# 把 --mirror / STX_MIRROR 的值解析成线路下标（0 起）；自定义 URL 返回 "custom:<url>"；auto/空 返回空。
# Resolve a --mirror / STX_MIRROR value into a line index (0-based); custom URL => "custom:<url>"; auto/empty => "".
stx_mirror_resolve() {
  local want="${1:-}" i id prefix
  want="${want%/}"
  case "$want" in
    ""|auto) echo ""; return 0 ;;
    direct|none|https://github.com) want="github" ;;
  esac
  if [[ "$want" =~ ^[0-9]+$ ]]; then
    if (( want >= 1 && want <= ${#STX_MIRROR_LINES[@]} )); then
      echo $((want - 1)); return 0
    fi
    echo "[ERROR] invalid mirror number: $want (1-${#STX_MIRROR_LINES[@]})" >&2
    return 1
  fi
  for i in "${!STX_MIRROR_LINES[@]}"; do
    id="$(stx_mirror_field "$i" 1)"
    prefix="$(stx_mirror_field "$i" 2)"
    if [[ "$want" == "$id" || ( -n "$prefix" && "$want" == "$prefix" ) ]]; then
      echo "$i"; return 0
    fi
  done
  if [[ "$want" =~ ^https?:// ]]; then
    echo "custom:$want"; return 0
  fi
  echo "[ERROR] unknown mirror: $want (use 1-${#STX_MIRROR_LINES[@]}, gh-proxy|v4|v6|cdn|axisnow|github, or a URL prefix)" >&2
  return 1
}

# 探测单条线路：小范围 Range 请求，输出 "<ok:0|1> <首字节毫秒> <KB/s>"。
# Probe one line with a small ranged request; prints "<ok:0|1> <ttfb-ms> <KB/s>".
stx_mirror_probe_one() {
  local url="$1" out code ttfb speed size
  out="$(curl -sS -L -o /dev/null -r 0-524287 --connect-timeout "${STX_MIRROR_CONNECT_TIMEOUT:-3}" \
    -m "${STX_MIRROR_PROBE_TIMEOUT:-6}" -w '%{http_code} %{time_starttransfer} %{speed_download} %{size_download}' \
    "$url" 2>/dev/null || true)"
  read -r code ttfb speed size <<<"${out:-000 0 0 0}"
  if [[ "$code" =~ ^(200|206)$ && "${size%.*}" -gt 0 ]]; then
    awk -v t="$ttfb" -v s="$speed" 'BEGIN { printf "1 %d %d\n", t * 1000, s / 1024 }'
  else
    echo "0 0 0"
  fi
}

# 测速所有线路并确定下载顺序，结果写入 STX_MIRROR_ORDER（前缀列表，空串=直连）。
# Probe every line and fix the download order into STX_MIRROR_ORDER (prefixes; empty string = direct).
# 用法 / Usage: stx_choose_mirror <github-release-url-of-a-probe-asset> [mirror-preference]
stx_choose_mirror() {
  local raw_url="$1" pref="${2:-${STX_MIRROR:-}}" forced="" i n ok ms kbs best="" best_kbs=-1 url label id prefix choice
  STX_MIRROR_ORDER=()
  n="${#STX_MIRROR_LINES[@]}"

  if [[ -z "$pref" && -n "${STX_DOWNLOAD_MIRROR_PREFIX:-}" ]]; then
    pref="${STX_DOWNLOAD_MIRROR_PREFIX}"
  fi
  forced="$(stx_mirror_resolve "$pref")" || return 1
  if [[ "$forced" == custom:* ]]; then
    stx_info "使用自定义下载前缀 / custom mirror: ${forced#custom:}"
    STX_MIRROR_ORDER=("${forced#custom:}")
    for i in $(seq 0 $((n - 1))); do STX_MIRROR_ORDER+=("$(stx_mirror_field "$i" 2)"); done
    STX_SELECTED_MIRROR_PREFIX="${STX_MIRROR_ORDER[0]}"
    export STX_SELECTED_MIRROR_PREFIX
    return 0
  fi

  local -a oks=() kbss=()
  stx_info "测速各下载线路（每条最多 ${STX_MIRROR_PROBE_TIMEOUT:-6}s）/ probing download lines:"
  for i in $(seq 0 $((n - 1))); do
    prefix="$(stx_mirror_field "$i" 2)"
    label="$(stx_mirror_field "$i" 3)"
    url="$raw_url"
    [[ -n "$prefix" ]] && url="${prefix}/${raw_url}"
    read -r ok ms kbs <<<"$(stx_mirror_probe_one "$url")"
    oks[i]="$ok"; kbss[i]="$kbs"
    if [[ "$ok" == "1" ]]; then
      printf '    %d) %-30s %-34s %6s ms  %8s KB/s\n' $((i + 1)) "${prefix:-https://github.com}" "$label" "$ms" "$kbs"
      if (( kbs > best_kbs )); then best_kbs="$kbs"; best="$i"; fi
    else
      printf '    %d) %-30s %-34s %s\n' $((i + 1)) "${prefix:-https://github.com}" "$label" "不可用 / unreachable"
    fi
  done

  if [[ -n "$forced" ]]; then
    choice="$forced"
    stx_info "按 --mirror/STX_MIRROR 使用线路 $((choice + 1))：$(stx_mirror_field "$choice" 3)"
  else
    [[ -z "$best" ]] && best=1  # 全部失败时先试推荐线路 v4。/ All failed: try the recommended v4 line first.
    choice="$best"
    if stx_is_interactive; then
      local ans
      while true; do
        ans="$(stx_prompt "选择下载线路编号 [回车=最快的 $((best + 1))]:" "$((best + 1))" 30)"
        if [[ "$ans" =~ ^[0-9]+$ ]] && (( ans >= 1 && ans <= n )); then choice=$((ans - 1)); break; fi
        stx_is_interactive || break
        echo "    请输入 1-${n} 之间的编号" >/dev/tty
      done
    fi
    stx_info "使用线路 $((choice + 1))：$(stx_mirror_field "$choice" 3) ${STX_C_BOLD}$(stx_mirror_field "$choice" 2)${STX_C_RESET}"
  fi

  # 下载顺序：所选 → 其余可用（按速度降序）→ 其余不可用。/ Order: chosen -> other OK lines by speed -> the rest.
  STX_MIRROR_ORDER=("$(stx_mirror_field "$choice" 2)")
  local -a rest=()
  for i in $(seq 0 $((n - 1))); do
    [[ "$i" == "$choice" ]] && continue
    [[ "${oks[i]}" == "1" ]] && rest+=("${kbss[i]} $i")
  done
  if [[ ${#rest[@]} -gt 0 ]]; then
    while read -r _ i; do STX_MIRROR_ORDER+=("$(stx_mirror_field "$i" 2)"); done < <(printf '%s\n' "${rest[@]}" | sort -rn)
  fi
  for i in $(seq 0 $((n - 1))); do
    [[ "$i" == "$choice" || "${oks[i]}" == "1" ]] && continue
    STX_MIRROR_ORDER+=("$(stx_mirror_field "$i" 2)")
  done
  STX_SELECTED_MIRROR_PREFIX="${STX_MIRROR_ORDER[0]}"
  export STX_SELECTED_MIRROR_PREFIX
}

# 带进度条下载（单条线路）。/ Download with a progress bar (one line).
stx_download_progress() {
  local url="$1" dest="$2"
  mkdir -p "$(dirname "$dest")"
  curl -fL --retry 2 --retry-delay 2 --connect-timeout 10 --speed-limit 1024 --speed-time 60 \
    --progress-bar -o "$dest" "$url"
}

# 按 STX_MIRROR_ORDER 依次尝试下载，失败自动换下一条线路。/ Try lines in STX_MIRROR_ORDER, falling back on failure.
# 用法 / Usage: stx_download_with_fallback <github-url> <dest> [quiet]
stx_download_with_fallback() {
  local raw_url="$1" dest="$2" quiet="${3:-}" prefix url tried=0
  if [[ ${#STX_MIRROR_ORDER[@]} -eq 0 ]]; then
    STX_MIRROR_ORDER=("")
  fi
  for prefix in "${STX_MIRROR_ORDER[@]}"; do
    url="$raw_url"
    [[ -n "$prefix" ]] && url="${prefix%/}/${raw_url}"
    tried=$((tried + 1))
    [[ "$tried" -gt 1 ]] && stx_warn "上一条线路失败，切换到 / falling back to: ${prefix:-https://github.com}"
    rm -f "$dest"
    if [[ -n "$quiet" ]]; then
      if curl -fsSL --retry 2 --retry-delay 1 --connect-timeout 10 -m 120 -o "$dest" "$url" 2>/dev/null; then
        return 0
      fi
    else
      stx_info "下载 / downloading: $url"
      if stx_download_progress "$url" "$dest"; then
        STX_SELECTED_MIRROR_PREFIX="$prefix"
        return 0
      fi
    fi
  done
  rm -f "$dest"
  stx_err "所有线路均下载失败 / all download lines failed: $raw_url"
  return 1
}

# ---------------------------------------------------------------------------
# 离线整包。/ Whole offline bundles.
# 变体：amd64（官方 Node，glibc ≥ 2.28）、arm64（官方 Node，glibc ≥ 2.28）、centos7（amd64 + Node 18 glibc217）。
# Variants: amd64 (official Node, glibc >= 2.28), arm64 (official Node), centos7 (amd64 + Node 18 glibc217).
# ---------------------------------------------------------------------------

# glibc 版本是否低于 2.28。/ True when glibc is older than 2.28.
stx_glibc_lt_228() {
  local ver major minor
  ver="${1:-$(stx_detect_glibc_version)}"
  [[ -z "$ver" ]] && return 1
  major="${ver%%.*}"; minor="${ver#*.}"; minor="${minor%%.*}"
  (( major < 2 || (major == 2 && minor < 28) ))
}

# 自动判定离线包变体。/ Auto-detect the bundle variant for this host.
stx_detect_bundle_variant() {
  local arch="${1:-}"
  [[ -z "$arch" ]] && { arch="$(stx_detect_arch)" || return 1; }
  if [[ "$arch" == "amd64" ]] && stx_glibc_lt_228 ""; then
    echo "centos7"
  else
    echo "$arch"
  fi
}

# 变体 → 架构。/ Variant -> CPU arch.
stx_bundle_variant_arch() {
  case "$1" in
    amd64|centos7) echo "amd64" ;;
    arm64) echo "arm64" ;;
    *) echo "[ERROR] unknown bundle variant: $1 (amd64|arm64|centos7)" >&2; return 1 ;;
  esac
}

# 变体 → 文件名中的平台段。/ Variant -> platform segment in file names.
stx_bundle_platform() {
  case "$1" in
    amd64) echo "linux-amd64" ;;
    arm64) echo "linux-arm64" ;;
    centos7) echo "linux-amd64-centos7" ;;
    *) echo "[ERROR] unknown bundle variant: $1 (amd64|arm64|centos7)" >&2; return 1 ;;
  esac
}

# 版本化包名：stx-<ver>-linux-amd64-offline.tar.gz。/ Versioned bundle asset name.
stx_bundle_asset_name() {
  local ver="$1" variant="$2" plat
  plat="$(stx_bundle_platform "$variant")" || return 1
  echo "stx-${ver}-${plat}-offline.tar.gz"
}

# 固定别名（供 releases/latest/download 直链）：stx-offline-linux-amd64.tar.gz。
# Stable alias for releases/latest/download links.
stx_bundle_alias_name() {
  local plat
  plat="$(stx_bundle_platform "$1")" || return 1
  echo "stx-offline-${plat}.tar.gz"
}

# 解析 Release 中离线包的 GitHub URL（latest 用固定别名，不需要调用 API）。
# GitHub URL of the release bundle (latest uses the stable alias, no API call needed).
# 用法 / Usage: stx_bundle_release_url <tag|latest> <variant> [asset-name-override]
stx_bundle_release_url() {
  local tag="$1" variant="$2" asset="${3:-}"
  if [[ -z "$asset" ]]; then
    if [[ "$tag" == "latest" ]]; then
      asset="$(stx_bundle_alias_name "$variant")" || return 1
    else
      asset="$(stx_bundle_asset_name "$tag" "$variant")" || return 1
    fi
  fi
  stx_build_github_release_url "$tag" "$asset" ""
}

# 打印 6 条线路的 wget 命令。/ Print wget commands for all 6 lines.
stx_print_wget_lines() {
  local raw_url="$1" i prefix label
  for i in "${!STX_MIRROR_LINES[@]}"; do
    prefix="$(stx_mirror_field "$i" 2)"
    label="$(stx_mirror_field "$i" 3)"
    echo "# $((i + 1))) ${label}"
    if [[ -n "$prefix" ]]; then
      echo "wget \"${prefix}/${raw_url}\""
    else
      echo "wget \"${raw_url}\""
    fi
  done
}

# 可用磁盘空间（MB）。/ Free disk space in MB for a path.
stx_free_mb() {
  local p="$1"
  while [[ ! -d "$p" && "$p" != "/" ]]; do p="$(dirname "$p")"; done
  df -Pm "$p" 2>/dev/null | awk 'NR==2 {print $4}'
}

# 下载并校验离线整包（测速选线 → 下载 .sha256 → 带进度下载 → 校验），结果写入 STX_BUNDLE_FILE。
# Download and verify a whole offline bundle (probe lines -> .sha256 -> download with progress -> verify).
# 调用方需先设置 STX_STEP_TOTAL；本函数占 3 步。/ Caller sets STX_STEP_TOTAL; this function uses 3 steps.
# 用法 / Usage: stx_fetch_bundle <tag|latest> <variant> <dest-dir> [asset-name-override]
stx_fetch_bundle() {
  local tag="$1" variant="$2" dest_dir="$3" asset="${4:-}" url name free
  url="$(stx_bundle_release_url "$tag" "$variant" "$asset")" || return 1
  name="${url##*/}"
  mkdir -p "$dest_dir"

  stx_step "选择下载线路 / choosing a download line"
  stx_info "离线包 / bundle: $name"
  stx_choose_mirror "$url" || return 1
  stx_step_ok "${STX_SELECTED_MIRROR_PREFIX:-https://github.com}"

  stx_step "下载离线包 / downloading the bundle"
  free="$(stx_free_mb "$dest_dir")"
  if [[ -n "$free" && "$free" -lt 1500 ]]; then
    stx_warn "$dest_dir 可用空间只有 ${free} MB，离线包约 500 MB，解压安装需要约 1.5 GB"
  fi
  if ! stx_download_with_fallback "${url}.sha256" "$dest_dir/${name}.sha256" quiet; then
    stx_err "无法下载 ${name}.sha256：该版本可能没有离线包（v1.2.0 及更早版本没有），请用 --version 指定更新的版本"
    return 1
  fi
  stx_download_with_fallback "$url" "$dest_dir/$name" || return 1
  stx_step_ok "$dest_dir/$name ($(( $(wc -c <"$dest_dir/$name") / 1024 / 1024 )) MB)"

  stx_step "校验 sha256 / verifying sha256"
  stx_verify_sha256 "$dest_dir/$name" "$dest_dir/${name}.sha256" || return 1
  stx_step_ok
  STX_BUNDLE_FILE="$dest_dir/$name"
  export STX_BUNDLE_FILE
}

