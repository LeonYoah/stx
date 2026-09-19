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

func TestClusterCreateSendsBodyAndSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeClusterCapabilities(t, writer, "cluster.create", "R1")
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/clusters" {
			t.Fatalf("创建集群请求错误: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "cluster-create-key" {
			t.Fatalf("集群安全请求头错误: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取创建集群请求失败: %v", err)
		}
		if body["name"] != "cli-cluster" || body["deployment_mode"] != "hybrid" || body["version"] != "2.3.13" {
			t.Fatalf("创建集群正文错误: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 9, "name": "cli-cluster"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runClusterCommand(t, store,
		"cluster", "create", "--name", "cli-cluster", "--deployment-mode", "hybrid", "--version", "2.3.13",
		"--confirm", "--idempotency-key", "cluster-create-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("创建集群命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestClusterUpdateAcceptsCompleteRequestFile(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "cluster-update.json")
	if err := os.WriteFile(requestFile, []byte(`{"description":"from-file","config":{"env":"test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeClusterCapabilities(t, writer, "cluster.update", "R1")
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/clusters/6" || body["description"] != "from-file" {
			t.Fatalf("更新集群请求错误: method=%s path=%s body=%#v", request.Method, request.URL.Path, body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 6, "description": "from-file"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runClusterCommand(t, store, "cluster", "update", "6", "--request-file", requestFile, "--confirm")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("更新集群命令失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestClusterNodeAddPreservesPortsAndSkipPrecheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeClusterCapabilities(t, writer, "cluster.node.add", "R1")
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if request.URL.Path != "/api/v1/clusters/6/nodes" || body["host_id"] != float64(10) || body["hazelcast_port"] != float64(5901) || body["skip_precheck"] != true {
			t.Fatalf("新增节点请求错误: path=%s body=%#v", request.URL.Path, body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 11, "cluster_id": 6}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runClusterCommand(t, store,
		"cluster", "node", "add", "6", "--host-id", "10", "--role", "master/worker", "--hazelcast-port", "5901",
		"--skip-precheck", "--confirm")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("新增节点命令失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestClusterWriteRequiresConfirmBeforeNetworkRequest(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runClusterCommand(t, store,
		"cluster", "create", "--name", "cli-cluster", "--deployment-mode", "hybrid", "--version", "2.3.13")
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍发起请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func runClusterCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addClusterWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeClusterCapabilities(t *testing.T, writer http.ResponseWriter, operationID, risk string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": risk}},
	}}); err != nil {
		t.Fatalf("写入能力响应失败: %v", err)
	}
}
