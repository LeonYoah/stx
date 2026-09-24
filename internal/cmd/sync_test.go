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
	"strings"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestSyncTaskCreateRequiresConfirmBeforeNetworkRequest(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runSyncCommand(t, store,
		"sync", "task", "create", "--name", "my-task", "--cluster-id", "1", "--mode", "STREAMING")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍发起请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func TestSyncTaskCreateSendsBodyAndSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeSyncCapabilities(t, writer, "sync.task.create", "R1")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/sync/tasks" {
			t.Fatalf("创建同步任务请求错误: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "sync-create-key" {
			t.Fatalf("安全请求头错误: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取请求体失败: %v", err)
		}
		if body["name"] != "cdc-task" || body["mode"] != "STREAMING" || body["cluster_id"] != float64(1) {
			t.Fatalf("创建任务请求体错误: %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 42, "name": "cdc-task"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncCommand(t, store,
		"sync", "task", "create",
		"--name", "cdc-task",
		"--cluster-id", "1",
		"--mode", "STREAMING",
		"--confirm",
		"--idempotency-key", "sync-create-key",
	)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("创建任务命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestSyncTaskPermissionsGetReturnsPermissionFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeSyncCapabilities(t, writer, "sync.task.permissions.get", "R0")
			return
		}
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/sync/tasks/42/permissions" {
			t.Fatalf("读取任务权限请求错误: %s %s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"task_id":          42,
			"collaborator_ids": []int{2, 3},
			"can_edit":         true,
			"can_manage":       true,
			"is_owner":         true,
			"is_collaborator":  false,
			"is_public":        false,
		}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncCommand(t, store, "sync", "task", "permissions", "get", "42")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("读取任务权限失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	var result struct {
		Data struct {
			TaskID          float64 `json:"task_id"`
			IsPublic        bool    `json:"is_public"`
			CollaboratorIDs []any   `json:"collaborator_ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("解析权限结果失败: %v; stdout=%s", err, stdout)
	}
	if result.Data.TaskID != 42 || result.Data.IsPublic || len(result.Data.CollaboratorIDs) != 2 {
		t.Fatalf("权限结果错误: %#v", result.Data)
	}
}

func TestSyncTaskPermissionsUpdateSendsOnlyPermissionChanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/api/v1/capabilities":
			writeSyncCapabilities(t, writer, "sync.task.permissions.update", "R1")
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/sync/tasks/42/permissions":
			if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "permissions-key" {
				t.Fatalf("权限更新安全请求头错误: %#v", request.Header)
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("解析权限更新请求体失败: %v", err)
			}
			if body["is_public"] != false {
				t.Fatalf("公开状态未更新: %#v", body)
			}
			collaborators, ok := body["collaborator_ids"].([]any)
			if !ok || len(collaborators) != 2 || collaborators[0] != float64(8) || collaborators[1] != float64(9) {
				t.Fatalf("协作者未更新: %#v", body)
			}
			if len(body) != 2 {
				t.Fatalf("权限更新请求不应携带任务正文: %#v", body)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 42, "is_public": false}})
		default:
			t.Fatalf("意外的请求: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runSyncCommand(t, store,
		"sync", "task", "permissions", "update", "42",
		"--private", "--collaborator-id", "8", "--collaborator-id", "9",
		"--confirm", "--idempotency-key", "permissions-key",
	)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("更新任务权限失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestSyncTaskSubmitSendsWaitAndPollsExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/api/v1/capabilities":
			writeSyncCapabilities(t, writer, "sync.task.submit", "R2")
		case request.URL.Path == "/api/v1/sync/tasks/42/submit":
			if request.Method != http.MethodPost {
				t.Fatalf("提交方法错误: %s", request.Method)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"job_id":       99,
					"execution_id": "exec-sync-42",
					"status":       "submitted",
				},
			})
		case request.URL.Path == "/api/v1/executions/exec-sync-42/wait":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": map[string]any{
					"execution": map[string]any{
						"execution_id": "exec-sync-42",
						"status":       "succeeded",
						"message":      "Execution succeeded",
					},
					"terminal": true,
				},
			})
		default:
			t.Fatalf("意外的请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncCommand(t, store,
		"sync", "task", "submit", "42",
		"--confirm",
		"--wait",
	)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("提交并等待任务失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestSyncJobCancelWithSavepointSendsFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			writeSyncCapabilities(t, writer, "sync.job.cancel", "R2")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/sync/jobs/99/cancel" {
			t.Fatalf("取消作业请求错误: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("解析取消请求体失败: %v", err)
		}
		if body["stop_with_savepoint"] != true {
			t.Fatalf("未携带 stop_with_savepoint=true 标记: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"job_id": 99,
				"status": "cancelling",
			},
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runSyncCommand(t, store,
		"sync", "job", "cancel", "99",
		"--savepoint",
		"--confirm",
	)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("取消作业命令失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestSyncJobRecoverSendsRequestAndNextCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			writeSyncCapabilities(t, writer, "sync.job.recover", "R2")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/sync/jobs/99/recover" {
			t.Fatalf("恢复作业请求错误: %s %s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"id":           100,
				"execution_id": "",
				"status":       "running",
			},
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runSyncCommand(t, store,
		"sync", "job", "recover", "99",
		"--confirm",
	)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("恢复作业命令失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestSyncCuratedListCommand(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/sync/curated-templates/list" {
			t.Fatalf("list request mismatch: %s %s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"name": "FakeSource 冒烟", "section": "source", "origin": "builtin"},
				},
				"total": 1,
			},
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncCommand(t, store, "sync", "curated", "list", "--section", "source")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("curated list failed: code=%d stderr=%s", exitCode, stderr)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if !strings.Contains(stdout, "FakeSource") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestSyncCuratedCreateRequiresConfirm(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runSyncCommand(t, store,
		"sync", "curated", "create",
		"--name", "my-env",
		"--section", "env",
		"--content", `env { job.mode = "BATCH" }`,
	)
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("expected confirm gate before network: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runSyncCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addSyncWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeSyncCapabilities(t *testing.T, writer http.ResponseWriter, operationID, risk string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{
			{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": risk},
			{"operation_id": "execution.get", "revision": 1, "allowed": true, "mode": "normal", "risk": "R0"},
		},
	}}); err != nil {
		t.Fatalf("写入能力响应失败: %v", err)
	}
}
