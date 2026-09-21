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

func TestConfigUpdateSendsContentAndSafetyHeaders(t *testing.T) {
	contentFile := filepath.Join(t.TempDir(), "jvm_options")
	if err := os.WriteFile(contentFile, []byte("-Xms2g\n-Xmx2g\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			writeConfigCapabilities(t, writer, "config.update", "R1")
			return
		}
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/configs/98" {
			t.Fatalf("配置更新地址错误 / invalid config update target: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "config-key" {
			t.Fatalf("配置更新安全请求头错误 / invalid config safety headers: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["content"] != "-Xms2g\n-Xmx2g\n" || body["comment"] != "cli test" {
			t.Fatalf("配置更新正文错误 / invalid config update body: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 98}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runConfigCommand(t, store, "config", "update", "98", "--content-file", contentFile, "--comment", "cli test", "--confirm", "--idempotency-key", "config-key")
	if exitCode != int(clioutput.ExitSuccess) || !strings.Contains(stdout, `"operation_id":"config.update"`) {
		t.Fatalf("配置更新命令失败 / config update command failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestConfigWriteRequiresConfirmBeforeNetworkRequest(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runConfigCommand(t, store, "config", "sync", "98")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍发起请求 / request was sent without confirmation: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runConfigCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addConfigWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeConfigCapabilities(t *testing.T, writer http.ResponseWriter, operationID, risk string) {
	t.Helper()
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": risk}}}}); err != nil {
		t.Fatal(err)
	}
}
