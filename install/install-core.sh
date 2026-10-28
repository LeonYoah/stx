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

  local host_ip="${STX_EXTERNAL_HOST:-}"
  [[ -z "$host_ip" ]] && host_ip="$(stx_guess_host_ip)"
  [[ -z "$host_ip" ]] && host_ip="127.0.0.1"

  stx_yaml_set "$config" "app.addr" ":${http_port}"
  stx_yaml_set "$config" "app.external_url" "http://${host_ip}:${http_port}"
  stx_yaml_set "$config" "grpc.port" "${grpc_port}"
  stx_set_console_url "$config" "http://${host_ip}:${frontend_port}"

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
      stx_yaml_set "$config" "observability.prometheus.url" "http://127.0.0.1:${STX_PROMETHEUS_PORT:-9090}" || true
      stx_yaml_set "$config" "observability.alertmanager.url" "http://127.0.0.1:${STX_ALERTMANAGER_PORT:-9093}" || true
      stx_yaml_set "$config" "observability.grafana.url" "http://127.0.0.1:${STX_GRAFANA_PORT:-3000}" || true
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
    local svc_user="${SERVICE_USER:-root}" svc_group
    svc_group="$(id -gn "$svc_user" 2>/dev/null || echo "$svc_user")"
    service_user_lines="User=${svc_user}
Group=${svc_group}"
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
EnvironmentFile=-${install_dir}/stx.env
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
    [[ "${ENABLE_AUTOSTART:-true}" == "true" ]] && { systemctl --user enable stx.service >/dev/null 2>&1 || true; }
    STX_SYSTEMD_MODE="user"
    echo "[OK] systemd user unit: $unit_path"
    echo "     start: systemctl --user start stx"
    echo "     stop : systemctl --user stop stx"
  else
    if ! systemctl daemon-reload; then
      echo "[WARN] systemctl daemon-reload failed; unit written to $unit_path but not enabled"
      return 0
    fi
    if [[ "${ENABLE_AUTOSTART:-true}" == "true" ]]; then
      systemctl enable stx.service >/dev/null 2>&1 || true
    else
      systemctl disable stx.service >/dev/null 2>&1 || true
    fi
    STX_SYSTEMD_MODE="system"
    echo "[OK] systemd unit: $unit_path"
    echo "     start: systemctl start stx"
    echo "     stop : systemctl stop stx"
  fi
}

# 探测默认端口集合。/ Probe default HTTP/frontend/gRPC ports.
stx_probe_default_ports() {
  STX_HTTP_PORT="$(stx_find_free_port "$STX_DEFAULT_HTTP_PORT")"
  STX_FRONTEND_PORT="$(stx_find_free_port "$STX_DEFAULT_FRONTEND_PORT")"
  STX_GRPC_PORT="$(stx_find_free_port "$STX_DEFAULT_GRPC_PORT")"
  export STX_HTTP_PORT STX_FRONTEND_PORT STX_GRPC_PORT
  echo "[INFO] ports: http=$STX_HTTP_PORT frontend=$STX_FRONTEND_PORT grpc=$STX_GRPC_PORT"
}

# 读取 yaml 简单键值（缩进感知，去引号与行尾注释）。/ Read a simple YAML key (indent-aware; strips quotes/comments).
stx_yaml_get() {
  local file="$1" key_path="$2"
  [[ -f "$file" ]] || return 0
  STX_YAML_KEY="$key_path" awk '
    BEGIN { n = split(ENVIRON["STX_YAML_KEY"], keys, "."); depth = 1; indent = 0 }
    function spaces(k,   s) { s = ""; while (k-- > 0) s = s " "; return s }
    /^[ \t]*$/ || /^[ \t]*#/ { next }
    {
      match($0, /^ */); cur = RLENGTH
      if (depth > 1 && cur < indent) exit
      key = keys[depth]
      if (substr($0, 1, indent) == spaces(indent) && substr($0, indent + 1, length(key) + 1) == key ":") {
        if (depth == n) {
          v = substr($0, indent + length(key) + 2)
          sub(/^[ \t]+/, "", v)
          if (ENVIRON["STX_YAML_RAW"] == "1") { sub(/[ \t\r]+$/, "", v); print v; exit }
          if (v ~ /^"/) {
            # 双引号：处理 \" 与 \\ 转义。/ Double-quoted: honor \" and \\ escapes.
            out = ""; i = 2
            while (i <= length(v)) {
              c = substr(v, i, 1)
              if (c == "\\" && i < length(v)) { out = out substr(v, i + 1, 1); i += 2; continue }
              if (c == "\"") break
              out = out c; i++
            }
            v = out
          }
          else if (v ~ /^\047/) { v = substr(v, 2); sub(/\047.*$/, "", v) }
          else { sub(/[ \t]+#.*$/, "", v); sub(/[ \t\r]+$/, "", v) }
          print v
          exit
        }
        depth++; indent += 2
      }
    }
  ' "$file"
}

# 重装前停止已在运行的 STX，释放端口。/ Stop a running STX before reinstalling so its ports are free.
stx_stop_existing_install() {
  local dir="$1"
  if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]] \
    && systemctl is-active --quiet stx.service 2>/dev/null; then
    stx_info "停止已在运行的 stx.service / stopping the running stx.service"
    systemctl stop stx.service >/dev/null 2>&1 || true
  fi
  if [[ -x "$dir/bin/stop.sh" ]] && compgen -G "$dir/run/*.pid" >/dev/null; then
    stx_info "停止已在运行的 STX 进程 / stopping running STX processes"
    (cd "$dir" && ./bin/stop.sh >/dev/null 2>&1) || true
  fi
}

# 管理员数据库状态：existing（SQLite 文件已存在）、new（SQLite 尚未创建）、external（MySQL/PostgreSQL，安装器无法判断）。
# Admin database state: existing (SQLite file present), new (no SQLite file yet), external (MySQL/PostgreSQL; unknown).
stx_admin_db_kind() {
  local dir="$1" cfg="${2:-}" dbtype path
  [[ -n "$cfg" && -f "$cfg" ]] || cfg="$dir/config.example.yaml"
  dbtype="$(stx_yaml_get "$cfg" database.type)"
  case "${STX_DB_TYPE_OVERRIDE:-${dbtype:-sqlite}}" in
    sqlite)
      path="$(stx_yaml_get "$cfg" database.sqlite_path)"
      path="${path:-./data/stx.db}"
      [[ "$path" != /* ]] && path="$dir/${path#./}"
      if [[ -s "$path" ]]; then echo existing; else echo new; fi
      ;;
    *) echo external ;;
  esac
}

# 兼容旧调用：数据库里是否已有管理员（外部库视为未知 => 否）。/ Back-compat helper: SQLite with data => true.
stx_existing_admin_db() {
  [[ "$(stx_admin_db_kind "$@")" == existing ]]
}

# 原样读取 YAML 值（含引号与转义），用于恢复。/ Read a YAML value verbatim (quotes/escapes kept) for restoring.
stx_yaml_get_raw() { STX_YAML_RAW=1 stx_yaml_get "$@"; }

# 用给定账号密码登录，判断初始密码是否生效。/ Log in with the given credentials to see whether the password applied.
stx_admin_login_ok() {
  local port="$1" user="$2" pw="$3" body esc_u esc_p
  esc_u="${user//\\/\\\\}"; esc_u="${esc_u//\"/\\\"}"
  esc_p="${pw//\\/\\\\}"; esc_p="${esc_p//\"/\\\"}"
  # 请求体经 stdin 传给 curl，密码不出现在进程参数里。/ Body goes via stdin so the password is not in argv.
  body="$(printf '{"username":"%s","password":"%s"}' "$esc_u" "$esc_p" \
    | curl -s -m 10 -H "Content-Type: application/json" --data-binary @- \
      "http://127.0.0.1:${port}/api/v1/auth/login" 2>/dev/null || true)"
  grep -q "\"username\":\"${esc_u}\"" <<<"$body"
}

# 端口合法且空闲。/ Port is valid and free.
stx_port_valid_free() {
  local port="$1" taken="${2:-}" t
  [[ "$port" =~ ^[0-9]+$ ]] && (( port >= 1 && port <= 65535 )) || { echo "端口必须是 1-65535 的数字 / invalid port: $port"; return 1; }
  for t in $taken; do
    [[ "$t" == "$port" ]] && { echo "端口 $port 已分配给其他服务 / already used by another STX service"; return 1; }
  done
  if stx_port_in_use "$port"; then
    echo "端口 $port 已被占用 / port $port is in use"
    return 1
  fi
}

# 已有安装的默认值（端口 / 对外 IP），没有时用内置默认。写入 STX_PREV_*。
# Defaults from an existing install (ports / advertised IP), falling back to built-ins. Sets STX_PREV_*.
stx_load_install_defaults() {
  local dir="$1" cfg="${2:-}" v
  STX_PREV_HTTP_PORT="$STX_DEFAULT_HTTP_PORT"; STX_PREV_GRPC_PORT="$STX_DEFAULT_GRPC_PORT"
  STX_PREV_FRONTEND_PORT="$STX_DEFAULT_FRONTEND_PORT"
  STX_PREV_PROM_PORT=9090; STX_PREV_AM_PORT=9093; STX_PREV_GF_PORT=3000; STX_PREV_EXTERNAL_IP=""
  if [[ -n "$cfg" && -f "$cfg" ]]; then
    v="$(stx_yaml_get "$cfg" app.addr)"; v="${v##*:}"; [[ "$v" =~ ^[0-9]+$ ]] && STX_PREV_HTTP_PORT="$v"
    v="$(stx_yaml_get "$cfg" grpc.port)"; [[ "$v" =~ ^[0-9]+$ ]] && STX_PREV_GRPC_PORT="$v"
    v="$(stx_yaml_get "$cfg" app.external_url)"; v="${v#*://}"; v="${v%%/*}"; v="${v%:*}"
    [[ -n "$v" && "$v" != "your-server-ip" ]] && STX_PREV_EXTERNAL_IP="$v"
  fi
  if [[ -f "$dir/stx.env" ]]; then
    v="$(sed -n 's/^FRONTEND_PORT=//p' "$dir/stx.env" | head -n1)"; [[ "$v" =~ ^[0-9]+$ ]] && STX_PREV_FRONTEND_PORT="$v"
    v="$(sed -n 's/^STX_PROMETHEUS_PORT=//p' "$dir/stx.env" | head -n1)"; [[ "$v" =~ ^[0-9]+$ ]] && STX_PREV_PROM_PORT="$v"
    v="$(sed -n 's/^STX_ALERTMANAGER_PORT=//p' "$dir/stx.env" | head -n1)"; [[ "$v" =~ ^[0-9]+$ ]] && STX_PREV_AM_PORT="$v"
    v="$(sed -n 's/^STX_GRAFANA_PORT=//p' "$dir/stx.env" | head -n1)"; [[ "$v" =~ ^[0-9]+$ ]] && STX_PREV_GF_PORT="$v"
  fi
  STX_PREV_PORTS="$STX_PREV_HTTP_PORT $STX_PREV_GRPC_PORT $STX_PREV_FRONTEND_PORT $STX_PREV_PROM_PORT $STX_PREV_AM_PORT $STX_PREV_GF_PORT"
}

# 安装目录中的 STX 是否在运行。/ Is the STX in this install dir running?
stx_install_running() {
  local dir="$1" f pid
  if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]] \
    && systemctl is-active --quiet stx.service 2>/dev/null; then
    return 0
  fi
  for f in "$dir/run/backend.pid" "$dir/run/frontend.pid"; do
    [[ -f "$f" ]] || continue
    pid="$(cat "$f" 2>/dev/null || true)"
    [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null && return 0
  done
  return 1
}

# 冲突端口：必须输入一个可用的新端口（建议值为最近的空闲端口）。
# Conflicting port: the user must enter a usable port (suggests the nearest free one).
stx_ask_port_forced() {
  local label="$1" cur="$2" taken="$3" p suggest="" ans msg
  for ((p = cur + 1; p <= cur + STX_PORT_PROBE_LIMIT; p++)); do
    if stx_port_valid_free "$p" "$taken" >/dev/null; then suggest="$p"; break; fi
  done
  while true; do
    ans="$(stx_prompt "${label}：${cur} 不可用，请输入新端口 [默认 ${suggest:-无}]:" "$suggest")"
    if [[ -n "$ans" ]] && msg="$(stx_port_valid_free "$ans" "$taken")"; then
      echo "$ans"
      return 0
    fi
    if ! stx_is_interactive; then
      stx_err "${label}：端口 ${cur} 被占用，且无法交互输入新端口 / port ${cur} is in use and no terminal input is available"
      return 1
    fi
    echo "    ${msg:-请输入端口}" >/dev/tty
  done
}

# 确认端口：回车保持当前值，输入新值时校验是否空闲（当前安装自己的端口视为可用）。
# Confirm a port: Enter keeps it; a new value must be free (ports of the current install count as free).
stx_ask_port_confirm() {
  local label="$1" cur="$2" taken="$3" own_ports="${5:-}" ans msg
  while true; do
    ans="$(stx_prompt "${label} [默认 ${cur}]:" "$cur")"
    if [[ "$ans" == "$cur" ]]; then
      echo "$cur"
      return 0
    fi
    if [[ " $own_ports " == *" $ans "* && " $taken " != *" $ans "* && "$ans" =~ ^[0-9]+$ ]]; then
      echo "$ans"
      return 0
    fi
    if msg="$(stx_port_valid_free "$ans" "$taken")"; then
      echo "$ans"
      return 0
    fi
    if ! stx_is_interactive; then
      echo "$cur"
      return 0
    fi
    echo "    $msg" >/dev/tty
  done
}

# TCP 可达性（bash /dev/tcp，3 秒超时）。/ TCP reachability via bash /dev/tcp (3s timeout).
stx_tcp_reachable() {
  local host="$1" port="$2"
  if command -v timeout >/dev/null 2>&1; then
    timeout 3 bash -c "echo >/dev/tcp/${host}/${port}" >/dev/null 2>&1
  else
    (echo >"/dev/tcp/${host}/${port}") >/dev/null 2>&1
  fi
}

# YAML 双引号字符串（转义 \ 与 "）。/ YAML double-quoted string (escapes \ and ").
stx_yaml_quote() {
  local v="$1"
  v="${v//\\/\\\\}"
  v="${v//\"/\\\"}"
  printf '"%s"' "$v"
}

# 数据库选择（菜单编号或名称，不区分大小写）规范化为 sqlite|mysql|postgres；无法识别返回 1。
# Normalize a database choice (menu number or name, case-insensitive) to sqlite|mysql|postgres; 1 if unknown.
stx_parse_db_type() {
  local v
  v="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$v" in
    1|sqlite|sqlite3) echo sqlite ;;
    2|mysql) echo mysql ;;
    3|postgres|postgresql|pg|pgsql) echo postgres ;;
    *) return 1 ;;
  esac
}

# 建库示例 SQL（按数据库类型）。/ Example CREATE DATABASE statement for the database type.
# 用法 / Usage: stx_db_create_hint <mysql|postgres> <db-name> [db-user]
stx_db_create_hint() {
  local type="$1" name="$2" user="${3:-}"
  case "$type" in
    mysql) echo "CREATE DATABASE ${name} DEFAULT CHARACTER SET utf8mb4;" ;;
    postgres)
      if [[ -z "$user" || "$user" == postgres ]]; then
        echo "CREATE DATABASE ${name};"
      else
        echo "CREATE DATABASE ${name} OWNER ${user};（用 postgres 超级用户执行，如 sudo -u postgres psql -c 'CREATE DATABASE ${name} OWNER ${user};'）"
      fi ;;
    *) echo "" ;;
  esac
}

# 写入数据库配置。/ Write database settings.
stx_set_database() {
  local file="$1" type="$2" host="$3" port="$4" user="$5" pw="$6" name="$7"
  stx_yaml_set "$file" "database.type" "$type"
  [[ "$type" == "sqlite" ]] && return 0
  stx_yaml_set "$file" "database.host" "$(stx_yaml_quote "$host")"
  stx_yaml_set "$file" "database.port" "$port"
  stx_yaml_set "$file" "database.username" "$(stx_yaml_quote "$user")"
  stx_yaml_set "$file" "database.password" "$(stx_yaml_quote "$pw")"
  stx_yaml_set "$file" "database.database" "$(stx_yaml_quote "$name")"
}

# 设置 app.console_url（模板中是注释行，必要时取消注释）。/ Set app.console_url (commented in the template).
stx_set_console_url() {
  local file="$1" url="$2"
  if stx_yaml_set "$file" "app.console_url" "$url" 2>/dev/null; then
    return 0
  fi
  if grep -qE '^  # console_url:' "$file"; then
    sed -i.bak -E "s|^  # console_url:.*$|  console_url: \"${url}\"|" "$file" && rm -f "${file}.bak"
  fi
}

# 交互设置初始管理员密码；回车=保持默认（输出空串）。/ Interactive initial admin password; Enter keeps the default.
# 用法 / Usage: stx_ask_admin_password <user> <keep-label，如「回车保持默认 admin123」；不得包含用户自己的密码>
stx_ask_admin_password() {
  local user="$1" keep_label="$2" p1 p2 tries=0
  while (( tries < 3 )); do
    p1="$(stx_prompt_secret "管理员 ${user} 的初始密码 [${keep_label}]:")"
    if [[ -z "$p1" ]] || ! stx_is_interactive; then
      return 0
    fi
    if (( ${#p1} < 6 )); then
      echo "    密码至少 6 位 / at least 6 characters" >/dev/tty
    elif [[ "$p1" =~ [[:space:]] ]]; then
      echo "    密码不能包含空白字符 / no whitespace" >/dev/tty
    else
      p2="$(stx_prompt_secret "再次输入确认 / confirm:")"
      if ! stx_is_interactive; then
        stx_warn "终端已关闭，保持原密码 / terminal closed, keeping the current password"
        return 0
      fi
      if [[ "$p1" == "$p2" ]]; then
        printf '%s' "$p1"
        return 0
      fi
      echo "    两次输入不一致 / passwords do not match" >/dev/tty
    fi
    tries=$((tries + 1))
  done
  stx_warn "多次输入无效，保持原密码 / keeping the current password"
}

# 写入 auth.default_admin_password（首次初始化数据库时生效）。/ Write auth.default_admin_password (used on first DB init).
stx_set_admin_password() {
  local file="$1" pw="$2" esc
  if [[ "$pw" == *$'\n'* ]]; then
    stx_err "admin password must not contain newlines"
    return 1
  fi
  esc="${pw//\\/\\\\}"
  esc="${esc//\"/\\\"}"
  stx_yaml_set "$file" "auth.default_admin_password" "\"${esc}\""
}

# 持久化前端端口等启动参数，start.sh 手动启动时也能读到。/ Persist frontend launch settings for start.sh.
stx_write_env_file() {
  local dir="$1"
  cat >"$dir/stx.env" <<EOF
# 由 install.sh 生成；start.sh 在对应环境变量未设置时读取。/ Generated by install.sh; read by start.sh when unset.
FRONTEND_PORT=${STX_FRONTEND_PORT:-17880}
NEXT_PUBLIC_BACKEND_BASE_URL=http://127.0.0.1:${STX_HTTP_PORT:-17800}
STX_HTTP_PORT=${STX_HTTP_PORT:-17800}
STX_GRPC_PORT=${STX_GRPC_PORT:-17890}
STX_PROMETHEUS_PORT=${STX_PROMETHEUS_PORT:-9090}
STX_ALERTMANAGER_PORT=${STX_ALERTMANAGER_PORT:-9093}
STX_GRAFANA_PORT=${STX_GRAFANA_PORT:-3000}
EOF
}

# HTTP 探测，返回状态码（无 curl 时用 /dev/tcp 只测端口）。/ HTTP probe returning the status code.
stx_http_code() {
  local url="$1" hostport
  if command -v curl >/dev/null 2>&1; then
    curl -s -o /dev/null -m 3 -w '%{http_code}' "$url" 2>/dev/null || true
    return 0
  fi
  hostport="${url#http://}"; hostport="${hostport%%/*}"
  if (echo >"/dev/tcp/${hostport%%:*}/${hostport##*:}") >/dev/null 2>&1; then echo 200; else echo 000; fi
}

# 等待后端与前端就绪。/ Wait for backend and frontend readiness.
stx_wait_ready() {
  local http="$1" fe="$2" dir="$3" limit="${STX_READY_TIMEOUT:-120}" waited=0 api="" web="" code
  stx_info "等待服务就绪（最多 ${limit}s）/ waiting for services"
  while (( waited < limit )); do
    if [[ -z "$api" ]]; then
      code="$(stx_http_code "http://127.0.0.1:${http}/api/v1/health")"
      [[ "$code" == 200 ]] && { api=ok; stx_info "✔ 后端 API 就绪 http://127.0.0.1:${http}/api/v1/health (${waited}s)"; }
    fi
    if [[ -z "$web" ]]; then
      code="$(stx_http_code "http://127.0.0.1:${fe}/")"
      [[ "$code" =~ ^[23] ]] && { web=ok; stx_info "✔ Web 控制台就绪 http://127.0.0.1:${fe}/ (${waited}s)"; }
    fi
    [[ -n "$api" && -n "$web" ]] && return 0
    sleep 3
    waited=$((waited + 3))
    (( waited % 15 == 0 )) && stx_info "… 已等待 ${waited}s"
  done
  [[ -z "$api" ]] && stx_err "后端 API 在 ${limit}s 内未就绪，查看日志：$dir/logs/backend.log"
  [[ -z "$web" ]] && stx_err "Web 控制台在 ${limit}s 内未就绪，查看日志：$dir/logs/frontend.log"
  if [[ -z "$api" && -f "$dir/logs/backend.log" ]]; then
    echo "  ---- backend.log (last 20 lines) ----" >&2
    tail -n 20 "$dir/logs/backend.log" >&2 || true
  fi
  return 1
}

# 安装完成摘要与后续步骤。/ Final summary and next steps.
# 用法 / Usage: stx_print_summary <install_dir> <admin_user> <default|changed|existing|pending|unknown|kept>
#                <factory_default_password_or_empty> <started:true|false> <with_obs>
# 第 4 个参数只能传出厂默认密码（config.example.yaml），非出厂值传空串，摘要永不显示用户自己的密码。
# Argument 4 may only be the factory default (config.example.yaml); pass "" otherwise so user passwords never show.
stx_print_summary() {
  local dir="$1" user="$2" state="$3" defpw="$4" started="${5:-true}" with_obs="${6:-false}" ip pw_line svc_start svc_stop svc_restart svc_status obs_line=""
  ip="${STX_EXTERNAL_HOST:-$(stx_guess_host_ip)}"
  [[ -z "$ip" ]] && ip="127.0.0.1"
  if [[ "$with_obs" == "true" && -d "$dir/observability" ]]; then
    obs_line="  可观测性       : Prometheus :${STX_PROMETHEUS_PORT:-9090} / Alertmanager :${STX_ALERTMANAGER_PORT:-9093} / Grafana :${STX_GRAFANA_PORT:-3000}（仅本机监听，经控制台访问）
                   占用磁盘 $(du -sh "$dir/observability" 2>/dev/null | awk '{print $1}')，Prometheus 数据在 $dir/observability/prometheus/data 会随时间增长（默认保留 15 天）；不需要可重装时加 --without-observability"
  fi
  case "$state" in
    changed) pw_line="（安装时已修改为你设置的密码，出于安全不在此显示）" ;;
    existing) pw_line="（沿用已有数据库中的密码，本次安装未修改）" ;;
    pending) pw_line="（已写入你设置的密码：外部数据库中还没有管理员时首次启动生效，否则沿用原密码）" ;;
    unknown)
      if [[ -n "$defpw" ]]; then
        pw_line="（外部数据库：沿用库中已有密码；全新的库使用默认密码 ${defpw}）"
      else
        pw_line="（外部数据库：沿用库中已有密码；全新的库使用原配置中的密码，出于安全不在此显示）"
      fi ;;
    kept) pw_line="（沿用原配置中的密码，出于安全不在此显示）" ;;
    *)
      if [[ -n "$defpw" ]]; then
        pw_line="${defpw}   ← 默认密码，请登录后尽快修改"
      else
        pw_line="（沿用原配置中的密码，出于安全不在此显示）"
      fi ;;
  esac
  svc_start="$dir/bin/start.sh"; svc_stop="$dir/bin/stop.sh"
  svc_restart="$dir/bin/stop.sh && $dir/bin/start.sh"; svc_status="$dir/bin/status.sh"
  case "${STX_SYSTEMD_MODE:-}" in
    system) svc_start="systemctl start stx"; svc_stop="systemctl stop stx"; svc_restart="systemctl restart stx"
      svc_status="systemctl status stx   或 / or  $dir/bin/status.sh" ;;
    user) svc_start="systemctl --user start stx"; svc_stop="systemctl --user stop stx"
      svc_restart="systemctl --user restart stx"; svc_status="systemctl --user status stx   或 / or  $dir/bin/status.sh" ;;
  esac
  cat <<EOF

${STX_C_GREEN}${STX_C_BOLD}==================== STX 安装完成 / installed ====================${STX_C_RESET}
  访问地址 / URL : ${STX_C_BOLD}http://${ip}:${STX_FRONTEND_PORT:-17880}${STX_C_RESET}$([[ "$started" == "true" ]] || echo "   （未启动，请先执行：$svc_start）")
  登录账号 / user: ${STX_C_BOLD}${user}${STX_C_RESET}
  登录密码 / pass: ${pw_line}
  后端 API       : http://${ip}:${STX_HTTP_PORT:-17800}    Agent gRPC 端口: ${STX_GRPC_PORT:-17890}（Agent 回连 ${ip}）
  数据库         : ${STX_SUMMARY_DB:-SQLite}
${obs_line:+${obs_line}
}  安装目录       : $dir
  配置文件       : $dir/config.yaml   （修改后重启生效）
  日志           : $dir/logs/backend.log  $dir/logs/frontend.log  $dir/logs/install.log
  启动 / start   : $svc_start
  停止 / stop    : $svc_stop
  重启 / restart : $svc_restart
  状态 / status  : $svc_status
  无法访问时请放行端口，例如：firewall-cmd --permanent --add-port=${STX_FRONTEND_PORT:-17880}/tcp --add-port=${STX_HTTP_PORT:-17800}/tcp --add-port=${STX_GRPC_PORT:-17890}/tcp && firewall-cmd --reload

${STX_C_BOLD}后续步骤 / next steps${STX_C_RESET}（浏览器打开上面的访问地址并登录）
  A. 还没有 SeaTunnel 集群：
     1) 安装包与插件 → 安装包管理：下载需要的 SeaTunnel 版本（离线环境可上传 apache-seatunnel-<版本>-bin.tar.gz）
     2) 安装包与插件 → 插件市场：下载需要的连接器
     3) 主机与集群 → 主机管理：添加给 SeaTunnel 集群安排的机器，并按页面提示安装 Agent
     4) 主机与集群 → 集群管理：创建集群（选择版本、主机与插件，一键安装部署）
  B. 已有 SeaTunnel 集群：跳过 1、2 步
     1) 主机与集群 → 主机管理：添加集群所在的机器并安装 Agent
     2) 在主机列表中点击「自动发现」，扫描该机器上运行中的 SeaTunnel 节点并导入为集群，之后在 集群管理 中统一管理
  注意：SeaTunnel 节点需要预先安装 JDK（STX 不负责安装 JDK），版本以所选 SeaTunnel 版本的要求为准（2.3.x 常用 JDK 8/11）。
EOF
}
