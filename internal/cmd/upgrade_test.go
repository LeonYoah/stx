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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestUpgradePrecheckSendsRequestAndNextCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.precheck", "R0")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/st-upgrade/precheck" {
			t.Fatalf("升级预检查请求错误: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["cluster_id"] != float64(8) || body["target_version"] != "2.3.13" || body["target_install_dir"] != "/tmp/seatunnel-2.3.13-new" {
			t.Fatalf("升级预检查正文错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"ready": true}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "precheck", "8", "--target-version", "2.3.13", "--target-install-dir", "/tmp/seatunnel-2.3.13-new")
	if exitCode != int(clioutput.ExitSuccess) || strings.TrimSpace(stderr) != "" {
		t.Fatalf("升级预检查失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"next_command":"stx upgrade plan create 8 --request-file resolved-upgrade-plan.json"`) {
		t.Fatalf("升级预检查缺少下一条命令: %s", stdout)
	}
}

func TestUpgradePlanCreateAcceptsCompleteRequestFile(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "upgrade-plan.json")
	requestBody := `{"target_version":"2.3.13","target_install_dir":"/tmp/seatunnel-2.3.13-new","config_merge_plan":{"ready":true,"has_conflicts":false,"conflict_count":0,"files":[]}}`
	if err := os.WriteFile(requestFile, []byte(requestBody), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.plan.create", "R0")
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if request.URL.Path != "/api/v1/st-upgrade/plan" || body["cluster_id"] != float64(8) || body["config_merge_plan"] == nil {
			t.Fatalf("升级计划请求错误: path=%s body=%#v", request.URL.Path, body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"plan": map[string]any{"id": 17, "status": "ready"}}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runUpgradeCommand(t, store, "upgrade", "plan", "create", "8", "--request-file", requestFile)
	if exitCode != int(clioutput.ExitSuccess) || strings.TrimSpace(stderr) != "" {
		t.Fatalf("创建升级计划失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"next_command":"stx upgrade plan execute 17 --confirm"`) {
		t.Fatalf("升级计划缺少下一条命令: %s", stdout)
	}
}

func TestUpgradePlanExecuteSendsSafetyHeadersAndNextCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.plan.execute", "R2")
			return
		}
		if request.URL.Path != "/api/v1/st-upgrade/execute" || request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "upgrade-key" {
			t.Fatalf("升级执行请求错误: path=%s headers=%#v", request.URL.Path, request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["plan_id"] != float64(17) {
			t.Fatalf("升级执行正文错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 23, "status": "pending"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "plan", "execute", "17", "--confirm", "--idempotency-key", "upgrade-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("执行升级计划失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"next_command":"stx upgrade task wait 23"`) {
		t.Fatalf("升级执行结果缺少等待命令: %s", stdout)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 1 || events[0]["event"] != "warning" {
		t.Fatalf("升级影响提示错误: %#v", events)
	}
}

func TestUpgradePlanExecuteRecognizesLegacyConfirmationResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.plan.execute", "R2")
			return
		}
		writer.WriteHeader(http.StatusPreconditionRequired)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"error_msg": "operation confirmation is required",
			"data": map[string]any{
				"confirmation_id": "upgrade-confirm-1", "risk_level": "R2",
				"impact": "upgrade impact", "expires_at": "2026-09-19T22:00:00+08:00",
			},
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "plan", "execute", "17", "--confirm", "--idempotency-key", "upgrade-key")
	if exitCode != int(clioutput.ExitConflict) {
		t.Fatalf("首次升级执行应返回确认错误: code=%d stderr=%s", exitCode, stderr)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) < 2 || events[len(events)-2]["event"] != "confirmation_required" || events[len(events)-2]["confirmation_id"] != "upgrade-confirm-1" {
		t.Fatalf("缺少兼容确认事件: %#v", events)
	}
}

func TestUpgradeTaskWaitPollsUntilSucceeded(t *testing.T) {
	var taskCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.task.get", "R0")
			return
		}
		if request.URL.Path != "/api/v1/st-upgrade/tasks/23" {
			t.Fatalf("升级任务查询路径错误: %s", request.URL.Path)
		}
		status, step := "running", "DISTRIBUTE_PACKAGE"
		if taskCalls.Add(1) > 1 {
			status, step = "succeeded", "COMPLETE"
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"id": 23, "execution_id": "exec-upgrade-23", "status": status, "current_step": step,
		}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "task", "wait", "23", "--timeout", "1s", "--interval", "1ms")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("等待升级任务失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"status":"succeeded"`) || taskCalls.Load() != 2 {
		t.Fatalf("升级等待结果错误: calls=%d stdout=%s", taskCalls.Load(), stdout)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 2 || events[0]["event"] != "upgrade_progress" || events[1]["message"] != "succeeded:COMPLETE" {
		t.Fatalf("升级进度事件错误: %#v", events)
	}
}

func runUpgradeCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addUpgradeCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeUpgradeCapabilities(t *testing.T, writer http.ResponseWriter, operationID, risk string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": risk}},
	}}); err != nil {
		t.Fatalf("写入升级能力响应失败: %v", err)
	}
}
