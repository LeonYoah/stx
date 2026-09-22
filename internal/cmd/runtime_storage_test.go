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
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestRuntimeStorageCommandsSendExpectedRequests(t *testing.T) {
	requests := make([]struct {
		path string
		body map[string]any
	}, 0, 6)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			operations := make([]map[string]any, 0, 6)
			for _, operationID := range []string{
				"cluster.runtime-storage.validate",
				"cluster.runtime-storage.list",
				"cluster.runtime-storage.preview",
				"cluster.runtime-storage.checkpoint.inspect",
				"cluster.runtime-storage.imap.inspect",
				"installer.runtime-storage.validate",
			} {
				operations = append(operations, map[string]any{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": "R0"})
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"operations": operations}})
			return
		}
		if request.Method != http.MethodPost {
			t.Fatalf("运行时存储请求方法错误: %s", request.Method)
		}
		body := make(map[string]any)
		if request.Body != nil && request.ContentLength != 0 {
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("读取运行时存储请求失败: %v", err)
			}
		}
		requests = append(requests, struct {
			path string
			body map[string]any
		}{path: request.URL.Path, body: body})
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"ok": true}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	commands := [][]string{
		{"cluster", "runtime-storage", "validate", "6", "checkpoint"},
		{"cluster", "runtime-storage", "list", "6", "imap", "--path", "wal", "--recursive", "--limit", "50"},
		{"cluster", "runtime-storage", "preview", "6", "checkpoint", "--path", "checkpoint.dat", "--max-bytes", "4096"},
		{"cluster", "runtime-storage", "checkpoint", "inspect", "6", "--path", "checkpoint.dat"},
		{"cluster", "runtime-storage", "imap", "inspect", "6", "--path", "imap.wal"},
		{"installer", "runtime-storage", "validate", "--host-id", "10", "--kind", "imap", "--imap-file", writeRuntimeStorageJSONFile(t, `{"storage_type":"DISABLED"}`)},
	}
	for _, args := range commands {
		stdout, stderr, exitCode := runRuntimeStorageCommand(t, store, args...)
		if exitCode != int(clioutput.ExitSuccess) {
			t.Fatalf("运行时存储命令失败: args=%v code=%d stdout=%s stderr=%s", args, exitCode, stdout, stderr)
		}
	}

	if len(requests) != 6 {
		t.Fatalf("运行时存储业务请求数错误: %d", len(requests))
	}
	if requests[0].path != "/api/v1/clusters/6/runtime-storage/checkpoint/validate" || len(requests[0].body) != 0 {
		t.Fatalf("集群存储校验请求错误: %#v", requests[0])
	}
	if requests[1].path != "/api/v1/clusters/6/runtime-storage/imap/list" || requests[1].body["path"] != "wal" || requests[1].body["recursive"] != true || requests[1].body["limit"] != float64(50) {
		t.Fatalf("存储列表请求错误: %#v", requests[1])
	}
	if requests[2].body["path"] != "checkpoint.dat" || requests[2].body["max_bytes"] != float64(4096) {
		t.Fatalf("存储预览请求错误: %#v", requests[2])
	}
	if requests[3].path != "/api/v1/clusters/6/runtime-storage/checkpoint/inspect" || requests[3].body["path"] != "checkpoint.dat" {
		t.Fatalf("Checkpoint 检查请求错误: %#v", requests[3])
	}
	if requests[4].path != "/api/v1/clusters/6/runtime-storage/imap/inspect" || requests[4].body["path"] != "imap.wal" {
		t.Fatalf("IMAP 检查请求错误: %#v", requests[4])
	}
	if requests[5].path != "/api/v1/installer/runtime-storage/validate" || requests[5].body["kind"] != "imap" {
		t.Fatalf("安装前存储校验请求错误: %#v", requests[5])
	}
	hostIDs, ok := requests[5].body["host_ids"].([]any)
	if !ok || len(hostIDs) != 1 || hostIDs[0] != float64(10) {
		t.Fatalf("安装前存储校验主机错误: %#v", requests[5].body["host_ids"])
	}
	imap, ok := requests[5].body["imap"].(map[string]any)
	if !ok || imap["storage_type"] != "DISABLED" {
		t.Fatalf("安装前 IMAP 配置错误: %#v", requests[5].body["imap"])
	}
}

func TestRuntimeStoragePreviewRequiresPathBeforeNetworkRequest(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runRuntimeStorageCommand(t, store, "cluster", "runtime-storage", "preview", "6", "checkpoint")
	if exitCode != int(clioutput.ExitUsage) || calls != 0 {
		t.Fatalf("缺少路径时仍发起请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runRuntimeStorageCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addRuntimeStorageCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeRuntimeStorageJSONFile(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/runtime-storage.json"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入运行时存储测试文件失败: %v", err)
	}
	return path
}
