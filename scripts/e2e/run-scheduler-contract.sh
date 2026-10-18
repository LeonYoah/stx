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

# CI-light scheduler contract suite (DS↔STX CLI path without real DolphinScheduler).
# 轻量调度契约：用 Go httptest 覆盖 DS SHELL 会调用的 --wait 路径，不拉起真实 DS/ST。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

echo "==> scheduler contract: CLI wait plain-text / fail / events-json"
go test ./internal/cmd/ -count=1 -timeout 120s \
  -run 'TestSchedulerE2E|TestSyncTaskSubmitSendsWaitAndPollsJob|TestFormatCLIJobMetricsBlock|TestNormalizeCLILogNewlines'

echo "==> scheduler contract: EventWriter text vs JSON"
go test ./internal/cli/output/ -count=1 -timeout 60s \
  -run 'TestTextEventWriterEmitsPlainLines|TestJSONEventWriterStillEmitsNDJSON'

echo "==> scheduler contract: status mapping + final metrics + dolphin script"
go test ./internal/apps/sync/ -count=1 -timeout 180s \
  -run 'TestNormalizeJobStatusEngineAligned|TestBuildDolphinShellScriptOmitsVarWithoutOverrides|TestHasFinalMetricsLogAndLatestBlockMarker|TestFormatJobMetricsLogBlockIncludesBilingualLabels|TestMaybeRecordMetricsLogRespectsIntervalAndForce|TestRefreshJobInstanceBackfillsFinalMetricsWhenOnlySnapshotExists'

echo "==> scheduler contract: OK"
