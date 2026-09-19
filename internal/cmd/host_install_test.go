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

func TestHostPrecheckSendsOptionalBodyWithoutConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeHostInstallCapabilities(t, writer, "host.precheck", "R0")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/hosts/10/precheck" {
			t.Fatalf("预检请求错误: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取预检正文失败: %v", err)
		}
		ports, ok := body["ports"].([]any)
		if body["install_dir"] != "/tmp/seatunnel-2.3.12" || !ok || len(ports) != 2 {
			t.Fatalf("预检正文错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"passed": true}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runHostInstallCommand(t, store, "host", "precheck", "10", "--install-dir", "/tmp/seatunnel-2.3.12", "--port", "15812,18092")
	if exitCode != int(clioutput.ExitSuccess) || strings.TrimSpace(stderr) != "" {
		t.Fatalf("预检命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"operation_id":"host.precheck"`) {
		t.Fatalf("预检结果缺少操作编号: %s", stdout)
	}
}

func TestHostInstallStartSendsBodySafetyHeadersAndNextCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeHostInstallCapabilities(t, writer, "host.install.start", "R1")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/hosts/10/install" {
			t.Fatalf("安装请求错误: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "install-key" {
			t.Fatalf("安装安全请求头错误: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取安装正文失败: %v", err)
		}
		if body["version"] != "2.3.12" || body["install_dir"] != "/tmp/seatunnel-2.3.12" || body["install_mode"] != "online" || body["deployment_mode"] != "hybrid" || body["node_role"] != "master/worker" {
			t.Fatalf("安装正文错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"host_id": "10", "status": "running"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runHostInstallCommand(t, store,
		"host", "install", "start", "10", "--version", "2.3.12", "--install-dir", "/tmp/seatunnel-2.3.12",
		"--cluster-port", "15812", "--http-port", "18092", "--confirm", "--idempotency-key", "install-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("安装命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"next_command":"stx host install status get 10"`) {
		t.Fatalf("安装结果缺少下一条命令: %s", stdout)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 1 || events[0]["event"] != "warning" {
		t.Fatalf("安装影响提示错误: %#v", events)
	}
}

func TestHostInstallStartRequestFileCanBeOverridden(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "install.json")
	if err := os.WriteFile(requestFile, []byte(`{"version":"2.3.11","install_dir":"/tmp/from-file","deployment_mode":"separated","node_role":"worker"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeHostInstallCapabilities(t, writer, "host.install.start", "R1")
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["version"] != "2.3.12" || body["deployment_mode"] != "separated" || body["node_role"] != "worker" {
			t.Fatalf("请求文件覆盖结果错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": body})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runHostInstallCommand(t, store, "host", "install", "start", "10", "--request-file", requestFile, "--version", "2.3.12", "--confirm")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("请求文件安装失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestHostInstallRetryAndCancelUseRegisteredRequests(t *testing.T) {
	tests := []struct {
		name        string
		operationID string
		path        string
		args        []string
		wantStep    string
	}{
		{name: "retry", operationID: "host.install.retry", path: "/api/v1/hosts/10/install/retry", args: []string{"host", "install", "retry", "10", "--step", "extract", "--confirm"}, wantStep: "extract"},
		{name: "cancel", operationID: "host.install.cancel", path: "/api/v1/hosts/10/install/cancel", args: []string{"host", "install", "cancel", "10", "--confirm"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v1/capabilities" {
					writeHostInstallCapabilities(t, writer, test.operationID, "R1")
					return
				}
				if request.Method != http.MethodPost || request.URL.Path != test.path || request.Header.Get("X-STX-Confirm") != "true" {
					t.Fatalf("安装控制请求错误: %s %s %#v", request.Method, request.URL.Path, request.Header)
				}
				if test.wantStep != "" {
					var body map[string]any
					if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["step"] != test.wantStep {
						t.Fatalf("重试正文错误: body=%#v err=%v", body, err)
					}
				}
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"host_id": "10"}})
			}))
			defer server.Close()

			store := newExecutionTestStore(t, server.URL, "test-token")
			_, stderr, exitCode := runHostInstallCommand(t, store, test.args...)
			if exitCode != int(clioutput.ExitSuccess) {
				t.Fatalf("安装控制命令失败: code=%d stderr=%s", exitCode, stderr)
			}
		})
	}
}

func TestHostInstallStartRequiresConfirmationBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runHostInstallCommand(t, store, "host", "install", "start", "10", "--version", "2.3.12")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍访问网络: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runHostInstallCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	hostCommand := &cobra.Command{Use: "host"}
	installCommand := &cobra.Command{Use: "install"}
	statusCommand := &cobra.Command{Use: "status"}
	statusCommand.AddCommand(&cobra.Command{Use: "get"})
	installCommand.AddCommand(statusCommand)
	hostCommand.AddCommand(installCommand)
	root.AddCommand(hostCommand)
	addHostInstallCommands(root, func() (*cliConfig.Store, error) { return store, nil })
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeHostInstallCapabilities(t *testing.T, writer http.ResponseWriter, operationID, risk string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": risk}},
	}}); err != nil {
		t.Fatalf("写入能力响应失败: %v", err)
	}
}
