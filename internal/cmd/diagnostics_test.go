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
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestDiagnosticsTaskCreateDefaultsToReadyWithoutAutoStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapabilities(t, writer)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/diagnostics/tasks" {
			t.Fatalf("诊断任务创建请求错误 / diagnostics task create request is incorrect: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Idempotency-Key") != "diagnostic-create-key" || request.Header.Get("X-STX-Confirm") != "true" {
			t.Fatalf("诊断任务安全请求头错误 / diagnostics safety headers are incorrect: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取诊断任务正文失败 / decoding diagnostics request failed: %v", err)
		}
		if body["cluster_id"] != float64(6) || body["auto_start"] != false || body["summary"] != "recent errors" {
			t.Fatalf("诊断任务正文错误 / diagnostics request body is incorrect: %#v", body)
		}
		options, ok := body["options"].(map[string]any)
		if !ok || options["include_thread_dump"] != true {
			t.Fatalf("诊断选项错误 / diagnostics options are incorrect: %#v", body["options"])
		}
		writeDiagnosticsTaskResponse(t, writer, map[string]any{
			"id":           42,
			"status":       "ready",
			"execution_id": "exec-diagnostic-42",
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runDiagnosticsCommand(t, store,
		"diagnostics", "task", "create", "--cluster-id", "6", "--summary", "recent errors",
		"--include-thread-dump", "--confirm", "--idempotency-key", "diagnostic-create-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("创建诊断任务失败 / creating diagnostics task failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stderr, `"event":"warning"`) {
		t.Fatalf("写操作应在 stderr 输出影响提示 / write operation must emit an impact event to stderr: %s", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout 不是合法 JSON / stdout is not valid JSON: %v\n%s", err, stdout)
	}
	meta, ok := result["result_meta"].(map[string]any)
	if !ok || meta["next_command"] != "stx diagnostics task get 42" {
		t.Fatalf("缺少诊断任务后续命令 / diagnostics next command is missing: %#v", result["result_meta"])
	}
}

func TestDiagnosticsTaskCreateAcceptsRequestFileAndJVMOptions(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "diagnostics-task.json")
	requestBody := `{"cluster_id":6,"trigger_source":"error_group","source_ref":{"error_group_id":12},"node_scope":"related","options":{"include_jvm_dump":true,"jvm_dump_min_free_mb":4096},"auto_start":true}`
	if err := os.WriteFile(requestFile, []byte(requestBody), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapabilities(t, writer)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取 request-file 正文失败 / decoding request-file body failed: %v", err)
		}
		if body["trigger_source"] != "error_group" || body["auto_start"] != true {
			t.Fatalf("request-file 顶层字段错误 / request-file top-level fields are incorrect: %#v", body)
		}
		options, ok := body["options"].(map[string]any)
		if !ok || options["include_jvm_dump"] != true || options["jvm_dump_min_free_mb"] != float64(4096) {
			t.Fatalf("request-file JVM 选项错误 / request-file JVM options are incorrect: %#v", body["options"])
		}
		writeDiagnosticsTaskResponse(t, writer, map[string]any{"id": 43, "status": "running"})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, _, exitCode := runDiagnosticsCommand(t, store, "diagnostics", "task", "create", "--request-file", requestFile, "--confirm")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("request-file 创建诊断任务失败 / request-file diagnostics create failed: code=%d", exitCode)
	}
}

func TestDiagnosticsTaskCreateRequiresConfirmationBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDiagnosticsCommand(t, store, "diagnostics", "task", "create", "--cluster-id", "6")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时不应访问服务 / service must not be called without confirmation: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runDiagnosticsCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addDiagnosticsWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeDiagnosticsCapabilities(t *testing.T, writer http.ResponseWriter) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": "diagnostics.task.create", "revision": 1, "allowed": true, "mode": "normal", "risk": "R0"}},
	}}); err != nil {
		t.Fatalf("写入诊断能力响应失败 / writing diagnostics capabilities failed: %v", err)
	}
}

func writeDiagnosticsTaskResponse(t *testing.T, writer http.ResponseWriter, data map[string]any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": data}); err != nil {
		t.Fatalf("写入诊断任务响应失败 / writing diagnostics task response failed: %v", err)
	}
}
