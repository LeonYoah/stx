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
