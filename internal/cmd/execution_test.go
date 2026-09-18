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
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestExecutionGetWritesJSONAndNextCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/executions/exec-1" {
			t.Fatalf("查询路径错误: %s", request.URL.Path)
		}
		writeExecutionResponse(t, writer, map[string]any{
			"execution_id": "exec-1",
			"status":       "running",
			"progress":     35,
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runExecutionCommand(t, store, "execution", "get", "exec-1")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("查询执行记录失败: code=%d stderr=%s", exitCode, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("成功查询不应写 stderr: %s", stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout 不是合法 JSON: %v\n%s", err, stdout)
	}
	if result["operation_id"] != "execution.get" {
		t.Fatalf("operation_id 错误: %#v", result)
	}
	meta, ok := result["result_meta"].(map[string]any)
	if !ok || meta["next_command"] != "stx execution wait exec-1" {
		t.Fatalf("非终态缺少 next_command: %#v", result["result_meta"])
	}
}

func TestExecutionWaitWritesProgressToStderrAndResultToStdout(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/executions/exec-2/wait" {
			t.Fatalf("等待路径错误: %s", request.URL.Path)
		}
		status := "running"
		progress := 45
		if calls.Add(1) >= 2 {
			status = "succeeded"
			progress = 100
		}
		writeExecutionWaitResponse(t, writer, map[string]any{
			"execution_id": "exec-2",
			"status":       status,
			"progress":     progress,
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runExecutionCommand(t, store, "execution", "wait", "exec-2", "--timeout", "2s")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("等待执行完成失败: code=%d stderr=%s", exitCode, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("最终 stdout 不是合法 JSON: %v\n%s", err, stdout)
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["status"] != "succeeded" {
		t.Fatalf("最终状态错误: %#v", result["data"])
	}

	events := decodeNDJSON(t, stderr)
	if len(events) != 2 {
		t.Fatalf("进度事件数量错误: %#v", events)
	}
	if events[0]["event"] != "execution_progress" || events[0]["message"] != "running" {
		t.Fatalf("首个进度事件错误: %#v", events[0])
	}
	if events[1]["message"] != "succeeded" {
		t.Fatalf("完成进度事件错误: %#v", events[1])
	}
}

func TestExecutionWaitFailedStatusReturnsExecutionExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeExecutionWaitResponse(t, writer, map[string]any{
			"execution_id":  "exec-failed",
			"status":        "failed",
			"progress":      80,
			"error_message": "remote execution failed",
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runExecutionCommand(t, store, "execution", "wait", "exec-failed", "--timeout", "2s")
	if exitCode != int(clioutput.ExitExecution) {
		t.Fatalf("失败终态退出码错误: code=%d stderr=%s", exitCode, stderr)
	}
	if !strings.Contains(stdout, `"status":"failed"`) {
		t.Fatalf("失败终态仍应写最终结果: %s", stdout)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 2 || events[1]["code"] != clioutput.CodeExecution {
		t.Fatalf("失败终态 stderr 事件错误: %#v", events)
	}
}

func TestExecutionCancelConfirmSendsSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/executions/exec-3/cancel" {
			t.Fatalf("取消请求错误: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" {
			t.Fatalf("缺少确认请求头: %q", request.Header.Get("X-STX-Confirm"))
		}
		if request.Header.Get("Idempotency-Key") == "" {
			t.Fatal("缺少幂等键")
		}
		writeExecutionResponse(t, writer, map[string]any{
			"execution_id": "exec-3",
			"status":       "cancel_requested",
			"progress":     20,
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runExecutionCommand(t, store, "execution", "cancel", "exec-3", "--confirm")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("取消请求失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestExecutionCommandWithoutTokenReturnsAuthenticationExitCode(t *testing.T) {
	store := newExecutionTestStore(t, "http://127.0.0.1:1", "")
	stdout, stderr, exitCode := runExecutionCommand(t, store, "execution", "get", "exec-4")
	if exitCode != int(clioutput.ExitAuthentication) {
		t.Fatalf("无令牌退出码错误: code=%d stderr=%s", exitCode, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("认证失败不应写 stdout: %s", stdout)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 1 || events[0]["code"] != clioutput.CodeAuthentication {
		t.Fatalf("认证错误事件错误: %#v", events)
	}
}

func TestExecutionGetMapsHTTPStatuses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		exitCode clioutput.ExitCode
		code     string
	}{
		{name: "forbidden", status: http.StatusForbidden, exitCode: clioutput.ExitPermission, code: clioutput.CodePermission},
		{name: "not found", status: http.StatusNotFound, exitCode: clioutput.ExitNotFound, code: clioutput.CodeNotFound},
		{name: "conflict", status: http.StatusConflict, exitCode: clioutput.ExitConflict, code: clioutput.CodeConflict},
		{name: "precondition required", status: http.StatusPreconditionRequired, exitCode: clioutput.ExitConflict, code: clioutput.CodeConflict},
		{name: "gateway timeout", status: http.StatusGatewayTimeout, exitCode: clioutput.ExitTimeout, code: clioutput.CodeTimeout},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				if err := json.NewEncoder(writer).Encode(map[string]any{"error_msg": test.name}); err != nil {
					t.Fatalf("写入错误响应失败: %v", err)
				}
			}))
			defer server.Close()

			store := newExecutionTestStore(t, server.URL, "test-token")
			_, stderr, exitCode := runExecutionCommand(t, store, "execution", "get", "exec-http")
			if exitCode != int(test.exitCode) {
				t.Fatalf("HTTP %d 退出码错误: code=%d stderr=%s", test.status, exitCode, stderr)
			}
			events := decodeNDJSON(t, stderr)
			if len(events) != 1 || events[0]["code"] != test.code {
				t.Fatalf("HTTP %d 错误分类错误: %#v", test.status, events)
			}
		})
	}
}

func TestExecutionWaitMapsLocalTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runExecutionCommand(t, store, "execution", "wait", "exec-timeout", "--timeout", "100ms")
	if exitCode != int(clioutput.ExitTimeout) {
		t.Fatalf("本地等待超时退出码错误: code=%d stderr=%s", exitCode, stderr)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 1 || events[0]["code"] != clioutput.CodeTimeout || events[0]["retryable"] != true {
		t.Fatalf("本地等待超时事件错误: %#v", events)
	}
}

func newExecutionTestStore(t *testing.T, serverURL, token string) *cliConfig.Store {
	t.Helper()
	store := cliConfig.NewStore(t.TempDir() + "/config.yaml")
	if err := store.Upsert("default", cliConfig.Namespace{
		Server:  serverURL,
		Token:   token,
		Timeout: "2s",
	}, true); err != nil {
		t.Fatalf("创建测试命名空间失败: %v", err)
	}
	return store
}

func runExecutionCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newExecutionCommandWithStore(func() (*cliConfig.Store, error) { return store, nil }))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeExecutionResponse(t *testing.T, writer http.ResponseWriter, execution map[string]any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": execution}); err != nil {
		t.Fatalf("写入执行响应失败: %v", err)
	}
}

func writeExecutionWaitResponse(t *testing.T, writer http.ResponseWriter, execution map[string]any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"execution":      execution,
		"wait_timed_out": false,
	}}); err != nil {
		t.Fatalf("写入等待响应失败: %v", err)
	}
}

func decodeNDJSON(t *testing.T, content string) []map[string]any {
	t.Helper()
	result := make([]map[string]any, 0)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		var event map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("stderr 不是合法 NDJSON: %v\n%s", err, content)
		}
		result = append(result, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("读取 stderr 失败: %v", err)
	}
	return result
}
