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

# 根据 glibc 选择 node variant：<2.27 用 glibc217。/ Pick node variant from glibc (<2.27 => glibc217).
stx_recommend_node_variant() {
  local ver
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
  minor="${v#*.}"
  minor="${minor%%.*}"
  [[ "$major" =~ ^[0-9]+$ ]] || return 1
  [[ "$minor" =~ ^[0-9]+$ ]] || minor=0
  if (( major > 18 )) || { (( major == 18 )) && (( minor >= 18 )); }; then
    return 0
  fi
  return 1
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
