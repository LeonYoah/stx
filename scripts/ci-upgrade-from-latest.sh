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
# STX cross-version upgrade CI: install the latest stable release (releases/latest,
# or --from), build an in-app upgrade bundle from the current tree, upgrade through
# the self-upgrade API and assert. Capability-dependent seeds/assertions probe first:
# when the latest release no longer ships the legacy monitoring stack / webhook /
# legacy values, those legs are skipped while the generic assertions still hold.
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
DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist/upgrade-ci}"
COLLECT_DIR="${COLLECT_DIR:-$ROOT_DIR/dist/upgrade-ci-artifacts}"
WORK_DIR="$(mktemp -d)"
SINK_PORT="${SINK_PORT:-18999}"
SINK_LOG="$WORK_DIR/webhook-sink.log"
COOKIE_JAR="$WORK_DIR/cookies.txt"

ADMIN_PASSWORD="${ADMIN_PASSWORD:-ci-upgrade-$(date +%s | tail -c 7)}"
FROM_VERSION="${FROM_VERSION:-}"
TARGET_VERSION="${TARGET_VERSION:-}"
LEGACY_STACK=false
SEEDED_CHANNEL=false
SEEDED_FOLDER=false
SEEDED_LEGACY_ALERT=false
SEEDED_LEGACY_ROUTE=false
SEEDED_LEGACY_POLICY=false
ALERT_NAME="ci_legacy_alert"
CHANNEL_NAME="ci-upgrade-channel"
FOLDER_NAME="ci-upgrade-folder"
ROUTE_NAME="ci-upgrade-route"
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

# http_request <method> <path> [curl 额外参数...]：带会话 cookie 调本机 API，输出 body。
# http_request <method> <path> [extra curl args...]: call the local API with the session cookie jar.
http_request() {
  local method="$1" path="$2"
  shift 2
  curl -fsS -X "$method" -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -H 'Content-Type: application/json' \
    "$@" "${API_BASE}${path}"
}

# http_status <method> <path>：只取状态码（允许失败），用于能力探测。
# http_status <method> <path>: probe-only variant returning the HTTP status code.
http_status() {
  local method="$1" path="$2" payload="${3:-}"
  local args=(-fsS -o /dev/null -w '%{http_code}' -X "$method" -b "$COOKIE_JAR" -c "$COOKIE_JAR")
  if [[ -n "$payload" ]]; then
    args+=(-H 'Content-Type: application/json' -d "$payload")
  fi
  curl "${args[@]}" "${API_BASE}${path}" 2>/dev/null || echo "000"
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

# ── 4. 在 FROM 上植入数据（先探测能力，不支持就跳过） ─────────────────────────

login_admin() {
  curl -fsS -c "$COOKIE_JAR" -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"${ADMIN_PASSWORD}\"}" \
    "${API_BASE}/auth/login" | jq -e '.data.id // .data.ID // empty' >/dev/null \
    || die "admin login failed"
  log "logged in as admin"
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

  # 通用数据：通知渠道（webhook 指向本机记录端口）+ 工作台目录。
  # Generic seeds: a notification channel (webhook to the local recorder) and a workbench folder.
  if http_request POST "/monitoring/notification-channels" -d "$(jq -nc \
      --arg name "$CHANNEL_NAME" --arg url "http://127.0.0.1:${SINK_PORT}/notify" \
      '{name:$name,type:"webhook",enabled:true,endpoint:$url,description:"ci upgrade probe"}')" \
      | jq -e '.data.id' >/dev/null; then
    SEEDED_CHANNEL=true
  else
    notice "FROM does not accept notification channel creation; data-retention channel assert skipped"
  fi

  if http_request POST "/sync/tasks" -d "$(jq -nc --arg name "$FOLDER_NAME" \
      '{name:$name,node_type:"folder",content_format:"hocon",content:"",mode:"streaming"}')" \
      | jq -e '.data.id' >/dev/null; then
    SEEDED_FOLDER=true
  else
    notice "FROM does not accept workbench folder creation; folder assert skipped"
  fi

  # 遗留告警：FROM 还暴露 Alertmanager webhook 时写入一条 firing 告警（generatorURL 非空）。
  # Legacy alert: when FROM still exposes the Alertmanager webhook, push one firing alert (non-empty generatorURL).
  if [[ "$(http_status POST "/monitoring/alertmanager/webhook")" != "404" ]]; then
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
    if [[ "$(http_status POST "/monitoring/alertmanager/webhook" "$payload")" == "200" ]]; then
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
    local channel_id
    channel_id="$(http_request GET "/monitoring/notification-channels" | jq -r --arg n "$CHANNEL_NAME" '.data.channels[]? // .data.items[]? // empty | select(.name==$n) | .id' | head -1)"
    if [[ -n "$channel_id" ]] && http_request POST "/monitoring/notification-routes" -d "$(jq -nc \
        --arg name "$ROUTE_NAME" --argjson ch "$channel_id" \
        '{name:$name,enabled:true,source_type:"remote_alertmanager",channel_id:$ch,send_resolved:true}')" \
        | jq -e '.data.id' >/dev/null; then
      SEEDED_LEGACY_ROUTE=true
      log "legacy route seeded (id via list)"
    else
      notice "FROM rejected legacy source_type route; invalid_reason route assert skipped"
    fi
  fi

  # 旧巡检策略：FROM 接受 PROM_CPU_HIGH 条件码时植入。
  # Legacy auto policy: seed one carrying the PROM_CPU_HIGH condition when FROM accepts it.
  if http_request POST "/diagnostics/auto-policies" -d "$(jq -nc --arg name "$POLICY_NAME" '{
      cluster_id:0,name:$name,enabled:true,cooldown_minutes:30,
      conditions:[{template_code:"PROM_CPU_HIGH",enabled:true}]}')" \
      | jq -e '.data.id' >/dev/null; then
    SEEDED_LEGACY_POLICY=true
    log "legacy auto policy seeded"
  else
    notice "FROM rejected PROM_CPU_HIGH condition; invalid_reason policy assert skipped"
  fi

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
SEEDED_LEGACY_POLICY=$SEEDED_LEGACY_POLICY
LEGACY_STACK=$LEGACY_STACK
EOF
  log "seed state: channel=$SEEDED_CHANNEL folder=$SEEDED_FOLDER alert=$SEEDED_LEGACY_ALERT route=$SEEDED_LEGACY_ROUTE policy=$SEEDED_LEGACY_POLICY stack=$LEGACY_STACK"
}

# ── 5. 一键升级到 TARGET（用 FROM 自带的升级器） ─────────────────────────────

run_self_upgrade() {
  log "checking target version via self-upgrade api ..."
  http_request POST "/self-upgrade/check" -d "$(jq -nc --arg v "$TARGET" '{force_latest:$v}')" >/dev/null

  log "importing upgrade bundle ..."
  curl -fsS -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -F "file=@${UPGRADE_PKG}" \
    "${API_BASE}/self-upgrade/stage/import" | jq -e '.data' >/dev/null

  local exec_resp task_id
  exec_resp="$(http_request POST "/self-upgrade/execute" -d '{"mode":"apply"}')"
  task_id="$(jq -r '.data.id // empty' <<<"$exec_resp")"
  [[ -n "$task_id" ]] || die "execute did not return a task id: $exec_resp"
  log "upgrade task: $task_id"

  # 轮询任务直到结束；控制面重启期间 API 短暂不可用属于正常。
  # Poll the task to completion; brief API outages during the restart are expected.
  local start now status
  start="$(date +%s)"
  while true; do
    status="$(curl -fsS -b "$COOKIE_JAR" -c "$COOKIE_JAR" "${API_BASE}/self-upgrade/tasks/${task_id}" 2>/dev/null \
      | jq -r '.data.status // empty' 2>/dev/null || true)"
    case "$status" in
      succeeded) log "upgrade task succeeded"; break ;;
      failed|cancelled)
        http_request GET "/self-upgrade/tasks/${task_id}" || true
        die "upgrade task $task_id ended with status=$status"
        ;;
    esac
    now="$(date +%s)"
    ((now - start < 360)) || die "upgrade task timed out (last status: ${status:-unreachable})"
    sleep 5
  done

  # 重启期间会话 cookie 可能失效：等新版本就绪后重新登录。
  # The restart can invalidate the session cookie: wait for the target version, then log in again.
  wait_for_version "$TARGET" 360
  login_admin
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

assert_generic() {
  log "assertions: generic"

  local version
  version="$(curl -fsS "${API_BASE}/version" | jq -r '.data.version // empty')"
  assert_true "version==$TARGET" [[ "$version" == "$TARGET" ]]

  assert_true "systemd active" sudo -n systemctl is-active --quiet stx

  local code
  code="$(curl -fsS -o /dev/null -w '%{http_code}' -L "http://127.0.0.1:17880/" 2>/dev/null || echo 000)"
  assert_true "frontend 200" [[ "$code" == "200" ]]

  if [[ "$SEEDED_CHANNEL" == "true" ]]; then
    local found
    found="$(http_request GET "/monitoring/notification-channels" | jq -r --arg n "$CHANNEL_NAME" '[.data.channels[]? // .data.items[]? | select(.name==$n)] | length')"
    assert_true "channel preserved" [[ "$found" -ge 1 ]]
  fi
  if [[ "$SEEDED_FOLDER" == "true" ]]; then
    local found
    found="$(http_request GET "/sync/tree" | jq -r --arg n "$FOLDER_NAME" '[.. | objects | select(.name? == $n)] | length')"
    assert_true "workbench folder preserved" [[ "$found" -ge 1 ]]
  fi

  # 安装布局：bin/start.sh 与仓库一致；observability.sh 已移走；旧文件有备份。
  # Install layout: bin/start.sh identical to the repo copy; observability.sh moved away; backups exist.
  assert_true "bin/start.sh identical" sudo -n cmp -s "$INSTALL_DIR/bin/start.sh" "$ROOT_DIR/install/bin/start.sh"
  assert_true "bin/stop.sh identical" sudo -n cmp -s "$INSTALL_DIR/bin/stop.sh" "$ROOT_DIR/install/bin/stop.sh"
  assert_true "bin/status.sh identical" sudo -n cmp -s "$INSTALL_DIR/bin/status.sh" "$ROOT_DIR/install/bin/status.sh"
  assert_true "observability.sh moved" bash -c "! sudo -n test -f '$INSTALL_DIR/bin/lib/observability.sh'"
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
}

assert_legacy_alert() {
  [[ "$SEEDED_LEGACY_ALERT" == "true" ]] || { notice "legacy alert assertions skipped"; return 0; }
  log "assertions: legacy alerts"

  # 植入的 firing 告警不应再出现在 firing 实例列表。
  # The seeded firing alert must no longer show up in firing instances.
  local count
  count="$(http_request GET "/monitoring/alert-instances?status=firing" | jq -r --arg n "$ALERT_NAME" '[.data.alerts[]? | select(.alert_name==$n)] | length' 2>/dev/null || echo -1)"
  assert_true "legacy alert not firing" [[ "$count" == "0" ]]

  # 渠道接收端不能收到 resolved 通知（收尾不发送恢复通知）。
  # The webhook sink must not have received a resolved notification (cleanup sends none).
  assert_true "no resolved notification" bash -c "! grep -qi 'resolved' '$SINK_LOG' 2>/dev/null || ! grep -qi '$ALERT_NAME' '$SINK_LOG'"
}

assert_invalid_configs() {
  log "assertions: invalid config surfacing"

  if [[ "$SEEDED_LEGACY_ROUTE" == "true" ]]; then
    local reason
    reason="$(http_request GET "/monitoring/notification-routes" | jq -r --arg n "$ROUTE_NAME" '[.data.routes[]? | select(.name==$n)][0].invalid_reason // empty' 2>/dev/null || true)"
    assert_true "route invalid_reason" [[ -n "$reason" ]]
    assert_true "route WARN in log" bash -c "sudo -n grep -qi '通知路由.*失效\|notification route.*invalid\|invalid.*notification route' '$INSTALL_DIR/logs/backend.log' 2>/dev/null"
  else
    notice "route invalid_reason assertions skipped"
  fi

  if [[ "$SEEDED_LEGACY_POLICY" == "true" ]]; then
    local reason
    reason="$(http_request GET "/diagnostics/auto-policies" | jq -r --arg n "$POLICY_NAME" '[.data.items[]? | select(.name==$n)][0].invalid_reason // empty' 2>/dev/null || true)"
    assert_true "policy invalid_reason" [[ -n "$reason" ]]
    assert_true "policy WARN in log" bash -c "sudo -n grep -qi '巡检.*失效\|auto.*polic.*invalid\|invalid.*auto.*polic' '$INSTALL_DIR/logs/backend.log' 2>/dev/null"
  else
    notice "policy invalid_reason assertions skipped"
  fi
}

assert_second_restart() {
  log "assertions: second restart"
  sudo -n systemctl restart stx
  wait_for_version "$TARGET" 240
  login_admin
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

main() {
  require_cmds curl jq python3 sudo
  mkdir -p "$COLLECT_DIR"
  resolve_from_version
  resolve_target_version
  build_upgrade_package
  install_from
  wait_for_version "$FROM_VER"
  login_admin
  seed_data
  run_self_upgrade
  assert_generic
  assert_legacy_stack
  assert_legacy_alert
  assert_invalid_configs
  assert_second_restart
  finish
}

main "$@"
