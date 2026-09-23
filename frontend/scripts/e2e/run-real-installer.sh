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

PREPARE_PACKAGE_ONLY="${1:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ROOT_DIR="$(cd "${FRONTEND_DIR}/.." && pwd)"
BASE_TMP_DIR="${ROOT_DIR}/tmp/e2e"
TMP_DIR=""
MINIO_NAME="${E2E_INSTALLER_REAL_MINIO_NAME:-stx-installer-real-minio}"
KEEP_ARTIFACTS="${E2E_INSTALLER_REAL_KEEP_ARTIFACTS:-0}"
SKIP_LOCAL_CLEANUP="${E2E_INSTALLER_REAL_SKIP_LOCAL_CLEANUP:-}"
BACKEND_TEMPLATE="${ROOT_DIR}/config.e2e.installer-real.yaml"
AGENT_TEMPLATE="${ROOT_DIR}/config.e2e.agent-real.yaml"
MINIO_MC_IMAGE="${E2E_INSTALLER_REAL_MINIO_MC_IMAGE:-quay.io/minio/mc:RELEASE.2025-03-12T17-29-24Z}"
PLAYWRIGHT_PROJECT="${PLAYWRIGHT_PROJECT:-}"
PLAYWRIGHT_GREP="${PLAYWRIGHT_GREP:-}"
PLAYWRIGHT_SPEC="${E2E_REAL_PLAYWRIGHT_SPEC:-e2e/install-wizard-real.spec.ts}"
JAVA_PROXY_VERSION="${E2E_INSTALLER_REAL_VERSION:-2.3.13}"
JAVA_PROXY_EPOCH="${STX_JAVA_PROXY_DEFAULT_VERSION:-v2}"
PACKAGE_PRELOAD_MIRROR="${E2E_INSTALLER_REAL_PRELOAD_MIRROR:-}"
PACKAGE_DOWNLOAD_TIMEOUT_SECONDS="${E2E_INSTALLER_REAL_DOWNLOAD_TIMEOUT_SECONDS:-1800}"
PACKAGE_CACHE_DIR="${E2E_INSTALLER_REAL_PACKAGE_CACHE_DIR:-}"

if [[ -z "${SKIP_LOCAL_CLEANUP}" ]]; then
  if [[ "${CI:-}" == "true" || "${GITHUB_ACTIONS:-}" == "true" ]]; then
    SKIP_LOCAL_CLEANUP="1"
  else
    SKIP_LOCAL_CLEANUP="0"
  fi
fi

pick_port() {
  local start_port="$1"
  python3 - "$start_port" <<'PY'
import socket
import sys

start = int(sys.argv[1])
for port in range(start, start + 200):
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.bind(("127.0.0.1", port))
        except OSError:
            continue
        print(port)
        sys.exit(0)

raise SystemExit("no free port found")
PY
}

resolve_preload_mirror() {
  local mirror="${PACKAGE_PRELOAD_MIRROR:-}"
  if [[ -n "${mirror}" ]]; then
    echo "${mirror}"
    return 0
  fi
  if [[ "${CI:-}" == "true" || "${GITHUB_ACTIONS:-}" == "true" ]]; then
    echo "apache"
    return 0
  fi
  echo "aliyun"
}

mirror_url() {
  local mirror="$1"
  case "$mirror" in
    apache|maven|central|official)
      echo "https://archive.apache.org/dist/seatunnel"
      ;;
    huaweicloud)
      echo "https://mirrors.huaweicloud.com/apache/seatunnel"
      ;;
    aliyun)
      echo "https://mirrors.aliyun.com/apache/seatunnel"
      ;;
    *)
      echo "unsupported SeaTunnel preload mirror: ${mirror}" >&2
      return 1
      ;;
  esac
}

download_package_if_missing() {
  local version="$1"
  local mirror="$2"
  local packages_dir="$3"

  if [[ -z "${version}" ]]; then
    return 0
  fi

  local file_name="apache-seatunnel-${version}-bin.tar.gz"
  local final_path="${packages_dir}/${file_name}"
  local temp_path="${final_path}.tmp"
  local base_url
  base_url="$(mirror_url "${mirror}")"
  local download_url="${base_url}/${version}/${file_name}"
  local timeout_seconds="${PACKAGE_DOWNLOAD_TIMEOUT_SECONDS}"

  if [[ -s "${final_path}" ]]; then
    echo "[e2e-real] package cache hit: ${file_name}"
    return 0
  fi

  echo "[e2e-real] preloading package ${file_name} from ${mirror} ..."
  mkdir -p "${packages_dir}"

  if command -v curl >/dev/null 2>&1; then
    curl -fL \
      --retry 3 \
      --retry-delay 2 \
      --connect-timeout 30 \
      --max-time "${timeout_seconds}" \
      --continue-at - \
      -o "${temp_path}" \
      "${download_url}"
  elif command -v wget >/dev/null 2>&1; then
    if command -v timeout >/dev/null 2>&1; then
      timeout "${timeout_seconds}" \
        wget \
        --tries=3 \
        --waitretry=2 \
        --timeout=30 \
        --read-timeout=30 \
        -O "${temp_path}" \
        "${download_url}"
    else
      wget \
        --tries=3 \
        --waitretry=2 \
        --timeout=30 \
        --read-timeout=30 \
        -O "${temp_path}" \
        "${download_url}"
    fi
  else
    echo "curl or wget is required to preload SeaTunnel packages" >&2
    exit 1
  fi

  if [[ ! -s "${temp_path}" ]]; then
    echo "failed to preload package ${file_name}" >&2
    exit 1
  fi

  mv "${temp_path}" "${final_path}"
  echo "[e2e-real] package preloaded: ${final_path}"
}

cleanup() {
  docker rm -f "${MINIO_NAME}" >/dev/null 2>&1 || true
  if [[ -n "${TMP_DIR}" && "${KEEP_ARTIFACTS}" != "1" && "${SKIP_LOCAL_CLEANUP}" != "1" ]]; then
    rm -rf "${TMP_DIR}" || true
  fi
}

trap cleanup EXIT

pkill -f 'node ./scripts/e2e/real-agent-supervisor.mjs' >/dev/null 2>&1 || true

mkdir -p "${BASE_TMP_DIR}"
TMP_DIR="$(mktemp -d "${BASE_TMP_DIR}/installer-real.XXXXXX")"
mkdir -p "${TMP_DIR}/logs" "${TMP_DIR}/storage" "${TMP_DIR}/install" "${TMP_DIR}/minio"
mkdir -p "${TMP_DIR}/storage/packages" "${TMP_DIR}/storage/temp"
if [[ -n "${PACKAGE_CACHE_DIR}" ]]; then
  mkdir -p "${PACKAGE_CACHE_DIR}"
fi
docker rm -f "${MINIO_NAME}" >/dev/null 2>&1 || true

JAVA_PROXY_SCRIPT_PATH="${ROOT_DIR}/scripts/stx-java-proxy.sh"
JAVA_PROXY_LIB_PATH="${ROOT_DIR}/lib/stx-java-proxy-${JAVA_PROXY_EPOCH}.jar"
if [[ ! -f "${JAVA_PROXY_LIB_PATH}" ]]; then
  mvn -q -DskipTests package -f "${ROOT_DIR}/tools/stx-java-proxy/pom.xml"
  BUILT_JAVA_PROXY_JAR="$(find "${ROOT_DIR}/tools/stx-java-proxy/target" -maxdepth 1 -type f -name "stx-java-proxy-${JAVA_PROXY_EPOCH}.jar" | sort | head -n 1 || true)"
  if [[ -z "${BUILT_JAVA_PROXY_JAR}" ]]; then
    echo "failed to build stx-java-proxy jar for epoch ${JAVA_PROXY_EPOCH}" >&2
    exit 1
  fi
  mkdir -p "${ROOT_DIR}/lib"
  cp "${BUILT_JAVA_PROXY_JAR}" "${JAVA_PROXY_LIB_PATH}"
fi
if [[ ! -f "${JAVA_PROXY_SCRIPT_PATH}" ]]; then
  echo "stx-java-proxy script is missing: ${JAVA_PROXY_SCRIPT_PATH}" >&2
  exit 1
fi
chmod +x "${JAVA_PROXY_SCRIPT_PATH}"

BACKEND_HTTP_PORT="$(pick_port "${E2E_INSTALLER_REAL_BACKEND_PORT:-18000}")"
BACKEND_GRPC_PORT="$(pick_port "${E2E_INSTALLER_REAL_GRPC_PORT:-19090}")"
FRONTEND_PORT="$(pick_port "${E2E_FRONTEND_PORT:-3300}")"
SUPERVISOR_PORT="$(pick_port "${E2E_AGENT_SUPERVISOR_PORT:-18181}")"
CLUSTER_PORT_PRIMARY="$(pick_port "${E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY:-38181}")"
CLUSTER_PORT_SECONDARY="$(pick_port "$((CLUSTER_PORT_PRIMARY + 1))")"
HTTP_PORT_PRIMARY="$(pick_port "${E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY:-38080}")"
HTTP_PORT_SECONDARY="$(pick_port "$((HTTP_PORT_PRIMARY + 1))")"

BACKEND_CONFIG_PATH="${TMP_DIR}/config.e2e.installer-real.yaml"
AGENT_CONFIG_PATH="${TMP_DIR}/config.e2e.agent-real.yaml"
TMP_DIR_ESCAPED="$(printf '%s\n' "${TMP_DIR}" | sed 's/[\/&]/\\&/g')"

declare -a PRELOAD_VERSIONS=()
PRELOAD_VERSIONS+=("${E2E_INSTALLER_REAL_VERSION:-2.3.13}")
if [[ -n "${E2E_UPGRADE_REAL_SOURCE_VERSION:-}" ]]; then
  PRELOAD_VERSIONS+=("${E2E_UPGRADE_REAL_SOURCE_VERSION}")
fi
if [[ -n "${E2E_UPGRADE_REAL_TARGET_VERSION:-}" ]]; then
  PRELOAD_VERSIONS+=("${E2E_UPGRADE_REAL_TARGET_VERSION}")
fi

declare -A PRELOAD_VERSION_SEEN=()
for version in "${PRELOAD_VERSIONS[@]}"; do
  if [[ -n "${version}" && -z "${PRELOAD_VERSION_SEEN[${version}]:-}" ]]; then
    PRELOAD_VERSION_SEEN["${version}"]=1
    resolved_mirror="$(resolve_preload_mirror)"
    download_package_if_missing "${version}" "${resolved_mirror}" "${PACKAGE_CACHE_DIR:-${TMP_DIR}/storage/packages}"
  fi
done

if [[ -n "${PACKAGE_CACHE_DIR}" ]]; then
  find "${PACKAGE_CACHE_DIR}" -maxdepth 1 -type f -name 'apache-seatunnel-*-bin.tar.gz' -print0 | while IFS= read -r -d '' package_file; do
    cp -f "${package_file}" "${TMP_DIR}/storage/packages/"
  done
fi

if [[ "${PREPARE_PACKAGE_ONLY}" == "--prepare-package-only" ]]; then
  echo "[e2e-real] package preload only mode completed"
  exit 0
fi

sed \
  -e "s/:18000/:${BACKEND_HTTP_PORT}/g" \
  -e "s|http://127.0.0.1:18000|http://127.0.0.1:${BACKEND_HTTP_PORT}|g" \
  -e "s/port: 19090/port: ${BACKEND_GRPC_PORT}/g" \
  -e "s|\.\./tmp/e2e/installer-real/stx\\.db|${TMP_DIR_ESCAPED}/stx.db|g" \
  -e "s|\.\./tmp/e2e/installer-real/storage|${TMP_DIR_ESCAPED}/storage|g" \
  -e "s|\.\./tmp/e2e/installer-real/logs/stx\\.log|${TMP_DIR_ESCAPED}/logs/stx.log|g" \
  "${BACKEND_TEMPLATE}" > "${BACKEND_CONFIG_PATH}"

sed \
  -e "s/127.0.0.1:19090/127.0.0.1:${BACKEND_GRPC_PORT}/g" \
  -e "s|tmp/e2e/installer-real/logs/stx-agent\\.log|${TMP_DIR_ESCAPED}/logs/stx-agent.log|g" \
  -e "s|tmp/e2e/installer-real/install/default|${TMP_DIR_ESCAPED}/install/default|g" \
  "${AGENT_TEMPLATE}" > "${AGENT_CONFIG_PATH}"

MINIO_CONTAINER_ID="$(
docker run -d \
  --name "${MINIO_NAME}" \
  -p 127.0.0.1::9000 \
  -p 127.0.0.1::9001 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  -v "${TMP_DIR}/minio:/data" \
  quay.io/minio/minio:RELEASE.2025-02-18T16-25-55Z \
  server /data --console-address :9001
)"

MINIO_API_PORT="$(
  docker port "${MINIO_CONTAINER_ID}" 9000/tcp | awk -F: 'NR==1{print $NF}'
)"
MINIO_CONSOLE_PORT="$(
  docker port "${MINIO_CONTAINER_ID}" 9001/tcp | awk -F: 'NR==1{print $NF}'
)"

for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${MINIO_API_PORT}/minio/health/live" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

export E2E_INSTALLER_REAL=1
export E2E_API_MODE=real
export STX_JAVA_PROXY_HOME="${ROOT_DIR}"
export STX_JAVA_PROXY_SCRIPT="${JAVA_PROXY_SCRIPT_PATH}"
export STX_JAVA_PROXY_JAR="${JAVA_PROXY_LIB_PATH}"
export E2E_BACKEND_BASE_URL="${E2E_BACKEND_BASE_URL:-http://127.0.0.1:${BACKEND_HTTP_PORT}}"
export E2E_FRONTEND_HOST="${E2E_FRONTEND_HOST:-127.0.0.1}"
export E2E_FRONTEND_PORT="${E2E_FRONTEND_PORT:-${FRONTEND_PORT}}"
export E2E_FRONTEND_BASE_URL="${E2E_FRONTEND_BASE_URL:-http://${E2E_FRONTEND_HOST}:${E2E_FRONTEND_PORT}}"
export E2E_AGENT_SUPERVISOR_PORT="${E2E_AGENT_SUPERVISOR_PORT:-${SUPERVISOR_PORT}}"
export E2E_INSTALLER_REAL_CONFIG_PATH="${E2E_INSTALLER_REAL_CONFIG_PATH:-${BACKEND_CONFIG_PATH}}"
export E2E_AGENT_REAL_CONFIG_PATH="${E2E_AGENT_REAL_CONFIG_PATH:-${AGENT_CONFIG_PATH}}"
export E2E_INSTALLER_REAL_VERSION="${E2E_INSTALLER_REAL_VERSION:-2.3.13}"
export E2E_INSTALLER_REAL_INSTALL_DIR="${E2E_INSTALLER_REAL_INSTALL_DIR:-${TMP_DIR}/install/seatunnel-${E2E_INSTALLER_REAL_VERSION}}"
export E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY="${E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY:-${CLUSTER_PORT_PRIMARY}}"
export E2E_INSTALLER_REAL_CLUSTER_PORT_SECONDARY="${E2E_INSTALLER_REAL_CLUSTER_PORT_SECONDARY:-${CLUSTER_PORT_SECONDARY}}"
export E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY="${E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY:-${HTTP_PORT_PRIMARY}}"
export E2E_INSTALLER_REAL_HTTP_PORT_SECONDARY="${E2E_INSTALLER_REAL_HTTP_PORT_SECONDARY:-${HTTP_PORT_SECONDARY}}"
export E2E_INSTALLER_REAL_MINIO_ENDPOINT="${E2E_INSTALLER_REAL_MINIO_ENDPOINT:-http://127.0.0.1:${MINIO_API_PORT}}"
export E2E_INSTALLER_REAL_MINIO_ACCESS_KEY="${E2E_INSTALLER_REAL_MINIO_ACCESS_KEY:-minioadmin}"
export E2E_INSTALLER_REAL_MINIO_SECRET_KEY="${E2E_INSTALLER_REAL_MINIO_SECRET_KEY:-minioadmin}"
export E2E_INSTALLER_REAL_CHECKPOINT_BUCKET="${E2E_INSTALLER_REAL_CHECKPOINT_BUCKET:-s3a://seatunnel-checkpoint}"
export E2E_INSTALLER_REAL_IMAP_BUCKET="${E2E_INSTALLER_REAL_IMAP_BUCKET:-s3a://seatunnel-imap}"
export E2E_INSTALLER_REAL_TMP_DIR="${TMP_DIR}"

CHECKPOINT_BUCKET_NAME="${E2E_INSTALLER_REAL_CHECKPOINT_BUCKET#s3a://}"
IMAP_BUCKET_NAME="${E2E_INSTALLER_REAL_IMAP_BUCKET#s3a://}"

docker run --rm --network host --entrypoint /bin/sh "${MINIO_MC_IMAGE}" -c "\
  mc alias set e2e ${E2E_INSTALLER_REAL_MINIO_ENDPOINT} ${E2E_INSTALLER_REAL_MINIO_ACCESS_KEY} ${E2E_INSTALLER_REAL_MINIO_SECRET_KEY} && \
  mc mb -p e2e/${CHECKPOINT_BUCKET_NAME} && \
  mc mb -p e2e/${IMAP_BUCKET_NAME}" >/dev/null

cd "${FRONTEND_DIR}"
playwright_args=("${PLAYWRIGHT_SPEC}")
if [[ -n "${PLAYWRIGHT_PROJECT}" ]]; then
  playwright_args+=(--project "${PLAYWRIGHT_PROJECT}")
fi
if [[ -n "${PLAYWRIGHT_GREP}" ]]; then
  playwright_args+=(--grep "${PLAYWRIGHT_GREP}")
fi
pnpm exec playwright test "${playwright_args[@]}"
