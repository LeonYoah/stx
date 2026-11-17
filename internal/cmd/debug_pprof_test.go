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

func TestDebugPprofHeapDownload(t *testing.T) {
	const payload = "heap-profile-bytes"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDebugPprofCapability(t, writer, true)
			return
		}
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/debug/pprof/heap" {
			t.Fatalf("pprof request is incorrect: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing bearer token")
		}
		_, _ = writer.Write([]byte(payload))
	}))
	defer server.Close()

	outputFile := filepath.Join(t.TempDir(), "heap.pprof")
	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runDebugPprofCommand(t, store, "debug", "pprof", "heap", "--file", outputFile)
	if exitCode != int(clioutput.ExitSuccess) || strings.TrimSpace(stderr) != "" {
		t.Fatalf("pprof download failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	got, err := os.ReadFile(outputFile)
	if err != nil || string(got) != payload {
		t.Fatalf("pprof file content=%q err=%v", got, err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	data, _ := result["data"].(map[string]any)
	if data["profile"] != "heap" || data["file"] != outputFile || data["size"] != float64(len(payload)) {
		t.Fatalf("result data incorrect: %#v", result["data"])
	}
}

func TestDebugPprofTraceDefaultsToFiveSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDebugPprofCapability(t, writer, true)
			return
		}
		if request.URL.Path != "/api/v1/debug/pprof/trace" || request.URL.Query().Get("seconds") != "5" {
			t.Fatalf("trace default seconds incorrect: %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte("trace"))
	}))
	defer server.Close()

	outputFile := filepath.Join(t.TempDir(), "trace.out")
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDebugPprofCommand(t, store, "debug", "pprof", "trace", "--file", outputFile)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("trace pprof failed: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestDebugPprofCPUDefaultsToThirtySeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDebugPprofCapability(t, writer, true)
			return
		}
		if request.URL.Path != "/api/v1/debug/pprof/profile" || request.URL.Query().Get("seconds") != "30" {
			t.Fatalf("cpu default seconds incorrect: %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte("cpu"))
	}))
	defer server.Close()

	outputFile := filepath.Join(t.TempDir(), "cpu.pprof")
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDebugPprofCommand(t, store, "debug", "pprof", "cpu", "--file", outputFile)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("cpu default pprof failed: code=%d stderr=%s", exitCode, stderr)
	}
}

func TestDebugPprofCPUMapsToProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDebugPprofCapability(t, writer, true)
			return
		}
		if request.URL.Path != "/api/v1/debug/pprof/profile" || request.URL.Query().Get("seconds") != "5" {
			t.Fatalf("cpu profile path incorrect: %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte("cpu"))
	}))
	defer server.Close()

	outputFile := filepath.Join(t.TempDir(), "cpu.pprof")
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDebugPprofCommand(t, store, "debug", "pprof", "cpu", "--seconds", "5", "--file", outputFile)
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("cpu pprof failed: code=%d stderr=%s", exitCode, stderr)
	}
}

// TestDebugPprofRejectsNonAdminCapability 确认 capabilities 拒绝非管理员时 CLI 不发起下载。
// TestDebugPprofRejectsNonAdminCapability checks the CLI does not download when capabilities deny a non-admin.
func TestDebugPprofRejectsNonAdminCapability(t *testing.T) {
	var downloads int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDebugPprofCapability(t, writer, false)
			return
		}
		downloads++
	}))
	defer server.Close()

	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDebugPprofCommand(t, store, "debug", "pprof", "heap", "--file", filepath.Join(t.TempDir(), "heap.pprof"))
	if exitCode != int(clioutput.ExitPermission) || downloads != 0 {
		t.Fatalf("non-admin must not download pprof: code=%d downloads=%d stderr=%s", exitCode, downloads, stderr)
	}
}

func TestDebugPprofRejectsUnknownName(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDebugPprofCommand(t, store, "debug", "pprof", "secrets", "--file", filepath.Join(t.TempDir(), "x"))
	if exitCode != int(clioutput.ExitUsage) || calls != 0 {
		t.Fatalf("unknown profile must not call server: code=%d calls=%d stderr=%s", exitCode, calls, stderr)
	}
}

func TestDebugPprofRejectsExistingFile(t *testing.T) {
	var downloads int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writeDebugPprofCapability(t, writer, true)
			return
		}
		downloads++
	}))
	defer server.Close()
	outputFile := filepath.Join(t.TempDir(), "heap.pprof")
	if err := os.WriteFile(outputFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runDebugPprofCommand(t, store, "debug", "pprof", "heap", "--file", outputFile)
	if exitCode != int(clioutput.ExitConflict) || downloads != 0 {
		t.Fatalf("existing file must not be overwritten: code=%d downloads=%d stderr=%s", exitCode, downloads, stderr)
	}
}

func runDebugPprofCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	storeProvider := func() (*cliConfig.Store, error) { return store, nil }
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	root.AddCommand(newDebugCommand(storeProvider))
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writeDebugPprofCapability(t *testing.T, writer http.ResponseWriter, allowed bool) {
	t.Helper()
	denialCode := ""
	if !allowed {
		denialCode = "admin_required"
	}
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{
			"operation_id": "debug.pprof.get", "revision": 1, "allowed": allowed,
			"denial_code": denialCode, "mode": "download", "risk": "R0",
		}},
	}}); err != nil {
		t.Fatalf("write pprof capability: %v", err)
	}
}
