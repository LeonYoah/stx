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

package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/agent/internal/installer"
)

func TestHandleSTXJavaProxyInspectCheckpointAcceptsConfigBackedRequest(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			return
		case "/api/v1/storage/checkpoint/inspect":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":          true,
				"message":     "inspect ok",
				"storageType": "hdfs",
				"path":        "/tmp/checkpoint",
				"fileName":    "checkpoint",
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("STX_JAVA_PROXY_ENDPOINT", server.URL)

	result, err := handleSTXJavaProxyInspectCheckpoint(context.Background(), map[string]string{
		"install_dir":  t.TempDir(),
		"version":      "2.3.13",
		"path":         "/tmp/checkpoint",
		"storage_type": string(installer.CheckpointStorageLocalFile),
		"namespace":    "/tmp",
	})
	if err != nil {
		t.Fatalf("handle inspect checkpoint returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %#v", result)
	}
	if received["contentBase64"] != nil {
		t.Fatalf("expected config-backed request without inline content, got %#v", received)
	}
	configMap, ok := received["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected config payload, got %#v", received["config"])
	}
	if configMap["namespace"] != "/tmp/" {
		t.Fatalf("expected normalized namespace /tmp/, got %#v", configMap["namespace"])
	}
	if received["path"] != "/tmp/checkpoint" {
		t.Fatalf("expected path /tmp/checkpoint, got %#v", received["path"])
	}
}

func TestHandleSTXJavaProxyInspectCheckpointAcceptsBase64Request(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			return
		case "/api/v1/storage/checkpoint/inspect":
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"message": "inspect ok",
				"path":    "/tmp/checkpoint",
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("STX_JAVA_PROXY_ENDPOINT", server.URL)

	result, err := handleSTXJavaProxyInspectCheckpoint(context.Background(), map[string]string{
		"install_dir":    t.TempDir(),
		"path":           "/tmp/checkpoint",
		"content_base64": "Y2hlY2twb2ludA==",
		"storage_type":   string(installer.CheckpointStorageLocalFile),
		"namespace":      "/tmp",
	})
	if err != nil {
		t.Fatalf("handle inspect checkpoint returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful result, got %#v", result)
	}
	if received["contentBase64"] != "Y2hlY2twb2ludA==" {
		t.Fatalf("expected inline content, got %#v", received["contentBase64"])
	}
}

func TestHandleSyncLocalLogsReadsAgentStdoutBuffer(t *testing.T) {
	platformJobID := "local-log-buffer"
	workDir := t.TempDir()
	logFile := workDir + "/job.log"
	if err := os.WriteFile(logFile, []byte("INFO boot\nWARN slow\nERROR failed\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	syncLocalRuntimeStore.Store(platformJobID, &syncLocalRuntime{
		Meta: syncLocalRuntimeMeta{
			PID:        12345,
			ConfigFile: workDir + "/job.conf",
			LogFile:    logFile,
			StatusFile: workDir + "/status.json",
		},
		Status: syncLocalStatusFile{
			State: "running",
		},
	})
	defer syncLocalRuntimeStore.Delete(platformJobID)

	result, err := handleSyncLocalLogs(context.Background(), map[string]string{
		"platform_job_id": platformJobID,
		"limit_bytes":     "64",
	})
	if err != nil {
		t.Fatalf("handle sync local logs returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result.Message), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	logs := payload["logs"].(string)
	if !strings.Contains(logs, "INFO boot") || !strings.Contains(logs, "WARN slow") || !strings.Contains(logs, "ERROR failed") {
		t.Fatalf("expected recent stdout/stderr lines, got %q", logs)
	}
}

func TestHandleSyncLocalStopMarksRuntimeCanceled(t *testing.T) {
	platformJobID := "local-stop-runtime"
	workDir := t.TempDir()
	logFile := workDir + "/job.log"
	statusFile := workDir + "/status.json"
	if err := os.WriteFile(logFile, []byte("INFO start\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	runtime := &syncLocalRuntime{
		Meta: syncLocalRuntimeMeta{
			LogFile:    logFile,
			StatusFile: statusFile,
		},
		Status: syncLocalStatusFile{State: "running"},
	}
	syncLocalRuntimeStore.Store(platformJobID, runtime)
	defer syncLocalRuntimeStore.Delete(platformJobID)

	result, err := handleSyncLocalStop(context.Background(), map[string]string{
		"platform_job_id": platformJobID,
		"pid":             "0",
	})
	if err != nil {
		t.Fatalf("handle sync local stop returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}

	loaded, ok := loadSyncLocalRuntime(platformJobID)
	if !ok {
		t.Fatalf("expected runtime to remain available")
	}
	loaded.mu.RLock()
	defer loaded.mu.RUnlock()
	if loaded.Status.State != "canceled" {
		t.Fatalf("expected canceled status, got %q", loaded.Status.State)
	}
	if loaded.Status.ExitCode != 130 {
		t.Fatalf("expected exit code 130, got %d", loaded.Status.ExitCode)
	}
	if loaded.Status.FinishedAt == "" {
		t.Fatalf("expected finished time to be set")
	}
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(content), "stopped by user") {
		t.Fatalf("expected stop marker in log file, got %q", string(content))
	}
}

func TestHandleSyncLocalStatusDetectsFinishedRuntimeWithoutProcess(t *testing.T) {
	platformJobID := "local-status-runtime"
	workDir := t.TempDir()
	syncLocalRuntimeStore.Store(platformJobID, &syncLocalRuntime{
		Meta: syncLocalRuntimeMeta{
			PID:        999999,
			ConfigFile: workDir + "/job.conf",
			StatusFile: workDir + "/status.json",
		},
		Status: syncLocalStatusFile{
			State:      "success",
			ExitCode:   0,
			FinishedAt: time.Now().Format(time.RFC3339),
			Message:    "done",
		},
	})
	defer syncLocalRuntimeStore.Delete(platformJobID)

	result, err := handleSyncLocalStatus(context.Background(), map[string]string{
		"platform_job_id": platformJobID,
		"pid":             "999999",
	})
	if err != nil {
		t.Fatalf("handle sync local status returned error: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result.Message), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["running"].(bool) {
		t.Fatalf("expected runtime to be finished")
	}
	if payload["status"].(string) != "success" {
		t.Fatalf("expected success status, got %#v", payload["status"])
	}
}
