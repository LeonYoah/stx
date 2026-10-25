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
