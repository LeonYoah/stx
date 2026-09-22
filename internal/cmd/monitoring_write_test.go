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

func TestMonitoringWriteCommandsSendExpectedRequests(t *testing.T) {
	requestFile := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(requestFile, []byte(`{"name":"cli-smoke","enabled":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, operationID, method, path string
		args                            []string
		assertBody                      func(*testing.T, map[string]any)
	}{
		{name: "policy create", operationID: "monitoring.alert-policy.create", method: http.MethodPost, path: "/api/v1/monitoring/alert-policies", args: []string{"monitoring", "alert-policy", "create", "--request-file", requestFile}},
		{name: "policy update", operationID: "monitoring.alert-policy.update", method: http.MethodPut, path: "/api/v1/monitoring/alert-policies/7", args: []string{"monitoring", "alert-policy", "update", "7", "--request-file", requestFile}},
		{name: "instance ack", operationID: "monitoring.alert-instance.ack", method: http.MethodPost, path: "/api/v1/monitoring/alert-instances/alert-1/ack", args: []string{"monitoring", "alert-instance", "ack", "alert-1", "--note", "checked"}, assertBody: assertMonitoringNote("checked")},
		{name: "instance silence", operationID: "monitoring.alert-instance.silence", method: http.MethodPost, path: "/api/v1/monitoring/alert-instances/alert-1/silence", args: []string{"monitoring", "alert-instance", "silence", "alert-1", "--duration-minutes", "30"}, assertBody: assertMonitoringDuration(30)},
		{name: "instance close", operationID: "monitoring.alert-instance.close", method: http.MethodPost, path: "/api/v1/monitoring/alert-instances/alert-1/close", args: []string{"monitoring", "alert-instance", "close", "alert-1", "--note", "resolved"}, assertBody: assertMonitoringNote("resolved")},
		{name: "alert ack", operationID: "monitoring.alert.ack", method: http.MethodPost, path: "/api/v1/monitoring/alerts/9/ack", args: []string{"monitoring", "alert", "ack", "9", "--note", "checked"}, assertBody: assertMonitoringNote("checked")},
		{name: "alert silence", operationID: "monitoring.alert.silence", method: http.MethodPost, path: "/api/v1/monitoring/alerts/9/silence", args: []string{"monitoring", "alert", "silence", "9", "--duration-minutes", "45"}, assertBody: assertMonitoringDuration(45)},
		{name: "rule update", operationID: "monitoring.cluster.rule.update", method: http.MethodPut, path: "/api/v1/monitoring/clusters/6/rules/3", args: []string{"monitoring", "cluster", "rule", "update", "6", "3", "--request-file", requestFile}},
		{name: "channel create", operationID: "monitoring.notification-channel.create", method: http.MethodPost, path: "/api/v1/monitoring/notification-channels", args: []string{"monitoring", "notification-channel", "create", "--request-file", requestFile}},
		{name: "channel update", operationID: "monitoring.notification-channel.update", method: http.MethodPut, path: "/api/v1/monitoring/notification-channels/4", args: []string{"monitoring", "notification-channel", "update", "4", "--request-file", requestFile}},
		{name: "channel test", operationID: "monitoring.notification-channel.test", method: http.MethodPost, path: "/api/v1/monitoring/notification-channels/4/test", args: []string{"monitoring", "notification-channel", "test", "4", "--receiver-user-id", "8"}, assertBody: func(t *testing.T, body map[string]any) {
			if body["receiver_user_id"] != float64(8) {
				t.Fatalf("接收用户正文错误: %#v", body)
			}
		}},
		{name: "channel draft test", operationID: "monitoring.notification-channel.test-draft", method: http.MethodPost, path: "/api/v1/monitoring/notification-channels/test", args: []string{"monitoring", "notification-channel", "test-draft", "--request-file", requestFile}},
		{name: "channel connection test", operationID: "monitoring.notification-channel.test-connection", method: http.MethodPost, path: "/api/v1/monitoring/notification-channels/test-connection", args: []string{"monitoring", "notification-channel", "test-connection", "--request-file", requestFile}},
		{name: "route create", operationID: "monitoring.notification-route.create", method: http.MethodPost, path: "/api/v1/monitoring/notification-routes", args: []string{"monitoring", "notification-route", "create", "--request-file", requestFile}},
		{name: "route update", operationID: "monitoring.notification-route.update", method: http.MethodPut, path: "/api/v1/monitoring/notification-routes/5", args: []string{"monitoring", "notification-route", "update", "5", "--request-file", requestFile}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v1/capabilities" {
					writeMonitoringCapabilities(t, writer, test.operationID)
					return
				}
				if request.Method != test.method || request.URL.Path != test.path {
					t.Fatalf("监控请求错误: got=%s %s want=%s %s", request.Method, request.URL.Path, test.method, test.path)
				}
				if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "monitoring-test-key" {
					t.Fatalf("监控写请求安全头错误: %#v", request.Header)
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatalf("读取监控请求失败: %v", err)
				}
				if test.assertBody != nil {
					test.assertBody(t, body)
				}
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"ok": true}})
			}))
			defer server.Close()
			store := newExecutionTestStore(t, server.URL, "test-token")
			args := append(append([]string{}, test.args...), "--confirm", "--idempotency-key", "monitoring-test-key")
			stdout, stderr, exitCode := runMonitoringWriteCommand(t, store, args...)
			if exitCode != int(clioutput.ExitSuccess) || !strings.Contains(stdout, `"operation_id":"`+test.operationID+`"`) {
				t.Fatalf("监控命令失败: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
			}
		})
	}
}

func TestMonitoringWriteRequiresConfirmationBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	requestFile := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(requestFile, []byte(`{"name":"test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runMonitoringWriteCommand(t, store, "monitoring", "alert-policy", "create", "--request-file", requestFile)
	if exitCode != int(clioutput.ExitConflict) || calls != 0 {
		t.Fatalf("缺少确认时仍发起请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func TestMonitoringSilenceValidatesDurationBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runMonitoringWriteCommand(t, store, "monitoring", "alert", "silence", "1", "--duration-minutes", "0", "--confirm")
	if exitCode != int(clioutput.ExitUsage) || calls != 0 {
		t.Fatalf("非法静默时长仍发起请求: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func TestRootCommandRegistersMonitoringWriteCommands(t *testing.T) {
	root := newRootCommand(func() error { return nil })
	paths := [][]string{
		{"monitoring", "alert-policy", "create"}, {"monitoring", "alert-policy", "update"},
		{"monitoring", "alert-instance", "ack"}, {"monitoring", "alert-instance", "silence"}, {"monitoring", "alert-instance", "close"},
		{"monitoring", "alert", "ack"}, {"monitoring", "alert", "silence"}, {"monitoring", "cluster", "rule", "update"},
		{"monitoring", "notification-channel", "create"}, {"monitoring", "notification-channel", "update"}, {"monitoring", "notification-channel", "test"},
		{"monitoring", "notification-channel", "test-draft"}, {"monitoring", "notification-channel", "test-connection"},
		{"monitoring", "notification-route", "create"}, {"monitoring", "notification-route", "update"},
	}
	for _, path := range paths {
		found, remaining, err := root.Find(path)
		if err != nil || len(remaining) != 0 || found.Name() != path[len(path)-1] {
			t.Fatalf("监控命令未完整注册: path=%v found=%s remaining=%v err=%v", path, found.CommandPath(), remaining, err)
		}
	}
}

func assertMonitoringNote(expected string) func(*testing.T, map[string]any) {
	return func(t *testing.T, body map[string]any) {
		if body["note"] != expected {
			t.Fatalf("处理说明正文错误: %#v", body)
		}
	}
}

func assertMonitoringDuration(expected int) func(*testing.T, map[string]any) {
	return func(t *testing.T, body map[string]any) {
		if body["duration_minutes"] != float64(expected) {
			t.Fatalf("静默时长正文错误: %#v", body)
		}
	}
}

func runMonitoringWriteCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addMonitoringWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeMonitoringCapabilities(t *testing.T, writer http.ResponseWriter, operationID string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": "R1"}},
	}}); err != nil {
		t.Fatalf("写入监控能力响应失败: %v", err)
	}
}
