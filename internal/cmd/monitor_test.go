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
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestMonitorConfigUpdateSendsBodyAndSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeClusterCapabilities(t, writer, "monitor.config.update", "R1")
			return
		}
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/clusters/6/monitor-config" {
			t.Fatalf("监控配置请求错误: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "monitor-update-key" {
			t.Fatalf("监控配置安全请求头错误: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["auto_restart"] != false || body["monitor_interval"] != float64(7) || len(body) != 2 {
			t.Fatalf("监控配置正文错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"cluster_id": 6, "auto_restart": false, "monitor_interval": 7}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runMonitorCommand(t, store,
		"monitor", "config", "update", "6", "--auto-restart=false", "--monitor-interval", "7",
		"--confirm", "--idempotency-key", "monitor-update-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("更新监控配置失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	var result clioutput.Result
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("监控配置输出不是合法 JSON: %v", err)
	}
	if result.ResultMeta.NextCommand != "stx monitor config get 6" {
		t.Fatalf("监控配置后续命令错误: %#v", result.ResultMeta)
	}
}

func TestMonitorConfigUpdateRequiresConfirmBeforeNetworkRequest(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runMonitorCommand(t, store, "monitor", "config", "update", "6", "--monitor-interval", "7")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍发起监控配置请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runMonitorCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addMonitorWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}
