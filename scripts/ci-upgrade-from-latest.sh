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

# STX 跨版本升级 CI：先装「最新正式版」（releases/latest 或 --from 指定），
# 再用当前代码构建一键升级包，通过应用内自升级完成升级并断言。
# 与「最新正式版具备什么能力」相关的断言先探测再执行：最新版本身已没有
# 旧外部监控三件套 / 旧 webhook / 旧取值时，对应植入与断言自动跳过，通用断言仍然有效。
#
# 全程 CLI 优先：登录一次后复用同一份 CLI 凭据跨升级，种子、升级与断言优先走
# 安装目录里的 stx CLI；FROM 版本缺某条命令时仅该操作回退 curl（带 CLI 登录拿到的
# Bearer token 与 X-STX-Client: cli 头）并打 ::notice::。这样同时覆盖 CLI 的请求
# 构造、确认处理与升级后 token 持久化，纯 API 调法测不到这些。
#
# STX cross-version upgrade CI: install the latest stable release (releases/latest,
# or --from), build an in-app upgrade bundle from the current tree, upgrade through
# the self-upgrade flow and assert. Capability-dependent seeds/assertions probe
# first: when the latest release no longer ships the legacy monitoring stack /
# webhook / legacy values, those legs are skipped while the generic assertions
# still hold.
#
# CLI-first: log in once, reuse the same CLI credentials across the upgrade, and
# prefer the installed stx CLI for seeding, the upgrade itself, and assertions.
# When the FROM release lacks a command, only that operation falls back to curl
# (with the Bearer token obtained via CLI login plus the X-STX-Client: cli header)
# and a ::notice:: is emitted. This also exercises CLI request construction,
# confirmation handling, and token persistence — things API-only calls miss.
#
# 警告：脚本会 sudo 安装并覆盖 systemd 服务，只能在一次性虚拟机或 CI runner 上运行，
# 不要在共享机器（如 t1 的 /opt/stx）上执行。
# WARNING: this script installs with sudo and manages a systemd unit; run it only on
# a disposable VM or CI runner, never on a shared machine (e.g. t1's /opt/stx).
#
# 用法 / Usage:
#   scripts/ci-upgrade-from-latest.sh [--from <tag>] [--target <ver>]
#
# 环境变量 / Environment:
#   FROM_VERSION    起始版本 tag（如 v1.4.0；留空用 releases/latest）
#   TARGET_VERSION  目标版本（留空 = FROM 的 major.minor.(patch+1)-ci.<run>）
#   GITHUB_REPOSITORY  gh 查询 releases/latest 所需（CI 自带）
#   GH_TOKEN / GITHUB_TOKEN  gh 认证（CI 自带）
#   ADMIN_PASSWORD  初始管理员密码（默认随机生成并打印到收集目录）
#   INSTALL_DIR     安装目录（默认 /opt/stx）
#   DIST_DIR        升级包输出目录（默认 dist/upgrade-ci）
#   COLLECT_DIR     现场收集目录（默认 dist/upgrade-ci-artifacts）

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="${INSTALL_DIR:-/opt/stx}"
HTTP_PORT="${HTTP_PORT:-17800}"
API_BASE="http://127.0.0.1:${HTTP_PORT}/api/v1"
SERVER_URL="http://127.0.0.1:${HTTP_PORT}"
DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist/upgrade-ci}"
COLLECT_DIR="${COLLECT_DIR:-$ROOT_DIR/dist/upgrade-ci-artifacts}"
WORK_DIR="$(mktemp -d)"
SINK_PORT="${SINK_PORT:-18999}"
SINK_LOG="$WORK_DIR/webhook-sink.log"

# 安装目录里的 stx 二进制同时是服务端与 CLI；升级后原位替换为新版本。
# The installed stx binary doubles as server and CLI; the upgrade replaces it in place.
STX="$INSTALL_DIR/stx"
CLI_NS="ci"

ADMIN_PASSWORD="${ADMIN_PASSWORD:-ci-upgrade-$(date +%s | tail -c 7)}"
FROM_VERSION="${FROM_VERSION:-}"
TARGET_VERSION="${TARGET_VERSION:-}"
LEGACY_STACK=false
SEEDED_CHANNEL=false
SEEDED_FOLDER=false
SEEDED_LEGACY_ALERT=false
SEEDED_LEGACY_ROUTE=false
SEEDED_CATCHALL_ROUTE=false
SEEDED_LEGACY_POLICY=false
ALERT_NAME="ci_legacy_alert"
CHANNEL_NAME="ci-upgrade-channel"
FOLDER_NAME="ci-upgrade-folder"
ROUTE_NAME="ci-upgrade-route"
ROUTE_ALL_NAME="ci-upgrade-route-all"
POLICY_NAME="ci-upgrade-policy"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --from) FROM_VERSION="${2:-}"; shift 2 ;;
    --from=*) FROM_VERSION="${1#*=}"; shift ;;
    --target) TARGET_VERSION="${2:-}"; shift 2 ;;
    --target=*) TARGET_VERSION="${1#*=}"; shift ;;
    -h|--help)
      sed -n '18,42p' "$0"
      exit 0
      ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

log() { echo "[ci-upgrade] $*"; }
notice() {
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    echo "::notice::$*"
  else
    echo "[ci-upgrade][notice] $*"
  fi
}
die() { echo "[ci-upgrade][ERROR] $*" >&2; exit 1; }

require_cmds() {
  local missing=()
  for cmd in "$@"; do
    command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
  done
  ((${#missing[@]} == 0)) || die "missing required commands: ${missing[*]}"
}

# ── CLI / API 调用助手 ────────────────────────────────────────────────────────

# cli_has <子命令路径...>：探测安装版 CLI 是否提供该命令。cobra 对不存在的子命令
# 会回落到父命令帮助且退出码仍为 0，不能只看退出码——必须确认 Usage 行真的指向被探测的命令路径。
# cli_has <subcommand path...>: probe whether the installed CLI provides the command.
# cobra falls back to the parent help with exit code 0 for an unknown subcommand, so the
# exit code alone is not enough — the Usage line must actually name the probed path.
cli_has() {
  local out
  out="$("$STX" "$@" --help 2>/dev/null)" || return 1
  grep -qF "stx $*" <<<"$out"
}

# cli_call <输出变量> <参数...>：带 namespace 调 CLI，stdout 是 JSON 结果信封。
# 服务端返回一次性确认要求时，从 stderr 事件里取 confirmation_id 带 --confirmation-id 重试一次。
# cli_call <out var> <args...>: invoke the CLI under the ci namespace; stdout is the
# JSON result envelope. When the server asks for a one-time confirmation, the
# confirmation_id is read from the stderr event and the call retried once with
# --confirmation-id.
cli_call() {
  local outvar="$1"; shift
  local out_file="$WORK_DIR/cli-out.json" err_file="$WORK_DIR/cli-err.ndjson"
  : >"$out_file"; : >"$err_file"
  if "$STX" "$@" --namespace "$CLI_NS" >"$out_file" 2>"$err_file"; then
    printf -v "$outvar" '%s' "$(cat "$out_file")"
    return 0
  fi
  local conf_id
  conf_id="$(jq -r 'select(.event? == "confirmation_required") | .confirmation_id // empty' "$err_file" 2>/dev/null | head -1)"
  if [[ -n "$conf_id" ]]; then
    notice "server requested one-time confirmation ($conf_id); retrying with --confirmation-id"
    if "$STX" "$@" --namespace "$CLI_NS" --confirmation-id "$conf_id" >"$out_file" 2>>"$err_file"; then
      printf -v "$outvar" '%s' "$(cat "$out_file")"
      return 0
    fi
  fi
  cat "$err_file" >&2 || true
  return 1
}

# cli_token：从 CLI 配置里取 ci namespace 的 Bearer token，供 curl 回退用。
# cli_token: read the ci namespace's bearer token from the CLI config for curl fallback.
cli_token() {
  local cfg="${XDG_CONFIG_HOME:-$HOME/.config}/stx/config.yaml"
  python3 - "$cfg" "$CLI_NS" <<'PY'
import sys
path, ns = sys.argv[1], sys.argv[2]
# Go yaml.v3 嵌套缩进不固定（实测 4 空格），按相对缩进层级解析而不是硬编码列号。
# Go yaml.v3 nested indentation is not fixed (4 spaces in practice), so parse by
# relative indent levels instead of hard-coded column positions.
lines = []
for raw in open(path):
    stripped = raw.strip()
    if stripped and not stripped.startswith("#"):
        lines.append((len(raw) - len(raw.lstrip()), stripped))
i = next((k for k, (ind, t) in enumerate(lines) if ind == 0 and t.startswith("namespaces:")), None)
if i is None:
    sys.exit(0)
base = lines[i][0]
j = i + 1
while j < len(lines):
    ind, text = lines[j]
    if ind <= base:
        break
    if text.endswith(":") and text[:-1].strip().strip('"').strip("'") == ns:
        k = j + 1
        while k < len(lines) and lines[k][0] > ind:
            child = lines[k][1]
            if child.startswith("token:"):
                print(child.split(":", 1)[1].strip().strip('"').strip("'"))
                sys.exit(0)
            k += 1
        break
    j += 1
PY
}

# api_call <method> <path> [curl 额外参数...]：CLI 缺失某命令时的 API 回退，
# 一律带 CLI 登录得到的 Bearer token 与 X-STX-Client: cli 头。
# api_call <method> <path> [extra curl args...]: API fallback for operations whose
# CLI command is missing; always carries the bearer token from CLI login and the
# X-STX-Client: cli header.
api_call() {
  local method="$1" path="$2"
  shift 2
  curl -fsS -X "$method" \
    -H "Authorization: Bearer $(cli_token)" \
    -H "X-STX-Client: cli" \
    -H 'Content-Type: application/json' \
    "$@" "${API_BASE}${path}"
}

# api_status <method> <path> [payload]：只取状态码（允许失败），用于能力探测。
# 注意不能带 -f：-f 让 4xx 返回非零但 -w 仍打印真实码，|| echo 000 会再追加一段，
# 把 404 变成 "404000"。
# api_status <method> <path> [payload]: probe-only variant returning the HTTP status
# code. Do NOT use -f here: it makes 4xx fail while -w still prints the real code,
# and a trailing `|| echo 000` would append, turning 404 into "404000".
api_status() {
  local method="$1" path="$2" payload="${3:-}"
  local args=(-sS -o /dev/null -w '%{http_code}' -X "$method"
    -H "Authorization: Bearer $(cli_token)" -H "X-STX-Client: cli")
  if [[ -n "$payload" ]]; then
    args+=(-H 'Content-Type: application/json' -d "$payload")
  fi
  local code
  code="$(curl "${args[@]}" "${API_BASE}${path}" 2>/dev/null || true)"
  echo "${code:-000}"
}

collect() {
  mkdir -p "$COLLECT_DIR"
  if sudo -n true 2>/dev/null; then
    sudo -n journalctl -u stx --no-pager >"$COLLECT_DIR/journalctl-stx.log" 2>&1 || true
    sudo -n ss -ltnp >"$COLLECT_DIR/ss-ltnp.txt" 2>&1 || true
    sudo -n ps aux >"$COLLECT_DIR/ps-aux.txt" 2>&1 || true
    if [[ -d "$INSTALL_DIR/logs" ]]; then
      sudo -n tar -czf "$COLLECT_DIR/stx-logs.tar.gz" -C "$INSTALL_DIR" logs 2>/dev/null || true
    fi
    if [[ -d "$INSTALL_DIR/bin" ]]; then
      sudo -n ls -laR "$INSTALL_DIR/bin" >"$COLLECT_DIR/bin-listing.txt" 2>&1 || true
    fi
    if [[ -d "$INSTALL_DIR/data/self-upgrade" ]]; then
      sudo -n tar -czf "$COLLECT_DIR/self-upgrade-history.tar.gz" -C "$INSTALL_DIR/data" self-upgrade 2>/dev/null || true
    fi
    if [[ -f "${XDG_CONFIG_HOME:-$HOME/.config}/stx/config.yaml" ]]; then
      sed -E 's/(token:).*/\1 <redacted>/' "${XDG_CONFIG_HOME:-$HOME/.config}/stx/config.yaml" \
        >"$COLLECT_DIR/cli-config-redacted.yaml" 2>/dev/null || true
    fi
  fi
  [[ -f "$SINK_LOG" ]] && cp "$SINK_LOG" "$COLLECT_DIR/webhook-sink.log" 2>/dev/null || true
  log "artifacts collected under $COLLECT_DIR"
}

on_exit() {
  local rc=$?
  collect || true
  exit "$rc"
}
trap on_exit EXIT

# ── 1. 确定 FROM / TARGET ────────────────────────────────────────────────────

resolve_from_version() {
  if [[ -z "$FROM_VERSION" ]]; then
    require_cmds gh
    FROM_VERSION="$(gh release view --repo "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required to resolve releases/latest}" --json tagName -q .tagName)"
  fi
  # 统一成 tag 形态（v 前缀）。/ Normalize to tag form (v prefix).
  FROM_TAG="v${FROM_VERSION#v}"
  FROM_VER="${FROM_TAG#v}"
  log "FROM = $FROM_TAG"
}

resolve_target_version() {
  if [[ -n "$TARGET_VERSION" ]]; then
    TARGET="${TARGET_VERSION#v}"
  else
    local base="${FROM_VER%%-*}"
    local run="${GITHUB_RUN_NUMBER:-$(date +%s)}"
    IFS='.' read -r major minor patch <<<"$base"
    [[ -n "${major:-}" && -n "${minor:-}" && -n "${patch:-}" ]] || die "cannot derive target version from FROM=$FROM_TAG; pass --target"
    TARGET="${major}.${minor}.$((patch + 1))-ci.${run}"
  fi
  log "TARGET = $TARGET"
}

# same_version_release：重发同版本 tag 时 latest release 恰等于 TARGET（例如强推
# v1.5.0 重新发版），self-upgrade 会拒绝「不比当前版本新」的同版本升级（execute 400
# ErrNotStaged），这条腿测不到任何东西，直接跳过。
# same_version_release: when re-releasing the same tag, the latest release equals the
# TARGET (e.g. force-pushed v1.5.0), and self-upgrade correctly refuses a same-version
# upgrade (execute 400 ErrNotStaged) — there is nothing to exercise, so skip the leg.
same_version_release() {
  [[ "$FROM_TAG" == "v$TARGET" ]]
}

# ── 2. 用当前代码构建一键升级包 ──────────────────────────────────────────────

build_upgrade_package() {
  log "building upgrade bundle for $TARGET ..."
  (cd "$ROOT_DIR" && bash install/package-release.sh \
    --arch amd64 \
    --layout split \
    --version "$TARGET" \
    --build-frontend \
    --output-dir "$DIST_DIR")
  UPGRADE_PKG="$DIST_DIR/stx-upgrade-${TARGET}-linux-amd64.tar.gz"
  [[ -f "$UPGRADE_PKG" ]] || die "upgrade bundle not produced: $UPGRADE_PKG"
  log "upgrade bundle: $UPGRADE_PKG"
}

# ── 3. 安装 FROM ─────────────────────────────────────────────────────────────

fetch_with_retry() {
  local url="$1" out="$2" attempt
  for attempt in 1 2 3; do
    if curl -fsSL --retry 2 --connect-timeout 15 "$url" -o "$out"; then
      return 0
    fi
    log "download attempt $attempt failed: $url"
    sleep 5
  done
  return 1
}

install_from() {
  local repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
  fetch_with_retry "https://github.com/${repo}/releases/download/${FROM_TAG}/install-online.sh" "$WORK_DIR/install-online.sh" \
    || die "cannot download install-online.sh for $FROM_TAG"

  # 探测 FROM 的安装器是否还支持三件套参数，支持就装上以复现旧环境。
  # Probe whether FROM's installer still accepts the observability flag; install it to reproduce the legacy layout.
  local install_sh="$WORK_DIR/install-${FROM_TAG}.sh"
  local extra_args=()
  if fetch_with_retry "https://raw.githubusercontent.com/${repo}/${FROM_TAG}/install/install.sh" "$install_sh" \
    && grep -q -- '--with-observability' "$install_sh"; then
    extra_args+=(--with-observability)
    log "FROM installer supports --with-observability; enabling legacy stack"
  else
    notice "FROM ($FROM_TAG) installer has no --with-observability flag; legacy stack leg will be skipped"
  fi

  log "installing $FROM_TAG ..."
  sudo bash "$WORK_DIR/install-online.sh" \
    --version "$FROM_TAG" \
    --yes \
    --admin-password "$ADMIN_PASSWORD" \
    ${extra_args[@]+"${extra_args[@]}"}
}

wait_for_version() {
  local expected="$1" timeout="${2:-240}" start now version
  start="$(date +%s)"
  while true; do
    version="$(curl -fsS --connect-timeout 3 "${API_BASE}/version" 2>/dev/null | jq -r '.data.version // empty' 2>/dev/null || true)"
    if [[ "$version" == "$expected" ]]; then
      log "server reports version $version"
      return 0
    fi
    now="$(date +%s)"
    ((now - start < timeout)) || die "timed out waiting for version=$expected (last seen: ${version:-none})"
    sleep 3
  done
}

# ── 4. 在 FROM 上植入数据（CLI 优先，逐命令探测，缺失回退 curl） ──────────────

# cli_login：升级前只用安装版 CLI 登录一次，凭据写入本地 ci namespace，全程复用。
# cli_login: log in once with the installed CLI before the upgrade; credentials are
# stored in the local ci namespace and reused for the whole run.
cli_login() {
  printf '%s' "$ADMIN_PASSWORD" | "$STX" login \
    --server "$SERVER_URL" \
    --username admin \
    --password-stdin \
    --namespace "$CLI_NS" >/dev/null \
    || die "CLI login failed"
  # 登录成功但 token 没落盘会让后续 API 回退变成静默 401，这里直接失败更直观。
  # A successful login without a persisted token turns later API fallbacks into a
  # silent 401, so fail fast and visibly here.
  [[ -n "$(cli_token)" ]] || die "CLI login did not persist a token for namespace $CLI_NS"
  log "logged in as admin via CLI (namespace $CLI_NS)"
}

# cli_credentials_ok：不重新登录，直接用已存凭据调 CLI 验证 token 仍有效。
# cli_credentials_ok: invoke the CLI with stored credentials (no re-login) to prove
# the token still works.
cli_credentials_ok() {
  local out
  cli_call out whoami >/dev/null || die "CLI credentials invalid (whoami failed)"
  cli_call out self-upgrade status >/dev/null || die "CLI credentials invalid (self-upgrade status failed)"
}

start_webhook_sink() {
  cat >"$WORK_DIR/sink.py" <<'PY'
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

log_path = sys.argv[2]

class Sink(BaseHTTPRequestHandler):
    def _record(self):
        length = int(self.headers.get('Content-Length') or 0)
        body = self.rfile.read(length).decode('utf-8', 'replace')
        with open(log_path, 'a') as fh:
            fh.write(json.dumps({'method': self.command, 'path': self.path, 'body': body}) + '\n')
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    do_POST = _record
    do_PUT = _record
    do_GET = _record

    def log_message(self, *args):
        pass

HTTPServer(('127.0.0.1', int(sys.argv[1])), Sink).serve_forever()
PY
  : >"$SINK_LOG"
  python3 "$WORK_DIR/sink.py" "$SINK_PORT" "$SINK_LOG" &
  SINK_PID=$!
  for _ in $(seq 1 30); do
    curl -fsS -o /dev/null "http://127.0.0.1:${SINK_PORT}/ping" 2>/dev/null && return 0
    sleep 1
  done
  die "webhook sink did not come up on port $SINK_PORT"
}

seed_data() {
  start_webhook_sink
  local out

  # 通用数据：通知渠道（webhook 指向本机记录端口）+ 工作台目录。
  # Generic seeds: a notification channel (webhook to the local recorder) and a workbench folder.
  jq -nc --arg name "$CHANNEL_NAME" --arg url "http://127.0.0.1:${SINK_PORT}/notify" \
    '{name:$name,type:"webhook",enabled:true,endpoint:$url,description:"ci upgrade probe"}' \
    >"$WORK_DIR/channel.json"
  local cli_present=false
  cli_has monitoring notification-channel create && cli_present=true
  if [[ "$cli_present" == "true" ]] \
      && cli_call out monitoring notification-channel create --request-file "$WORK_DIR/channel.json" --confirm \
      && jq -e '.data.id' <<<"$out" >/dev/null; then
    SEEDED_CHANNEL=true
    log "channel seeded via CLI"
  fi
  if [[ "$SEEDED_CHANNEL" != "true" ]] && api_call POST "/monitoring/notification-channels" -d @"$WORK_DIR/channel.json" | jq -e '.data.id' >/dev/null; then
    if [[ "$cli_present" == "true" ]]; then
      notice "CLI channel create failed on FROM; seeded via API fallback"
    else
      notice "installed CLI has no 'monitoring notification-channel create'; seeded via API fallback"
    fi
    SEEDED_CHANNEL=true
  fi
  [[ "$SEEDED_CHANNEL" == "true" ]] || notice "FROM does not accept notification channel creation; data-retention channel assert skipped"

  cli_present=false
  cli_has sync task create && cli_present=true
  if [[ "$cli_present" == "true" ]] \
      && cli_call out sync task create --name "$FOLDER_NAME" --type folder --mode streaming --confirm \
      && jq -e '.data.id' <<<"$out" >/dev/null; then
    SEEDED_FOLDER=true
    log "workbench folder seeded via CLI"
  fi
  if [[ "$SEEDED_FOLDER" != "true" ]] && api_call POST "/sync/tasks" -d "$(jq -nc --arg name "$FOLDER_NAME" \
      '{name:$name,node_type:"folder",content_format:"hocon",content:"",mode:"streaming"}')" \
      | jq -e '.data.id' >/dev/null; then
    if [[ "$cli_present" == "true" ]]; then
      notice "CLI sync folder create failed on FROM; seeded via API fallback"
    else
      notice "installed CLI has no 'sync task create'; seeded folder via API fallback"
    fi
    SEEDED_FOLDER=true
  fi
  [[ "$SEEDED_FOLDER" == "true" ]] || notice "FROM does not accept workbench folder creation; folder assert skipped"

  # 遗留告警：FROM 还暴露 Alertmanager webhook 时写入一条 firing 告警（generatorURL 非空）。
  # 该端点没有对应 CLI 操作，始终走 API。
  # Legacy alert: when FROM still exposes the Alertmanager webhook, push one firing
  # alert (non-empty generatorURL). This endpoint has no CLI operation, always via API.
  if [[ "$(api_status POST "/monitoring/alertmanager/webhook")" != "404" ]]; then
    local payload
    payload="$(jq -nc --arg name "$ALERT_NAME" --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '{
      version:"4",status:"firing",receiver:"stx-webhook",groupKey:"ci:{}",
      commonLabels:{alertname:$name,severity:"warning",cluster_id:"legacy",cluster_name:"legacy",env:"ci"},
      alerts:[{
        status:"firing",
        labels:{alertname:$name,severity:"warning",cluster_id:"legacy",cluster_name:"legacy",env:"ci"},
        annotations:{summary:"ci legacy alert",description:"seeded by ci-upgrade-from-latest"},
        startsAt:$ts,endsAt:"0001-01-01T00:00:00Z",
        generatorURL:"http://prometheus.legacy/graph?g0.expr=ci",
        fingerprint:"ci-fingerprint-1"
      }]}')"
    if [[ "$(api_status POST "/monitoring/alertmanager/webhook" "$payload")" == "200" ]]; then
      SEEDED_LEGACY_ALERT=true
      log "legacy alert seeded via alertmanager webhook"
    else
      notice "alertmanager webhook probe succeeded but ingest failed; legacy alert assert skipped"
    fi
  else
    notice "FROM has no alertmanager webhook; legacy alert assert skipped"
  fi

  # 旧通知路由：FROM 接受 source_type=remote_alertmanager 时植入。
  # Legacy route: seed one with source_type=remote_alertmanager when FROM accepts it.
  if [[ "$SEEDED_CHANNEL" == "true" ]]; then
    local channel_id=""
    if cli_has monitoring notification-channel list; then
      channel_id="$(cli_call out monitoring notification-channel list \
        && jq -r --arg n "$CHANNEL_NAME" '.data.channels[]? // .data.items[]? // empty | select(.name==$n) | .id' <<<"$out" | head -1)" \
        || channel_id=""
    fi
    if [[ -z "$channel_id" ]]; then
      channel_id="$(api_call GET "/monitoring/notification-channels" | jq -r --arg n "$CHANNEL_NAME" '.data.channels[]? // .data.items[]? // empty | select(.name==$n) | .id' | head -1)"
    fi

    cli_present=false
    cli_has monitoring notification-route create && cli_present=true
    # channel_id 为空时 --argjson 会直接失败，路由两条腿整体跳过。
    # --argjson fails outright when channel_id is empty; both route legs are skipped then.
    if [[ -n "$channel_id" ]]; then
      jq -nc --arg name "$ROUTE_NAME" --argjson ch "$channel_id" \
        '{name:$name,enabled:true,source_type:"remote_alertmanager",channel_id:$ch,send_resolved:true}' \
        >"$WORK_DIR/route-legacy.json"
      jq -nc --arg name "$ROUTE_ALL_NAME" --argjson ch "$channel_id" \
        '{name:$name,enabled:true,source_type:"",channel_id:$ch,send_resolved:true}' \
        >"$WORK_DIR/route-catchall.json"
    fi
    if [[ -n "$channel_id" && "$cli_present" == "true" ]] \
        && cli_call out monitoring notification-route create --request-file "$WORK_DIR/route-legacy.json" --confirm \
        && jq -e '.data.id' <<<"$out" >/dev/null; then
      SEEDED_LEGACY_ROUTE=true
      log "legacy route seeded via CLI"
    fi
    if [[ "$SEEDED_LEGACY_ROUTE" != "true" && -n "$channel_id" ]] \
        && api_call POST "/monitoring/notification-routes" -d @"$WORK_DIR/route-legacy.json" | jq -e '.data.id' >/dev/null; then
      if [[ "$cli_present" == "true" ]]; then
        notice "CLI route create failed on FROM; seeded legacy route via API fallback"
      else
        notice "installed CLI has no 'monitoring notification-route create'; seeded legacy route via API fallback"
      fi
      SEEDED_LEGACY_ROUTE=true
    fi
    [[ "$SEEDED_LEGACY_ROUTE" == "true" ]] || notice "FROM rejected legacy source_type route; invalid_reason route assert skipped"

    # 兜底路由：source_type 为空匹配全部告警，同样指向接收端。旧路由升级后失效，
    # 若遗留告警迁移误发恢复通知，没有它就到不了 sink，"no resolved notification"
    # 断言就会空转；有了它误发通知才会被真正记录。
    # Catch-all route: an empty source_type matches every alert into the same sink. The
    # legacy route is dead after upgrade, so without it a wrongly sent resolved
    # notification never reaches the sink and "no resolved notification" vacuously passes.
    if [[ -n "$channel_id" && "$cli_present" == "true" ]] \
        && cli_call out monitoring notification-route create --request-file "$WORK_DIR/route-catchall.json" --confirm \
        && jq -e '.data.id' <<<"$out" >/dev/null; then
      SEEDED_CATCHALL_ROUTE=true
      log "catch-all route seeded via CLI"
    fi
    if [[ "$SEEDED_CATCHALL_ROUTE" != "true" && -n "$channel_id" ]] \
        && api_call POST "/monitoring/notification-routes" -d @"$WORK_DIR/route-catchall.json" | jq -e '.data.id' >/dev/null; then
      SEEDED_CATCHALL_ROUTE=true
    fi
    [[ "$SEEDED_CATCHALL_ROUTE" == "true" ]] || notice "FROM rejected catch-all route; resolved-notification assert may be vacuous"
  fi

  # 旧巡检策略：FROM 接受 PROM_CPU_HIGH 条件码时植入。
  # Legacy auto policy: seed one carrying the PROM_CPU_HIGH condition when FROM accepts it.
  jq -nc --arg name "$POLICY_NAME" '{
    cluster_id:0,name:$name,enabled:true,cooldown_minutes:30,
    conditions:[{template_code:"PROM_CPU_HIGH",enabled:true}]}' \
    >"$WORK_DIR/policy.json"
  cli_present=false
  cli_has diagnostics auto-policy create && cli_present=true
  if [[ "$cli_present" == "true" ]] \
      && cli_call out diagnostics auto-policy create --request-file "$WORK_DIR/policy.json" --confirm \
      && jq -e '.data.id' <<<"$out" >/dev/null; then
    SEEDED_LEGACY_POLICY=true
    log "legacy auto policy seeded via CLI"
  fi
  if [[ "$SEEDED_LEGACY_POLICY" != "true" ]] && api_call POST "/diagnostics/auto-policies" -d @"$WORK_DIR/policy.json" | jq -e '.data.id' >/dev/null; then
    if [[ "$cli_present" == "true" ]]; then
      notice "CLI auto-policy create failed on FROM; seeded via API fallback"
    else
      notice "installed CLI has no 'diagnostics auto-policy create'; seeded via API fallback"
    fi
    SEEDED_LEGACY_POLICY=true
  fi
  [[ "$SEEDED_LEGACY_POLICY" == "true" ]] || notice "FROM rejected PROM_CPU_HIGH condition; invalid_reason policy assert skipped"

  # 三件套：记录 pid 文件与进程是否在跑。/ Legacy stack: record pid files and live processes.
  local pid
  for pid in prometheus/prometheus alertmanager/alertmanager grafana/grafana; do
    if sudo -n test -f "$INSTALL_DIR/observability/$pid.pid" 2>/dev/null; then
      local pidnum
      pidnum="$(sudo -n cat "$INSTALL_DIR/observability/$pid.pid" 2>/dev/null || true)"
      if [[ -n "$pidnum" ]] && sudo -n kill -0 "$pidnum" 2>/dev/null; then
        LEGACY_STACK=true
      fi
    fi
  done
  if [[ "$LEGACY_STACK" != "true" ]]; then
    notice "no running legacy observability stack detected; stack assertions will be skipped"
  fi

  cat >"$WORK_DIR/seeded.env" <<EOF
SEEDED_CHANNEL=$SEEDED_CHANNEL
SEEDED_FOLDER=$SEEDED_FOLDER
SEEDED_LEGACY_ALERT=$SEEDED_LEGACY_ALERT
SEEDED_LEGACY_ROUTE=$SEEDED_LEGACY_ROUTE
SEEDED_CATCHALL_ROUTE=$SEEDED_CATCHALL_ROUTE
SEEDED_LEGACY_POLICY=$SEEDED_LEGACY_POLICY
LEGACY_STACK=$LEGACY_STACK
EOF
  log "seed state: channel=$SEEDED_CHANNEL folder=$SEEDED_FOLDER alert=$SEEDED_LEGACY_ALERT route=$SEEDED_LEGACY_ROUTE catchall=$SEEDED_CATCHALL_ROUTE policy=$SEEDED_LEGACY_POLICY stack=$LEGACY_STACK"
}

# ── 5. 一键升级到 TARGET（CLI 优先，逐命令探测回退） ─────────────────────────

# probe_script_replace：给 status.sh 追加一行注释，强制升级时的脚本替换路径一定走
# InstallScriptIfChanged——否则当 FROM 与当前版本的 bin 脚本逐字节一致时（例如最新
# 发布与 main 同步后），没有任何 .bak-* 产生，"backup exists" 断言就退化为运气。
# probe_script_replace: appends a comment to status.sh so the upgrade's script-replace
# path through InstallScriptIfChanged is exercised deterministically — without it, a
# FROM whose bin scripts are byte-identical to the target (e.g. the latest release cut
# from current main) produces no .bak-* and the "backup exists" assert is luck.
probe_script_replace() {
  if sudo -n test -f "$INSTALL_DIR/bin/status.sh"; then
    sudo -n bash -c "printf '# ci-upgrade-probe\n' >> '$INSTALL_DIR/bin/status.sh'" \
      || notice "could not touch status.sh; backup-exists assert may be vacuous"
  else
    notice "no bin/status.sh in FROM layout; backup-exists assert may be vacuous"
  fi
}

run_self_upgrade() {
  local out

  PRE_FRONTEND_BUILD_ID="$(sudo -n cat "$INSTALL_DIR/frontend/.next/BUILD_ID" 2>/dev/null || true)"

  log "checking target version via self-upgrade check ..."
  if cli_has self-upgrade check; then
    cli_call out self-upgrade check --force-latest "$TARGET" >/dev/null \
      || die "self-upgrade check failed via CLI"
  else
    notice "installed CLI has no 'self-upgrade check'; checking via API fallback"
    api_call POST "/self-upgrade/check" -d "$(jq -nc --arg v "$TARGET" '{force_latest:$v}')" >/dev/null
  fi

  log "importing upgrade bundle ..."
  if cli_has self-upgrade import; then
    cli_call out self-upgrade import "$UPGRADE_PKG" --confirm >/dev/null \
      || die "self-upgrade import failed via CLI"
    log "bundle imported via CLI"
  else
    notice "installed CLI has no 'self-upgrade import'; importing bundle via API fallback"
    curl -fsS \
      -H "Authorization: Bearer $(cli_token)" \
      -H "X-STX-Client: cli" \
      -F "file=@${UPGRADE_PKG}" \
      "${API_BASE}/self-upgrade/stage/import" | jq -e '.data' >/dev/null \
      || die "upgrade bundle import failed via API fallback"
  fi

  local task_id
  if cli_has self-upgrade execute; then
    cli_call out self-upgrade execute || die "self-upgrade execute failed via CLI"
    task_id="$(jq -r '.data.id // empty' <<<"$out")"
  else
    notice "installed CLI has no 'self-upgrade execute'; executing via API fallback"
    local exec_resp
    exec_resp="$(api_call POST "/self-upgrade/execute" -d '{"mode":"apply"}')"
    task_id="$(jq -r '.data.id // empty' <<<"$exec_resp")"
  fi
  [[ -n "$task_id" ]] || die "execute did not return a task id"
  log "upgrade task: $task_id"

  # 等待任务结束；CLI 的 task get --wait 自带控制面重启瞬断重试。
  # Wait for the task; the CLI's task get --wait already retries transient
  # connection errors while the control plane restarts.
  if cli_has self-upgrade task get; then
    cli_call out self-upgrade task get "$task_id" --wait --timeout 6m >/dev/null \
      || die "upgrade task $task_id did not succeed (see stderr above)"
    log "upgrade task succeeded (waited via CLI)"
  else
    notice "installed CLI has no 'self-upgrade task get'; polling task via API fallback"
    local start now status
    start="$(date +%s)"
    while true; do
      status="$(api_call GET "/self-upgrade/tasks/${task_id}" 2>/dev/null \
        | jq -r '.data.status // empty' 2>/dev/null || true)"
      case "$status" in
        succeeded) log "upgrade task succeeded"; break ;;
        failed|cancelled)
          api_call GET "/self-upgrade/tasks/${task_id}" || true
          die "upgrade task $task_id ended with status=$status"
          ;;
      esac
      now="$(date +%s)"
      ((now - start < 360)) || die "upgrade task timed out (last status: ${status:-unreachable})"
      sleep 5
    done
  fi

  wait_for_version "$TARGET" 360

  # 不重新登录：同一份 CLI 凭据必须在升级后仍有效，并顺手验证新二进制 CLI 可用。
  # No re-login: the same CLI credentials must still work after the upgrade, which
  # also smoke-tests the new binary's CLI surface.
  log "verifying CLI credentials survived the upgrade ..."
  cli_credentials_ok
  log "CLI credentials valid after upgrade"
}

# ── 6. 断言 ──────────────────────────────────────────────────────────────────

FAILED_ASSERTS=()
assert_true() {
  local name="$1"; shift
  if "$@"; then
    log "  PASS $name"
  else
    log "  FAIL $name"
    FAILED_ASSERTS+=("$name")
  fi
}

# 升级重启后 systemd 状态可能短暂处于 activating：轮询最多 ~10s 再判失败。
# systemd may briefly report activating right after the upgrade restart: poll up to ~10s before failing.
systemd_active() {
  local i
  for i in 1 2 3 4 5; do
    sudo -n systemctl is-active --quiet stx && return 0
    sleep 2
  done
  return 1
}

assert_generic() {
  log "assertions: generic"
  local out

  local version
  version="$(curl -fsS "${API_BASE}/version" | jq -r '.data.version // empty')"
  assert_true "version==$TARGET" [ "$version" = "$TARGET" ]

  assert_true "systemd active" systemd_active

  local code
  code="$(curl -fsS -o /dev/null -w '%{http_code}' -L "http://127.0.0.1:17880/" 2>/dev/null || echo 000)"
  assert_true "frontend 200" [ "$code" = "200" ]

  local post_build
  post_build="$(sudo -n cat "$INSTALL_DIR/frontend/.next/BUILD_ID" 2>/dev/null || true)"
  if sudo -n test -d "$INSTALL_DIR/frontend.bak" || sudo -n test -d "$INSTALL_DIR/frontend.failed"; then
    assert_true "frontend files swapped" bash -c "[ -n '$post_build' ] && [ '$post_build' != '$PRE_FRONTEND_BUILD_ID' ]"
  else
    notice "no frontend.bak/frontend.failed: FROM upgrader predates the frontend file swap; swap assert skipped"
  fi

  # 断言一律走 TARGET CLI 的读命令（升级后二进制就位，缺命令即真失败）。
  # Assertions go through the TARGET CLI read commands (post-upgrade binary is in
  # place; a missing command is a real failure).
  if [[ "$SEEDED_CHANNEL" == "true" ]]; then
    local found
    found="$(cli_call out monitoring notification-channel list \
      && jq -r --arg n "$CHANNEL_NAME" '[.data.channels[]? // .data.items[]? | select(.name==$n)] | length' <<<"$out")" \
      || found=0
    assert_true "channel preserved" [ "$found" -ge 1 ]
  fi
  if [[ "$SEEDED_FOLDER" == "true" ]]; then
    local found
    found="$(cli_call out sync tree \
      && jq -r --arg n "$FOLDER_NAME" '[.. | objects | select(.name? == $n)] | length' <<<"$out")" \
      || found=0
    assert_true "workbench folder preserved" [ "$found" -ge 1 ]
  fi

  # 安装布局：bin/start.sh 与仓库一致；observability.sh 已移走；旧文件有备份。
  # Install layout: bin/start.sh identical to the repo copy; observability.sh moved away; backups exist.
  assert_true "bin/start.sh identical" sudo -n cmp -s "$INSTALL_DIR/bin/start.sh" "$ROOT_DIR/install/bin/start.sh"
  assert_true "bin/stop.sh identical" sudo -n cmp -s "$INSTALL_DIR/bin/stop.sh" "$ROOT_DIR/install/bin/stop.sh"
  assert_true "bin/status.sh identical" sudo -n cmp -s "$INSTALL_DIR/bin/status.sh" "$ROOT_DIR/install/bin/status.sh"
  assert_true "observability.sh moved" bash -c "! sudo -n test -f '$INSTALL_DIR/bin/lib/observability.sh'"
  # probe_script_replace 在升级前改动了 status.sh，升级必须替换它并留下 .bak-* 备份。
  # probe_script_replace modified status.sh pre-upgrade; the upgrade must replace
  # it and leave a .bak-* backup.
  assert_true "backup exists" bash -c "sudo -n find '$INSTALL_DIR/bin' -name '*.bak-*' -print -quit | grep -q ."

  assert_true "no panic in backend.log" bash -c "! sudo -n grep -qE 'panic|fatal error' '$INSTALL_DIR/logs/backend.log' 2>/dev/null"
}

assert_legacy_stack() {
  [[ "$LEGACY_STACK" == "true" ]] || { notice "legacy stack assertions skipped"; return 0; }
  log "assertions: legacy observability stack"

  # 升级完成后 3 分 30 秒内三个旧进程全部退出，端口不再监听，数据目录保留。
  # Within 3m30s of the upgrade all three legacy processes exit, ports stop listening, data dir stays.
  local start now alive
  start="$(date +%s)"
  while true; do
    alive=0
    for name in prometheus alertmanager grafana grafana-server; do
      if pgrep -x "$name" >/dev/null 2>&1; then
        alive=$((alive + 1))
      fi
    done
    ((alive == 0)) && break
    now="$(date +%s)"
    if ((now - start >= 210)); then
      FAILED_ASSERTS+=("legacy processes exited")
      log "  FAIL legacy processes exited ($alive still running)"
      break
    fi
    sleep 5
  done
  if ((alive == 0)); then log "  PASS legacy processes exited"; fi

  for port in 9090 9093 3000; do
    assert_true "port $port closed" bash -c "! sudo -n ss -ltn 2>/dev/null | grep -q ':$port '"
  done
  assert_true "observability dir kept" sudo -n test -d "$INSTALL_DIR/observability"

  # --with-observability 安装的 v1.4.0 配置里必有 observability.prometheus 等旧段，
  # 新版启动应记一条「已废弃的旧外部监控配置段」WARN。
  # A v1.4.0 install with --with-observability always writes observability.prometheus
  # style legacy sections; the new binary must WARN about them at startup.
  assert_true "legacy config sections WARN in log" bash -c "sudo -n grep -q '已废弃的旧外部监控配置段\|deprecated external monitoring sections' '$INSTALL_DIR/logs/backend.log' 2>/dev/null"
}

assert_legacy_alert() {
  [[ "$SEEDED_LEGACY_ALERT" == "true" ]] || { notice "legacy alert assertions skipped"; return 0; }
  log "assertions: legacy alerts"
  local out

  # 正向断言：植入的告警必须出现在 resolved 列表——只查 firing 列表没有它，若它根本没
  # 出现在任何列表（字段名/过滤条件不对）断言同样会空转通过。
  # Positive check first: the seeded alert must show up in the resolved list — asserting
  # absence from firing alone vacuously passes when the alert is missing from every list
  # (wrong field name or filter).
  local resolved_count
  resolved_count="$(cli_call out monitoring alert-instance list --status resolved \
    && jq -r --arg n "$ALERT_NAME" '[.data.alerts[]? | select(.alert_name==$n and .status=="resolved")] | length' <<<"$out" 2>/dev/null)" \
    || resolved_count=-1
  assert_true "legacy alert resolved" [ "$resolved_count" -ge 1 ]

  local count
  count="$(cli_call out monitoring alert-instance list --status firing \
    && jq -r --arg n "$ALERT_NAME" '[.data.alerts[]? | select(.alert_name==$n)] | length' <<<"$out" 2>/dev/null)" \
    || count=-1
  assert_true "legacy alert not firing" [ "$count" = "0" ]

  # 渠道接收端不能收到 resolved 通知（收尾不发送恢复通知）。只有兜底路由植入成功
  # 这条断言才有意义：匹配全告警的路由若收到恢复通知一定会写进 sink。
  # The webhook sink must not have received a resolved notification (cleanup sends none).
  # This only proves something once the catch-all route exists: an all-alert route would
  # deliver a wrongly sent resolved event into the sink.
  if [[ "$SEEDED_CATCHALL_ROUTE" == "true" ]]; then
    assert_true "no resolved notification" bash -c "! grep -qi 'resolved' '$SINK_LOG' 2>/dev/null || ! grep -qi '$ALERT_NAME' '$SINK_LOG'"
  else
    notice "catch-all route not seeded; resolved-notification assert skipped"
  fi
}

assert_invalid_configs() {
  log "assertions: invalid config surfacing"
  local out

  if [[ "$SEEDED_LEGACY_ROUTE" == "true" ]]; then
    # 目标版本可能已移除 notification-route API（告警模型整合后路由并入通知渠道）；
    # list 探活失败说明该版本不再暴露路由实体，也就没有可标记 invalid 的对象。
    # The target may have dropped the notification-route API (routes folded into
    # notification channels by the alert-model consolidation); when the list probe
    # fails there is no route entity left to mark invalid, so skip the asserts.
    if cli_call out monitoring notification-route list >/dev/null 2>&1; then
      local reason detail
      reason="$(cli_call out monitoring notification-route list \
        && jq -r --arg n "$ROUTE_NAME" '[.data.routes[]? | select(.name==$n)][0].invalid_reason // empty' <<<"$out" 2>/dev/null)" \
        || reason=""
      detail="$(cli_call out monitoring notification-route list \
        && jq -r --arg n "$ROUTE_NAME" '[.data.routes[]? | select(.name==$n)][0].invalid_detail // empty' <<<"$out" 2>/dev/null)" \
        || detail=""
      assert_true "route invalid_reason=legacy_source_type" [ "$reason" = "legacy_source_type" ]
      assert_true "route invalid_detail=remote_alertmanager" [ "$detail" = "remote_alertmanager" ]
      assert_true "route WARN in log" bash -c "sudo -n grep -qi '通知路由.*失效\|notification route.*invalid\|invalid.*notification route' '$INSTALL_DIR/logs/backend.log' 2>/dev/null"
    else
      notice "target has no 'monitoring notification-route' API; route invalid_reason assertions skipped"
    fi
  else
    notice "route invalid_reason assertions skipped"
  fi

  if [[ "$SEEDED_LEGACY_POLICY" == "true" ]]; then
    local reason detail
    reason="$(cli_call out diagnostics auto-policy list \
      && jq -r --arg n "$POLICY_NAME" '[.data.items[]? | select(.name==$n)][0].invalid_reason // empty' <<<"$out" 2>/dev/null)" \
      || reason=""
    detail="$(cli_call out diagnostics auto-policy list \
      && jq -r --arg n "$POLICY_NAME" '[.data.items[]? | select(.name==$n)][0].invalid_detail // empty' <<<"$out" 2>/dev/null)" \
      || detail=""
    assert_true "policy invalid_reason=legacy_condition_code" [ "$reason" = "legacy_condition_code" ]
    assert_true "policy invalid_detail=PROM_CPU_HIGH" [ "$detail" = "PROM_CPU_HIGH" ]
    assert_true "policy WARN in log" bash -c "sudo -n grep -qi '巡检.*失效\|auto.*polic.*invalid\|invalid.*auto.*polic' '$INSTALL_DIR/logs/backend.log' 2>/dev/null"
  else
    notice "policy invalid_reason assertions skipped"
  fi
}

assert_second_restart() {
  log "assertions: second restart"
  sudo -n systemctl restart stx
  wait_for_version "$TARGET" 240
  # 第二次重启后再验证一次凭据，仍不重新登录。/ Verify stored credentials again
  # after the second restart, still without re-login.
  cli_credentials_ok
  assert_generic
  if [[ "$LEGACY_STACK" == "true" ]]; then
    for name in prometheus alertmanager grafana grafana-server; do
      assert_true "no $name after restart" bash -c "! pgrep -x '$name' >/dev/null 2>&1"
    done
  fi
}

# ── 7. 汇总 ──────────────────────────────────────────────────────────────────

finish() {
  cat >"$COLLECT_DIR/summary.txt" <<EOF
from=$FROM_TAG target=$TARGET
channel=$SEEDED_CHANNEL folder=$SEEDED_FOLDER alert=$SEEDED_LEGACY_ALERT route=$SEEDED_LEGACY_ROUTE policy=$SEEDED_LEGACY_POLICY stack=$LEGACY_STACK
failed_asserts=${FAILED_ASSERTS[*]:-none}
EOF
  if ((${#FAILED_ASSERTS[@]} > 0)); then
    log "FAILED: ${FAILED_ASSERTS[*]}"
    exit 1
  fi
  log "all assertions passed: $FROM_TAG -> $TARGET"
}

# open_install_dir_for_cli：install.sh 用 tar 管道复制源码树，顶层条目继承了打包
# 暂存目录的 700 权限，/opt/stx 会变成 drwx------，runner 用户进不去也就跑不了其中的
# stx CLI；放开目录与二进制的执行位（内容不变，不影响布局断言）。
# open_install_dir_for_cli: install.sh copies the source tree through a tar pipe, so the
# top-level entry inherits mode 700 from the packaging stage dir — /opt/stx ends up
# drwx------ and the runner cannot traverse it to exec the installed CLI. Open the
# execute bits on the dir and binary (contents unchanged; layout asserts unaffected).
open_install_dir_for_cli() {
  sudo -n chmod o+x "$INSTALL_DIR" || die "cannot open execute bit on $INSTALL_DIR"
  sudo -n chmod a+x "$INSTALL_DIR/stx" || die "cannot chmod +x $INSTALL_DIR/stx"
}

main() {
  require_cmds curl jq python3 sudo
  mkdir -p "$COLLECT_DIR"
  resolve_from_version
  resolve_target_version
  if same_version_release; then
    notice "FROM ($FROM_TAG) == TARGET (v$TARGET): same-version re-release, upgrade leg skipped"
    exit 0
  fi
  build_upgrade_package
  install_from
  wait_for_version "$FROM_VER"
  open_install_dir_for_cli
  cli_login
  seed_data
  probe_script_replace
  run_self_upgrade
  assert_generic
  assert_legacy_stack
  assert_legacy_alert
  assert_invalid_configs
  assert_second_restart
  finish
}

main "$@"
