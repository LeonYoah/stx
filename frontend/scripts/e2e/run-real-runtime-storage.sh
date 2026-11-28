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

# 运行时存储（Checkpoint + IMAP）真实全链路 E2E 入口：编排 Hadoop HDFS HA + MinIO S3 + Local + STX/Agent/SeaTunnel。
# Real runtime-storage (Checkpoint + IMAP) E2E runner: orchestrates Hadoop HDFS HA + MinIO S3 + Local + STX/Agent/SeaTunnel.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ROOT_DIR="$(cd "${FRONTEND_DIR}/.." && pwd)"

HDFS_HA_SCRIPT="${ROOT_DIR}/scripts/e2e/hdfs-ha/hdfs-ha.sh"
HDFS_HA_PROJECT="${E2E_HDFS_HA_PROJECT:-stx-e2e-runtime-storage-hdfs-ha}"
HDFS_HA_STATE_DIR="${E2E_HDFS_HA_STATE_DIR:-${ROOT_DIR}/tmp/e2e/${HDFS_HA_PROJECT}}"
HDFS_HA_LOG_EXPORT_DIR="${FRONTEND_DIR}/test-results/hdfs-ha-logs"

export E2E_HDFS_HA_PROJECT="${HDFS_HA_PROJECT}"
export E2E_HDFS_HA_STATE_DIR="${HDFS_HA_STATE_DIR}"
export E2E_REAL_PLAYWRIGHT_SPEC="${E2E_REAL_PLAYWRIGHT_SPEC:-e2e/runtime-storage-real.spec.ts}"

# Java Proxy 源码新于已有 jar 时强制重编，避免 E2E 误测旧 jar。
# Rebuild stx-java-proxy jar whenever Java sources are newer than the cached jar.
JAVA_PROXY_EPOCH="${STX_JAVA_PROXY_DEFAULT_VERSION:-v2}"
JAVA_PROXY_JAR="${ROOT_DIR}/lib/stx-java-proxy-${JAVA_PROXY_EPOCH}.jar"
if [[ -f "${JAVA_PROXY_JAR}" ]]; then
  if [[ "${E2E_FORCE_REBUILD_JAVA_PROXY:-}" == "1" ]] || \
     [[ -n "$(find "${ROOT_DIR}/tools/stx-java-proxy/src/main" "${ROOT_DIR}/tools/stx-java-proxy/pom.xml" -type f -newer "${JAVA_PROXY_JAR}" 2>/dev/null | head -n 1)" ]]; then
    echo "[runtime-storage-real] stx-java-proxy sources changed; removing stale jar for rebuild"
    rm -f "${JAVA_PROXY_JAR}"
  fi
fi

cleanup_hdfs_ha() {
  mkdir -p "${HDFS_HA_LOG_EXPORT_DIR}" || true
  bash "${HDFS_HA_SCRIPT}" logs "${HDFS_HA_LOG_EXPORT_DIR}" >/dev/null 2>&1 || true
  # 导出最新 SeaTunnel / Agent / Backend 日志与渲染配置，便于 CI 失败直接定位。
  # Export latest SeaTunnel / Agent / Backend logs and rendered configs for fast CI triage.
  local diag_dir="${FRONTEND_DIR}/test-results/runtime-diagnostics"
  local latest_tmp
  latest_tmp="$(ls -td "${ROOT_DIR}"/tmp/e2e/installer-real.* 2>/dev/null | head -n 1 || true)"
  if [[ -n "${latest_tmp}" && -d "${latest_tmp}" ]]; then
    mkdir -p "${diag_dir}" || true
    cp -f "${latest_tmp}"/logs/*.log "${diag_dir}/" 2>/dev/null || true
    find "${latest_tmp}/install" -maxdepth 3 -type f \( -name 'seatunnel-engine-server.log' -o -name 'seatunnel.yaml' -o -name 'hazelcast.yaml' \) -exec cp -f {} "${diag_dir}/" \; 2>/dev/null || true
  fi
  bash "${HDFS_HA_SCRIPT}" stop >/dev/null 2>&1 || true
}
trap cleanup_hdfs_ha EXIT

echo "[runtime-storage-real] starting Hadoop HDFS HA stack (${HDFS_HA_PROJECT})..."
bash "${HDFS_HA_SCRIPT}" start

# 导出 HDFS HA 连接参数与宿主机 hdfs-site.xml 路径供 Playwright 用例读取。
# Export HDFS HA connection settings and host hdfs-site.xml path for Playwright specs.
set -a
# shellcheck disable=SC1091
source "${HDFS_HA_STATE_DIR}/hdfs-ha.env"
set +a

echo "[runtime-storage-real] HDFS HA ready: hdfs://${E2E_HDFS_HA_NAMESERVICE} (${E2E_HDFS_HA_NAMENODES} -> ${E2E_HDFS_HA_RPC_1}, ${E2E_HDFS_HA_RPC_2})"
bash "${SCRIPT_DIR}/run-real-installer.sh"
