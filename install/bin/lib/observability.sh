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

# Internal helpers for bundled Prometheus / Alertmanager / Grafana.
# Sourced by bin/start.sh, stop.sh, status.sh — not a user-facing entry.
# 内部三件套辅助函数；由 bin 启停脚本 source，不对用户暴露。

# Resolve observability mode from CLI/env. Default: auto.
# 解析可观测性模式，默认 auto。
stx_obs_resolve_mode() {
  local from_cli="${1:-}"
  local from_env="${2:-}"
  local mode="${from_cli:-${from_env:-auto}}"
  case "$mode" in
    auto|on|off|true|false|1|0) ;;
    *)
      echo "invalid observability mode: $mode (expected auto|on|off)" >&2
      return 1
      ;;
  esac
  case "$mode" in
    true|1) mode=on ;;
    false|0) mode=off ;;
  esac
  printf '%s' "$mode"
}

# Read observability.enabled from config.yaml (best-effort). Empty if unset.
# 尽力读取 config.yaml 中 observability.enabled。
stx_obs_config_enabled() {
  local config="${1:-}"
  [[ -f "$config" ]] || { printf ''; return 0; }
  local line
  line="$(awk '
    /^[[:space:]]*observability:[[:space:]]*$/ { in_obs=1; next }
    in_obs && /^[^[:space:]#]/ { in_obs=0 }
    in_obs && /^[[:space:]]*enabled:[[:space:]]*/ {
      gsub(/^[[:space:]]*enabled:[[:space:]]*/, "", $0)
      gsub(/[[:space:]]*(#.*)?$/, "", $0)
      gsub(/["'\'']/, "", $0)
      print $0
      exit
    }
  ' "$config" 2>/dev/null || true)"
  printf '%s' "$line"
}

# Fixed layout under $BASE_DIR/observability/{prometheus,alertmanager,grafana}.
# 固定目录布局。
stx_obs_dirs() {
  OBS_ROOT="${BASE_DIR}/observability"
  PROM_DIR="${OBS_ROOT}/prometheus"
  ALERT_DIR="${OBS_ROOT}/alertmanager"
  GRAFANA_DIR="${OBS_ROOT}/grafana"
}

# Return 0 if local stack binaries look runnable.
# 本地三件套二进制是否齐全。
stx_obs_stack_present() {
  stx_obs_dirs
  [[ -x "$PROM_DIR/prometheus" ]] || return 1
  [[ -x "$ALERT_DIR/alertmanager" ]] || return 1
  [[ -x "$GRAFANA_DIR/bin/grafana" || -x "$GRAFANA_DIR/bin/grafana-server" ]] || return 1
  return 0
}

# Grafana binary path (oss layout varies by version).
stx_obs_grafana_bin() {
  stx_obs_dirs
  if [[ -x "$GRAFANA_DIR/bin/grafana" ]]; then
    printf '%s' "$GRAFANA_DIR/bin/grafana"
  elif [[ -x "$GRAFANA_DIR/bin/grafana-server" ]]; then
    printf '%s' "$GRAFANA_DIR/bin/grafana-server"
  else
    return 1
  fi
}

# Decide whether to start local processes for a given mode.
# 按模式决定是否启动本地进程。打印原因到 stdout；返回 0=启动 1=跳过 2=硬失败。
stx_obs_should_start() {
  local mode="$1"
  local config="${2:-$CONFIG_PATH}"
  case "$mode" in
    off)
      echo "observability: skipped (--observability off)"
      return 1
      ;;
    on)
      if ! stx_obs_stack_present; then
        echo "observability: required by --observability on but stack missing under $BASE_DIR/observability" >&2
        return 2
      fi
      local enabled
      enabled="$(stx_obs_config_enabled "$config")"
      if [[ "$enabled" == "false" || "$enabled" == "0" ]]; then
        echo "observability: warning — config observability.enabled=$enabled but --observability on forces start" >&2
      fi
      return 0
      ;;
    auto|*)
      if ! stx_obs_stack_present; then
        echo "observability: skipped (no local stack under observability/)"
        return 1
      fi
      local enabled
      enabled="$(stx_obs_config_enabled "$config")"
      if [[ "$enabled" == "false" || "$enabled" == "0" ]]; then
        echo "observability: skipped (observability.enabled=$enabled)"
        return 1
      fi
      return 0
      ;;
  esac
}

# Generate default configs from templates into component dirs.
# 从模板生成默认配置到组件目录。
stx_obs_init_defaults() {
  local obs_root="${1:-$BASE_DIR/observability}"
  local prom_dir="$obs_root/prometheus"
  local alert_dir="$obs_root/alertmanager"
  local grafana_dir="$obs_root/grafana"

  if [[ ! -d "$prom_dir" || ! -d "$alert_dir" || ! -d "$grafana_dir" ]]; then
    echo "observability init skipped: component dirs missing under $obs_root" >&2
    return 1
  fi

  local render="$obs_root/render.sh"
  if [[ ! -x "$render" ]]; then
    echo "observability render.sh missing under $obs_root" >&2
    return 1
  fi

  # Render prometheus / alertmanager / grafana provisioning into obs_root.
  # 渲染 prometheus / alertmanager / grafana provisioning 到 obs_root。
  GRAFANA_DASHBOARDS_PATH="$grafana_dir/dashboards" \
    "$render" bare "$obs_root"

  # Binary layout expects configs inside component dirs (already rendered for prom/am).
  # Grafana still needs grafana.ini with local data paths.
  local prometheus_url="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
  local grafana_url="${GRAFANA_URL:-http://127.0.0.1:3000}"
  grafana_url="${grafana_url%/}"
  local grafana_domain="${GRAFANA_DOMAIN:-}"
  local grafana_proxy_subpath="${GRAFANA_PROXY_SUBPATH:-/api/v1/monitoring/proxy/grafana}"
  local grafana_root_url="${GRAFANA_ROOT_URL:-${grafana_proxy_subpath%/}/}"
  local grafana_admin_user="${GRAFANA_ADMIN_USER:-admin}"
  local grafana_admin_password="${GRAFANA_ADMIN_PASSWORD:-admin}"

  if [[ -z "$grafana_domain" ]]; then
    grafana_domain="${grafana_url#*://}"
    grafana_domain="${grafana_domain%%/*}"
    grafana_domain="${grafana_domain%%:*}"
    [[ -z "$grafana_domain" ]] && grafana_domain="127.0.0.1"
  fi

  mkdir -p \
    "$prom_dir/rules" "$prom_dir/data" "$prom_dir/logs" \
    "$alert_dir/data" "$alert_dir/logs" \
    "$grafana_dir/data" "$grafana_dir/logs" "$grafana_dir/plugins" "$grafana_dir/conf" \
    "$grafana_dir/provisioning/datasources" \
    "$grafana_dir/provisioning/dashboards" \
    "$grafana_dir/dashboards"

  # Move rendered files into component runtime dirs if render wrote top-level layout.
  # render.sh 输出为 prometheus/ prometheus.yml；与二进制目录同级即可直接用。
  if [[ -f "$obs_root/prometheus/prometheus.yml" ]]; then
    :
  fi
  if [[ -f "$obs_root/alertmanager/alertmanager.yml" ]]; then
    :
  fi

  # Copy grafana provisioning from render output into grafana runtime tree.
  if [[ -d "$obs_root/grafana/provisioning" ]]; then
    cp -a "$obs_root/grafana/provisioning/." "$grafana_dir/provisioning/"
  fi
  if [[ -d "$obs_root/grafana/dashboards" ]]; then
    cp -a "$obs_root/grafana/dashboards/." "$grafana_dir/dashboards/"
  fi

  if [[ -f "$obs_root/grafana_config/grafana.ini.tpl" ]]; then
    sed \
      -e "s#__GRAFANA_DATA__#$grafana_dir/data#g" \
      -e "s#__GRAFANA_LOGS__#$grafana_dir/logs#g" \
      -e "s#__GRAFANA_PLUGINS__#$grafana_dir/plugins#g" \
      -e "s#__GRAFANA_PROVISIONING__#$grafana_dir/provisioning#g" \
      -e "s#__GRAFANA_DOMAIN__#$grafana_domain#g" \
      -e "s#__GRAFANA_ROOT_URL__#$grafana_root_url#g" \
      -e "s#__GRAFANA_ADMIN_USER__#$grafana_admin_user#g" \
      -e "s#__GRAFANA_ADMIN_PASSWORD__#$grafana_admin_password#g" \
      "$obs_root/grafana_config/grafana.ini.tpl" \
      > "$grafana_dir/conf/grafana.ini"
  fi

  # Keep a copy of prometheus.yml next to binary if render put it under prometheus/.
  if [[ -f "$obs_root/prometheus/prometheus.yml" && -d "$prom_dir" ]]; then
    # Already in place when OUT_DIR==obs_root and prom_dir==obs_root/prometheus
    if [[ "$(cd "$prom_dir" && pwd)" != "$(cd "$obs_root/prometheus" && pwd)" ]]; then
      cp "$obs_root/prometheus/prometheus.yml" "$prom_dir/prometheus.yml"
      cp -a "$obs_root/prometheus/rules/." "$prom_dir/rules/" 2>/dev/null || true
    fi
  fi
  if [[ -f "$obs_root/alertmanager/alertmanager.yml" && -d "$alert_dir" ]]; then
    if [[ "$(cd "$alert_dir" && pwd)" != "$(cd "$obs_root/alertmanager" && pwd)" ]]; then
      cp "$obs_root/alertmanager/alertmanager.yml" "$alert_dir/alertmanager.yml"
    fi
  fi

  echo "observability defaults initialized under $obs_root"
}

stx_obs_ensure_configs() {
  stx_obs_dirs
  if [[ ! -f "$PROM_DIR/prometheus.yml" || ! -f "$ALERT_DIR/alertmanager.yml" || ! -f "$GRAFANA_DIR/conf/grafana.ini" ]]; then
    stx_obs_init_defaults "$OBS_ROOT" || return 1
  fi
}

stx_obs_stop_one() {
  local name="$1"
  local pidfile="$2"
  if [[ ! -f "$pidfile" ]]; then
    echo "$name not running (no pidfile)"
    return 0
  fi
  local pid
  pid="$(cat "$pidfile" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" || true
    sleep 1
    if kill -0 "$pid" 2>/dev/null; then
      kill -9 "$pid" || true
    fi
    echo "stopped $name (pid=$pid)"
  else
    echo "$name already stopped"
  fi
  rm -f "$pidfile"
}

stx_obs_start_stack() {
  stx_obs_dirs
  stx_obs_ensure_configs || return 1

  local grafana_bin
  grafana_bin="$(stx_obs_grafana_bin)" || {
    echo "grafana binary not found under $GRAFANA_DIR/bin" >&2
    return 1
  }

  mkdir -p "$ALERT_DIR/logs" "$PROM_DIR/logs" "$GRAFANA_DIR/logs"

  for pidfile in \
    "$ALERT_DIR/alertmanager.pid" \
    "$PROM_DIR/prometheus.pid" \
    "$GRAFANA_DIR/grafana.pid"; do
    if [[ -f "$pidfile" ]]; then
      local pid
      pid="$(cat "$pidfile" 2>/dev/null || true)"
      if [[ -n "${pid}" ]] && kill -0 "$pid" 2>/dev/null; then
        kill "$pid" || true
        sleep 1
      fi
      rm -f "$pidfile"
    fi
  done

  setsid sh -c "exec $ALERT_DIR/alertmanager --config.file=$ALERT_DIR/alertmanager.yml --storage.path=$ALERT_DIR/data --web.listen-address=:9093 >> $ALERT_DIR/logs/alertmanager.log 2>&1" < /dev/null &
  echo $! >"$ALERT_DIR/alertmanager.pid"

  setsid sh -c "exec $PROM_DIR/prometheus --config.file=$PROM_DIR/prometheus.yml --storage.tsdb.path=$PROM_DIR/data --web.listen-address=:9090 --web.enable-lifecycle >> $PROM_DIR/logs/prometheus.log 2>&1" < /dev/null &
  echo $! >"$PROM_DIR/prometheus.pid"

  if [[ "$(basename "$grafana_bin")" == "grafana" ]]; then
    setsid sh -c "exec $grafana_bin server --homepath=$GRAFANA_DIR --config=$GRAFANA_DIR/conf/grafana.ini >> $GRAFANA_DIR/logs/grafana.log 2>&1" < /dev/null &
  else
    setsid sh -c "exec $grafana_bin --homepath=$GRAFANA_DIR --config=$GRAFANA_DIR/conf/grafana.ini >> $GRAFANA_DIR/logs/grafana.log 2>&1" < /dev/null &
  fi
  echo $! >"$GRAFANA_DIR/grafana.pid"

  sleep 2
  echo "observability started:"
  echo "  - Grafana     : http://127.0.0.1:3000"
  echo "  - Prometheus  : http://127.0.0.1:9090"
  echo "  - Alertmanager: http://127.0.0.1:9093"
}

stx_obs_stop_stack() {
  stx_obs_dirs
  if [[ ! -d "$OBS_ROOT" ]]; then
    echo "observability: not installed"
    return 0
  fi
  stx_obs_stop_one "grafana" "$GRAFANA_DIR/grafana.pid"
  stx_obs_stop_one "prometheus" "$PROM_DIR/prometheus.pid"
  stx_obs_stop_one "alertmanager" "$ALERT_DIR/alertmanager.pid"
}

stx_obs_status_stack() {
  stx_obs_dirs
  if ! stx_obs_stack_present; then
    local enabled
    enabled="$(stx_obs_config_enabled "${CONFIG_PATH:-$BASE_DIR/config.yaml}")"
    if [[ "$enabled" == "true" || "$enabled" == "1" ]]; then
      echo "observability: remote-only (enabled in config, no local stack)"
    else
      echo "observability: absent"
    fi
    return 0
  fi

  local svc pidfile pid
  for svc in alertmanager prometheus grafana; do
    case "$svc" in
      alertmanager) pidfile="$ALERT_DIR/alertmanager.pid" ;;
      prometheus) pidfile="$PROM_DIR/prometheus.pid" ;;
      grafana) pidfile="$GRAFANA_DIR/grafana.pid" ;;
    esac
    if [[ -f "$pidfile" ]]; then
      pid="$(cat "$pidfile" 2>/dev/null || true)"
      if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
        echo "$svc: running (pid=$pid)"
      else
        echo "$svc: pidfile exists but process not running"
      fi
    else
      echo "$svc: stopped"
    fi
  done
}
