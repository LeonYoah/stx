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

# Shared install-core: ports, config, systemd, finish tips.
# 共享安装核心：端口探测、配置写入、systemd、收尾提示。
# shellcheck shell=bash

: "${STX_DEFAULT_HTTP_PORT:=17800}"
: "${STX_DEFAULT_FRONTEND_PORT:=17880}"
: "${STX_DEFAULT_GRPC_PORT:=17890}"
: "${STX_PORT_PROBE_LIMIT:=20}"

# 判断 TCP 端口是否已被占用。/ Return 0 when TCP port is already in use.
stx_port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -lnt 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${port}$"
    return $?
  fi
  if command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  # Best-effort fallback via bash /dev/tcp. / 退化：用 bash /dev/tcp 探测。
  (echo >/dev/tcp/127.0.0.1/"$port") >/dev/null 2>&1
}

# 从 preferred 起找空闲端口，最多偏移 limit。/ Find free port from preferred with max offset.
stx_find_free_port() {
  local preferred="$1"
  local limit="${2:-$STX_PORT_PROBE_LIMIT}"
  local p
  for ((p = preferred; p <= preferred + limit; p++)); do
    if ! stx_port_in_use "$p"; then
      echo "$p"
      return 0
    fi
  done
  echo "[ERROR] no free port near $preferred (tried +$limit)" >&2
  return 1
}

# 猜测本机对外可达 IP（非回环）。/ Guess a non-loopback host IP for external_url.
stx_guess_host_ip() {
  if command -v ip >/dev/null 2>&1; then
    ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}'
    return 0
  fi
  if command -v hostname >/dev/null 2>&1; then
    hostname -I 2>/dev/null | awk '{print $1}'
    return 0
  fi
  echo "127.0.0.1"
}

# 就地改写 yaml 简单键值（仅匹配顶层/缩进键）。/ Rewrite simple YAML keys in-place (indent-aware).
stx_yaml_set() {
  local file="$1"
  local key_path="$2" # e.g. app.addr or grpc.port
  local value="$3"
  local tmp noeol=0 rc=0
  if [[ ! -f "$file" ]]; then
    echo "yaml file not found: $file" >&2
    return 1
  fi
  # 纯 awk 实现（不依赖 python3）：按缩进逐级定位 key，仅替换命中行，其余行原样保留。
  # Pure awk (no python3): walk keys level by level by indent, rewrite only the matched line, keep the rest.
  if [[ -s "$file" && -n "$(tail -c1 "$file")" ]]; then
    noeol=1
  fi
  tmp="$(mktemp "${TMPDIR:-/tmp}/stx-yaml.XXXXXX")"
  STX_YAML_KEY="$key_path" STX_YAML_VALUE="$value" awk -v noeol="$noeol" '
    BEGIN {
      depth_n = split(ENVIRON["STX_YAML_KEY"], keys, ".")
      value = ENVIRON["STX_YAML_VALUE"]
      depth = 1; indent = 0; done = 0; failed = 0
    }
    { lines[NR] = $0 }
    function spaces(n,   s) { s = ""; while (n-- > 0) s = s " "; return s }
    function is_digits(v) { return v != "" && v ~ /^[0-9]+$/ }
    function render(key,   q) {
      if (substr(value, 1, 1) == ":" || is_digits(value) || value == "true" || value == "false") {
        return spaces(indent) key ": " value
      }
      q = "\""
      if (substr(value, 1, 1) == "\"" || substr(value, 1, 1) == "\047") q = ""
      return spaces(indent) key ": " q value q
    }
    END {
      n = NR
      idx = 1
      for (depth = 1; depth <= depth_n && !failed && !done; depth++) {
        key = keys[depth]
        found = 0
        while (idx <= n) {
          line = lines[idx]
          if (line !~ /^[ \t\r\f\v]*$/ && line !~ /^[ \t\r\f\v]*#/) {
            match(line, /^ */); cur = RLENGTH
            if (cur < indent && depth > 1) break
            if (substr(line, 1, indent) == spaces(indent) && substr(line, indent + 1, length(key)) == key \
                && substr(line, indent + length(key) + 1) ~ /^[ \t\r\f\v]*:/) {
              found = 1
              if (depth == depth_n) {
                # 保留原行的 CRLF 行尾。/ Keep CRLF line ending of the original line.
                lines[idx] = render(key) (line ~ /\r$/ ? "\r" : "")
                done = 1
              } else {
                indent += 2
                idx++
              }
              break
            }
          }
          idx++
        }
        if (!found) failed = 1
      }
      if (!done) {
        printf "yaml key not found: %s\n", ENVIRON["STX_YAML_KEY"] > "/dev/stderr"
        exit 1
      }
      for (i = 1; i <= n; i++) {
        if (i == n && noeol) printf "%s", lines[i]
        else printf "%s\n", lines[i]
      }
    }
  ' "$file" >"$tmp" || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    rm -f "$tmp"
    return "$rc"
  fi
  # 原地覆盖（保留权限/属主）。/ Overwrite in place to keep mode/owner.
  cat "$tmp" >"$file"
  rm -f "$tmp"
}

# 写入/更新安装目录的默认配置端口与 external_url。/ Write default ports and external_url into install config.
stx_configure_defaults() {
  local install_dir="$1"
  local http_port="$2"
  local grpc_port="$3"
  local frontend_port="$4"
  local with_obs="${5:-false}"
  local config="$install_dir/config.yaml"
  local example="$install_dir/config.example.yaml"

  if [[ ! -f "$config" ]]; then
    if [[ -f "$example" ]]; then
      cp "$example" "$config"
    else
      echo "[ERROR] missing config.yaml and config.example.yaml under $install_dir" >&2
      return 1
    fi
  fi

  local host_ip
  host_ip="$(stx_guess_host_ip)"
  [[ -z "$host_ip" ]] && host_ip="127.0.0.1"

  stx_yaml_set "$config" "app.addr" ":${http_port}"
  stx_yaml_set "$config" "app.external_url" "http://${host_ip}:${http_port}"
  stx_yaml_set "$config" "grpc.port" "${grpc_port}"

  if [[ "$with_obs" == "true" || "$with_obs" == "1" ]]; then
    # Init bundled stack configs (logic lives in bin/lib/observability.sh).
    # 初始化本地三件套配置（逻辑在 bin/lib/observability.sh）。
    if [[ -f "$install_dir/bin/lib/observability.sh" && -d "$install_dir/observability/prometheus" ]]; then
      BASE_DIR="$install_dir"
      # shellcheck source=bin/lib/observability.sh
      source "$install_dir/bin/lib/observability.sh"
      stx_obs_init_defaults "$install_dir/observability" || true
    fi
    stx_yaml_set "$config" "observability.enabled" "true"
    # Best-effort local stack URLs. / 尽力写入本地三件套 URL。
    if grep -q 'prometheus:' "$config"; then
      stx_yaml_set "$config" "observability.prometheus.url" "http://127.0.0.1:9090" || true
      stx_yaml_set "$config" "observability.alertmanager.url" "http://127.0.0.1:9093" || true
      stx_yaml_set "$config" "observability.grafana.url" "http://127.0.0.1:3000" || true
    fi
  fi

  # Export for callers / start.sh. / 导出供调用方与 start.sh 使用。
  export FRONTEND_PORT="$frontend_port"
  export NEXT_PUBLIC_BACKEND_BASE_URL="http://127.0.0.1:${http_port}"
  export STX_HTTP_PORT="$http_port"
  export STX_GRPC_PORT="$grpc_port"
  export STX_FRONTEND_PORT="$frontend_port"
  export STX_EXTERNAL_URL="http://${host_ip}:${http_port}"
}

# 安装 systemd unit（root system / 非 root user）。/ Install systemd unit (system or user).
stx_install_systemd() {
  local install_dir="$1"
  # 安装成功后回写 system|user，供 install.sh 决定用 systemctl 还是 start.sh 启动。
  # Set to system|user on success so install.sh can start via systemctl instead of start.sh.
  STX_SYSTEMD_MODE=""
  if [[ "${STX_SKIP_SYSTEMD:-false}" == "true" ]]; then
    echo "[INFO] skip systemd (STX_SKIP_SYSTEMD=true)"
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    echo "[WARN] systemctl not found; skip systemd"
    return 0
  fi

  local unit_path service_user_lines wanted_by use_user=0
  if [[ "$(id -u)" -eq 0 ]]; then
    # 有 systemctl 但 systemd 未运行（容器、chroot 等）时跳过，避免 daemon-reload 失败中断安装。
    # systemctl present but systemd not running (containers, chroot): skip so daemon-reload does not abort install.
    if [[ ! -d /run/systemd/system ]]; then
      echo "[WARN] systemd is not running (no /run/systemd/system); skip systemd"
      return 0
    fi
    unit_path="/etc/systemd/system/stx.service"
    wanted_by="multi-user.target"
    service_user_lines="User=root
Group=root"
  else
    use_user=1
    unit_path="${HOME}/.config/systemd/user/stx.service"
    wanted_by="default.target"
    service_user_lines=""
    mkdir -p "$(dirname "$unit_path")"
  fi

  cat >"$unit_path" <<EOF
[Unit]
Description=STX Control Plane
Documentation=https://github.com/${STX_GITHUB_OWNER:-LeonYoah}/${STX_GITHUB_REPO:-stx}
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
${service_user_lines}
WorkingDirectory=${install_dir}
Environment=CONFIG_PATH=${install_dir}/config.yaml
Environment=FRONTEND_PORT=${STX_FRONTEND_PORT:-17880}
Environment=NEXT_PUBLIC_BACKEND_BASE_URL=${NEXT_PUBLIC_BACKEND_BASE_URL:-http://127.0.0.1:17800}
ExecStart=${install_dir}/bin/start.sh
ExecStop=${install_dir}/bin/stop.sh
TimeoutStartSec=120

[Install]
WantedBy=${wanted_by}
EOF

  if [[ "$use_user" -eq 1 ]]; then
    if ! systemctl --user daemon-reload; then
      echo "[WARN] systemctl --user unavailable (no user session?); unit written to $unit_path but not enabled"
      return 0
    fi
    systemctl --user enable stx.service >/dev/null 2>&1 || true
    STX_SYSTEMD_MODE="user"
    echo "[OK] systemd user unit: $unit_path"
    echo "     start: systemctl --user start stx"
    echo "     stop : systemctl --user stop stx"
  else
    if ! systemctl daemon-reload; then
      echo "[WARN] systemctl daemon-reload failed; unit written to $unit_path but not enabled"
      return 0
    fi
    systemctl enable stx.service >/dev/null 2>&1 || true
    STX_SYSTEMD_MODE="system"
    echo "[OK] systemd unit: $unit_path"
    echo "     start: systemctl start stx"
    echo "     stop : systemctl stop stx"
  fi
}

# 打印安装完成提示。/ Print post-install tips.
stx_print_finish_tips() {
  local install_dir="$1"
  local start_cmd="$install_dir/bin/start.sh" stop_cmd="$install_dir/bin/stop.sh" restart_cmd
  restart_cmd="$install_dir/bin/stop.sh && $install_dir/bin/start.sh"
  case "${STX_SYSTEMD_MODE:-}" in
    system) start_cmd="systemctl start stx"; stop_cmd="systemctl stop stx"; restart_cmd="systemctl restart stx" ;;
    user) start_cmd="systemctl --user start stx"; stop_cmd="systemctl --user stop stx"; restart_cmd="systemctl --user restart stx" ;;
  esac
  cat <<EOF

[OK] STX installed at: $install_dir
     UI (default) : http://127.0.0.1:${STX_FRONTEND_PORT:-17880}
     API          : ${STX_EXTERNAL_URL:-http://127.0.0.1:17800}
     Config       : $install_dir/config.yaml
     Start        : $start_cmd
     Stop         : $stop_cmd
     Status       : $install_dir/bin/status.sh
     Observability: bundled stack starts with start.sh (default --observability auto)

To change settings:
  1) edit $install_dir/config.yaml
  2) restart: $restart_cmd
  Optional: $install_dir/bin/start.sh --observability off

Note: stx-all-in-one Docker image does NOT include Prometheus/Grafana/Alertmanager.
EOF
}

# 探测默认端口集合。/ Probe default HTTP/frontend/gRPC ports.
stx_probe_default_ports() {
  STX_HTTP_PORT="$(stx_find_free_port "$STX_DEFAULT_HTTP_PORT")"
  STX_FRONTEND_PORT="$(stx_find_free_port "$STX_DEFAULT_FRONTEND_PORT")"
  STX_GRPC_PORT="$(stx_find_free_port "$STX_DEFAULT_GRPC_PORT")"
  export STX_HTTP_PORT STX_FRONTEND_PORT STX_GRPC_PORT
  echo "[INFO] ports: http=$STX_HTTP_PORT frontend=$STX_FRONTEND_PORT grpc=$STX_GRPC_PORT"
}
