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

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
)

// javaHomeCaptureServer 记录请求正文中的 java_home，并对能力查询返回指定操作。
// javaHomeCaptureServer records java_home from the request body and answers the capability query.
func javaHomeCaptureServer(t *testing.T, operationID, risk, wantPath string, writeCaps func(*testing.T, http.ResponseWriter, string, string), got *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeCaps(t, writer, operationID, risk)
			return
		}
		if request.URL.Path != wantPath {
			t.Fatalf("请求路径错误 / unexpected path: %s", request.URL.Path)
		}
		body := map[string]any{}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取请求正文失败 / decode body: %v", err)
		}
		*got = body
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"ok": true}})
	}))
}

// TestJavaHomeFlagsReachRequestBody 覆盖 issue #91 的 CLI：安装预检、安装、新增节点都能指定节点 JDK。
// TestJavaHomeFlagsReachRequestBody covers the issue #91 CLI: install precheck, install and node add accept a node JDK.
func TestJavaHomeFlagsReachRequestBody(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		risk      string
		path      string
		run       func(t *testing.T, serverURL string) (string, int)
	}{
		{name: "host precheck", operation: "host.precheck", risk: "R0", path: "/api/v1/hosts/10/precheck",
			run: func(t *testing.T, serverURL string) (string, int) {
				_, stderr, code := runHostInstallCommand(t, newExecutionTestStore(t, serverURL, "test-token"), "host", "precheck", "10", "--java-home", " /opt/jdk8 ")
				return stderr, code
			}},
		{name: "host install start", operation: "host.install.start", risk: "R1", path: "/api/v1/hosts/10/install",
			run: func(t *testing.T, serverURL string) (string, int) {
				_, stderr, code := runHostInstallCommand(t, newExecutionTestStore(t, serverURL, "test-token"), "host", "install", "start", "10", "--version", "2.3.12", "--java-home", "/opt/jdk8", "--confirm")
				return stderr, code
			}},
		{name: "cluster node add", operation: "cluster.node.add", risk: "R1", path: "/api/v1/clusters/6/nodes",
			run: func(t *testing.T, serverURL string) (string, int) {
				_, stderr, code := runClusterCommand(t, newExecutionTestStore(t, serverURL, "test-token"), "cluster", "node", "add", "6", "--host-id", "10", "--role", "worker", "--java-home", "/opt/jdk8", "--confirm")
				return stderr, code
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			caps := writeClusterCapabilities
			if strings.HasPrefix(tc.operation, "host.") {
				caps = writeHostInstallCapabilities
			}
			server := javaHomeCaptureServer(t, tc.operation, tc.risk, tc.path, caps, &body)
			defer server.Close()
			stderr, code := tc.run(t, server.URL)
			if code != int(clioutput.ExitSuccess) {
				t.Fatalf("命令失败 / command failed: code=%d stderr=%s", code, stderr)
			}
			if body["java_home"] != "/opt/jdk8" {
				t.Fatalf("java_home = %#v, want /opt/jdk8 (body=%#v)", body["java_home"], body)
			}
		})
	}
}

// TestJavaHomeFlagOmittedKeepsBody 覆盖 AC6：未传 --java-home 时请求正文不出现 java_home。
// TestJavaHomeFlagOmittedKeepsBody covers AC6: without --java-home the request body has no java_home.
func TestJavaHomeFlagOmittedKeepsBody(t *testing.T) {
	var body map[string]any
	server := javaHomeCaptureServer(t, "cluster.node.add", "R1", "/api/v1/clusters/6/nodes", writeClusterCapabilities, &body)
	defer server.Close()
	_, stderr, code := runClusterCommand(t, newExecutionTestStore(t, server.URL, "test-token"), "cluster", "node", "add", "6", "--host-id", "10", "--role", "worker", "--confirm")
	if code != int(clioutput.ExitSuccess) {
		t.Fatalf("命令失败 / command failed: code=%d stderr=%s", code, stderr)
	}
	if _, ok := body["java_home"]; ok {
		t.Fatalf("java_home must be absent without the flag: %#v", body)
	}
}

// TestClusterNodeJavaRuntimeCommand 覆盖 AC7 的 CLI：stx cluster node java-runtime 读取节点生效的 JDK。
// TestClusterNodeJavaRuntimeCommand covers the AC7 CLI: stx cluster node java-runtime reads the node's effective JDK.
func TestClusterNodeJavaRuntimeCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeClusterCapabilities(t, writer, "cluster.node.java-runtime", "R0")
			return
		}
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/clusters/6/nodes/1/java-runtime" {
			t.Fatalf("请求错误 / unexpected request: %s %s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"node_id": 1, "java_home": "/opt/jdk8", "java_bin": "/opt/jdk8/bin/java", "version": 8, "status": "passed", "source": "custom",
		}})
	}))
	defer server.Close()
	stdout, stderr, code := runClusterCommand(t, newExecutionTestStore(t, server.URL, "test-token"), "cluster", "node", "java-runtime", "6", "1")
	if code != int(clioutput.ExitSuccess) {
		t.Fatalf("命令失败 / command failed: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "/opt/jdk8/bin/java") {
		t.Fatalf("输出缺少生效 JDK / output misses the effective JDK: %s", stdout)
	}
}

// TestHostJavaRuntimesCommand 覆盖 AC1b 的 CLI：stx host java-runtimes 列出主机可探测 JDK。
// TestHostJavaRuntimesCommand covers the AC1b CLI: stx host java-runtimes lists detectable JDKs on a host.
func TestHostJavaRuntimesCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeClusterCapabilities(t, writer, "host.java-runtimes.list", "R0")
			return
		}
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/hosts/10/java-runtimes" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"api_version":"v1","operation_id":"host.java-runtimes.list","request_id":"r1","data":[{"java_home":"/opt/jdk8","version":8,"source":"scan"}],"result_meta":{"complete":true}}`))
	}))
	defer server.Close()
	stdout, stderr, code := runClusterCommand(t, newExecutionTestStore(t, server.URL, "test-token"), "host", "java-runtimes", "10")
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "/opt/jdk8") {
		t.Fatalf("stdout=%s", stdout)
	}
}
