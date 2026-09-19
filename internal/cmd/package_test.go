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
	"sync/atomic"
	"testing"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

func TestPackageUploadStreamsMultipartAndSafetyHeaders(t *testing.T) {
	var uploadCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/capabilities":
			writePackageCapabilities(t, writer, "package.upload")
		case "/api/v1/packages/upload":
			uploadCalls.Add(1)
			if request.Header.Get("X-STX-Client") != "cli" || request.Header.Get("X-STX-Confirm") != "true" {
				t.Fatalf("安全请求头错误 / invalid safety headers: %#v", request.Header)
			}
			if request.Header.Get("Idempotency-Key") != "upload-key" {
				t.Fatalf("幂等键错误 / invalid idempotency key: %q", request.Header.Get("Idempotency-Key"))
			}
			if err := request.ParseMultipartForm(1024); err != nil {
				t.Fatalf("解析 multipart 失败 / parse multipart failed: %v", err)
			}
			if request.FormValue("version") != "9.9.91" {
				t.Fatalf("版本字段错误 / invalid version field: %q", request.FormValue("version"))
			}
			file, header, err := request.FormFile("file")
			if err != nil {
				t.Fatalf("读取上传文件失败 / read upload file failed: %v", err)
			}
			defer file.Close()
			content, _ := io.ReadAll(file)
			if header.Filename != "apache-seatunnel-9.9.91-bin.tar.gz" || string(content) != "package-data" {
				t.Fatalf("上传文件错误 / invalid uploaded file: name=%s content=%q", header.Filename, content)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"version": "9.9.91", "is_local": true}})
		default:
			t.Fatalf("意外请求 / unexpected request: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "apache-seatunnel-9.9.91-bin.tar.gz")
	if err := os.WriteFile(path, []byte("package-data"), 0o600); err != nil {
		t.Fatalf("创建测试文件失败 / create test file failed: %v", err)
	}
	store := newExecutionTestStore(t, server.URL, "test-token")
	stdout, stderr, exitCode := runPackageCommand(t, store, "package", "upload", path, "--version", "9.9.91", "--confirm", "--idempotency-key", "upload-key")
	if exitCode != int(clioutput.ExitSuccess) || uploadCalls.Load() != 1 {
		t.Fatalf("上传命令失败 / upload command failed: code=%d stdout=%s stderr=%s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, `"operation_id":"package.upload"`) {
		t.Fatalf("stdout 缺少上传结果 / upload result missing: %s", stdout)
	}
	events := decodeNDJSON(t, stderr)
	if len(events) != 1 || events[0]["event"] != "warning" {
		t.Fatalf("影响提示错误 / invalid impact event: %#v", events)
	}
}

func TestPackageWriteRequiresConfirmBeforeNetworkRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runPackageCommand(t, store, "package", "delete", "9.9.91")
	if exitCode != int(clioutput.ExitConflict) || calls.Load() != 0 {
		t.Fatalf("缺少确认时仍发起请求 / request sent without confirmation: code=%d calls=%d stderr=%s", exitCode, calls.Load(), stderr)
	}
}

func TestPackageChunkUsesDerivedIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/capabilities" {
			writePackageCapabilities(t, writer, "package.upload.chunk")
			return
		}
		if request.Header.Get("Idempotency-Key") != "session-key-chunk-1" {
			t.Fatalf("分片幂等键错误 / invalid chunk idempotency key: %q", request.Header.Get("Idempotency-Key"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"upload_id": "upload_12345678", "completed": true, "received_chunks": 2, "total_chunks": 2,
		}})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "part-001")
	if err := os.WriteFile(path, []byte("part"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newExecutionTestStore(t, server.URL, "test-token")
	_, stderr, exitCode := runPackageCommand(t, store,
		"package", "upload", "chunk", path, "--version", "9.9.92", "--upload-id", "upload_12345678",
		"--chunk-index", "1", "--total-chunks", "2", "--total-size", "8", "--confirm", "--idempotency-key", "session-key")
	if exitCode != int(clioutput.ExitSuccess) {
		t.Fatalf("分片上传失败 / chunk upload failed: code=%d stderr=%s", exitCode, stderr)
	}
}

func runPackageCommand(t *testing.T, store *cliConfig.Store, args ...string) (string, string, int) {
	t.Helper()
	root := &cobra.Command{Use: "stx", SilenceUsage: true, SilenceErrors: true}
	clioutput.AddGlobalFlags(root)
	packageCommand := &cobra.Command{Use: "package"}
	packageCommand.AddCommand(&cobra.Command{Use: "download"})
	root.AddCommand(packageCommand)
	addPackageWriteCommands(root, func() (*cliConfig.Store, error) { return store, nil })
	var stdout, stderr bytes.Buffer
	exitCode := executeCommand(root, args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exitCode
}

func writePackageCapabilities(t *testing.T, writer http.ResponseWriter, operationID string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
		"operations": []map[string]any{{"operation_id": operationID, "revision": 1, "allowed": true, "mode": "normal", "risk": "R1"}},
	}}); err != nil {
		t.Fatalf("写入能力响应失败 / write capability response failed: %v", err)
	}
}
