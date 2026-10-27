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
	"reflect"
	"strings"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestSyncTaskAlertSetSendsBodyAndSafetyHeaders(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			writeConfigCapabilities(t, writer, "sync.task.alert.set", "R1")
			return
		}
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/sync/tasks/4/alert" {
			t.Fatalf("任务告警地址错误 / invalid target: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "alert-key" {
			t.Fatalf("任务告警安全请求头错误 / invalid safety headers: %#v", request.Header)
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"configured": true, "enabled": true}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncWriteCommand(t, store, "sync", "task", "alert", "set", "4",
		"--rules", "sync_job_failed,sync_stop_timeout", "--rules", "sync_job_failed",
		"--channel", "2", "--channel", "3", "--receiver", "9", "--notify-owner", "--cooldown", "0", "--no-recovery",
		"--confirm", "--idempotency-key", "alert-key")
	if exitCode != int(clioutput.ExitSuccess) || !strings.Contains(stdout, `"operation_id":"sync.task.alert.set"`) {
		t.Fatalf("任务告警命令失败 / command failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !reflect.DeepEqual(body["rules"], []any{"sync_job_failed", "sync_stop_timeout"}) ||
		!reflect.DeepEqual(body["channel_ids"], []any{float64(2), float64(3)}) ||
		!reflect.DeepEqual(body["receiver_user_ids"], []any{float64(9)}) ||
		body["notify_task_owner"] != true || body["send_recovery"] != false || body["enabled"] != true || body["cooldown_minutes"] != float64(0) {
		t.Fatalf("任务告警正文错误 / invalid body: %#v", body)
	}
}

func TestSyncTaskAlertSetDisableKeepsCurrentConfig(t *testing.T) {
	var methods []string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			writeConfigCapabilities(t, writer, "sync.task.alert.set", "R1")
			return
		}
		methods = append(methods, request.Method)
		if request.Method == http.MethodGet {
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
				"configured": true, "enabled": true, "rules": []string{"sync_submit_failed"}, "channel_ids": []uint{5},
				"receiver_user_ids": []uint{}, "notify_task_owner": true, "cooldown_minutes": 30, "send_recovery": true,
			}})
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"configured": true, "enabled": false}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runSyncWriteCommand(t, store, "sync", "task", "alert", "set", "4", "--disable", "--confirm")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("停用命令失败 / disable failed: code=%d stderr=%s", exitCode, stderr)
	}
	if !reflect.DeepEqual(methods, []string{http.MethodGet, http.MethodPut}) {
		t.Fatalf("停用应先读后写 / disable reads then writes: %v", methods)
	}
	if body["enabled"] != false || !reflect.DeepEqual(body["rules"], []any{"sync_submit_failed"}) || body["cooldown_minutes"] != float64(30) || body["notify_task_owner"] != true {
		t.Fatalf("停用应保留其余配置 / disable keeps the rest: %#v", body)
	}
}

func TestSyncTaskAlertSetValidatesBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")

	cases := [][]string{
		{"sync", "task", "alert", "set", "4", "--channel", "2", "--confirm"},
		{"sync", "task", "alert", "set", "4", "--rules", "sync_job_failed", "--confirm"},
		{"sync", "task", "alert", "set", "abc", "--rules", "sync_job_failed", "--channel", "2", "--confirm"},
		{"sync", "task", "alert", "set", "4", "--rules", "sync_job_failed", "--channel", "2", "--cooldown", "-1", "--confirm"},
	}
	for _, args := range cases {
		_, stderr, exitCode := runSyncWriteCommand(t, store, args...)
		if exitCode != int(clioutput.ExitUsage) {
			t.Fatalf("expected usage error for %v, got code=%d stderr=%s", args, exitCode, stderr)
		}
	}
	if _, _, exitCode := runSyncWriteCommand(t, store, "sync", "task", "alert", "set", "4", "--rules", "sync_job_failed", "--channel", "2"); exitCode == int(clioutput.ExitSuccess) {
		t.Fatalf("missing --confirm must fail")
	}
	if calls != 0 {
		t.Fatalf("invalid input must not reach the network, calls=%d", calls)
	}
}

func TestSyncTaskAlertCommandsHaveGetAndNoDelete(t *testing.T) {
	provider := func() (*cliConfig.Store, error) { return nil, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(provider)...)
	addSyncWriteCommands(root, provider)
	alert := childCommand(childCommand(childCommand(root, "sync"), "task"), "alert")
	if alert == nil {
		t.Fatal("sync task alert command is missing")
	}
	names := map[string]bool{}
	for _, child := range alert.Commands() {
		names[child.Name()] = true
	}
	if !names["get"] || !names["set"] || names["delete"] || names["remove"] {
		t.Fatalf("expected get and set only, got %v", names)
	}
	var stdout, stderr bytes.Buffer
	if code := executeCommand(root, []string{"sync", "task", "alert", "delete", "4"}, &stdout, &stderr); code == int(clioutput.ExitSuccess) {
		t.Fatalf("delete must not exist in the CLI")
	}
}

func TestMonitoringAlertPolicyListFiltersJobAlerts(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			writeConfigCapabilities(t, writer, "monitoring.alert-policy.list", "R0")
			return
		}
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/monitoring/alert-policies" {
			t.Fatalf("策略列表地址错误 / invalid target: %s %s", request.Method, request.URL.Path)
		}
		query = request.URL.RawQuery
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"total": 1, "policies": []map[string]any{{"id": 3, "policy_type": "sync_job", "task_id": 4}}}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncWriteCommand(t, store, "monitoring", "alert-policy", "list", "--policy_type", "sync_job", "--task_id", "4")
	if exitCode != int(clioutput.ExitSuccess) || !strings.Contains(stdout, `"sync_job"`) {
		t.Fatalf("策略列表命令失败 / command failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if query != "policy_type=sync_job&task_id=4" {
		t.Fatalf("筛选参数没有带上 / filters not sent: %q", query)
	}
}
