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
	"fmt"
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
	confirmationEvent := events[len(events)-2]
	if confirmationEvent["idempotency_key"] != "upgrade-key" {
		t.Fatalf("确认事件缺少幂等键: %#v", confirmationEvent)
	}
	if confirmationEvent["next_command"] != "stx upgrade plan execute 17 --confirm --idempotency-key upgrade-key --confirmation-id upgrade-confirm-1" {
		t.Fatalf("确认事件缺少重试命令提示: %#v", confirmationEvent)
	}
}

func TestUpgradePlanExecuteConfirmationWithoutIdempotencyKeyFailsLocally(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.plan.execute", "R2")
			return
		}
		t.Fatalf("带确认编号且未复用幂等键时不应发起业务请求: %s %s", request.Method, request.URL.Path)
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "plan", "execute", "17", "--confirm", "--confirmation-id", "upgrade-confirm-1")
	if exitCode != int(clioutput.ExitUsage) {
		t.Fatalf("期望用法错误退出码: code=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "--idempotency-key") {
		t.Fatalf("错误信息应提示复用幂等键: %s", stderr)
	}
	if requests.Load() != 1 {
		t.Fatalf("只应请求一次能力清单: %d", requests.Load())
	}
}

func TestUpgradePrecheckRendersConflictData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.precheck", "R0")
			return
		}
		writer.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"error_msg": "upgrade precheck not ready",
			"data": map[string]any{
				"ready": false,
				"issues": []map[string]any{
					{"code": "package_missing", "message": "target package not found"},
				},
			},
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, _, exitCode := runUpgradeCommand(t, store,
		"upgrade", "precheck", "8", "--target-version", "3.1.0")
	if exitCode != int(clioutput.ExitConflict) {
		t.Fatalf("预检查未就绪应返回冲突退出码: code=%d stdout=%s", exitCode, stdout)
	}
	if !strings.Contains(stdout, `"package_missing"`) || !strings.Contains(stdout, `"ready":false`) {
		t.Fatalf("409 响应详情未输出到 stdout: %s", stdout)
	}
}

func TestUpgradePlanCreateRendersConflictData(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "upgrade-plan.json")
	requestBody := `{"target_version":"3.1.0","config_merge_plan":{"ready":false,"has_conflicts":true,"conflict_count":1,"files":[]}}`
	if err := os.WriteFile(requestFile, []byte(requestBody), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.plan.create", "R0")
			return
		}
		writer.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"error_msg": "config merge plan unresolved",
			"data": map[string]any{
				"precheck": map[string]any{
					"ready":             true,
					"config_merge_plan": map[string]any{"conflict_count": 1},
				},
			},
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, _, exitCode := runUpgradeCommand(t, store, "upgrade", "plan", "create", "8", "--request-file", requestFile)
	if exitCode != int(clioutput.ExitConflict) {
		t.Fatalf("建计划冲突应返回冲突退出码: code=%d stdout=%s", exitCode, stdout)
	}
	if !strings.Contains(stdout, `"config_merge_plan"`) || !strings.Contains(stdout, `"conflict_count":1`) {
		t.Fatalf("409 内嵌预检查详情未输出到 stdout: %s", stdout)
	}
}

func TestUpgradePlanCreateMergeStrategyResolvesPendingConflicts(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "upgrade-plan.json")
	requestBody := `{"target_version":"3.1.0","config_merge_plan":{"ready":false,"has_conflicts":true,"conflict_count":1,"files":[{"config_type":"seatunnel","target_path":"config/seatunnel.yaml","base_content":"a: 1","local_content":"a: 2","target_content":"a: 3","merged_content":"<<<<<<< LOCAL [c1]\na: 2\n||||||| BASE\na: 1\n=======\na: 3\n>>>>>>> TARGET [c1]","conflict_count":1,"resolved":false,"conflicts":[{"id":"c1","config_type":"seatunnel","path":"config/seatunnel.yaml","base_value":"a: 1","local_value":"a: 2","target_value":"a: 3","status":"pending"}]}]}}`
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
		plan, _ := body["config_merge_plan"].(map[string]any)
		if plan["ready"] != true || plan["has_conflicts"] != false || plan["conflict_count"] != float64(0) {
			t.Fatalf("合并计划未被策略解决: %#v", plan)
		}
		files, _ := plan["files"].([]any)
		file, _ := files[0].(map[string]any)
		merged, _ := file["merged_content"].(string)
		if merged != "a: 3" {
			t.Fatalf("use-target 应写入目标版本内容: %q", merged)
		}
		conflict, _ := file["conflicts"].([]any)[0].(map[string]any)
		if conflict["status"] != "resolved" || conflict["resolved_value"] != "a: 3" {
			t.Fatalf("冲突解决状态错误: %#v", conflict)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"plan": map[string]any{"id": 17}}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "plan", "create", "8", "--request-file", requestFile, "--merge-strategy", "use-target")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("合并策略建计划失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestUpgradePlanCreateMergeStrategyRejectsInvalidValue(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "upgrade-plan.json")
	requestBody := `{"target_version":"3.1.0","config_merge_plan":{"ready":false,"has_conflicts":true,"conflict_count":0,"files":[]}}`
	if err := os.WriteFile(requestFile, []byte(requestBody), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newExecutionTestStore(t, "http://127.0.0.1:0", "test-token")
	_, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "plan", "create", "8", "--request-file", requestFile, "--merge-strategy", "bogus")
	if exitCode != int(clioutput.ExitUsage) || !strings.Contains(stderr, "merge") {
		t.Fatalf("非法合并策略应报用法错误: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestUpgradePrecheckWarnsOnPendingMergeConflicts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUpgradeCapabilities(t, writer, "stupgrade.precheck", "R0")
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"ready":             true,
			"issues":            []any{},
			"config_merge_plan": map[string]any{"ready": false, "has_conflicts": true, "conflict_count": 3},
		}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runUpgradeCommand(t, store,
		"upgrade", "precheck", "8", "--target-version", "3.1.0")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("预检查应成功: code=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, `"ready":true`) {
		t.Fatalf("预检查结果缺失: %s", stdout)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 1 || events[0]["event"] != "warning" || !strings.Contains(fmt.Sprint(events[0]["message"]), "3") {
		t.Fatalf("应输出待决冲突告警: %#v", events)
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
