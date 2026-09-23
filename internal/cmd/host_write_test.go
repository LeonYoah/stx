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
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestHostCreateSendsBodyAndSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeHostCapabilities(t, writer, "host.create")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/hosts" {
			t.Fatalf("创建主机请求错误: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "host-create-key" {
			t.Fatalf("主机安全请求头错误: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["name"] != "cli-host" || body["host_type"] != "bare_metal" || body["ip_address"] != "192.0.2.20" || body["ssh_port"] != float64(2222) {
			t.Fatalf("创建主机正文错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 11, "name": "cli-host"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runHostWriteCommand(t, store, "host", "create", "--name", "cli-host", "--host-type", "bare_metal", "--ip-address", "192.0.2.20", "--ssh-port", "2222", "--confirm", "--idempotency-key", "host-create-key")
	if exitCode != int(clioutput.ExitSuccess) || !bytes.Contains([]byte(stdout), []byte(`"operation_id":"host.create"`)) {
		t.Fatalf("创建主机命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestHostUpdateAcceptsCompleteRequestFile(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "host-update.json")
	if err := os.WriteFile(requestFile, []byte(`{"description":"from-file","ssh_port":2200}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeHostCapabilities(t, writer, "host.update")
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/hosts/10" || body["description"] != "from-file" || body["ssh_port"] != float64(2200) {
			t.Fatalf("更新主机请求错误: method=%s path=%s body=%#v", request.Method, request.URL.Path, body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 10, "description": "from-file"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runHostWriteCommand(t, store, "host", "update", "10", "--request-file", requestFile, "--confirm")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("更新主机命令失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestHostWriteRequiresConfirmationBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runHostWriteCommand(t, store, "host", "create", "--name", "cli-host", "--ip-address", "192.0.2.20")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍发起请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runHostWriteCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addHostWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeHostCapabilities(t *testing.T, writer http.ResponseWriter, operationID string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": "R1"}},
	}}); err != nil {
		t.Fatalf("写入主机能力响应失败: %v", err)
	}
}
