/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
)

// schedulerE2EContract simulates the DolphinScheduler SHELL path:
//   stx sync task submit <id> --wait --confirm --external-ref ds:...
// without running a real DS or SeaTunnel cluster (CI-light contract).
//
// 本文件用 httptest 模拟 DS SHELL 调用的 CLI 路径，验证纯文本日志、终态指标与失败退出，
// 不拉起真实 DolphinScheduler / SeaTunnel（适合 CI 的轻量契约）。

func TestSchedulerE2EWaitSuccessPlainTextAndFinalMetrics(t *testing.T) {
	var polls atomic.Int32
	var gotBody map[string]any
	metricsBlock := "========== STX metrics final / 任务终态指标 ==========\n" +
		"time:                  2026-10-17T00:00:00Z\n" +
		"read_count:            50\n" +
		"write_count:           50\n" +
		"==============================================\n"

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/api/v1/capabilities":
			writeSyncCapabilities(t, writer, "sync.task.submit", "R2")
		case request.URL.Path == "/api/v1/sync/tasks/7":
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 7, "mode": "batch"}})
		case request.URL.Path == "/api/v1/sync/tasks/7/submit":
			if err := json.NewDecoder(request.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode submit body: %v", err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"id": 101, "task_id": 7, "status": "running",
					"attach_generation": 1, "sync_state": "ok",
					"external_ref": "ds:11:22",
				},
			})
		case request.URL.Path == "/api/v1/sync/jobs/101":
			n := polls.Add(1)
			status := "running"
			preview := map[string]any{}
			if n >= 2 {
				status = "success"
				preview["stx_metrics_log"] = metricsBlock
				preview["metrics"] = map[string]any{
					"SourceReceivedCount": float64(50),
					"SinkWriteCount":      float64(50),
				}
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"id": 101, "status": status, "sync_state": "ok",
					"external_ref": "ds:11:22", "result_preview": preview,
				},
			})
		case request.URL.Path == "/api/v1/sync/jobs/101/logs":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"logs":        "2026-10-17 INFO starting seatunnel source task\n",
					"next_offset": "64",
				},
			})
		case strings.HasSuffix(request.URL.Path, "/cancel"):
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 101, "status": "canceled"}})
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	shortenWaitIntervals(t)

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncCommand(t, store,
		"sync", "task", "submit", "7",
		"--confirm", "--wait", "--close-job=false",
		"--external-ref", "ds:11:22",
	)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("success wait failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if gotBody["external_ref"] != "ds:11:22" {
		t.Fatalf("submit must forward DS external_ref: %#v", gotBody)
	}
	assertSchedulerPlainText(t, stderr)
	if !strings.Contains(stderr, "starting seatunnel source task") {
		t.Fatalf("stderr missing engine log line: %s", stderr)
	}
	if !strings.Contains(stderr, "[stx] job_status=success") {
		t.Fatalf("stderr missing success status: %s", stderr)
	}
	if !strings.Contains(stderr, "STX metrics final") || !strings.Contains(stderr, "read_count:") {
		t.Fatalf("stderr missing final metrics block: %s", stderr)
	}
	if !strings.Contains(stdout, "[stx] done job_id=101 status=success") {
		t.Fatalf("stdout missing human done line: %s", stdout)
	}
	if strings.Contains(stdout, `"api_version"`) {
		t.Fatalf("text wait must not dump full JSON result: %s", stdout)
	}
}

func TestSchedulerE2EWaitFailurePlainTextAndFinalMetrics(t *testing.T) {
	var polls atomic.Int32
	metricsBlock := "========== STX metrics final / 任务终态指标 ==========\n" +
		"time:                  2026-10-17T00:00:01Z\n" +
		"read_count:            4\n" +
		"write_count:           0\n" +
		"==============================================\n"

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/api/v1/capabilities":
			writeSyncCapabilities(t, writer, "sync.task.submit", "R2")
		case request.URL.Path == "/api/v1/sync/tasks/8":
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 8, "mode": "batch"}})
		case request.URL.Path == "/api/v1/sync/tasks/8/submit":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"id": 202, "task_id": 8, "status": "running",
					"attach_generation": 1, "sync_state": "ok",
				},
			})
		case request.URL.Path == "/api/v1/sync/jobs/202":
			n := polls.Add(1)
			status := "running"
			preview := map[string]any{}
			if n >= 2 {
				status = "failed"
				preview["stx_metrics_log"] = metricsBlock
				preview["job_status"] = "FAILED"
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"id": 202, "status": status, "sync_state": "ok",
					"error_message": "job ended with status failed",
					"result_preview": preview,
				},
			})
		case request.URL.Path == "/api/v1/sync/jobs/202/logs":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"logs":        "ArithmeticException: / by zero\n",
					"next_offset": "32",
				},
			})
		case strings.HasSuffix(request.URL.Path, "/cancel"):
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 202, "status": "canceled"}})
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	shortenWaitIntervals(t)

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncCommand(t, store,
		"sync", "task", "submit", "8",
		"--confirm", "--wait", "--close-job=false",
	)
	if exitCode != int(clioutput.ExitExecution) {
		t.Fatalf("failed wait should exit execution: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	assertSchedulerPlainText(t, stderr)
	if !strings.Contains(stderr, "ArithmeticException") {
		t.Fatalf("stderr missing failure log: %s", stderr)
	}
	if !strings.Contains(stderr, "[stx] job_status=failed") {
		t.Fatalf("stderr missing failed status: %s", stderr)
	}
	if !strings.Contains(stderr, "STX metrics final") {
		t.Fatalf("stderr missing final metrics on failure: %s", stderr)
	}
	if !strings.Contains(stdout, "[stx] done job_id=202 status=failed") && !strings.Contains(stderr, "[stx] error:") {
		t.Fatalf("expected done/error human lines: stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestSchedulerE2EEventsJSONKeepsNDJSON(t *testing.T) {
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/api/v1/capabilities":
			writeSyncCapabilities(t, writer, "sync.task.submit", "R2")
		case request.URL.Path == "/api/v1/sync/tasks/9":
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 9, "mode": "batch"}})
		case request.URL.Path == "/api/v1/sync/tasks/9/submit":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"id": 303, "task_id": 9, "status": "running",
					"attach_generation": 1, "sync_state": "ok",
				},
			})
		case request.URL.Path == "/api/v1/sync/jobs/303":
			n := polls.Add(1)
			status := "running"
			if n >= 2 {
				status = "success"
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{"id": 303, "status": status, "sync_state": "ok"},
			})
		case request.URL.Path == "/api/v1/sync/jobs/303/logs":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{"logs": "machine-line\n", "next_offset": "8"},
			})
		case strings.HasSuffix(request.URL.Path, "/cancel"):
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 303, "status": "canceled"}})
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	shortenWaitIntervals(t)

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runSyncCommand(t, store,
		"sync", "task", "submit", "9",
		"--confirm", "--wait", "--close-job=false", "--events-json",
	)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("events-json wait failed: code=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stderr, `"event":"log_line"`) {
		t.Fatalf("--events-json must emit NDJSON log_line: %s", stderr)
	}
	if !strings.Contains(stderr, `"event":"job_status"`) {
		t.Fatalf("--events-json must emit NDJSON job_status: %s", stderr)
	}
}

func shortenWaitIntervals(t *testing.T) {
	t.Helper()
	oldPoll := jobWaitPollInterval
	oldLog := jobLogFollowInterval
	oldMetrics := jobMetricsLogInterval
	jobWaitPollInterval = 5 * time.Millisecond
	jobLogFollowInterval = 1 * time.Millisecond
	jobMetricsLogInterval = 1 * time.Millisecond
	t.Cleanup(func() {
		jobWaitPollInterval = oldPoll
		jobLogFollowInterval = oldLog
		jobMetricsLogInterval = oldMetrics
	})
}

func assertSchedulerPlainText(t *testing.T, stderr string) {
	t.Helper()
	if strings.Contains(stderr, `"event":`) {
		t.Fatalf("DS-facing wait stderr must be plain text, found NDJSON: %s", stderr)
	}
}
