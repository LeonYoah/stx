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

func TestDiagnosticsResourceRunSendsSingleResourceRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapability(t, writer, "diagnostics.resource.run", "normal")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/diagnostics/resources/thread_dump/run" {
			t.Fatalf("单项诊断资源请求错误 / single-resource request is incorrect: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Idempotency-Key") != "resource-run-key" || request.Header.Get("X-STX-Confirm") != "true" {
			t.Fatalf("单项诊断资源安全请求头错误 / single-resource safety headers are incorrect: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取单项诊断资源正文失败 / decoding single-resource request failed: %v", err)
		}
		if body["cluster_id"] != float64(6) || body["lookback_minutes"] != float64(30) {
			t.Fatalf("单项诊断资源正文错误 / single-resource request body is incorrect: %#v", body)
		}
		nodes, ok := body["selected_node_ids"].([]any)
		if !ok || len(nodes) != 1 || nodes[0] != float64(9) {
			t.Fatalf("单项诊断资源节点错误 / single-resource node selection is incorrect: %#v", body["selected_node_ids"])
		}
		writeDiagnosticsTaskResponse(t, writer, map[string]any{
			"id":           43,
			"status":       "running",
			"execution_id": "exec-resource-43",
		})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runDiagnosticsCommand(t, store,
		"diagnostics", "resource", "run", "thread_dump", "--cluster-id", "6", "--node-id", "9",
		"--lookback-minutes", "30", "--confirm", "--idempotency-key", "resource-run-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("执行单项诊断资源失败 / running single diagnostics resource failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("单项诊断资源 stdout 不是合法 JSON / single-resource stdout is not valid JSON: %v", err)
	}
	meta, ok := result["result_meta"].(map[string]any)
	if !ok || meta["next_command"] != "stx execution wait exec-resource-43" {
		t.Fatalf("单项诊断资源后续命令错误 / single-resource next command is incorrect: %#v", result["result_meta"])
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

func TestTroubleshootingMemoryCreateSendsBodyAndSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapability(t, writer, "diagnostics.troubleshooting-memory.create", "normal")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/diagnostics/troubleshooting-memories" {
			t.Fatalf("排障经验创建请求错误 / troubleshooting memory create request is incorrect: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Idempotency-Key") != "memory-create-key" || request.Header.Get("X-STX-Confirm") != "true" {
			t.Fatalf("排障经验创建安全请求头错误 / troubleshooting memory create safety headers are incorrect: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取排障经验创建正文失败 / decoding troubleshooting memory create body failed: %v", err)
		}
		if body["target_type"] != "error" || body["fingerprint"] != "network-timeout" || body["title"] != "Network timeout" || body["solution"] != "Check connectivity" {
			t.Fatalf("排障经验创建正文错误 / troubleshooting memory create body is incorrect: %#v", body)
		}
		writeDiagnosticsTaskResponse(t, writer, map[string]any{"id": "91", "title": "Network timeout"})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runDiagnosticsCommand(t, store,
		"diagnostics", "troubleshooting-memory", "create",
		"--target-type", "error", "--fingerprint", "network-timeout", "--title", "Network timeout",
		"--solution", "Check connectivity", "--confirm", "--idempotency-key", "memory-create-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("创建排障经验失败 / creating troubleshooting memory failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("排障经验创建输出不是合法 JSON / troubleshooting memory create output is not valid JSON: %v", err)
	}
	meta := result["result_meta"].(map[string]any)
	if meta["next_command"] != "stx diagnostics troubleshooting-memory get 91" {
		t.Fatalf("排障经验创建后续命令错误 / troubleshooting memory create next command is incorrect: %#v", meta)
	}
}

func TestTroubleshootingMemoryUpdateAcceptsRequestFile(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "memory-update.json")
	if err := os.WriteFile(requestFile, []byte(`{"solution":"Updated steps","tags":["network","timeout"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapability(t, writer, "diagnostics.troubleshooting-memory.update", "normal")
			return
		}
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/diagnostics/troubleshooting-memories/91" {
			t.Fatalf("排障经验修改请求错误 / troubleshooting memory update request is incorrect: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取排障经验修改正文失败 / decoding troubleshooting memory update body failed: %v", err)
		}
		if body["solution"] != "Updated steps" {
			t.Fatalf("排障经验修改正文错误 / troubleshooting memory update body is incorrect: %#v", body)
		}
		writeDiagnosticsTaskResponse(t, writer, map[string]any{"id": "91", "solution": "Updated steps"})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDiagnosticsCommand(t, store,
		"diagnostics", "troubleshooting-memory", "update", "91", "--request-file", requestFile, "--confirm", "--idempotency-key", "memory-update-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("修改排障经验失败 / updating troubleshooting memory failed: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestTroubleshootingMemoryWriteRequiresConfirmationBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDiagnosticsCommand(t, store,
		"diagnostics", "troubleshooting-memory", "create",
		"--target-type", "error", "--fingerprint", "test", "--title", "test", "--solution", "test")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时不应访问服务 / service must not be called without confirmation: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func TestDiagnosticsTaskDownloadsBundleAndReturnsChecksum(t *testing.T) {
	const content = "diagnostic zip content"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapability(t, writer, "diagnostics.task.bundle.download", "download")
			return
		}
		if request.URL.Path != "/api/v1/diagnostics/tasks/4/bundle" {
			t.Fatalf("诊断包下载路径错误 / diagnostics bundle download path is incorrect: %s", request.URL.Path)
		}
		_, _ = writer.Write([]byte(content))
	}))
	defer server.Close()

	outputFile := filepath.Join(t.TempDir(), "diagnostics-4.zip")
	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runDiagnosticsCommand(t, store, "diagnostics", "task", "bundle", "4", "--file", outputFile)
	if exitCode != int(clioutput.ExitSuccess) || strings.TrimSpace(stderr) != "" {
		t.Fatalf("下载诊断包失败 / downloading diagnostics bundle failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if contentBytes, err := os.ReadFile(outputFile); err != nil || string(contentBytes) != content {
		t.Fatalf("诊断包内容错误 / diagnostics bundle content is incorrect: content=%q err=%v", contentBytes, err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("诊断下载结果不是合法 JSON / diagnostics download result is not valid JSON: %v", err)
	}
	data, ok := result["data"].(map[string]any)
	if !ok || data["file"] != outputFile || data["size"] != float64(len(content)) || strings.TrimSpace(data["sha256"].(string)) == "" {
		t.Fatalf("诊断下载结果错误 / diagnostics download result is incorrect: %#v", result["data"])
	}
}

func TestDiagnosticsTaskFilePreservesArtifactPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapability(t, writer, "diagnostics.task.file.download", "download")
			return
		}
		if request.URL.EscapedPath() != "/api/v1/diagnostics/tasks/4/files/logs/node%201.log" {
			t.Fatalf("诊断文件路径错误 / diagnostics file path is incorrect: path=%s escaped=%s", request.URL.Path, request.URL.EscapedPath())
		}
		_, _ = writer.Write([]byte("node log"))
	}))
	defer server.Close()

	outputFile := filepath.Join(t.TempDir(), "node.log")
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDiagnosticsCommand(t, store, "diagnostics", "task", "file", "4", "logs/node 1.log", "--file", outputFile)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("下载诊断文件失败 / downloading diagnostics file failed: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestDiagnosticsTaskDownloadRejectsExistingTargetBeforeDownload(t *testing.T) {
	var downloadCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDiagnosticsCapability(t, writer, "diagnostics.task.html.download", "download")
			return
		}
		downloadCalls++
	}))
	defer server.Close()

	outputFile := filepath.Join(t.TempDir(), "diagnostics.html")
	if err := os.WriteFile(outputFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDiagnosticsCommand(t, store, "diagnostics", "task", "html", "4", "--file", outputFile)
	if exitCode != int(clioutput.ExitConflict) || downloadCalls != 0 {
		t.Fatalf("已存在文件不应被覆盖 / existing file must not be overwritten: code=%d calls=%d stderr=%s", exitCode, downloadCalls, stderr)
	}
}

func TestDiagnosticsTaskFileRejectsParentDirectoryPathBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { calls++ }))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDiagnosticsCommand(t, store, "diagnostics", "task", "file", "4", "../secret.txt")
	if exitCode != int(clioutput.ExitUsage) || calls != 0 {
		t.Fatalf("目录回退路径不应访问服务 / parent-directory path must not call the service: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
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
	writeDiagnosticsCapability(t, writer, "diagnostics.task.create", "normal")
}

func writeDiagnosticsCapability(t *testing.T, writer http.ResponseWriter, operationID, mode string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": mode, "risk": "R0"}},
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
