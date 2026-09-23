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
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestAuditCommandListBuildsFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeAuditCapabilities(t, writer, "audit.command.list")
			return
		}
		if request.URL.Path != "/api/v1/commands" {
			t.Fatalf("命令记录路径错误: %s", request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("agent_id") != "agent-1" || query.Get("status") != "failed" || query.Get("size") != "50" || query.Get("current") != "2" {
			t.Fatalf("命令记录查询参数错误: %s", request.URL.RawQuery)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"total": 1, "commands": []any{map[string]any{"id": 42, "status": "failed"}}}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runAuditCommand(t, store, "audit", "command", "list", "--agent_id", "agent-1", "--status", "failed", "--current", "2", "--size", "50")
	if exitCode != int(clioutput.ExitSuccess) || strings.TrimSpace(stderr) != "" {
		t.Fatalf("查询命令记录失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"operation_id":"audit.command.list"`) || !strings.Contains(stdout, `"total":1`) {
		t.Fatalf("命令记录输出错误: %s", stdout)
	}
}

func TestAuditLogGetBuildsPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeAuditCapabilities(t, writer, "audit.log.get")
			return
		}
		if request.URL.Path != "/api/v1/audit-logs/42" {
			t.Fatalf("审计日志路径错误: %s", request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 42, "action": "start"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runAuditCommand(t, store, "audit", "get", "42", "--pick", "action")
	if exitCode != int(clioutput.ExitSuccess) || strings.TrimSpace(stderr) != "" {
		t.Fatalf("查询审计日志失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"operation_id":"audit.log.get"`) || !strings.Contains(stdout, `"action":"start"`) {
		t.Fatalf("审计日志输出错误: %s", stdout)
	}
}

func runAuditCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	var stdout, stderr strings.Builder
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeAuditCapabilities(t *testing.T, writer http.ResponseWriter, operationID string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": "R0"}},
	}}); err != nil {
		t.Fatalf("写入审计能力响应失败: %v", err)
	}
}
