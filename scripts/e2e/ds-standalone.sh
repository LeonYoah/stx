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

# Start/stop DolphinScheduler standalone for real scheduler E2E.
# Pitfalls encoded here:
# - H2 wipes on every container recreate → always bootstrap token/project/env after start
# - Bind-mount stx AFTER go build so the inode matches (never rebuild without recreate)
# - Prefer --network host on Linux CI so Worker reaches STX on 127.0.0.1
set -euo pipefail

CMD="${1:-}"
NAME="${E2E_DS_CONTAINER_NAME:-stx-ds-e2e-scheduler}"
IMAGE="${E2E_DS_IMAGE:-apache/dolphinscheduler-standalone-server:3.2.2}"
STX_BIN="${E2E_STX_CLI_BIN:-}"
DS_PORT="${E2E_DS_PORT:-12345}"

usage() {
  echo "usage: $0 start|stop|wait|status" >&2
  exit 2
}

stop_ds() {
  docker rm -f "${NAME}" >/dev/null 2>&1 || true
}

start_ds() {
  if [[ -z "${STX_BIN}" || ! -x "${STX_BIN}" ]]; then
    echo "E2E_STX_CLI_BIN must point to a built stx binary (got: ${STX_BIN:-empty})" >&2
    exit 1
  fi
  stop_ds
  # Host network: DS Worker and STX share loopback (CI Linux). Avoid 172.17.0.1 guesswork.
  docker run -d \
    --name "${NAME}" \
    --network host \
    -e TZ=Asia/Shanghai \
    -v "${STX_BIN}:/usr/local/bin/stx:ro" \
    "${IMAGE}" >/dev/null
  echo "started ${NAME} image=${IMAGE} stx=${STX_BIN}"
  # Static CLI must run inside DS (Ubuntu 22.04). Catch glibc/PATH issues before Playwright.
  if ! docker exec "${NAME}" /usr/local/bin/stx version >/dev/null 2>&1; then
    echo "stx binary not runnable inside DS container:" >&2
    docker exec "${NAME}" ls -la /usr/local/bin/stx >&2 || true
    docker exec "${NAME}" /usr/local/bin/stx version >&2 || true
    exit 1
  fi
}

wait_ds() {
  local base="http://127.0.0.1:${DS_PORT}/dolphinscheduler"
  for _ in $(seq 1 90); do
    if curl -sf -o /dev/null -X POST "${base}/login" \
      -H 'Content-Type: application/x-www-form-urlencoded' \
      -d 'userName=admin&userPassword=dolphinscheduler123'; then
      echo "DS ready at ${base}"
      return 0
    fi
    sleep 2
  done
  echo "DS did not become ready within 180s" >&2
  docker logs "${NAME}" 2>&1 | tail -80 >&2 || true
  return 1
}

case "${CMD}" in
  start) start_ds; wait_ds ;;
  stop) stop_ds ;;
  wait) wait_ds ;;
  status) docker ps -a --filter "name=${NAME}" --format '{{.Status}}' ;;
  *) usage ;;
esac
