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

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Case A (profiles_missing fail-closed) — dedicated harness / CI job.
# Must not share a Playwright process or TMP_DIR with upgrade-real.spec.ts:
# both install a live SeaTunnel and Case A also seeds target jdbc with empty
# profiles that would poison Case B's mysql metadata if storage were shared.
export E2E_REAL_PLAYWRIGHT_SPEC="${E2E_REAL_PLAYWRIGHT_SPEC:-e2e/upgrade-real-profiles-missing.spec.ts}"
export E2E_UPGRADE_REAL_SOURCE_VERSION="${E2E_UPGRADE_REAL_SOURCE_VERSION:-2.3.13}"
export E2E_UPGRADE_REAL_TARGET_VERSION="${E2E_UPGRADE_REAL_TARGET_VERSION:-3.0.0}"

exec bash "${SCRIPT_DIR}/run-real-installer.sh"
