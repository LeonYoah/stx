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

# Real DolphinScheduler ↔ STX E2E: reuses installer-real stack (STX+Agent+SeaTunnel)
# then boots DS standalone. Playwright spec drives push/start/assert on real DS logs.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

export E2E_REAL_PLAYWRIGHT_SPEC="${E2E_REAL_PLAYWRIGHT_SPEC:-e2e/scheduler-real.spec.ts}"
export E2E_SCHEDULER_REAL=1
export E2E_DS_CONTAINER_NAME="${E2E_DS_CONTAINER_NAME:-stx-ds-e2e-scheduler}"
export E2E_DS_IMAGE="${E2E_DS_IMAGE:-apache/dolphinscheduler-standalone-server:3.2.2}"
export E2E_DS_PORT="${E2E_DS_PORT:-12345}"
export E2E_DS_BASE_URL="${E2E_DS_BASE_URL:-http://127.0.0.1:${E2E_DS_PORT}/dolphinscheduler}"

# Ensure DS is removed even if installer cleanup runs first.
_orig_cleanup_note=1
cleanup_ds() {
  bash "${ROOT_DIR}/scripts/e2e/ds-standalone.sh" stop || true
}
trap cleanup_ds EXIT

# Pre-pull DS image unless CI already loaded it from cache.
if [[ "${E2E_DS_SKIP_PULL:-}" != "1" ]]; then
  docker pull "${E2E_DS_IMAGE}" >/dev/null 2>&1 || docker pull "${E2E_DS_IMAGE}"
fi

exec bash "${SCRIPT_DIR}/run-real-installer.sh"
