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

# 轻量单机 Hadoop HDFS HA 生命周期管理脚本（支持自定义 NameNode 别名 nn-alpha,nn-beta 与宿主机 hdfs-site.xml）。
# Lifecycle manager for a lightweight local Hadoop HDFS HA stack (custom NameNode aliases nn-alpha,nn-beta + host hdfs-site.xml).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.yml"
ACTION="${1:-start}"

PROJECT_NAME="${E2E_HDFS_HA_PROJECT:-stx-e2e-hdfs-ha}"
STATE_DIR="${E2E_HDFS_HA_STATE_DIR:-${ROOT_DIR}/tmp/e2e/${PROJECT_NAME}}"
CONF_DIR="${STATE_DIR}/conf"
ENV_FILE="${STATE_DIR}/hdfs-ha.env"

HADOOP_IMAGE="${E2E_HDFS_HADOOP_IMAGE:-apache/hadoop:3.3.6}"
ZK_IMAGE="${E2E_HDFS_ZK_IMAGE:-zookeeper:3.9.2}"
NAMESERVICE="${E2E_HDFS_HA_NAMESERVICE:-stx-ha}"
# 故意使用非默认 NameNode 别名，持续验证自定义 HA 别名解析契约。
# Intentionally use non-default NameNode aliases to guard custom HA alias resolution.
NAMENODE_ALPHA="${E2E_HDFS_HA_NN_ALPHA:-nn-alpha}"
NAMENODE_BETA="${E2E_HDFS_HA_NN_BETA:-nn-beta}"
WAIT_TIMEOUT_SECONDS="${E2E_HDFS_HA_WAIT_TIMEOUT:-180}"

pick_port() {
  local start_port="$1"
  shift || true
  python3 - "$start_port" "$@" <<'PY'
import socket
import sys

start = int(sys.argv[1])
excluded = {int(v) for v in sys.argv[2:] if v}
for port in range(start, start + 200):
    if port in excluded:
        continue
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.bind(("127.0.0.1", port))
        except OSError:
            continue
        print(port)
        sys.exit(0)

raise SystemExit(f"no free port found near {start}")
PY
}

compose_cmd() {
  docker compose -p "${PROJECT_NAME}" --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" "$@"
}

write_hdfs_site_xml() {
  local target_file="$1"
  local nn_id="$2"
  local zkfc_port="$3"
  {
    cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <property>
    <name>fs.defaultFS</name>
    <value>hdfs://${NAMESERVICE}</value>
  </property>
  <property>
    <name>dfs.nameservices</name>
    <value>${NAMESERVICE}</value>
  </property>
  <property>
    <name>dfs.ha.namenodes.${NAMESERVICE}</name>
    <value>${NAMENODE_ALPHA},${NAMENODE_BETA}</value>
  </property>
  <property>
    <name>dfs.namenode.rpc-address.${NAMESERVICE}.${NAMENODE_ALPHA}</name>
    <value>127.0.0.1:${HDFS_NN_ALPHA_RPC_PORT}</value>
  </property>
  <property>
    <name>dfs.namenode.rpc-address.${NAMESERVICE}.${NAMENODE_BETA}</name>
    <value>127.0.0.1:${HDFS_NN_BETA_RPC_PORT}</value>
  </property>
  <property>
    <name>dfs.namenode.http-address.${NAMESERVICE}.${NAMENODE_ALPHA}</name>
    <value>127.0.0.1:${HDFS_NN_ALPHA_HTTP_PORT}</value>
  </property>
  <property>
    <name>dfs.namenode.http-address.${NAMESERVICE}.${NAMENODE_BETA}</name>
    <value>127.0.0.1:${HDFS_NN_BETA_HTTP_PORT}</value>
  </property>
  <property>
    <name>dfs.namenode.rpc-bind-host</name>
    <value>0.0.0.0</value>
  </property>
  <property>
    <name>dfs.namenode.http-bind-host</name>
    <value>0.0.0.0</value>
  </property>
  <property>
    <name>dfs.namenode.shared.edits.dir</name>
    <value>qjournal://127.0.0.1:${HDFS_JN_RPC_PORT}/${NAMESERVICE}</value>
  </property>
  <property>
    <name>dfs.journalnode.rpc-address</name>
    <value>0.0.0.0:${HDFS_JN_RPC_PORT}</value>
  </property>
  <property>
    <name>dfs.journalnode.http-address</name>
    <value>0.0.0.0:${HDFS_JN_HTTP_PORT}</value>
  </property>
  <property>
    <name>dfs.journalnode.edits.dir</name>
    <value>/tmp/hadoop-jn</value>
  </property>
  <property>
    <name>dfs.client.failover.proxy.provider.${NAMESERVICE}</name>
    <value>org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider</value>
  </property>
  <property>
    <name>dfs.ha.automatic-failover.enabled</name>
    <value>true</value>
  </property>
  <property>
    <name>dfs.ha.zkfc.port.${NAMESERVICE}.${NAMENODE_ALPHA}</name>
    <value>${HDFS_ZKFC_ALPHA_PORT}</value>
  </property>
  <property>
    <name>dfs.ha.zkfc.port.${NAMESERVICE}.${NAMENODE_BETA}</name>
    <value>${HDFS_ZKFC_BETA_PORT}</value>
  </property>
  <property>
    <name>dfs.ha.fencing.methods</name>
    <value>shell(/bin/true)</value>
  </property>
  <property>
    <name>dfs.replication</name>
    <value>1</value>
  </property>
  <property>
    <name>dfs.permissions.enabled</name>
    <value>false</value>
  </property>
  <property>
    <name>dfs.namenode.datanode.registration.ip-hostname-check</name>
    <value>false</value>
  </property>
  <property>
    <name>dfs.client.use.datanode.hostname</name>
    <value>false</value>
  </property>
  <property>
    <name>dfs.namenode.safemode.min.datanodes</name>
    <value>1</value>
  </property>
  <property>
    <name>dfs.namenode.safemode.extension</name>
    <value>0</value>
  </property>
  <property>
    <name>dfs.namenode.name.dir</name>
    <value>file:///tmp/hadoop-nn</value>
  </property>
  <property>
    <name>dfs.datanode.data.dir</name>
    <value>file:///tmp/hadoop-dn</value>
  </property>
  <property>
    <name>dfs.datanode.address</name>
    <value>0.0.0.0:${HDFS_DN_XFER_PORT}</value>
  </property>
  <property>
    <name>dfs.datanode.ipc.address</name>
    <value>0.0.0.0:${HDFS_DN_IPC_PORT}</value>
  </property>
  <property>
    <name>dfs.datanode.http.address</name>
    <value>0.0.0.0:${HDFS_DN_HTTP_PORT}</value>
  </property>
EOF
    if [[ -n "${nn_id}" ]]; then
      cat <<EOF
  <property>
    <name>dfs.ha.namenode.id</name>
    <value>${nn_id}</value>
  </property>
EOF
    fi
    if [[ -n "${zkfc_port}" ]]; then
      cat <<EOF
  <property>
    <name>dfs.ha.zkfc.port</name>
    <value>${zkfc_port}</value>
  </property>
EOF
    fi
    echo "</configuration>"
  } > "${target_file}"
}

render_configs() {
  mkdir -p "${CONF_DIR}/client" "${CONF_DIR}/nn-alpha" "${CONF_DIR}/nn-beta"

  # 容器内独占端口位于隔离的 service:zk 网络命名空间内，无需在宿主机探测；默认使用标准 2181。
  # Internal-only ports live inside the isolated service:zk netns and need no host probing; default ZK to 2181.
  HDFS_ZK_PORT="${E2E_HDFS_ZK_PORT:-2181}"
  HDFS_JN_RPC_PORT="${E2E_HDFS_JN_RPC_PORT:-18485}"
  HDFS_JN_HTTP_PORT="${E2E_HDFS_JN_HTTP_PORT:-18480}"
  HDFS_ZKFC_ALPHA_PORT="${E2E_HDFS_ZKFC_ALPHA_PORT:-18019}"
  HDFS_ZKFC_BETA_PORT="${E2E_HDFS_ZKFC_BETA_PORT:-28019}"

  # 宿主机发布端口按排除列表依次挑选，防止连续端口彼此撞车。
  # Host-published ports are picked sequentially with an exclusion list so ports never collide.
  local used_ports=("${HDFS_ZK_PORT}" "${HDFS_JN_RPC_PORT}" "${HDFS_JN_HTTP_PORT}" "${HDFS_ZKFC_ALPHA_PORT}" "${HDFS_ZKFC_BETA_PORT}")
  HDFS_NN_ALPHA_RPC_PORT="${E2E_HDFS_NN_ALPHA_RPC_PORT:-$(pick_port 18020 "${used_ports[@]}")}"
  used_ports+=("${HDFS_NN_ALPHA_RPC_PORT}")
  HDFS_NN_BETA_RPC_PORT="${E2E_HDFS_NN_BETA_RPC_PORT:-$(pick_port 28020 "${used_ports[@]}")}"
  used_ports+=("${HDFS_NN_BETA_RPC_PORT}")
  HDFS_NN_ALPHA_HTTP_PORT="${E2E_HDFS_NN_ALPHA_HTTP_PORT:-$(pick_port 19870 "${used_ports[@]}")}"
  used_ports+=("${HDFS_NN_ALPHA_HTTP_PORT}")
  HDFS_NN_BETA_HTTP_PORT="${E2E_HDFS_NN_BETA_HTTP_PORT:-$(pick_port 29870 "${used_ports[@]}")}"
  used_ports+=("${HDFS_NN_BETA_HTTP_PORT}")
  HDFS_DN_XFER_PORT="${E2E_HDFS_DN_XFER_PORT:-$(pick_port 19866 "${used_ports[@]}")}"
  used_ports+=("${HDFS_DN_XFER_PORT}")
  HDFS_DN_IPC_PORT="${E2E_HDFS_DN_IPC_PORT:-$(pick_port 19867 "${used_ports[@]}")}"
  used_ports+=("${HDFS_DN_IPC_PORT}")
  HDFS_DN_HTTP_PORT="${E2E_HDFS_DN_HTTP_PORT:-$(pick_port 19864 "${used_ports[@]}")}"

  cat > "${ENV_FILE}" <<EOF
E2E_HDFS_HADOOP_IMAGE=${HADOOP_IMAGE}
E2E_HDFS_ZK_IMAGE=${ZK_IMAGE}
HDFS_HA_CONF_DIR=${CONF_DIR}
HDFS_ZK_PORT=${HDFS_ZK_PORT}
HDFS_JN_RPC_PORT=${HDFS_JN_RPC_PORT}
HDFS_JN_HTTP_PORT=${HDFS_JN_HTTP_PORT}
HDFS_NN_ALPHA_RPC_PORT=${HDFS_NN_ALPHA_RPC_PORT}
HDFS_NN_BETA_RPC_PORT=${HDFS_NN_BETA_RPC_PORT}
HDFS_NN_ALPHA_HTTP_PORT=${HDFS_NN_ALPHA_HTTP_PORT}
HDFS_NN_BETA_HTTP_PORT=${HDFS_NN_BETA_HTTP_PORT}
HDFS_ZKFC_ALPHA_PORT=${HDFS_ZKFC_ALPHA_PORT}
HDFS_ZKFC_BETA_PORT=${HDFS_ZKFC_BETA_PORT}
HDFS_DN_XFER_PORT=${HDFS_DN_XFER_PORT}
HDFS_DN_IPC_PORT=${HDFS_DN_IPC_PORT}
HDFS_DN_HTTP_PORT=${HDFS_DN_HTTP_PORT}
E2E_HDFS_HA_NAMESERVICE=${NAMESERVICE}
E2E_HDFS_HA_NAMENODES=${NAMENODE_ALPHA},${NAMENODE_BETA}
E2E_HDFS_HA_RPC_1=127.0.0.1:${HDFS_NN_ALPHA_RPC_PORT}
E2E_HDFS_HA_RPC_2=127.0.0.1:${HDFS_NN_BETA_RPC_PORT}
E2E_HDFS_HA_FAILOVER_PROVIDER=org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider
E2E_HDFS_HA_SITE_XML=${CONF_DIR}/client/hdfs-site.xml
EOF

  cat > "${CONF_DIR}/core-site.xml" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<configuration>
  <property>
    <name>fs.defaultFS</name>
    <value>hdfs://${NAMESERVICE}</value>
  </property>
  <property>
    <name>ha.zookeeper.quorum</name>
    <value>127.0.0.1:${HDFS_ZK_PORT}</value>
  </property>
  <property>
    <name>hadoop.tmp.dir</name>
    <value>/tmp/hadoop-data</value>
  </property>
</configuration>
EOF

  write_hdfs_site_xml "${CONF_DIR}/client/hdfs-site.xml" "" ""
  write_hdfs_site_xml "${CONF_DIR}/nn-alpha/hdfs-site.xml" "${NAMENODE_ALPHA}" "${HDFS_ZKFC_ALPHA_PORT}"
  write_hdfs_site_xml "${CONF_DIR}/nn-beta/hdfs-site.xml" "${NAMENODE_BETA}" "${HDFS_ZKFC_BETA_PORT}"
  chmod -R a+rX "${CONF_DIR}"
}

nn_jmx_state() {
  local http_port="$1"
  curl -fsS --max-time 3 "http://127.0.0.1:${http_port}/jmx?qry=Hadoop:service=NameNode,name=NameNodeStatus" 2>/dev/null \
    | python3 -c 'import json,sys; d=json.load(sys.stdin); print((d.get("beans") or [{}])[0].get("State",""))' 2>/dev/null || true
}

nn_live_dn_and_safemode() {
  local http_port="$1"
  # 使用 FSNamesystemState MBean（NumLiveDataNodes + FSState=Operational），避免 f-string 转义兼容问题。
  # Query FSNamesystemState MBean (NumLiveDataNodes + FSState=Operational) without backslash f-strings.
  curl -fsS --max-time 3 "http://127.0.0.1:${http_port}/jmx?qry=Hadoop:service=NameNode,name=FSNamesystemState" 2>/dev/null \
    | python3 -c 'import json,sys; b=(json.load(sys.stdin).get("beans") or [{}])[0]; print(str(b.get("NumLiveDataNodes", 0)) + "|" + str(b.get("FSState", "")))' 2>/dev/null || true
}

wait_ready() {
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
  local deadline=$((SECONDS + WAIT_TIMEOUT_SECONDS))
  local active_http=""
  echo "[hdfs-ha] waiting for Active/Standby NameNodes (${NAMENODE_ALPHA}:${HDFS_NN_ALPHA_HTTP_PORT}, ${NAMENODE_BETA}:${HDFS_NN_BETA_HTTP_PORT})..."
  while [[ "${SECONDS}" -lt "${deadline}" ]]; do
    local s1 s2
    s1="$(nn_jmx_state "${HDFS_NN_ALPHA_HTTP_PORT}")"
    s2="$(nn_jmx_state "${HDFS_NN_BETA_HTTP_PORT}")"
    if [[ "${s1}" == "active" && "${s2}" == "standby" ]]; then
      active_http="${HDFS_NN_ALPHA_HTTP_PORT}"
      break
    fi
    if [[ "${s2}" == "active" && "${s1}" == "standby" ]]; then
      active_http="${HDFS_NN_BETA_HTTP_PORT}"
      break
    fi
    sleep 2
  done
  if [[ -z "${active_http}" ]]; then
    echo "[hdfs-ha] timed out waiting for Active/Standby NameNodes" >&2
    compose_cmd ps -a >&2 || true
    for svc in zk jn nn-alpha nn-beta dn; do
      echo "===== [hdfs-ha] logs: ${svc} =====" >&2
      compose_cmd logs --no-color --tail=60 "${svc}" >&2 || true
    done
    return 1
  fi
  echo "[hdfs-ha] HA pair ready (active HTTP port=${active_http}); waiting for DataNode + SafeMode exit..."
  local ready_dn=0
  local last_dn_status=""
  while [[ "${SECONDS}" -lt "${deadline}" ]]; do
    local status live_dns fs_state
    status="$(nn_live_dn_and_safemode "${active_http}")"
    last_dn_status="${status}"
    live_dns="${status%%|*}"
    fs_state="${status#*|}"
    if [[ "${live_dns:-0}" =~ ^[0-9]+$ && "${live_dns:-0}" -ge 1 && "${fs_state}" == "Operational" ]]; then
      ready_dn=1
      break
    fi
    sleep 2
  done
  if [[ "${ready_dn}" -ne 1 ]]; then
    echo "[hdfs-ha] timed out waiting for live DataNode or SafeMode exit (last_status=${last_dn_status:-empty})" >&2
    compose_cmd ps -a >&2 || true
    for svc in zk jn nn-alpha nn-beta dn; do
      echo "===== [hdfs-ha] logs: ${svc} =====" >&2
      compose_cmd logs --no-color --tail=60 "${svc}" >&2 || true
    done
    return 1
  fi

  echo "[hdfs-ha] initializing /seatunnel/checkpoint and /seatunnel/imap directories and verifying write/read..."
  local init_ok=0
  for attempt in 1 2 3 4 5; do
    if compose_cmd exec -T nn-alpha bash -lc 'set -euo pipefail; hdfs dfsadmin -report | grep -E "Live datanodes|Name:|Hostname:" || true; hdfs dfs -mkdir -p /seatunnel/checkpoint /seatunnel/imap; hdfs dfs -chmod -R 777 /seatunnel; printf "ok\n" | hdfs dfs -put -f - /seatunnel/_ready; test "$(hdfs dfs -cat /seatunnel/_ready)" = "ok"'; then
      init_ok=1
      break
    fi
    echo "[hdfs-ha] init attempt ${attempt}/5 failed, retrying in 3s..." >&2
    sleep 3
  done
  if [[ "${init_ok}" -ne 1 ]]; then
    echo "[hdfs-ha] failed to initialize /seatunnel directories after 5 attempts" >&2
    compose_cmd ps -a >&2 || true
    return 1
  fi
  echo "[hdfs-ha] ready: hdfs://${NAMESERVICE} (${NAMENODE_ALPHA}=${E2E_HDFS_HA_RPC_1}, ${NAMENODE_BETA}=${E2E_HDFS_HA_RPC_2}, site=${E2E_HDFS_HA_SITE_XML})"
}

pull_image_with_retry() {
  local image="$1"
  if docker image inspect "${image}" >/dev/null 2>&1; then
    return 0
  fi
  for attempt in 1 2 3; do
    if docker pull "${image}"; then
      return 0
    fi
    if [[ "${attempt}" -lt 3 ]]; then
      echo "[hdfs-ha] docker pull ${image} failed (attempt ${attempt}/3), retrying in $((attempt * 5))s..." >&2
      sleep $((attempt * 5))
    fi
  done
  echo "[hdfs-ha] failed to pull ${image} after 3 attempts" >&2
  return 1
}

start_stack() {
  if [[ -f "${ENV_FILE}" ]]; then
    compose_cmd down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  render_configs
  if [[ "${E2E_HDFS_SKIP_PULL:-}" != "1" ]]; then
    pull_image_with_retry "${ZK_IMAGE}"
    pull_image_with_retry "${HADOOP_IMAGE}"
  fi
  compose_cmd up -d
  wait_ready
}

stop_stack() {
  if [[ -f "${ENV_FILE}" ]]; then
    compose_cmd down -v --remove-orphans >/dev/null 2>&1 || true
  else
    docker compose -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}" down -v --remove-orphans >/dev/null 2>&1 || true
  fi
}

dump_logs() {
  local out_dir="${2:-${STATE_DIR}/logs}"
  mkdir -p "${out_dir}"
  if [[ -f "${ENV_FILE}" ]]; then
    for svc in zk jn nn-alpha nn-beta dn; do
      compose_cmd logs --no-color "${svc}" > "${out_dir}/${svc}.log" 2>&1 || true
    done
  fi
}

case "${ACTION}" in
  start)
    start_stack
    ;;
  stop)
    stop_stack
    ;;
  wait-ready)
    wait_ready
    ;;
  env)
    cat "${ENV_FILE}"
    ;;
  logs)
    dump_logs "$@"
    ;;
  *)
    echo "Usage: $0 {start|stop|wait-ready|env|logs [out-dir]}" >&2
    exit 2
    ;;
esac
