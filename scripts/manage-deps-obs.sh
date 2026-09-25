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

# 本机可观测性三件套管理脚本（Prometheus / Alertmanager / Grafana）
# Management script for local host observability suite (Prometheus / Alertmanager / Grafana)

set -euo pipefail

BASE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DEPS_DIR="$BASE_DIR/deps"
ALERT_DIR="$DEPS_DIR/alertmanager-0.31.1"
PROM_DIR="$DEPS_DIR/prometheus-3.9.1"
GRAFANA_DIR="$DEPS_DIR/grafana-12.3.3"

# 检查进程运行状态
# Check running status of a process
is_running() {
  local pidfile="$1"
  if [[ -f "$pidfile" ]]; then
    local pid
    pid="$(cat "$pidfile" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
  fi
  return 1
}

# 停止单服务
# Stop a single service
stop_service() {
  local name="$1"
  local pidfile="$2"
  if [[ -f "$pidfile" ]]; then
    local pid
    pid="$(cat "$pidfile" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      echo "停止 / Stopping $name (pid=$pid)..."
      kill "$pid" || true
      sleep 1
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" || true
      fi
    else
      echo "$name 未在运行 / not running"
    fi
    rm -f "$pidfile"
  else
    echo "$name 未在运行 (无 pid 文件) / not running (no pidfile)"
  fi
}

# 启动单服务
# Start all three services
start() {
  mkdir -p "$ALERT_DIR/logs" "$PROM_DIR/logs" "$GRAFANA_DIR/logs"

  # 1. Alertmanager (9093)
  if is_running "$ALERT_DIR/alertmanager.pid"; then
    echo "Alertmanager 已在运行 / Alertmanager is already running (pid=$(cat "$ALERT_DIR/alertmanager.pid"))"
  else
    echo "启动 / Starting Alertmanager on :9093..."
    setsid sh -c "exec $ALERT_DIR/alertmanager --config.file=$ALERT_DIR/alertmanager.yml --storage.path=$ALERT_DIR/data --web.listen-address=:9093 >> $ALERT_DIR/logs/alertmanager.log 2>&1" < /dev/null &
    echo $! >"$ALERT_DIR/alertmanager.pid"
  fi

  # 2. Prometheus (9090)
  if is_running "$PROM_DIR/prometheus.pid"; then
    echo "Prometheus 已在运行 / Prometheus is already running (pid=$(cat "$PROM_DIR/prometheus.pid"))"
  else
    echo "启动 / Starting Prometheus on :9090..."
    setsid sh -c "exec $PROM_DIR/prometheus --config.file=$PROM_DIR/prometheus.yml --storage.tsdb.path=$PROM_DIR/data --web.listen-address=:9090 --web.enable-lifecycle >> $PROM_DIR/logs/prometheus.log 2>&1" < /dev/null &
    echo $! >"$PROM_DIR/prometheus.pid"
  fi

  # 3. Grafana (3000)
  if is_running "$GRAFANA_DIR/grafana.pid"; then
    echo "Grafana 已在运行 / Grafana is already running (pid=$(cat "$GRAFANA_DIR/grafana.pid"))"
  else
    echo "启动 / Starting Grafana on :3000..."
    setsid sh -c "exec $GRAFANA_DIR/bin/grafana server --homepath=$GRAFANA_DIR --config=$GRAFANA_DIR/conf/grafana.ini >> $GRAFANA_DIR/logs/grafana.log 2>&1" < /dev/null &
    echo $! >"$GRAFANA_DIR/grafana.pid"
  fi

  sleep 2
  echo "可观测性三件套状态检查 / Checking status..."
  status
}

# 停止所有服务
# Stop all three services
stop() {
  stop_service "Grafana" "$GRAFANA_DIR/grafana.pid"
  stop_service "Prometheus" "$PROM_DIR/prometheus.pid"
  stop_service "Alertmanager" "$ALERT_DIR/alertmanager.pid"
}

# 状态汇报
# Report status
status() {
  echo "--- 进程状态 / Process Status ---"
  for item in "Alertmanager:$ALERT_DIR/alertmanager.pid" "Prometheus:$PROM_DIR/prometheus.pid" "Grafana:$GRAFANA_DIR/grafana.pid"; do
    local name="${item%%:*}"
    local pidfile="${item##*:}"
    if is_running "$pidfile"; then
      echo "  $name: 运行中 / running (pid=$(cat "$pidfile"))"
    else
      echo "  $name: 已停止 / stopped"
    fi
  done

  echo "--- 端口监听 / Listening Ports ---"
  ss -lntp 2>/dev/null | grep -E ":9090|:9093|:3000\b" || echo "  (无监听端口 / no active ports)"
}

case "${1:-status}" in
  start)
    start
    ;;
  stop)
    stop
    ;;
  restart)
    stop
    sleep 1
    start
    ;;
  status)
    status
    ;;
  *)
    echo "用法 / Usage: $0 {start|stop|restart|status}"
    exit 1
    ;;
esac
