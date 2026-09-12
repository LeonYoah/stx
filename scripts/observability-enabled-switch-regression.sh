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

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN_PATH="${STX_BIN:-$ROOT_DIR/stx}"
RUN_MODE="${STX_RUN_MODE:-go_run}" # go_run | binary
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

if [[ "$RUN_MODE" == "binary" ]]; then
  if [[ ! -x "$BIN_PATH" ]]; then
    echo "stx binary not found or not executable: $BIN_PATH" >&2
    exit 1
  fi
fi

wait_health() {
  local base_url="$1"
  for _ in $(seq 1 60); do
    if curl -sS -m 1 "$base_url/api/v1/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

# 为开关回归用例生成独立的 STX 配置。/ Generate an isolated STX configuration for a switch regression case.
write_config() {
  local cfg_path="$1"
  local addr="$2"
  local external_url="$3"
  local db_path="$4"
  local enabled="$5"
  cat >"$cfg_path" <<YAML
app:
  app_name: "STX"
  env: "development"
  addr: "$addr"
  external_url: "$external_url"
  session_cookie_name: "stx_session_id"
  session_secret: "regression-secret"
  session_domain: ""
  session_age: 86400
  session_secure: false
  session_http_only: true
  api_prefix: "/api"

auth:
  default_admin_username: "admin"
  default_admin_password: "admin123"
  bcrypt_cost: 10

database:
  enabled: true
  type: "sqlite"
  sqlite_path: "$db_path"

redis:
  enabled: false

grpc:
  enabled: false

log:
  level: "error"
  format: "text"
  output: "stdout"

observability:
  enabled: $enabled
  prometheus:
    url: "http://127.0.0.1:9090"
    http_sd_path: "/api/v1/monitoring/prometheus/discovery"
  alertmanager:
    url: "http://127.0.0.1:9093"
    webhook_path: "/api/v1/monitoring/alertmanager/webhook"
  grafana:
    url: "http://127.0.0.1:3000"
  seatunnel_metrics:
    path: "/metrics"
    probe_timeout_seconds: 1
YAML
}

# 启动并验证一个可观测性开关场景。/ Start and verify one observability switch scenario.
run_case() {
  local mode="$1" # enabled|disabled
  local enabled="$2"
  local port="$3"

  local case_dir="$TMP_ROOT/$mode"
  mkdir -p "$case_dir"
  local cfg="$case_dir/config.yaml"
  local log_file="$case_dir/app.log"
  local db_file="$case_dir/stx.db"
  local base_url="http://127.0.0.1:$port"

  write_config "$cfg" ":$port" "$base_url" "$db_file" "$enabled"

  echo "=== CASE: observability.enabled=$enabled ($mode) ==="
  if [[ "$RUN_MODE" == "binary" ]]; then
    CONFIG_PATH="$cfg" "$BIN_PATH" api >"$log_file" 2>&1 &
  else
    (
      cd "$ROOT_DIR"
      CONFIG_PATH="$cfg" go run . api >"$log_file" 2>&1
    ) &
  fi
  local pid=$!
  trap 'kill "$pid" >/dev/null 2>&1 || true' RETURN

  if ! wait_health "$base_url"; then
    echo "[ERROR] health endpoint not ready for case: $mode"
    tail -n 80 "$log_file" || true
    exit 1
  fi

  local status_health
  status_health=$(curl -sS -o "$case_dir/health.json" -w '%{http_code}' "$base_url/api/v1/health")
  echo "health => HTTP $status_health"

  local status_sd
  status_sd=$(curl -sS -o "$case_dir/discovery.json" -w '%{http_code}' "$base_url/api/v1/monitoring/prometheus/discovery")
  echo "prometheus/discovery => HTTP $status_sd"

  local webhook_payload
  webhook_payload='{"receiver":"stx","status":"firing","alerts":[{"status":"firing","labels":{"alertname":"SmokeAlert","severity":"warning","cluster_id":"1","cluster_name":"demo","env":"test"},"annotations":{"summary":"regression"},"startsAt":"2026-02-27T00:00:00Z","endsAt":"2026-02-27T00:05:00Z","generatorURL":"http://example.com"}]}'
  local status_webhook
  status_webhook=$(curl -sS -o "$case_dir/webhook.json" -w '%{http_code}' -X POST "$base_url/api/v1/monitoring/alertmanager/webhook" \
    -H 'Content-Type: application/json' \
    --data "$webhook_payload")
  echo "alertmanager/webhook => HTTP $status_webhook"

  if [[ "$enabled" == "true" ]]; then
    [[ "$status_sd" == "200" ]] || { echo "[FAIL] enabled=true but discovery is not 200"; exit 1; }
    [[ "$status_webhook" == "200" ]] || { echo "[FAIL] enabled=true but webhook is not 200"; exit 1; }
  else
    [[ "$status_sd" == "404" ]] || { echo "[FAIL] enabled=false but discovery is not 404"; exit 1; }
    [[ "$status_webhook" == "404" ]] || { echo "[FAIL] enabled=false but webhook is not 404"; exit 1; }
  fi

  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" 2>/dev/null || true
  trap - RETURN

  echo "[PASS] case $mode"
  echo
}

run_case "enabled" "true" "18881"
run_case "disabled" "false" "18882"

echo "All observability enabled-switch regression cases passed."
