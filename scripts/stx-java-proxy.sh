#!/bin/bash
#
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
#

set -eu

PRG="$0"
while [ -h "$PRG" ]; do
  ls_output=$(ls -ld "$PRG")
  link=$(expr "$ls_output" : '.*-> \(.*\)$')
  if expr "$link" : '/.*' > /dev/null; then
    PRG="$link"
  else
    PRG=$(dirname "$PRG")/"$link"
  fi
done

PRG_DIR=$(dirname "$PRG")
PROXY_HOME=$(cd "$PRG_DIR/.." >/dev/null; pwd)
if [ -n "${STX_JAVA_PROXY_HOME:-}" ] && [ -d "${STX_JAVA_PROXY_HOME}" ]; then
  PROXY_HOME="${STX_JAVA_PROXY_HOME}"
fi
if [ -z "${SEATUNNEL_HOME:-}" ] && [ -f "${PROXY_HOME}/starter/seatunnel-starter.jar" ]; then
  SEATUNNEL_HOME="${PROXY_HOME}"
fi
if [ -z "${SEATUNNEL_HOME:-}" ]; then
  for candidate in /opt/seatunnel-2.3.13 /opt/seatunnel /usr/local/seatunnel; do
    if [ -f "${candidate}/starter/seatunnel-starter.jar" ]; then
      SEATUNNEL_HOME="${candidate}"
      break
    fi
  done
fi

APP_JAR=${SEATUNNEL_HOME:-}/starter/seatunnel-starter.jar
# 代际标签是 stx-java-proxy jar 命名的唯一依据，与 SeaTunnel 具体小版本无关。
# 仅在真正出现 breaking API 变更时才需要引入新代际（如 v3）。
# The epoch label is the sole naming basis for the proxy jar, independent of
# the exact SeaTunnel patch version.  Introduce a new epoch (e.g. v3) only on
# a genuine breaking API change.
DEFAULT_PROXY_VERSION="${STX_JAVA_PROXY_DEFAULT_VERSION:-v2}"
APP_MAIN="io.github.leonyoah.stx.proxy.StxJavaProxyApplication"
DEFAULT_PROXY_PORT="18080"

fail_preflight() {
  echo "stx-java-proxy preflight failed: $1" >&2
  exit 1
}

validate_seatunnel_home() {
  if [ -z "${SEATUNNEL_HOME:-}" ]; then
    fail_preflight "SEATUNNEL_HOME is not set"
  fi
  if [ ! -d "${SEATUNNEL_HOME}" ]; then
    fail_preflight "SEATUNNEL_HOME does not exist: ${SEATUNNEL_HOME}"
  fi
  if [ ! -f "${SEATUNNEL_HOME}/starter/seatunnel-starter.jar" ]; then
    fail_preflight "starter jar missing under ${SEATUNNEL_HOME}/starter/seatunnel-starter.jar"
  fi
  if [ ! -f "${SEATUNNEL_HOME}/bin/seatunnel.sh" ]; then
    fail_preflight "seatunnel.sh missing under ${SEATUNNEL_HOME}/bin/seatunnel.sh"
  fi
}

# proxy_epoch_for_version 将 SeaTunnel 版本字符串映射为 stx-java-proxy 代际标签。
# proxy_epoch_for_version maps a SeaTunnel version string to the proxy epoch label.
proxy_epoch_for_version() {
  local version="${1:-}"
  local major
  major="${version%%.*}"
  case "${major}" in
    3)
      # 3.x 与 v2 jar 核心存储接口兼容；真正 breaking 时改为 echo "v3"。
      # 3.x is storage-API compatible with v2; change to "v3" on genuine break.
      echo "v2"
      ;;
    *)
      # 2.x 及未知版本均使用 v2 代际。
      # 2.x and unknown versions use the v2 epoch.
      echo "v2"
      ;;
  esac
}

proxy_version_candidates() {
  local requested_version="${STX_JAVA_PROXY_VERSION:-${SEATUNNEL_VERSION:-}}"
  local epoch
  # 优先使用显式指定的 epoch（如 v2/v3），否则按版本号推导。
  # If the caller sets an explicit epoch label use it, else derive from version.
  if [ -n "${requested_version}" ]; then
    epoch=$(proxy_epoch_for_version "${requested_version}")
  else
    epoch="${DEFAULT_PROXY_VERSION}"
  fi
  printf '%s\n' "${epoch}"
}

find_proxy_jar() {
  local version candidate
  while IFS= read -r version; do
    [ -z "${version}" ] && continue

    candidate="${PROXY_HOME}/lib/stx-java-proxy-${version}.jar"
    if [ -f "${candidate}" ]; then
      echo "${candidate}"
      return 0
    fi

    candidate=$(find "${PROXY_HOME}/tools/stx-java-proxy/target" -maxdepth 1 -type f -name "stx-java-proxy-${version}*.jar" 2>/dev/null | grep -v '\-bin\.jar$' | sort | head -n 1 || true)
    if [ -n "${candidate}" ]; then
      echo "${candidate}"
      return 0
    fi
  done < <(proxy_version_candidates)

  if [ -f "${PROXY_HOME}/lib/stx-java-proxy.jar" ]; then
    echo "${PROXY_HOME}/lib/stx-java-proxy.jar"
    return 0
  fi

  find "${PROXY_HOME}/tools/stx-java-proxy/target" -maxdepth 1 -type f -name 'stx-java-proxy-*.jar' 2>/dev/null | grep -v '\-bin\.jar$' | sort | head -n 1 || true
}

DEFAULT_PROXY_JAR="$(find_proxy_jar)"
PROXY_JAR=${STX_JAVA_PROXY_JAR:-${DEFAULT_PROXY_JAR}}

validate_seatunnel_home

if [ ! -f "${APP_JAR}" ]; then
  echo "seatunnel-starter.jar not found under ${SEATUNNEL_HOME:-<unset>}/starter; please set SEATUNNEL_HOME" >&2
  exit 1
fi

if [ ! -f "${PROXY_JAR}" ]; then
  echo "proxy jar not found: ${PROXY_JAR}" >&2
  exit 1
fi

if [ -f "${SEATUNNEL_HOME}/config/seatunnel-env.sh" ]; then
  # shellcheck disable=SC1091
  . "${SEATUNNEL_HOME}/config/seatunnel-env.sh"
fi

# Load proxy-specific environment configuration if present
if [ -n "${STX_JAVA_PROXY_CONF_DIR:-}" ] && [ -f "${STX_JAVA_PROXY_CONF_DIR}/stx-java-proxy-env.sh" ]; then
  # shellcheck disable=SC1091
  . "${STX_JAVA_PROXY_CONF_DIR}/stx-java-proxy-env.sh"
elif [ -f "${PROXY_HOME}/conf/stx-java-proxy-env.sh" ]; then
  # shellcheck disable=SC1091
  . "${PROXY_HOME}/conf/stx-java-proxy-env.sh"
elif [ -f "${PROXY_HOME}/config/stx-java-proxy-env.sh" ]; then
  # shellcheck disable=SC1091
  . "${PROXY_HOME}/config/stx-java-proxy-env.sh"
elif [ -f "${SEATUNNEL_HOME}/config/stx-java-proxy-env.sh" ]; then
  # shellcheck disable=SC1091
  . "${SEATUNNEL_HOME}/config/stx-java-proxy-env.sh"
fi

DEFAULT_PROXY_JVM_OPTS="-Xms64m -Xmx512m"
PROXY_JVM_OPTS="${STX_JAVA_PROXY_JVM_OPTS:-${JAVA_OPTS:-}}"

APP_ARGS=()
CLI_JVM_OPTS=""
for arg in "$@"; do
  case "${arg}" in
    -D*|-X*|-XX:*)
      CLI_JVM_OPTS="${CLI_JVM_OPTS} ${arg}"
      ;;
    *)
      APP_ARGS+=("${arg}")
      ;;
  esac
done

COMBINED_JVM_OPTS="${PROXY_JVM_OPTS} ${CLI_JVM_OPTS}"

# If no -Xmx memory cap is specified, inject default 512MB limit
if [[ "${COMBINED_JVM_OPTS}" != *-Xmx* ]]; then
  CUSTOM_XMX=$(printf '%s' "${COMBINED_JVM_OPTS}" | grep -o '\-Dstx\.java\.proxy\.xmx=[^ ]*' | head -n 1 | cut -d= -f2 || true)
  if [ -n "${CUSTOM_XMX}" ]; then
    COMBINED_JVM_OPTS="${COMBINED_JVM_OPTS} -Xmx${CUSTOM_XMX}"
  else
    COMBINED_JVM_OPTS="${DEFAULT_PROXY_JVM_OPTS} ${COMBINED_JVM_OPTS}"
  fi
fi

JAVA_OPTS="${COMBINED_JVM_OPTS} -DSEATUNNEL_HOME=${SEATUNNEL_HOME} -Dstx.java.proxy.seatunnel.home=${SEATUNNEL_HOME}"

CLASS_PATH=${SEATUNNEL_HOME}/lib/*:${APP_JAR}:${PROXY_JAR}

resolve_proxy_port() {
  local port="${STX_JAVA_PROXY_PORT:-}"
  local arg
  for arg in "$@"; do
    case "$arg" in
      -Dstx.java.proxy.port=*)
        port="${arg#-Dstx.java.proxy.port=}"
        ;;
    esac
  done
  if [ -z "${port}" ]; then
    port="${DEFAULT_PROXY_PORT}"
  fi
  printf '%s\n' "${port}"
}

# 读取进程命令行。Linux 用 /proc，macOS 没有 /proc，改走 ps。
# Read a process command line. Linux uses /proc; macOS has no /proc, so fall back to ps.
read_process_command() {
  local pid="$1"
  if [ -r "/proc/${pid}/cmdline" ]; then
    tr '\0' ' ' < "/proc/${pid}/cmdline" 2>/dev/null || true
    return 0
  fi
  if command -v ps >/dev/null 2>&1; then
    ps -ww -p "${pid}" -o command= 2>/dev/null || true
  fi
}

is_proxy_process() {
  local cmdline="$1"
  printf '%s' "${cmdline}" | grep -q -e "${APP_MAIN}" -e 'stx-java-proxy'
}

kill_proxy_pid() {
  local pid="$1"
  local port="$2"
  echo "stx-java-proxy detected existing listener on port ${port}, killing pid=${pid}" >&2
  kill "${pid}" 2>/dev/null || true
  local retries=30
  while kill -0 "${pid}" 2>/dev/null && [ "${retries}" -gt 0 ]; do
    sleep 1
    retries=$((retries - 1))
  done
  if kill -0 "${pid}" 2>/dev/null; then
    echo "stx-java-proxy pid=${pid} did not exit gracefully, killing -9" >&2
    kill -9 "${pid}" 2>/dev/null || true
  fi
}

kill_existing_proxy_listener() {
  local port="$1"
  local pids=""
  if command -v ss >/dev/null 2>&1; then
    pids=$(ss -lntp 2>/dev/null | awk -v port=":${port}" '$4 ~ port {print $NF}' | grep -o 'pid=[0-9]\+' | cut -d= -f2 | sort -u || true)
  fi
  if [ -z "${pids}" ] && command -v lsof >/dev/null 2>&1; then
    pids=$(lsof -ti TCP:"${port}" -sTCP:LISTEN 2>/dev/null | sort -u || true)
  fi
  [ -z "${pids}" ] && return 0

  local pid cmdline
  for pid in ${pids}; do
    [ -z "${pid}" ] && continue
    cmdline="$(read_process_command "${pid}")"
    if is_proxy_process "${cmdline}"; then
      kill_proxy_pid "${pid}" "${port}"
    else
      echo "stx-java-proxy port ${port} is held by pid=${pid}, not a proxy process; refusing to kill" >&2
    fi
  done
}

if [ -n "${EXTRA_PROXY_CLASSPATH:-}" ]; then
  CLASS_PATH=${CLASS_PATH}:${EXTRA_PROXY_CLASSPATH}
fi

PROXY_PORT="$(resolve_proxy_port "$@")"
kill_existing_proxy_listener "${PROXY_PORT}"

if [ ${#APP_ARGS[@]} -eq 0 ]; then
  exec java ${JAVA_OPTS} -cp "${CLASS_PATH}" ${APP_MAIN}
fi
exec java ${JAVA_OPTS} -cp "${CLASS_PATH}" ${APP_MAIN} "${APP_ARGS[@]}"
