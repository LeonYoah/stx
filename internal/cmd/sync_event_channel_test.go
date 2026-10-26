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
	"strings"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func runSyncWriteCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newGeneratedCommands(storeProvider)...)
	addSyncWriteCommands(root, storeProvider)
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func TestSyncEventChannelUpdateSendsBodyAndSafetyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/capabilities" {
			writeConfigCapabilities(t, writer, "sync.event-channel.update", "R2")
			return
		}
		if request.Method != http.MethodPut || request.URL.Path != "/api/v1/sync/event-channels/3" {
			t.Fatalf("事件通道更新地址错误 / invalid event channel target: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-STX-Confirm") != "true" || request.Header.Get("Idempotency-Key") != "evt-key" || request.Header.Get("X-STX-Confirmation-ID") != "confirm-1" {
			t.Fatalf("事件通道安全请求头错误 / invalid safety headers: %#v", request.Header)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["enabled"] != true || body["rotate_token"] != false {
			t.Fatalf("事件通道更新正文错误 / invalid body: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"cluster_id": 3, "enabled": true, "token_set": true}})
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runSyncWriteCommand(t, store, "sync", "event-channel", "update", "3", "--enabled=true", "--confirm", "--idempotency-key", "evt-key", "--confirmation-id", "confirm-1")
	if exitCode != int(clioutput.ExitSuccess) || !strings.Contains(stdout, `"operation_id":"sync.event-channel.update"`) {
		t.Fatalf("事件通道更新命令失败 / command failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
}

func TestSyncEventChannelUpdateValidatesFlagsBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")

	cases := [][]string{
		{"sync", "event-channel", "update", "3", "--confirm"},
		{"sync", "event-channel", "update", "3", "--enabled=maybe", "--confirm"},
		{"sync", "event-channel", "update", "abc", "--enabled=true", "--confirm"},
	}
	for _, args := range cases {
		_, stderr, exitCode := runSyncWriteCommand(t, store, args...)
		if exitCode != int(clioutput.ExitUsage) {
			t.Fatalf("expected usage error for %v, got code=%d stderr=%s", args, exitCode, stderr)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid flags must not reach the network, calls=%d", calls)
	}
}
