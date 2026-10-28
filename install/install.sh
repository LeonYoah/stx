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
STX installer（离线包 / offline bundle）

Usage:
  sudo ./install.sh [options]
  需要 root；不用 root 时须同时指定可写的 --install-dir 与 --no-systemd
  needs root; non-root only with a writable --install-dir plus --no-systemd

Options:
  -y, --yes                非交互：不提问，全部使用默认值或参数值 / no prompts, use defaults/flags
  --install-dir <path>     安装目录 / install directory (default: /opt/stx)
  --admin-password <pwd>   初始管理员密码（默认 admin123）/ initial admin password (env STX_ADMIN_PASSWORD)
  --frontend-port <port>   Web 控制台端口 / web console port (default: 17880, env STX_FRONTEND_PORT)
  --http-port <port>       后端 API 端口 / backend API port (default: 17800, env STX_HTTP_PORT)
  --grpc-port <port>       Agent gRPC 端口（Agent 回连）/ agent gRPC port (default: 17890, env STX_GRPC_PORT)
  --external-ip <ip|host>  对外访问 IP/域名，写入 app.external_url（默认自动探测，env STX_EXTERNAL_IP）
  --db-type <sqlite|mysql|postgres>  数据库类型（默认 sqlite；postgres 也可写 pg/postgresql，env STX_DB_TYPE）
  --db-host / --db-port / --db-user / --db-password / --db-name   外部数据库连接参数（env STX_DB_*）

以下只能用参数指定（不提问）/ flags only (never prompted):
  --prometheus-port <p>    Prometheus 端口（默认 9090）
  --alertmanager-port <p>  Alertmanager 端口（默认 9093）
  --grafana-port <p>       Grafana 端口（默认 3000）
  --without-node           不用包内 Node，改用本机 Node ≥ 18.18
  --service-user <user>    systemd 服务运行用户（默认 root；会 chown 安装目录）
  --no-autostart           注册 systemd 服务但不设置开机自启
  --offline                只用 ./packages，禁止联网（离线包默认即如此）/ packages only, no network
  --with-observability     启用内置 Prometheus/Alertmanager/Grafana（包内有时默认启用）
  --without-observability  不启用内置三件套 / disable the bundled observability stack
  --force                  先备份已有安装目录再重装 / back up the existing install dir first
  --no-preserve-config     不保留已有 config.yaml / do not keep the existing config.yaml
  --no-start               只安装不启动 / install only
  --no-systemd             不注册 systemd 服务 / skip the systemd unit
  -h, --help               显示帮助 / show this help

Environment:
  STX_ASSUME_YES=true      同 -y / same as -y
  STX_INSTALL_LOG=<file>   安装日志位置 / install log path
  STX_SKIP_GLIBC_CHECK=true  跳过 glibc 与离线包变体的匹配检查 / skip the glibc vs bundle variant check
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
ADMIN_PASSWORD="${STX_ADMIN_PASSWORD:-}"
ARG_HTTP_PORT="${STX_HTTP_PORT:-}"
ARG_FRONTEND_PORT="${STX_FRONTEND_PORT:-}"
ARG_GRPC_PORT="${STX_GRPC_PORT:-}"
ARG_PROM_PORT="${STX_PROMETHEUS_PORT:-}"
ARG_AM_PORT="${STX_ALERTMANAGER_PORT:-}"
ARG_GF_PORT="${STX_GRAFANA_PORT:-}"
ARG_EXTERNAL_IP="${STX_EXTERNAL_IP:-}"
ARG_DB_TYPE="${STX_DB_TYPE:-}"
ARG_DB_HOST="${STX_DB_HOST:-}"
ARG_DB_PORT="${STX_DB_PORT:-}"
ARG_DB_USER="${STX_DB_USER:-}"
ARG_DB_PASSWORD="${STX_DB_PASSWORD:-}"
ARG_DB_NAME="${STX_DB_NAME:-}"
WITHOUT_NODE=false
SERVICE_USER="${STX_SERVICE_USER:-}"
ENABLE_AUTOSTART=true
INSTALL_DIR_EXPLICIT=false
[[ -n "${STX_INSTALL_DIR:-}" ]] && INSTALL_DIR_EXPLICIT=true
ORIG_ARGS=("$@")

while [[ $# -gt 0 ]]; do
  case "$1" in
    -y|--yes) STX_ASSUME_YES=true; export STX_ASSUME_YES; shift ;;
    --install-dir) INSTALL_DIR="${2:-}"; INSTALL_DIR_EXPLICIT=true; shift 2 ;;
    --admin-password) ADMIN_PASSWORD="${2:-}"; shift 2 ;;
    --frontend-port) ARG_FRONTEND_PORT="${2:-}"; shift 2 ;;
    --http-port|--api-port) ARG_HTTP_PORT="${2:-}"; shift 2 ;;
    --grpc-port) ARG_GRPC_PORT="${2:-}"; shift 2 ;;
    --prometheus-port) ARG_PROM_PORT="${2:-}"; shift 2 ;;
    --alertmanager-port) ARG_AM_PORT="${2:-}"; shift 2 ;;
    --grafana-port) ARG_GF_PORT="${2:-}"; shift 2 ;;
    --external-ip|--external-host) ARG_EXTERNAL_IP="${2:-}"; shift 2 ;;
    --db-type) ARG_DB_TYPE="${2:-}"; shift 2 ;;
    --db-host) ARG_DB_HOST="${2:-}"; shift 2 ;;
    --db-port) ARG_DB_PORT="${2:-}"; shift 2 ;;
    --db-user) ARG_DB_USER="${2:-}"; shift 2 ;;
    --db-password) ARG_DB_PASSWORD="${2:-}"; shift 2 ;;
    --db-name) ARG_DB_NAME="${2:-}"; shift 2 ;;
    --without-node) WITHOUT_NODE=true; shift ;;
    --service-user) SERVICE_USER="${2:-}"; shift 2 ;;
    --no-autostart) ENABLE_AUTOSTART=false; shift ;;
    --force) FORCE=true; shift ;;
    --no-preserve-config) PRESERVE_CONFIG=false; shift ;;
    --no-start) AUTO_START=false; shift ;;
    --offline) OFFLINE=true; STX_OFFLINE=true; shift ;;
    --with-observability) WITH_OBS=true; shift ;;
    --without-observability) WITH_OBS=false; shift ;;
    --no-systemd) SKIP_SYSTEMD=true; STX_SKIP_SYSTEMD=true; shift ;;
    -h|--help) usage; exit 0 ;;
    --mirror|--mirror=*|--variant|--variant=*|--version|--version=*|--bundle-file|--bundle-file=*)
      echo "[ERROR] $1 只用于下载（install-online.sh / download-bundle.sh），install.sh 安装的是当前目录的离线包" >&2
      echo "        $1 is a download option of install-online.sh / download-bundle.sh; install.sh installs this bundle" >&2
      exit 1 ;;
    *) echo "[ERROR] unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$INSTALL_DIR" ]]; then
  echo "[ERROR] --install-dir cannot be empty" >&2
  exit 1
fi
if [[ -n "$ARG_DB_TYPE" ]]; then
  ARG_DB_TYPE="$(stx_parse_db_type "$ARG_DB_TYPE")" || {
    echo "[ERROR] unsupported --db-type: ${STX_DB_TYPE:-$ARG_DB_TYPE} (sqlite|mysql|postgres)" >&2
    exit 1
  }
fi

# 先判断权限，再做任何检查或提问。/ Check privileges before any pre-flight check or prompt.
_stx_self="$0"
[[ "$_stx_self" == */* ]] || _stx_self="./$_stx_self"
if [[ -x "$_stx_self" ]]; then _stx_self="$(printf '%q' "$_stx_self")"; else _stx_self="bash $(printf '%q' "$_stx_self")"; fi
stx_require_root "sudo $_stx_self" "$INSTALL_DIR_EXPLICIT" "$INSTALL_DIR" "$SKIP_SYSTEMD" "$SERVICE_USER" \
  ${ORIG_ARGS[@]+"${ORIG_ARGS[@]}"}
unset _stx_self

stx_setup_log "${TMPDIR:-/tmp}/stx-install-$(date +%Y%m%d-%H%M%S).log"

STAGE_DIR=""
STX_STEP_TOTAL=7
on_exit() {
  local rc=$?
  if [[ -n "$STAGE_DIR" && -d "$STAGE_DIR" ]]; then
    rm -rf "$STAGE_DIR"
  fi
  stx_report_failure "$rc" "安装"
  rm -f "$(stx_tty_eof_flag)" 2>/dev/null || true
  if [[ -n "${STX_INSTALL_LOG:-}" && -d "$INSTALL_DIR/logs" ]]; then
    cp "$STX_INSTALL_LOG" "$INSTALL_DIR/logs/install.log" 2>/dev/null || true
  fi
}
trap on_exit EXIT

echo "${STX_C_BOLD}STX 安装程序 / installer${STX_C_RESET}  安装目录 / install dir: $INSTALL_DIR"
[[ -n "${STX_INSTALL_LOG:-}" ]] && echo "安装日志 / install log: $STX_INSTALL_LOG"

# 返回目录下第一个子目录（按名称排序，含隐藏目录；不跟随软链接）。/ Print the first subdirectory (sorted, incl. dot dirs; symlinks skipped).
stx_first_subdir() {
  local parent="$1" d
  local -a cands=()
  shopt -s nullglob dotglob
  cands=("$parent"/*/)
  shopt -u nullglob dotglob
  for d in ${cands[@]+"${cands[@]}"}; do
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

# ---------------------------------------------------------------------------
stx_step "安装前检查 / pre-flight checks"
# 前置依赖检查：缺命令时直接给出清晰错误（不需要 python3）。/ Upfront dependency check (no python3 needed).
stx_require_cmds tar gzip find awk sed grep mktemp cp mv || exit 1
arch="$(stx_detect_arch)"
host_glibc="$(stx_detect_glibc_version)"
stx_info "CPU: $arch   glibc: ${host_glibc:-unknown}"

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
  stx_err "install.sh must run from package root, offline bundle, or bin/."
  exit 1
fi

if [[ "$OFFLINE" == "true" ]]; then
  if [[ -z "$PACKAGES_DIR" && -d "$SOURCE_DIR/packages" ]]; then
    PACKAGES_DIR="$SOURCE_DIR/packages"
  fi
  if [[ -z "$PACKAGES_DIR" ]]; then
    stx_err "--offline requires packages/ directory"
    exit 1
  fi
fi

# 离线包变体与本机 glibc 匹配：官方 Node 需要 glibc ≥ 2.28，CentOS 7 等老系统要用 centos7 包。
# Bundle variant vs host glibc: official Node needs glibc >= 2.28; old hosts (CentOS 7) need the centos7 bundle.
bundle_variant=""
bundle_min_glibc=""
if [[ -f "$SOURCE_DIR/MANIFEST.json" ]]; then
  bundle_variant="$(tr -d '\r\n' <"$SOURCE_DIR/MANIFEST.json" | grep -o '"variant"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/' || true)"
  bundle_min_glibc="$(tr -d '\r\n' <"$SOURCE_DIR/MANIFEST.json" | grep -o '"min_glibc"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/' || true)"
  stx_info "离线包 / bundle: variant=${bundle_variant:-unknown} min_glibc=${bundle_min_glibc:-unknown}"
fi
if [[ "${STX_SKIP_GLIBC_CHECK:-false}" != "true" && "$bundle_min_glibc" == "2.28" ]] && stx_glibc_lt_228 "$host_glibc"; then
  stx_err "本机 glibc ${host_glibc} < 2.28，此离线包内置的官方 Node 无法运行。"
  stx_err "请改用 CentOS 7 离线包：stx-<版本>-linux-amd64-centos7-offline.tar.gz（或设置 STX_SKIP_GLIBC_CHECK=true 跳过检查）"
  exit 1
fi
if [[ -n "$bundle_variant" && "$(stx_bundle_variant_arch "$bundle_variant" 2>/dev/null || echo "")" != "$arch" ]]; then
  stx_err "离线包架构（$bundle_variant）与本机 CPU（$arch）不一致 / bundle arch does not match this host"
  exit 1
fi

# 已有安装：沿用其配置中的端口 / IP 作为默认值；若正在运行，其占用的端口不算冲突（复制前会先停止）。
# Existing install: reuse its ports / IP as defaults; ports held by the running install are not conflicts.
EXISTING_CONFIG=""
if [[ -f "$INSTALL_DIR/config.yaml" && "$PRESERVE_CONFIG" == "true" ]]; then
  EXISTING_CONFIG="$INSTALL_DIR/config.yaml"
fi
stx_load_install_defaults "$INSTALL_DIR" "$EXISTING_CONFIG"
OWN_PORTS=""
if stx_install_running "$INSTALL_DIR"; then
  OWN_PORTS="$STX_PREV_PORTS"
  stx_info "检测到 $INSTALL_DIR 中的 STX 正在运行，安装时会先停止它 / the running install will be stopped first"
fi

WITH_OBS_EFFECTIVE=false
if [[ "$WITH_OBS" == "true" ]] || { [[ "$WITH_OBS" == "auto" ]] && compgen -G "$SOURCE_DIR/packages/observability-*-linux-${arch}.tar.gz" >/dev/null; }; then
  WITH_OBS_EFFECTIVE=true
fi
[[ "$WITH_OBS" == "auto" && -z "$PACKAGES_DIR" && -x "$SOURCE_DIR/observability/prometheus/prometheus" ]] && WITH_OBS_EFFECTIVE=true

# 端口：参数 > 已有安装 > 默认。/ Ports: flag > existing install > default.
PORT_KEYS=(frontend http grpc)
[[ "$WITH_OBS_EFFECTIVE" == "true" ]] && PORT_KEYS+=(prometheus alertmanager grafana)
declare -A PORT_LABEL=(
  [frontend]="Web 控制台端口 / web console port"
  [http]="后端 API 端口 / backend API port"
  [grpc]="Agent gRPC 端口（Agent 回连）/ agent gRPC port"
  [prometheus]="Prometheus 端口"
  [alertmanager]="Alertmanager 端口"
  [grafana]="Grafana 端口"
)
declare -A PORT_FLAG=(
  [frontend]="$ARG_FRONTEND_PORT" [http]="$ARG_HTTP_PORT" [grpc]="$ARG_GRPC_PORT"
  [prometheus]="$ARG_PROM_PORT" [alertmanager]="$ARG_AM_PORT" [grafana]="$ARG_GF_PORT"
)
declare -A PORT_VAL=(
  [frontend]="$STX_PREV_FRONTEND_PORT" [http]="$STX_PREV_HTTP_PORT" [grpc]="$STX_PREV_GRPC_PORT"
  [prometheus]="$STX_PREV_PROM_PORT" [alertmanager]="$STX_PREV_AM_PORT" [grafana]="$STX_PREV_GF_PORT"
)
declare -A PORT_CONFLICT=()
for k in "${PORT_KEYS[@]}"; do
  [[ -n "${PORT_FLAG[$k]}" ]] && PORT_VAL[$k]="${PORT_FLAG[$k]}"
done

preflight_fail=0
echo "  检查结果 / results:"
printf '    %s %s：%s\n' "✔" "CPU / glibc" "$arch / ${host_glibc:-unknown}"
for k in "${PORT_KEYS[@]}"; do
  p="${PORT_VAL[$k]}"
  taken=""
  for o in "${PORT_KEYS[@]}"; do [[ "$o" != "$k" ]] && taken="$taken ${PORT_VAL[$o]}"; done
  if [[ " $OWN_PORTS " == *" $p "* ]] && [[ " $taken " != *" $p "* ]]; then
    printf '    %s %s：%s\n' "✔" "${PORT_LABEL[$k]}" "$p（当前 STX 占用，安装时释放）"
  elif msg="$(stx_port_valid_free "$p" "$taken")"; then
    printf '    %s %s：%s\n' "✔" "${PORT_LABEL[$k]}" "$p 空闲 / free"
  else
    printf '    %s %s：%s\n' "✘" "${PORT_LABEL[$k]}" "$msg"
    PORT_CONFLICT[$k]=1
  fi
done

need_mb=700
[[ "$WITH_OBS_EFFECTIVE" == "true" ]] && need_mb=2200
free_install="$(stx_free_mb "$INSTALL_DIR")"
free_tmp="$(stx_free_mb "${TMPDIR:-/tmp}")"
if [[ -n "$free_install" && "$free_install" -lt "$need_mb" && "${STX_SKIP_DISK_CHECK:-false}" != "true" ]]; then
  printf '    %s %s：%s\n' "✘" "磁盘 / disk ($INSTALL_DIR)" "可用 ${free_install} MB，至少需要 ${need_mb} MB"
  preflight_fail=1
else
  printf '    %s %s：%s\n' "✔" "磁盘 / disk ($INSTALL_DIR)" "可用 ${free_install:-?} MB（需要约 ${need_mb} MB）"
fi
if [[ -n "$PACKAGES_DIR" && -n "$free_tmp" && "$free_tmp" -lt "$need_mb" ]]; then
  printf '    %s %s：%s\n' "!" "临时目录 / temp (${TMPDIR:-/tmp})" "可用 ${free_tmp} MB，解压需要约 ${need_mb} MB（可设置 TMPDIR=<其他目录>）"
fi
if [[ "$SKIP_SYSTEMD" == "true" ]]; then
  printf '    %s %s：%s\n' "-" "systemd" "跳过（--no-systemd），用 bin/start.sh 启动"
elif command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
  printf '    %s %s：%s\n' "✔" "systemd" "可用：注册 stx.service$([[ "$ENABLE_AUTOSTART" == "true" ]] && echo "，开机自启")"
else
  printf '    %s %s：%s\n' "-" "systemd" "不可用（容器 / 无 systemd），用 bin/start.sh 启动"
fi
if [[ -n "$SERVICE_USER" ]]; then
  if id "$SERVICE_USER" >/dev/null 2>&1; then
    printf '    %s %s：%s\n' "✔" "服务用户 / service user" "$SERVICE_USER"
  else
    printf '    %s %s：%s\n' "✘" "服务用户 / service user" "用户 $SERVICE_USER 不存在（先 useradd）"
    preflight_fail=1
  fi
fi
if [[ "$preflight_fail" -ne 0 ]]; then
  stx_err "安装前检查未通过，请按上面标 ✘ 的项处理后重试"
  exit 1
fi
if [[ ${#PORT_CONFLICT[@]} -gt 0 ]] && ! stx_is_interactive; then
  stx_err "端口冲突：${!PORT_CONFLICT[*]}。非交互模式下请用参数指定其他端口，例如 --frontend-port / --http-port / --grpc-port / --prometheus-port / --alertmanager-port / --grafana-port"
  exit 1
fi
stx_step_ok "$([[ ${#PORT_CONFLICT[@]} -gt 0 ]] && echo "有 ${#PORT_CONFLICT[@]} 个端口冲突，下一步需要重新指定" || echo "全部通过 / all passed")"

# ---------------------------------------------------------------------------
stx_step "安装配置 / settings"
if [[ ! -d "$INSTALL_DIR" ]]; then
  mkdir -p "$INSTALL_DIR" 2>/dev/null || {
    stx_err "cannot create $INSTALL_DIR, please retry with sudo"
    exit 1
  }
elif [[ ! -w "$INSTALL_DIR" ]]; then
  stx_err "no write permission on $INSTALL_DIR, please retry with sudo"
  exit 1
fi
stx_is_interactive && stx_info "直接回车 = 使用 [ ] 中的默认值 / press Enter to accept the default in [ ]"

# 1) 端口：Web / API / gRPC 每个都可回车确认；三件套端口只在冲突时提问。
#    Ports: web / API / gRPC can each be confirmed with Enter; observability ports are asked only on conflict.
for k in "${PORT_KEYS[@]}"; do
  taken=""
  for o in "${PORT_KEYS[@]}"; do [[ "$o" != "$k" ]] && taken="$taken ${PORT_VAL[$o]}"; done
  own=""
  [[ " $OWN_PORTS " == *" ${PORT_VAL[$k]} "* ]] && own=1
  if [[ -n "${PORT_CONFLICT[$k]:-}" ]]; then
    PORT_VAL[$k]="$(stx_ask_port_forced "${PORT_LABEL[$k]}" "${PORT_VAL[$k]}" "$taken")" \
      || { stx_err "端口冲突：请用 ${PORT_FLAG[$k]} 指定其它端口 / use ${PORT_FLAG[$k]} to pick another port"; exit 1; }
  elif [[ -z "${PORT_FLAG[$k]}" ]] && [[ "$k" == frontend || "$k" == http || "$k" == grpc ]] && stx_is_interactive; then
    PORT_VAL[$k]="$(stx_ask_port_confirm "${PORT_LABEL[$k]}" "${PORT_VAL[$k]}" "$taken" "$own" "$OWN_PORTS")"
  fi
done
STX_FRONTEND_PORT="${PORT_VAL[frontend]}"
STX_HTTP_PORT="${PORT_VAL[http]}"
STX_GRPC_PORT="${PORT_VAL[grpc]}"
STX_PROMETHEUS_PORT="${PORT_VAL[prometheus]:-$STX_PREV_PROM_PORT}"
STX_ALERTMANAGER_PORT="${PORT_VAL[alertmanager]:-$STX_PREV_AM_PORT}"
STX_GRAFANA_PORT="${PORT_VAL[grafana]:-$STX_PREV_GF_PORT}"
export STX_HTTP_PORT STX_FRONTEND_PORT STX_GRPC_PORT STX_PROMETHEUS_PORT STX_ALERTMANAGER_PORT STX_GRAFANA_PORT

# 2) 对外访问 IP：写入 app.external_url（Agent 回连控制面）与 app.console_url（告警链接），也用于最后的访问地址。
#    Advertised IP: app.external_url (Agent callback) and app.console_url (alert links), also the final access URL.
def_ip="${ARG_EXTERNAL_IP:-$STX_PREV_EXTERNAL_IP}"
[[ -z "$def_ip" ]] && def_ip="$(stx_guess_host_ip)"
[[ -z "$def_ip" ]] && def_ip="127.0.0.1"
STX_EXTERNAL_HOST="$def_ip"
if [[ -z "$ARG_EXTERNAL_IP" ]] && stx_is_interactive; then
  while true; do
    ans="$(stx_prompt "对外访问 IP / 域名（Agent 回连与浏览器访问用）[默认 ${def_ip}]:" "$def_ip")"
    if [[ "$ans" =~ ^[A-Za-z0-9.:_-]+$ ]]; then STX_EXTERNAL_HOST="$ans"; break; fi
    stx_is_interactive || break
    echo "    只能包含字母、数字、. : - _ / letters, digits, . : - _ only" >/dev/tty
  done
fi
export STX_EXTERNAL_HOST

# 3) 数据库：SQLite（默认）/ MySQL / PostgreSQL；重装且保留配置时不改。
#    Database: SQLite (default) / MySQL / PostgreSQL; kept on reinstall.
DB_STATE="keep"
if [[ -n "$EXISTING_CONFIG" && -z "$ARG_DB_TYPE" ]]; then
  DB_TYPE="$(stx_yaml_get "$EXISTING_CONFIG" database.type)"
  DB_TYPE="${DB_TYPE:-sqlite}"
  stx_info "沿用已有配置中的数据库 / keeping the existing database: $DB_TYPE"
else
  DB_TYPE="${ARG_DB_TYPE:-sqlite}"
  if [[ -z "$ARG_DB_TYPE" ]] && stx_is_interactive; then
    while true; do
      ans="$(stx_prompt "数据库 / database：1) SQLite（内置，无需安装） 2) MySQL 3) PostgreSQL [默认 1]:" "1")"
      if v="$(stx_parse_db_type "$ans")"; then DB_TYPE="$v"; break; fi
      stx_is_interactive || break
      echo "    请输入 1、2 或 3 / enter 1, 2 or 3" >/dev/tty
    done
  fi
  case "$DB_TYPE" in
    sqlite) DB_STATE="sqlite" ;;
    mysql|postgres)
      DB_STATE="$DB_TYPE"
      def_db_port=3306; [[ "$DB_TYPE" == postgres ]] && def_db_port=5432
      def_db_user=root; [[ "$DB_TYPE" == postgres ]] && def_db_user=postgres
      DB_HOST="${ARG_DB_HOST:-}"; DB_PORT="${ARG_DB_PORT:-}"; DB_USER="${ARG_DB_USER:-}"; DB_NAME="${ARG_DB_NAME:-}"
      DB_PASSWORD="${ARG_DB_PASSWORD:-}"
      if stx_is_interactive; then
        [[ -z "$DB_HOST" ]] && DB_HOST="$(stx_prompt "  ${DB_TYPE} 地址 / host [默认 127.0.0.1]:" "127.0.0.1")"
        [[ -z "$DB_PORT" ]] && DB_PORT="$(stx_prompt "  ${DB_TYPE} 端口 / port [默认 ${def_db_port}]:" "$def_db_port")"
        [[ -z "$DB_USER" ]] && DB_USER="$(stx_prompt "  ${DB_TYPE} 用户 / user [默认 ${def_db_user}]:" "$def_db_user")"
        [[ -z "$ARG_DB_PASSWORD" ]] && DB_PASSWORD="$(stx_prompt_secret "  ${DB_TYPE} 密码 / password（输入不回显）:")"
        [[ -z "$DB_NAME" ]] && DB_NAME="$(stx_prompt "  数据库名 / database name [默认 stx]:" "stx")"
      fi
      DB_HOST="${DB_HOST:-127.0.0.1}"; DB_PORT="${DB_PORT:-$def_db_port}"; DB_USER="${DB_USER:-$def_db_user}"; DB_NAME="${DB_NAME:-stx}"
      [[ "$DB_PORT" =~ ^[0-9]+$ ]] || { stx_err "invalid database port: $DB_PORT"; exit 1; }
      if stx_tcp_reachable "$DB_HOST" "$DB_PORT"; then
        stx_info "✔ ${DB_TYPE} ${DB_HOST}:${DB_PORT} 可连通 / reachable"
      else
        stx_warn "${DB_TYPE} ${DB_HOST}:${DB_PORT} 当前连不上；请确认服务已启动，否则 STX 启动会失败"
      fi
      stx_info "提示：请确保数据库「${DB_NAME}」已创建（表结构由 STX 首次启动自动创建），例如：$(stx_db_create_hint "$DB_TYPE" "$DB_NAME" "$DB_USER")"
      ;;
    *) stx_err "unsupported --db-type: $DB_TYPE (sqlite|mysql|postgres)"; exit 1 ;;
  esac
fi
# 4) 管理员密码（放在数据库之后，按所选数据库判断是否已初始化）。
# 4) Admin password (after the database choice, so initialization is judged against the chosen database).
ADMIN_STATE="default"
ADMIN_USER="admin"
cfg_for_auth="${EXISTING_CONFIG:-$SOURCE_DIR/config.example.yaml}"
v="$(stx_yaml_get "$cfg_for_auth" auth.default_admin_username)"; [[ -n "$v" ]] && ADMIN_USER="$v"
# 出厂默认密码只取自包内 config.example.yaml；保留的旧 config.yaml 里可能是用户自己的密码，只用于比较与回滚，
# 绝不打印、不写日志。
# The factory default comes only from the bundled config.example.yaml. A preserved config.yaml may hold the user's
# own password: it is used only for comparison / rollback and is never printed or logged.
FACTORY_ADMIN_PASSWORD="$(stx_yaml_get "$SOURCE_DIR/config.example.yaml" auth.default_admin_password)"
FACTORY_ADMIN_PASSWORD="${FACTORY_ADMIN_PASSWORD:-admin123}"
CFG_ADMIN_PASSWORD="$(stx_yaml_get "$cfg_for_auth" auth.default_admin_password)"
CFG_ADMIN_PASSWORD="${CFG_ADMIN_PASSWORD:-$FACTORY_ADMIN_PASSWORD}"
if [[ "$CFG_ADMIN_PASSWORD" == "$FACTORY_ADMIN_PASSWORD" ]]; then
  ADMIN_KEEP_LABEL="回车保持默认 ${FACTORY_ADMIN_PASSWORD}"
  SUMMARY_DEFAULT_PASSWORD="$FACTORY_ADMIN_PASSWORD"
else
  ADMIN_KEEP_LABEL="回车保持原配置中的密码"
  SUMMARY_DEFAULT_PASSWORD=""   # 非出厂值：摘要里不显示。/ Not the factory value: never shown.
fi
ADMIN_DB_KIND="$(STX_DB_TYPE_OVERRIDE="$DB_TYPE" stx_admin_db_kind "$INSTALL_DIR" "$EXISTING_CONFIG")"
if [[ "$ADMIN_DB_KIND" == external ]]; then
  # 外部 MySQL/PostgreSQL：安装器看不到库里有没有管理员，统一写入配置；STX 只在用户表为空时采用它，
  # 启动后再用登录结果判定是否生效（未生效则把配置恢复原值）。
  # External MySQL/PostgreSQL: the installer cannot see whether the admin exists, so the password is written;
  # STX only uses it when the users table is empty. After start a login decides; if unapplied the config is restored.
  if [[ -z "$ADMIN_PASSWORD" ]] && stx_is_interactive; then
    stx_info "外部数据库：新密码只在库中还没有管理员（首次初始化）时生效 / applies only if the database has no admin yet"
    ADMIN_PASSWORD="$(stx_ask_admin_password "$ADMIN_USER" "$ADMIN_KEEP_LABEL")"
  fi
  if [[ -n "$ADMIN_PASSWORD" && "$ADMIN_PASSWORD" != "$CFG_ADMIN_PASSWORD" ]]; then
    ADMIN_STATE="pending"
  else
    ADMIN_PASSWORD=""
    ADMIN_STATE="unknown"
  fi
elif [[ "$ADMIN_DB_KIND" == existing ]]; then
  ADMIN_STATE="existing"
  stx_info "检测到已有数据库，管理员账号沿用原密码（安装器不修改）/ existing database: admin password unchanged"
  if [[ -n "$ADMIN_PASSWORD" && "$ADMIN_PASSWORD" != "$CFG_ADMIN_PASSWORD" ]]; then
    # 仍写入配置：若库里其实还没有管理员，STX 启动时会采用它；启动后用登录结果确认。
    # Still written: if the database has no admin yet STX uses it on start; confirmed by logging in afterwards.
    stx_warn "--admin-password 只在库中还没有管理员时生效；已有管理员时请登录后在控制台修改密码"
    ADMIN_STATE="pending"
  else
    ADMIN_PASSWORD=""
  fi
else
  if [[ -z "$ADMIN_PASSWORD" ]] && stx_is_interactive; then
    ADMIN_PASSWORD="$(stx_ask_admin_password "$ADMIN_USER" "$ADMIN_KEEP_LABEL")"
  fi
  if [[ -n "$ADMIN_PASSWORD" && "$ADMIN_PASSWORD" != "$CFG_ADMIN_PASSWORD" ]]; then
    ADMIN_STATE="changed"
  else
    ADMIN_PASSWORD=""
    # 沿用配置里的非出厂密码：摘要只说「沿用」。/ Keeping a non-factory config password: summary says "kept".
    [[ -z "$SUMMARY_DEFAULT_PASSWORD" ]] && ADMIN_STATE="kept"
  fi
fi

stx_step_ok "web=$STX_FRONTEND_PORT api=$STX_HTTP_PORT grpc=$STX_GRPC_PORT ip=$STX_EXTERNAL_HOST db=$DB_TYPE admin=${ADMIN_STATE}"


# ---------------------------------------------------------------------------
stx_step "解压安装包 / unpacking packages"
if [[ -n "$PACKAGES_DIR" ]]; then
  stx_info "从 $PACKAGES_DIR 组装安装文件"
  STAGE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/stx-install.XXXXXX")"

  stx_bin="$PACKAGES_DIR/stx-linux-${arch}"
  [[ -f "$stx_bin" ]] || { stx_err "missing $stx_bin"; exit 1; }
  if [[ -f "${stx_bin}.sha256" ]]; then
    stx_verify_sha256 "$stx_bin" "${stx_bin}.sha256"
  fi
  cp "$stx_bin" "$STAGE_DIR/stx"
  chmod +x "$STAGE_DIR/stx"
  stx_info "✔ stx 后端 / backend"

  frontend_tar="$(ls "$PACKAGES_DIR"/frontend-standalone-*-linux-"${arch}".tar.gz 2>/dev/null | head -n1 || true)"
  if [[ -z "$frontend_tar" ]]; then
    stx_err "missing frontend-standalone-*-linux-${arch}.tar.gz in packages/"
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
  stx_info "✔ 前端 / frontend ($(basename "$frontend_tar"))"

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
  for f in download-lib.sh install-core.sh MANIFEST.json; do
    [[ -f "$SCRIPT_DIR/$f" ]] && cp "$SCRIPT_DIR/$f" "$STAGE_DIR/$f"
  done
  if [[ -d "$SCRIPT_DIR/lib" ]]; then
    cp -a "$SCRIPT_DIR/lib/." "$STAGE_DIR/lib/"
  fi

  if [[ -f "$SOURCE_DIR/config.example.yaml" ]]; then
    cp "$SOURCE_DIR/config.example.yaml" "$STAGE_DIR/config.example.yaml"
  elif [[ -f "$SCRIPT_DIR/../config.example.yaml" ]]; then
    cp "$SCRIPT_DIR/../config.example.yaml" "$STAGE_DIR/config.example.yaml"
  fi

  # Optional node / observability / agent / java-proxy from packages.
  node_tar="$(ls "$PACKAGES_DIR"/node-*-linux-"${arch}".tar.gz 2>/dev/null | head -n1 || true)"
  if [[ "$WITHOUT_NODE" == "true" ]]; then
    node_tar=""
    stx_info "--without-node：不使用包内 Node"
  fi
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
      stx_err "bundled node package $node_tar does not contain bin/node"
      exit 1
    fi
    node_ver="$("$STAGE_DIR/runtime/node/bin/node" -v 2>/dev/null || true)"
    if [[ -z "$node_ver" ]]; then
      stx_err "内置 Node 无法在本机运行（$(basename "$node_tar")，glibc ${host_glibc:-unknown}）。CentOS 7 等 glibc < 2.28 的系统请使用 centos7 离线包。"
      exit 1
    fi
    stx_info "✔ Node 运行时 / runtime $node_ver ($(basename "$node_tar"))"
  else
    # 包内无 Node：仅当本机 Node ≥ 18.18 时复用（软链接，保证 systemd 等无 NVM 环境可用）；否则立即报错。
    # No node package: reuse local Node only when it is >= 18.18 (symlinked for systemd); otherwise fail now.
    if ! stx_probe_system_node; then
      stx_system_node_error "no node-*-linux-${arch}.tar.gz in $PACKAGES_DIR" \
        "this package was likely built with --without-node. Either install Node >= 18.18 on this host" \
        "(and make sure it is in PATH), or rebuild the offline bundle WITHOUT --without-node."
      exit 1
    fi
    stx_info "no bundled node package; using system node $STX_SYSTEM_NODE ($STX_SYSTEM_NODE_VER)"
    mkdir -p "$STAGE_DIR/runtime/node/bin"
    ln -sf "$STX_SYSTEM_NODE" "$STAGE_DIR/runtime/node/bin/node"
  fi

  obs_tar="$(ls "$PACKAGES_DIR"/observability-*-linux-"${arch}".tar.gz 2>/dev/null | head -n1 || true)"
  if [[ -n "$obs_tar" && "$WITH_OBS" != "false" ]]; then
    stx_info "… 解压 Prometheus/Alertmanager/Grafana（约 $(( $(wc -c <"$obs_tar") / 1024 / 1024 )) MB，需要十几秒）"
    mkdir -p "$STAGE_DIR/observability"
    tar -xzf "$obs_tar" -C "$STAGE_DIR/observability"
    stx_info "✔ 可观测性三件套 / observability stack"
  elif [[ -n "$obs_tar" ]]; then
    stx_info "- 跳过可观测性三件套（--without-observability）"
  fi

  agent_bin="$PACKAGES_DIR/stx-agent-linux-${arch}"
  if [[ -f "$agent_bin" ]]; then
    cp "$agent_bin" "$STAGE_DIR/lib/agent/stx-agent-linux-${arch}"
    chmod +x "$STAGE_DIR/lib/agent/stx-agent-linux-${arch}"
    stx_info "✔ stx-agent"
  fi

  # 同步全部代际 jar（v2/v3），供 2.x / 3.x 集群按版本选型。/ Stage every epoch jar (v2/v3).
  if compgen -G "$PACKAGES_DIR/stx-java-proxy-*.jar" >/dev/null; then
    cp "$PACKAGES_DIR"/stx-java-proxy-*.jar "$STAGE_DIR/lib/"
    stx_info "✔ stx-java-proxy (v2/v3)"
  fi
  if [[ -f "$SOURCE_DIR/scripts/stx-java-proxy.sh" ]]; then
    cp "$SOURCE_DIR/scripts/stx-java-proxy.sh" "$STAGE_DIR/scripts/"
  fi

  SOURCE_DIR="$STAGE_DIR"
else
  # 非 packages 组装时校验经典包内容。/ Classic package validation.
  for f in stx bin/start.sh bin/stop.sh bin/status.sh config.example.yaml; do
    if [[ ! -e "$SOURCE_DIR/$f" ]]; then
      stx_err "package payload missing: $f"
      exit 1
    fi
  done
  if ! compgen -G "$SOURCE_DIR/lib/stx-java-proxy-*.jar" >/dev/null; then
    stx_warn "missing lib/stx-java-proxy-*.jar (Agent capability proxy epochs v2/v3)"
  elif [[ ! -e "$SOURCE_DIR/lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar" ]]; then
    stx_warn "missing default lib/stx-java-proxy-${CAPABILITY_PROXY_DEFAULT_VERSION}.jar (Agent capability proxy)"
  fi
fi
stx_step_ok

# ---------------------------------------------------------------------------
stx_step "复制文件到 $INSTALL_DIR / copying files"
stx_stop_existing_install "$INSTALL_DIR"
BACKUP_CONFIG=""
if [[ -n "$EXISTING_CONFIG" ]]; then
  BACKUP_CONFIG="$(mktemp)"
  cp "$EXISTING_CONFIG" "$BACKUP_CONFIG"
fi

if [[ "$FORCE" == "true" && -d "$INSTALL_DIR" && "$(ls -A "$INSTALL_DIR" 2>/dev/null || true)" != "" ]]; then
  BACKUP_DIR="${INSTALL_DIR}.bak.$(date +%Y%m%d%H%M%S)"
  stx_info "backup existing directory to: $BACKUP_DIR"
  mv "$INSTALL_DIR" "$BACKUP_DIR"
  mkdir -p "$INSTALL_DIR"
fi

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
  stx_info "保留已有 config.yaml / existing config.yaml restored"
elif [[ ! -f "$INSTALL_DIR/config.yaml" || "$PRESERVE_CONFIG" != "true" ]]; then
  cp "$INSTALL_DIR/config.example.yaml" "$INSTALL_DIR/config.yaml"
  stx_info "由 config.example.yaml 生成 config.yaml / generated config.yaml"
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
stx_step_ok "$(du -sh "$INSTALL_DIR" 2>/dev/null | awk '{print $1}')"

# ---------------------------------------------------------------------------
stx_step "写入配置 / writing config.yaml"
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
ADMIN_PW_RAW_BEFORE=""
if [[ -n "$ADMIN_PASSWORD" ]]; then
  # 记下原值（原样），新密码未生效时恢复。/ Remember the verbatim old value to restore if the new one is unapplied.
  ADMIN_PW_RAW_BEFORE="$(stx_yaml_get_raw "$INSTALL_DIR/config.yaml" auth.default_admin_password)"
  stx_set_admin_password "$INSTALL_DIR/config.yaml" "$ADMIN_PASSWORD"
  stx_info "已写入新的初始管理员密码 / new initial admin password written"
fi
if [[ "$DB_STATE" != "keep" ]]; then
  stx_set_database "$INSTALL_DIR/config.yaml" "$DB_TYPE" "${DB_HOST:-}" "${DB_PORT:-}" "${DB_USER:-}" "${DB_PASSWORD:-}" "${DB_NAME:-}"
  stx_info "数据库 / database: $DB_TYPE${DB_HOST:+ ${DB_HOST}:${DB_PORT}/${DB_NAME}}"
fi
stx_write_env_file "$INSTALL_DIR"
if [[ -n "$SERVICE_USER" ]]; then
  chown -R "$SERVICE_USER": "$INSTALL_DIR" 2>/dev/null || stx_warn "chown -R $SERVICE_USER $INSTALL_DIR failed"
fi
stx_step_ok "$INSTALL_DIR/config.yaml"

# ---------------------------------------------------------------------------
stx_step "注册系统服务 / registering systemd service"
STX_SYSTEMD_MODE=""
if [[ "$SKIP_SYSTEMD" != "true" ]]; then
  stx_install_systemd "$INSTALL_DIR"
else
  stx_info "跳过 systemd（--no-systemd）"
fi
stx_step_ok "${STX_SYSTEMD_MODE:-none}"

# ---------------------------------------------------------------------------
stx_step "启动服务 / starting services"
STARTED=false
if [[ "$AUTO_START" == "true" ]]; then
  # 已安装 systemd unit 时经 systemctl 启动（并 enable）；unit 是 oneshot + RemainAfterExit，重装时用 restart。
  # With a systemd unit start via systemctl (enabled); the unit is oneshot + RemainAfterExit, so use restart.
  if [[ -n "$STX_SYSTEMD_MODE" ]]; then
    sysctl_cmd=(systemctl)
    journal_hint="journalctl -u stx -n 200"
    if [[ "$STX_SYSTEMD_MODE" == "user" ]]; then
      sysctl_cmd=(systemctl --user)
      journal_hint="journalctl --user -u stx -n 200"
    fi
    if [[ "$ENABLE_AUTOSTART" == "true" ]]; then
      "${sysctl_cmd[@]}" enable stx.service >/dev/null 2>&1 || true
    fi
    if ! "${sysctl_cmd[@]}" restart stx.service; then
      stx_err "${sysctl_cmd[*]} restart stx failed; inspect: ${sysctl_cmd[*]} status stx / $journal_hint"
      exit 1
    fi
    # 先取完整输出再匹配：pipefail 下 `status.sh | grep -q` 可能因 SIGPIPE 误判失败。
    status_out="$(cd "$INSTALL_DIR" && ./bin/status.sh 2>/dev/null || true)"
    if grep -q 'backend: running' <<<"$status_out"; then
      stx_info "[OK] started via systemd: ${sysctl_cmd[*]} status stx"
    else
      stx_err "${sysctl_cmd[*]} restart stx returned but backend is not running; inspect: $INSTALL_DIR/bin/status.sh / $journal_hint"
      exit 1
    fi
  else
    (
      cd "$INSTALL_DIR"
      if [[ -n "$SERVICE_USER" && "$(id -u)" -eq 0 && "$SERVICE_USER" != root ]] && command -v su >/dev/null 2>&1; then
        su -s /bin/bash "$SERVICE_USER" -c "cd '$INSTALL_DIR' && CONFIG_PATH='$INSTALL_DIR/config.yaml' ./bin/start.sh"
      else
        CONFIG_PATH="$INSTALL_DIR/config.yaml" ./bin/start.sh
      fi
    )
  fi
  stx_wait_ready "$STX_HTTP_PORT" "$STX_FRONTEND_PORT" "$INSTALL_DIR" || exit 1
  STARTED=true
  if [[ "$ADMIN_STATE" == pending ]]; then
    if stx_admin_login_ok "$STX_HTTP_PORT" "$ADMIN_USER" "$ADMIN_PASSWORD"; then
      ADMIN_STATE="changed"
    else
      ADMIN_STATE="existing"
      # 未生效的密码不留在配置里：恢复原值。/ Do not leave an unapplied password in the config: restore it.
      if [[ -n "$ADMIN_PW_RAW_BEFORE" ]]; then
        stx_yaml_set "$INSTALL_DIR/config.yaml" "auth.default_admin_password" "$ADMIN_PW_RAW_BEFORE" || true
      else
        stx_set_admin_password "$INSTALL_DIR/config.yaml" "$FACTORY_ADMIN_PASSWORD" || true
      fi
      stx_warn "数据库中已有管理员账号，新密码未生效（沿用原密码，配置已恢复原值）/ the database already has an admin; password unchanged, config restored"
    fi
  fi
  if [[ "$ADMIN_STATE" == unknown ]]; then
    # 只用出厂默认密码探测，且仅当配置里就是出厂值时才可能显示它。
    # Probe with the factory default only; it can only be shown when the config holds the factory value.
    if [[ -n "$SUMMARY_DEFAULT_PASSWORD" ]] && stx_admin_login_ok "$STX_HTTP_PORT" "$ADMIN_USER" "$FACTORY_ADMIN_PASSWORD"; then
      ADMIN_STATE="default"
    else
      ADMIN_STATE="existing"
    fi
  fi
  stx_step_ok "API :$STX_HTTP_PORT / Web :$STX_FRONTEND_PORT"
else
  stx_info "跳过启动（--no-start）"
  stx_step_ok "skipped"
fi

STX_SUMMARY_DB="$DB_TYPE${DB_HOST:+ (${DB_HOST}:${DB_PORT}/${DB_NAME})}"
[[ "$DB_TYPE" == sqlite ]] && STX_SUMMARY_DB="SQLite（$INSTALL_DIR/data/stx.db）"
export STX_SUMMARY_DB
stx_print_summary "$INSTALL_DIR" "$ADMIN_USER" "$ADMIN_STATE" "$SUMMARY_DEFAULT_PASSWORD" "$STARTED" "$obs_flag"
