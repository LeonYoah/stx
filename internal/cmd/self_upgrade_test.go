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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSelfUpgradeHelp 验证 self-upgrade 帮助命令输出结构。
// TestSelfUpgradeHelp verifies that self-upgrade help output renders expected command structure.
func TestSelfUpgradeHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{"self-upgrade", "--help"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("stx self-upgrade --help exit = %d, stderr=%s", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "self-upgrade") || !strings.Contains(out, "status") {
		t.Fatalf("help text missing self-upgrade or status: %s", out)
	}
}

// TestSelfUpgradeStatusAndCheckCLI 验证 status 与 check 子命令的 CLI 调用与输出格式。
// TestSelfUpgradeStatusAndCheckCLI verifies the CLI execution and output formatting for status and check subcommands.
func TestSelfUpgradeStatusAndCheckCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/self-upgrade/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error_code": "",
				"error_msg":  "",
				"data": map[string]any{
					"current_version": "1.0.0",
					"deployment_type": "host",
					"architecture":    "linux/arm64",
					"overall_status":  "idle",
				},
			})
		case "/api/v1/self-upgrade/check":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error_code": "",
				"error_msg":  "",
				"data": map[string]any{
					"current_version":  "1.0.0",
					"latest_version":   "1.1.0",
					"update_available": true,
				},
			})
		case "/api/v1/self-upgrade/history":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error_code": "",
				"error_msg":  "",
				"data": []map[string]any{
					{
						"id":             "task-123",
						"from_version":   "1.0.0",
						"target_version": "1.1.0",
						"status":         "succeeded",
						"duration_ms":    1500,
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// 1. 测试 self-upgrade status / Test self-upgrade status
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{"self-upgrade", "status", "--server", server.URL}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("stx self-upgrade status exit = %d, stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "self_upgrade.status") {
		t.Fatalf("expected operation self_upgrade.status in %s", stdout.String())
	}

	// 2. 测试 self-upgrade check / Test self-upgrade check
	stdout.Reset()
	stderr.Reset()
	exitCode = executeCommand(NewRootCommand(), []string{"self-upgrade", "check", "--server", server.URL}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("stx self-upgrade check exit = %d, stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "self_upgrade.check") {
		t.Fatalf("expected operation self_upgrade.check in %s", stdout.String())
	}

	// 3. 测试 self-upgrade history / Test self-upgrade history
	stdout.Reset()
	stderr.Reset()
	exitCode = executeCommand(NewRootCommand(), []string{"self-upgrade", "history", "--server", server.URL}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("stx self-upgrade history exit = %d, stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "self_upgrade.history") {
		t.Fatalf("expected operation self_upgrade.history in %s", stdout.String())
	}
}

// TestSelfUpgradeRejectsInvalidVersionFlags 验证 CLI 在请求前拒绝路径穿越等非法版本号。
// TestSelfUpgradeRejectsInvalidVersionFlags verifies that the CLI rejects invalid versions such as path traversal before any request.
func TestSelfUpgradeRejectsInvalidVersionFlags(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.NotFound(w, r)
	}))
	defer server.Close()

	cases := [][]string{
		{"self-upgrade", "stage", "--target-version", "../../etc", "--server", server.URL},
		{"self-upgrade", "stage", "--target-version", "/tmp/evil", "--server", server.URL},
		{"self-upgrade", "stage", "--target-version", `..\evil`, "--server", server.URL},
		{"self-upgrade", "stage", "--target-version", "latest", "--server", server.URL},
		{"self-upgrade", "check", "--force-latest", "v1.2.3/../../x", "--server", server.URL},
	}
	for _, args := range cases {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := executeCommand(NewRootCommand(), args, stdout, stderr)
		if exitCode == 0 {
			t.Fatalf("%v exit = 0, want usage error; stdout=%s", args, stdout.String())
		}
		if !strings.Contains(stderr.String()+stdout.String(), "invalid --") {
			t.Fatalf("%v missing validation message: stdout=%s stderr=%s", args, stdout.String(), stderr.String())
		}
	}
	if called {
		t.Fatalf("server must not be called for invalid versions")
	}
}

// TestSelfUpgradeExecuteSendsExplicitMode 验证 execute 默认发送 mode=apply，--dry-run 发送 mode=dry_run，且帮助说明默认行为。
// TestSelfUpgradeExecuteSendsExplicitMode verifies execute sends mode=apply by default, mode=dry_run with --dry-run,
// and that the help text documents the default.
func TestSelfUpgradeExecuteSendsExplicitMode(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/self-upgrade/execute" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "t1", "execution_mode": body["mode"]}})
	}))
	defer server.Close()

	for _, args := range [][]string{
		{"self-upgrade", "execute", "--server", server.URL},
		{"self-upgrade", "execute", "--dry-run", "--server", server.URL},
	} {
		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		if code := executeCommand(NewRootCommand(), args, stdout, stderr); code != 0 {
			t.Fatalf("%v exit = %d stderr=%s", args, code, stderr.String())
		}
	}
	if len(bodies) != 2 || bodies[0]["mode"] != "apply" || bodies[1]["mode"] != "dry_run" {
		t.Fatalf("request bodies = %+v", bodies)
	}
	if _, legacy := bodies[0]["apply"]; legacy {
		t.Fatalf("execute should send the explicit mode field, got %+v", bodies[0])
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := executeCommand(NewRootCommand(), []string{"self-upgrade", "execute", "--help"}, stdout, stderr); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	help := stdout.String()
	for _, want := range []string{"--dry-run", "APPLIES the upgrade for real", "mode=apply", "SeaTunnel"} {
		if !strings.Contains(help, want) {
			t.Fatalf("execute help missing %q:\n%s", want, help)
		}
	}
}

// TestSelfUpgradeWatchdogCommandIsHidden 验证重启 watchdog 子命令已注册但隐藏。
// TestSelfUpgradeWatchdogCommandIsHidden verifies the restart watchdog sub-command is registered but hidden.
func TestSelfUpgradeWatchdogCommandIsHidden(t *testing.T) {
	for _, c := range NewRootCommand().Commands() {
		if c.Name() == "__self-upgrade-restart" {
			if !c.Hidden {
				t.Fatal("watchdog command must be hidden")
			}
			return
		}
	}
	t.Fatal("watchdog command not registered")
}

// writeSelfUpgradeCapabilities 写出包含指定操作的 capabilities 响应。
// writeSelfUpgradeCapabilities writes a capabilities response containing the given operation.
func writeSelfUpgradeCapabilities(t *testing.T, writer http.ResponseWriter, operationID, risk string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": risk}},
	}}); err != nil {
		t.Fatalf("写入能力响应失败 / write capabilities response: %v", err)
	}
}

// TestSelfUpgradeImportStreamsMultipart 验证 import 以 file 字段流式上传并携带确认头。
// TestSelfUpgradeImportStreamsMultipart verifies import uploads via the `file` multipart field in streaming
// mode and carries the confirmation headers.
func TestSelfUpgradeImportStreamsMultipart(t *testing.T) {
	packagePath := filepath.Join(t.TempDir(), "stx-upgrade-9.9.9.tar.gz")
	payload := bytes.Repeat([]byte("pkg"), 1024)
	if err := os.WriteFile(packagePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	var gotConfirm, gotConfirmID, gotFileName, gotFieldName string
	var gotBodyLen int
	var chunked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			writeSelfUpgradeCapabilities(t, w, "self_upgrade.stage.import", "R1")
		case "/api/v1/self-upgrade/stage/import":
			gotConfirm = r.Header.Get("X-STX-Confirm")
			gotConfirmID = r.Header.Get("X-STX-Confirmation-ID")
			chunked = len(r.TransferEncoding) > 0 && r.TransferEncoding[0] == "chunked" && r.ContentLength == -1
			reader, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("multipart 解析失败 / multipart parse: %v", err)
			}
			part, err := reader.NextPart()
			if err != nil {
				t.Fatalf("读取 multipart 分块失败 / read multipart part: %v", err)
			}
			gotFieldName, gotFileName = part.FormName(), part.FileName()
			body, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("读取上传内容失败 / read upload body: %v", err)
			}
			gotBodyLen = len(body)
			if !bytes.Equal(body, payload) {
				t.Fatalf("上传内容与本地文件不一致 / uploaded payload differs from file")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "stage-1", "status": "staged"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{
		"self-upgrade", "import", packagePath, "--confirm", "--confirmation-id", "conf-1", "--server", server.URL,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("import exit = %d stderr=%s", exitCode, stderr.String())
	}
	if gotFieldName != "file" || gotFileName != "stx-upgrade-9.9.9.tar.gz" || gotBodyLen != len(payload) {
		t.Fatalf("multipart field=%s file=%s len=%d", gotFieldName, gotFileName, gotBodyLen)
	}
	if gotConfirm != "true" || gotConfirmID != "conf-1" {
		t.Fatalf("confirmation headers missing: confirm=%s id=%s", gotConfirm, gotConfirmID)
	}
	if !chunked {
		t.Fatalf("expected streaming chunked upload without Content-Length")
	}
}

// TestSelfUpgradeImportRequiresConfirm 验证未带 --confirm 时 import 不发起请求。
// TestSelfUpgradeImportRequiresConfirm verifies import sends no request without --confirm.
func TestSelfUpgradeImportRequiresConfirm(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.NotFound(w, r)
	}))
	defer server.Close()
	packagePath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	if err := os.WriteFile(packagePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{
		"self-upgrade", "import", packagePath, "--server", server.URL,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("import without --confirm exit = 0")
	}
	if called {
		t.Fatalf("server must not be called without --confirm")
	}
}

// TestSelfUpgradeTaskGetWaitStatuses 验证 --wait 在成功、失败、取消三种终态下的退出码。
// TestSelfUpgradeTaskGetWaitStatuses verifies --wait exit codes for succeeded, failed, and cancelled tasks.
func TestSelfUpgradeTaskGetWaitStatuses(t *testing.T) {
	old := selfUpgradeTaskPollInterval
	selfUpgradeTaskPollInterval = time.Millisecond
	defer func() { selfUpgradeTaskPollInterval = old }()

	cases := []struct {
		name       string
		sequence   []string
		failure    string
		wantExit0  bool
		wantDetail string
	}{
		{name: "succeeds", sequence: []string{"upgrading", "succeeded"}, wantExit0: true},
		{name: "failed", sequence: []string{"failed"}, failure: "apply failed", wantDetail: "apply failed"},
		{name: "cancelled", sequence: []string{"cancelled"}, wantDetail: "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idx := calls
				if idx >= len(tc.sequence) {
					idx = len(tc.sequence) - 1
				}
				calls++
				data := map[string]any{"id": "task-1", "kind": "execute", "status": tc.sequence[idx]}
				if tc.failure != "" {
					data["failure_reason"] = tc.failure
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
			}))
			defer server.Close()
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			exitCode := executeCommand(NewRootCommand(), []string{
				"self-upgrade", "task", "get", "task-1", "--wait", "--timeout", "5s", "--server", server.URL,
			}, stdout, stderr)
			if tc.wantExit0 && exitCode != 0 {
				t.Fatalf("wait exit = %d stderr=%s", exitCode, stderr.String())
			}
			if !tc.wantExit0 && exitCode == 0 {
				t.Fatalf("wait exit = 0 for %s", tc.name)
			}
			if tc.wantDetail != "" && !strings.Contains(stdout.String()+stderr.String(), tc.wantDetail) {
				t.Fatalf("missing %q in stdout=%s stderr=%s", tc.wantDetail, stdout.String(), stderr.String())
			}
		})
	}
}

// TestSelfUpgradeTaskWaitRetriesConnectionFailure 验证控制面重启导致的连接级错误会被重试。
// TestSelfUpgradeTaskWaitRetriesConnectionFailure verifies transport-level errors during control-plane
// restart are retried until the task reaches a terminal status.
func TestSelfUpgradeTaskWaitRetriesConnectionFailure(t *testing.T) {
	old := selfUpgradeTaskPollInterval
	selfUpgradeTaskPollInterval = time.Millisecond
	defer func() { selfUpgradeTaskPollInterval = old }()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// 第一次请求直接断开连接，模拟控制面重启中的瞬断。/ First request drops the connection,
			// simulating the control-plane restart window.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijacker unavailable")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "task-1", "status": "succeeded"}})
	}))
	defer server.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{
		"self-upgrade", "task", "get", "task-1", "--wait", "--timeout", "5s", "--server", server.URL,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("wait exit = %d stderr=%s", exitCode, stderr.String())
	}
	if calls < 2 {
		t.Fatalf("expected retry after connection drop, calls=%d", calls)
	}
}

// TestSelfUpgradeTaskWaitTimeout 验证 --wait 超时返回非零并汇报最后状态。
// TestSelfUpgradeTaskWaitTimeout verifies --wait timeout exits non-zero and reports the last status.
func TestSelfUpgradeTaskWaitTimeout(t *testing.T) {
	old := selfUpgradeTaskPollInterval
	selfUpgradeTaskPollInterval = 5 * time.Millisecond
	defer func() { selfUpgradeTaskPollInterval = old }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "task-1", "status": "upgrading"}})
	}))
	defer server.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{
		"self-upgrade", "task", "get", "task-1", "--wait", "--timeout", "60ms", "--server", server.URL,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("wait timeout exit = 0")
	}
	if !strings.Contains(stderr.String(), "last status: upgrading") {
		t.Fatalf("timeout error missing last status: %s", stderr.String())
	}
}

// TestSelfUpgradeTaskCancel 验证 task cancel 需要 --confirm 并调用取消路由。
// TestSelfUpgradeTaskCancel verifies task cancel requires --confirm and hits the cancel route.
func TestSelfUpgradeTaskCancel(t *testing.T) {
	var cancelled bool
	var gotConfirm string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			writeSelfUpgradeCapabilities(t, w, "self_upgrade.task.cancel", "R1")
		case "/api/v1/self-upgrade/tasks/task-1/cancel":
			cancelled = true
			gotConfirm = r.Header.Get("X-STX-Confirm")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "task-1", "status": "cancelled"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if code := executeCommand(NewRootCommand(), []string{
		"self-upgrade", "task", "cancel", "task-1", "--server", server.URL,
	}, stdout, stderr); code == 0 {
		t.Fatalf("cancel without --confirm exit = 0")
	}
	if cancelled {
		t.Fatalf("server must not be called without --confirm")
	}

	stdout.Reset()
	stderr.Reset()
	exitCode := executeCommand(NewRootCommand(), []string{
		"self-upgrade", "task", "cancel", "task-1", "--confirm", "--server", server.URL,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("cancel exit = %d stderr=%s", exitCode, stderr.String())
	}
	if !cancelled || gotConfirm != "true" {
		t.Fatalf("cancel not invoked correctly: cancelled=%v confirm=%s", cancelled, gotConfirm)
	}
}

// TestSelfUpgradeImportConfirmationRequired 验证服务端要求一次性确认时透传 confirmation_id。
// TestSelfUpgradeImportConfirmationRequired verifies the confirmation_required response surfaces the
// confirmation_id and --confirmation-id retries send the header.
func TestSelfUpgradeImportConfirmationRequired(t *testing.T) {
	packagePath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	if err := os.WriteFile(packagePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var uploads int
	var gotConfirmID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/capabilities":
			writeSelfUpgradeCapabilities(t, w, "self_upgrade.stage.import", "R1")
		case "/api/v1/self-upgrade/stage/import":
			uploads++
			gotConfirmID = r.Header.Get("X-STX-Confirmation-ID")
			if gotConfirmID == "" {
				w.WriteHeader(http.StatusPreconditionRequired)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error_code": "confirmation_required",
					"error_msg":  "confirmation required",
					"data": map[string]any{
						"confirmation_id": "conf-42",
						"risk_level":      "R1",
						"impact":          "uploads and unpacks an offline package",
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "stage-1", "status": "staged"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{
		"self-upgrade", "import", packagePath, "--confirm", "--server", server.URL,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("import without confirmation-id exit = 0")
	}
	if !strings.Contains(stderr.String(), "conf-42") {
		t.Fatalf("confirmation_required event missing confirmation id: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = executeCommand(NewRootCommand(), []string{
		"self-upgrade", "import", packagePath, "--confirm", "--confirmation-id", "conf-42", "--server", server.URL,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("retry with confirmation-id exit = %d stderr=%s", exitCode, stderr.String())
	}
	if uploads != 2 || gotConfirmID != "conf-42" {
		t.Fatalf("uploads=%d confirmID=%s", uploads, gotConfirmID)
	}
}
