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

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestPluginJSONWriteCommandsSendExpectedRequests(t *testing.T) {
	profilesFile := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(profilesFile, []byte(`{"jdbc":["mysql"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		operationID string
		method      string
		path        string
		args        []string
		assertBody  func(*testing.T, map[string]any)
	}{
		{name: "refresh", operationID: "plugin.refresh", method: http.MethodPost, path: "/api/v1/plugins/refresh", args: []string{"plugin", "refresh", "--version", "2.3.13", "--mirror", "apache"}, assertBody: func(t *testing.T, body map[string]any) {
			if body["version"] != "2.3.13" || body["mirror"] != "apache" {
				t.Fatalf("刷新正文错误: %#v", body)
			}
		}},
		{name: "download", operationID: "plugin.download.start", method: http.MethodPost, path: "/api/v1/plugins/jdbc/download", args: []string{"plugin", "download", "start", "jdbc", "--version", "2.3.13", "--profile-key", "mysql,postgresql"}, assertBody: func(t *testing.T, body map[string]any) {
			profiles, _ := body["profile_keys"].([]any)
			if body["version"] != "2.3.13" || len(profiles) != 2 {
				t.Fatalf("下载正文错误: %#v", body)
			}
		}},
		{name: "download all", operationID: "plugin.download-all.start", method: http.MethodPost, path: "/api/v1/plugins/download-all", args: []string{"plugin", "download", "all", "--version", "2.3.13", "--profiles-file", profilesFile}, assertBody: func(t *testing.T, body map[string]any) {
			if body["version"] != "2.3.13" || body["selected_plugin_profiles"] == nil {
				t.Fatalf("批量下载正文错误: %#v", body)
			}
		}},
		{name: "dependency add", operationID: "plugin.dependency.add", method: http.MethodPost, path: "/api/v1/plugins/jdbc/dependencies", args: []string{"plugin", "dependency", "add", "jdbc", "--seatunnel-version", "2.3.13", "--group-id", "com.example", "--artifact-id", "driver", "--version", "1.0.0"}, assertBody: func(t *testing.T, body map[string]any) {
			if body["seatunnel_version"] != "2.3.13" || body["artifact_id"] != "driver" {
				t.Fatalf("新增依赖正文错误: %#v", body)
			}
		}},
		{name: "dependency disable", operationID: "plugin.dependency.disable", method: http.MethodPost, path: "/api/v1/plugins/jdbc/dependencies/disables", args: []string{"plugin", "dependency", "disable", "jdbc", "--group-id", "mysql", "--artifact-id", "mysql-driver", "--version", "8.0.0", "--target-dir", "lib"}, assertBody: func(t *testing.T, body map[string]any) {
			if body["target_dir"] != "lib" || body["group_id"] != "mysql" {
				t.Fatalf("禁用依赖正文错误: %#v", body)
			}
		}},
		{name: "official analyze", operationID: "plugin.official-dependency.analyze", method: http.MethodPost, path: "/api/v1/plugins/jdbc/official-dependencies/analyze", args: []string{"plugin", "official-dependency", "analyze", "jdbc", "--version", "2.3.13", "--force-refresh"}, assertBody: func(t *testing.T, body map[string]any) {
			if body["version"] != "2.3.13" || body["force_refresh"] != true {
				t.Fatalf("官方依赖分析正文错误: %#v", body)
			}
		}},
		{name: "cluster install", operationID: "cluster.plugin.install", method: http.MethodPost, path: "/api/v1/clusters/6/plugins", args: []string{"cluster", "plugin", "install", "6", "--plugin", "jdbc", "--version", "2.3.13", "--profile-key", "mysql"}, assertBody: func(t *testing.T, body map[string]any) {
			if body["plugin_name"] != "jdbc" || body["version"] != "2.3.13" {
				t.Fatalf("集群安装正文错误: %#v", body)
			}
		}},
		{name: "cluster enable", operationID: "cluster.plugin.enable", method: http.MethodPut, path: "/api/v1/clusters/6/plugins/jdbc/enable", args: []string{"cluster", "plugin", "enable", "6", "jdbc"}},
		{name: "cluster disable", operationID: "cluster.plugin.disable", method: http.MethodPut, path: "/api/v1/clusters/6/plugins/jdbc/disable", args: []string{"cluster", "plugin", "disable", "6", "jdbc"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v1/capabilities" {
					writePluginCapabilities(t, writer, test.operationID)
					return
				}
				if request.Method != test.method || request.URL.Path != test.path {
					t.Fatalf("插件请求错误: %s %s", request.Method, request.URL.Path)
				}
				if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "plugin-test-key" {
					t.Fatalf("插件安全请求头错误: %#v", request.Header)
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatalf("读取插件请求失败: %v", err)
				}
				if test.assertBody != nil {
					test.assertBody(t, body)
				}
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"ok": true}})
			}))
			defer server.Close()
			store := newExecutionTestStore(t, server.URL, "test-token")
			args := append(append([]string{}, test.args...), "--confirm", "--idempotency-key", "plugin-test-key")
			stdout, stderr, exitCode := runPluginCommand(t, store, args...)
			if exitCode != int(clioutput.ExitSuccess) || !strings.Contains(stdout, `"operation_id":"`+test.operationID+`"`) {
				t.Fatalf("插件命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
			}
		})
	}
}

func TestPluginDependencyUploadStreamsFileAndSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writePluginCapabilities(t, writer, "plugin.dependency.upload")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/plugins/jdbc/dependencies/upload" {
			t.Fatalf("上传依赖请求错误: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "upload-plugin-key" {
			t.Fatalf("上传依赖安全请求头错误: %#v", request.Header)
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("解析上传请求失败: %v", err)
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			t.Fatalf("读取上传文件失败: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if header.Filename != "driver.jar" || string(content) != "jar-data" || request.FormValue("seatunnel_version") != "2.3.13" {
			t.Fatalf("上传依赖内容错误: name=%s content=%q fields=%#v", header.Filename, content, request.MultipartForm.Value)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"artifact_id": "driver"}})
	}))
	defer server.Close()
	jarPath := filepath.Join(t.TempDir(), "driver.jar")
	if err := os.WriteFile(jarPath, []byte("jar-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runPluginCommand(t, store, "plugin", "dependency", "upload", "jdbc", jarPath, "--seatunnel-version", "2.3.13", "--confirm", "--idempotency-key", "upload-plugin-key")
	if exitCode != int(clioutput.ExitSuccess) || !strings.Contains(stdout, `"operation_id":"plugin.dependency.upload"`) {
		t.Fatalf("上传依赖命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestPluginWriteRequiresConfirmationBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runPluginCommand(t, store, "plugin", "refresh")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍发起请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func TestRootCommandRegistersPluginWriteCommands(t *testing.T) {
	root := newRootCommand(func() error { return nil })
	paths := [][]string{
		{"plugin", "refresh"}, {"plugin", "download", "start"}, {"plugin", "download", "all"},
		{"plugin", "dependency", "add"}, {"plugin", "dependency", "upload"}, {"plugin", "dependency", "disable"},
		{"plugin", "official-dependency", "analyze"}, {"cluster", "plugin", "install"},
		{"cluster", "plugin", "enable"}, {"cluster", "plugin", "disable"},
	}
	for _, path := range paths {
		found, remaining, err := root.Find(path)
		if err != nil || len(remaining) != 0 || found.Name() != path[len(path)-1] {
			t.Fatalf("插件命令未完整注册: path=%v found=%s remaining=%v err=%v", path, found.CommandPath(), remaining, err)
		}
	}
}

func runPluginCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addPluginWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writePluginCapabilities(t *testing.T, writer http.ResponseWriter, operationID string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": "R1"}},
	}}); err != nil {
		t.Fatalf("写入插件能力响应失败: %v", err)
	}
}
