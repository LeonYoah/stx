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
	"strings"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestAdminUserCreateReadsPasswordFromStdinWithoutPrintingIt(t *testing.T) {
	const password = "secret-value"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUserWriteCapabilities(t, writer, "admin.user.create", true)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取创建用户请求失败: %v", err)
		}
		if body["password"] != password || body["username"] != "test-user" {
			t.Fatalf("创建用户请求内容错误: %#v", body)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "create-key" {
			t.Fatalf("安全请求头错误: %#v", request.Header)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 2, "username": "test-user"}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runUserWriteCommand(t, store, strings.NewReader(password+"\n"),
		"admin", "user", "create", "--username", "test-user", "--password-stdin", "--confirm", "--idempotency-key", "create-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("创建用户命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if strings.Contains(stdout, password) || strings.Contains(stderr, password) {
		t.Fatalf("密码不应出现在命令输出中: stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestAdminUserUpdatePreservesExplicitFalseFlags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUserWriteCapabilities(t, writer, "admin.user.update", true)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("读取更新用户请求失败: %v", err)
		}
		if active, ok := body["is_active"].(bool); !ok || active {
			t.Fatalf("--active=false 未进入请求: %#v", body)
		}
		if adminFlag, ok := body["is_admin"].(bool); !ok || adminFlag {
			t.Fatalf("--admin=false 未进入请求: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"id": 2, "is_active": false, "is_admin": false}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runUserWriteCommand(t, store, strings.NewReader(""),
		"admin", "user", "update", "2", "--active=false", "--admin=false", "--confirm", "--idempotency-key", "update-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("更新用户命令失败: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestAdminUserDeleteIsNotExposed(t *testing.T) {
	store := newExecutionTestStore(t, "http://127.0.0.1:1", "test-token")
	root := &cobra.Command{Use: "stx"}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(func() (*cliConfig.Store, error) { return store, nil })...)
	addUserWriteCommands(root, userWriteCommandOptions{storeProvider: func() (*cliConfig.Store, error) { return store, nil }})
	command, remaining, err := root.Find([]string{"admin", "user", "delete"})
	if err == nil && len(remaining) == 0 && command.Name() == "delete" {
		t.Fatal("CLI 不应暴露用户删除命令 / user delete must not be exposed")
	}
}

func TestAdminUserCreateStopsWhenCapabilityDeniesAdmin(t *testing.T) {
	var writeCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeUserWriteCapabilities(t, writer, "admin.user.create", false)
			return
		}
		writeCalls++
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runUserWriteCommand(t, store, strings.NewReader("secret\n"),
		"admin", "user", "create", "--username", "test-user", "--password-stdin", "--confirm")
	if exitCode != int(clioutput.ExitPermission) || writeCalls != 0 {
		t.Fatalf("权限拒绝后不应调用写接口: code=%d calls=%d stderr=%s", exitCode, writeCalls, stderr)
	}
}

func runUserWriteCommand(t *testing.T, store *cliConfig.Store, stdin io.Reader, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addUserWriteCommands(root, userWriteCommandOptions{
		storeProvider: storeProvider,
		stdin:         stdin,
		isTerminal:    func(io.Reader) bool { return false },
		readTerminalPassword: func(io.Reader, io.Writer) (string, error) {
			return "", nil
		},
	})
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeUserWriteCapabilities(t *testing.T, writer http.ResponseWriter, operationID string, allowed bool) {
	t.Helper()
	denialCode := ""
	if !allowed {
		denialCode = "admin_required"
	}
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{
			"operation_id": operationID, "revision": 1, "allowed": allowed,
			"denial_code": denialCode, "mode": "normal", "risk": "R1",
		}},
	}})
}
