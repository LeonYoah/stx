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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
)

type syncLocalStatusFile struct {
	State      string `json:"state"`
	ExitCode   int    `json:"exit_code"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	Message    string `json:"message,omitempty"`
}

type syncLocalRuntimeMeta struct {
	PID        int    `json:"pid"`
	ConfigFile string `json:"config_file"`
	LogFile    string `json:"log_file"`
	StatusFile string `json:"status_file"`
	RunnerFile string `json:"runner_file"`
}

type syncLocalRuntime struct {
	mu     sync.RWMutex
	Meta   syncLocalRuntimeMeta
	Status syncLocalStatusFile
}

var syncLocalRuntimeStore sync.Map

const syncLocalRunnerTemplate = `#!/usr/bin/env bash
set -euo pipefail

STATUS_FILE={{.StatusFile}}
LOG_FILE={{.LogFile}}
CONFIG_FILE={{.ConfigFile}}
INSTALL_DIR={{.InstallDir}}

write_status() {
  local state="$1"
  local exit_code="$2"
  local message="${3:-}"
  local finished_at=""
  if [[ "$state" != "running" ]]; then
    finished_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  fi
  python3 - "$STATUS_FILE" "$state" "$exit_code" "$message" "$finished_at" <<'PY'
import json
import sys

path, state, exit_code, message, finished_at = sys.argv[1:6]
data = {"state": state, "exit_code": int(exit_code)}
if finished_at:
    data["finished_at"] = finished_at
if message:
    data["message"] = message
with open(path, "w", encoding="utf-8") as fh:
    json.dump(data, fh)
PY
}

cleanup_on_signal() {
  if [[ -n "${child_pid:-}" ]]; then
    kill -TERM "$child_pid" 2>/dev/null || true
    wait "$child_pid" || true
  fi
  echo "[STX] local job stopped by user" >> "$LOG_FILE"
  write_status "canceled" "130" "stopped by STX"
  exit 130
}

trap cleanup_on_signal TERM INT

echo "[STX] local job bootstrap started at $(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> "$LOG_FILE"
cd "$INSTALL_DIR"
./bin/seatunnel.sh -m local -c "$CONFIG_FILE" >> "$LOG_FILE" 2>&1 &
child_pid=$!
wait "$child_pid"
exit_code=$?
if [[ "$exit_code" -eq 0 ]]; then
  write_status "success" "0" ""
else
  write_status "failed" "$exit_code" "seatunnel.sh exited with code $exit_code"
fi
`

type syncLocalRunnerTemplateData struct {
	StatusFile string
	LogFile    string
	ConfigFile string
	InstallDir string
}

func handleSyncLocalRun(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	installDir := strings.TrimSpace(params["install_dir"])
	platformJobID := strings.TrimSpace(params["platform_job_id"])
	content := params["content"]
	contentFormat := strings.TrimSpace(params["content_format"])
	if installDir == "" || platformJobID == "" || strings.TrimSpace(content) == "" {
		return &PrecheckResult{Success: false, Message: "install_dir, platform_job_id and content are required"}, nil
	}
	if contentFormat == "" {
		contentFormat = "hocon"
	}

	workDir := syncLocalWorkDir(installDir, platformJobID)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	ext := ".conf"
	if contentFormat == "json" {
		ext = ".json"
	}
	configFile := filepath.Join(workDir, "job"+ext)
	logFile := filepath.Join(workDir, "job.log")
	statusFile := filepath.Join(workDir, "status.json")
	runnerFile := filepath.Join(workDir, "runner.sh")
	metaFile := filepath.Join(workDir, "meta.json")

	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		return nil, err
	}
	if _, err := os.Stat(logFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.WriteFile(logFile, []byte{}, 0o644); err != nil {
			return nil, err
		}
	}
	runtime := &syncLocalRuntime{
		Meta: syncLocalRuntimeMeta{
			ConfigFile: configFile,
			LogFile:    logFile,
			StatusFile: statusFile,
			RunnerFile: runnerFile,
		},
		Status: syncLocalStatusFile{
			State:     "running",
			ExitCode:  0,
			StartedAt: time.Now().Format(time.RFC3339),
		},
	}
	if err := writeSyncLocalStatus(statusFile, runtime.Status); err != nil {
		return nil, err
	}
	if err := writeSyncLocalRunner(runnerFile, syncLocalRunnerTemplateData{
		StatusFile: shellQuote(statusFile),
		LogFile:    shellQuote(logFile),
		ConfigFile: shellQuote(configFile),
		InstallDir: shellQuote(installDir),
	}); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", fmt.Sprintf("nohup %s >/dev/null 2>&1 & echo $!", shellQuote(runnerFile)))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("start local sync job: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil || pid <= 0 {
		return nil, fmt.Errorf("start local sync job: invalid pid output %q", strings.TrimSpace(string(output)))
	}
	runtime.Meta.PID = pid
	if err := writeSyncLocalMeta(metaFile, runtime.Meta); err != nil {
		return nil, err
	}
	syncLocalRuntimeStore.Store(platformJobID, runtime)

	payload, _ := json.Marshal(map[string]interface{}{
		"pid":         pid,
		"config_file": configFile,
		"log_file":    logFile,
		"status_file": statusFile,
	})
	return &PrecheckResult{Success: true, Message: string(payload)}, nil
}

func handleSyncLocalStatus(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	platformJobID := strings.TrimSpace(params["platform_job_id"])
	installDir := strings.TrimSpace(params["install_dir"])
	pid, _ := strconv.Atoi(strings.TrimSpace(params["pid"]))
	runtime, err := loadOrCreateSyncLocalRuntime(installDir, platformJobID)
	if err != nil {
		return &PrecheckResult{Success: false, Message: err.Error()}, nil
	}
	runtime.mu.Lock()
	if runtime.Meta.PID > 0 {
		pid = runtime.Meta.PID
	}
	if diskStatus, statusErr := readSyncLocalStatus(runtime.Meta.StatusFile); statusErr == nil {
		runtime.Status = diskStatus
	}
	status := runtime.Status
	runtime.mu.Unlock()

	alive := pid > 0 && syscall.Kill(pid, 0) == nil
	if alive {
		status.State = "running"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"running":     alive,
		"status":      status.State,
		"exit_code":   status.ExitCode,
		"message":     status.Message,
		"finished_at": status.FinishedAt,
	})
	return &PrecheckResult{Success: true, Message: string(payload)}, nil
}

func handleSyncLocalLogs(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	platformJobID := strings.TrimSpace(params["platform_job_id"])
	installDir := strings.TrimSpace(params["install_dir"])
	if platformJobID == "" {
		return &PrecheckResult{Success: false, Message: "platform_job_id is required"}, nil
	}
	runtime, err := loadOrCreateSyncLocalRuntime(installDir, platformJobID)
	if err != nil {
		return &PrecheckResult{Success: false, Message: err.Error()}, nil
	}
	offset := strings.TrimSpace(params["offset"])
	limitBytes, _ := strconv.Atoi(strings.TrimSpace(params["limit_bytes"]))
	var (
		logs       string
		nextOffset int64
		fileSize   int64
	)
	logs, nextOffset, fileSize, err = readSyncLogChunk(
		runtime.Meta.LogFile,
		offset,
		limitBytes,
		strings.TrimSpace(params["keyword"]),
		strings.TrimSpace(params["level"]),
	)
	if err != nil {
		return nil, err
	}
	status, statusErr := readSyncLocalStatus(runtime.Meta.StatusFile)
	if statusErr == nil {
		runtime.mu.Lock()
		runtime.Status = status
		runtime.mu.Unlock()
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"logs":        logs,
		"status":      runtime.Status.State,
		"exit_code":   runtime.Status.ExitCode,
		"finished_at": runtime.Status.FinishedAt,
		"message":     runtime.Status.Message,
		"next_offset": strconv.FormatInt(nextOffset, 10),
		"file_size":   fileSize,
	})
	return &PrecheckResult{Success: true, Message: string(payload)}, nil
}

func handleSyncJobLogs(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	installDir := strings.TrimSpace(params["install_dir"])
	platformJobID := strings.TrimSpace(params["platform_job_id"])
	engineJobID := strings.TrimSpace(params["engine_job_id"])
	if installDir == "" {
		return &PrecheckResult{Success: false, Message: "install_dir is required"}, nil
	}
	jobIDs := make([]string, 0, 2)
	if platformJobID != "" {
		jobIDs = append(jobIDs, platformJobID)
	}
	if engineJobID != "" && engineJobID != platformJobID {
		jobIDs = append(jobIDs, engineJobID)
	}
	if len(jobIDs) == 0 {
		return &PrecheckResult{Success: false, Message: "platform_job_id or engine_job_id is required"}, nil
	}
	offset := strings.TrimSpace(params["offset"])
	limitBytes, _ := strconv.Atoi(strings.TrimSpace(params["limit_bytes"]))
	logFile, err := resolveClusterJobLogFile(installDir, jobIDs)
	if err != nil {
		return &PrecheckResult{Success: false, Message: err.Error()}, nil
	}
	var (
		logs       string
		nextOffset int64
		fileSize   int64
	)
	logs, nextOffset, fileSize, err = readSyncLogChunk(
		logFile,
		offset,
		limitBytes,
		strings.TrimSpace(params["keyword"]),
		strings.TrimSpace(params["level"]),
	)
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"logs":        logs,
		"path":        logFile,
		"next_offset": strconv.FormatInt(nextOffset, 10),
		"file_size":   fileSize,
	})
	return &PrecheckResult{Success: true, Message: string(payload)}, nil
}

func handleSyncLocalStop(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	platformJobID := strings.TrimSpace(params["platform_job_id"])
	installDir := strings.TrimSpace(params["install_dir"])
	pid, _ := strconv.Atoi(strings.TrimSpace(params["pid"]))
	runtime, err := loadOrCreateSyncLocalRuntime(installDir, platformJobID)
	if err == nil {
		runtime.mu.RLock()
		if runtime.Meta.PID > 0 {
			pid = runtime.Meta.PID
		}
		runtime.mu.RUnlock()
	}
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if syscall.Kill(pid, 0) != nil {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if syscall.Kill(pid, 0) == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	if runtime != nil {
		runtime.mu.Lock()
		runtime.Status = syncLocalStatusFile{
			State:      "canceled",
			ExitCode:   130,
			FinishedAt: time.Now().Format(time.RFC3339),
			Message:    "stopped by STX",
		}
		runtime.mu.Unlock()
		if runtime.Meta.StatusFile != "" {
			_ = writeSyncLocalStatus(runtime.Meta.StatusFile, runtime.Status)
		}
		if runtime.Meta.LogFile != "" {
			appendSyncLocalLog(runtime.Meta.LogFile, "[STX] local job stopped by user")
		}
	}
	return &PrecheckResult{Success: true, Message: "stopped"}, nil
}

func syncLocalWorkDir(installDir, platformJobID string) string {
	return filepath.Join(installDir, ".stx", "sync-local-jobs", platformJobID)
}

func loadOrCreateSyncLocalRuntime(installDir, platformJobID string) (*syncLocalRuntime, error) {
	if platformJobID == "" {
		return nil, fmt.Errorf("platform_job_id is required")
	}
	if runtime, ok := loadSyncLocalRuntime(platformJobID); ok {
		return runtime, nil
	}
	if strings.TrimSpace(installDir) == "" {
		return nil, fmt.Errorf("install_dir is required")
	}
	metaFile := filepath.Join(syncLocalWorkDir(installDir, platformJobID), "meta.json")
	meta, err := readSyncLocalMeta(metaFile)
	if err != nil {
		return nil, fmt.Errorf("local runtime not found")
	}
	runtime := &syncLocalRuntime{Meta: meta}
	if status, statusErr := readSyncLocalStatus(meta.StatusFile); statusErr == nil {
		runtime.Status = status
	}
	syncLocalRuntimeStore.Store(platformJobID, runtime)
	return runtime, nil
}

func loadSyncLocalRuntime(platformJobID string) (*syncLocalRuntime, bool) {
	value, ok := syncLocalRuntimeStore.Load(platformJobID)
	if !ok {
		return nil, false
	}
	runtime, ok := value.(*syncLocalRuntime)
	return runtime, ok
}

func writeSyncLocalMeta(path string, meta syncLocalRuntimeMeta) error {
	body, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func readSyncLocalMeta(path string) (syncLocalRuntimeMeta, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return syncLocalRuntimeMeta{}, err
	}
	var meta syncLocalRuntimeMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return syncLocalRuntimeMeta{}, err
	}
	return meta, nil
}

func writeSyncLocalStatus(path string, status syncLocalStatusFile) error {
	body, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func readSyncLocalStatus(path string) (syncLocalStatusFile, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return syncLocalStatusFile{}, err
	}
	var status syncLocalStatusFile
	if err := json.Unmarshal(body, &status); err != nil {
		return syncLocalStatusFile{}, err
	}
	return status, nil
}

func readSyncLogChunk(path string, rawOffset string, limitBytes int, keyword string, level string) (string, int64, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, 0, err
	}
	fileSize := info.Size()
	offset := parseInt64OrZero(rawOffset)
	if offset < 0 {
		offset = maxInt64(fileSize-int64(limitBytes), 0)
	}
	if strings.TrimSpace(rawOffset) == "" {
		offset = maxInt64(fileSize-int64(limitBytes), 0)
	}
	if offset > fileSize {
		offset = 0
	}
	if limitBytes <= 0 {
		limitBytes = 64 * 1024
	}
	if limitBytes > 1024*1024 {
		limitBytes = 1024 * 1024
	}
	fh, err := os.Open(path)
	if err != nil {
		return "", 0, fileSize, err
	}
	defer fh.Close()
	if _, err := fh.Seek(offset, io.SeekStart); err != nil {
		return "", 0, fileSize, err
	}
	buffer := make([]byte, limitBytes)
	readBytes, readErr := fh.Read(buffer)
	if readErr != nil && readErr != io.EOF {
		return "", 0, fileSize, readErr
	}
	buffer = buffer[:readBytes]
	consumed := readBytes
	if readErr != io.EOF && len(buffer) > 0 {
		if idx := bytes.LastIndexByte(buffer, '\n'); idx >= 0 {
			consumed = idx + 1
			buffer = buffer[:consumed]
		}
	}
	if consumed == 0 && readBytes > 0 {
		consumed = readBytes
		buffer = buffer[:consumed]
	}
	content := filterLogContent(stripANSI(strings.TrimSpace(string(buffer))), keyword, level)
	return content, offset + int64(consumed), fileSize, nil
}

func appendSyncLocalLog(path string, line string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	_, _ = fh.WriteString(line + "\n")
}

func parseInt64OrZero(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func fileSizeOrZero(path string) int64 {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil {
		return 0
	}
	return info.Size()
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func filterLogContent(content string, keyword string, level string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	level = strings.ToUpper(strings.TrimSpace(level))
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	lines := strings.Split(content, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if level == "WARN" && !strings.Contains(upper, " WARN ") && !strings.Contains(upper, "WARNING") {
			continue
		}
		if level == "ERROR" && !strings.Contains(upper, " ERROR ") && !strings.Contains(upper, "ERROR") {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(trimmed), keyword) {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return strings.Join(filtered, "\n")
}

func resolveClusterJobLogFile(installDir string, jobIDs []string) (string, error) {
	baseDir := filepath.Join(installDir, "logs")
	for _, jobID := range jobIDs {
		candidate := filepath.Join(baseDir, fmt.Sprintf("job-%s.log", jobID))
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", fmt.Errorf("cluster job log not found")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, jobID := range jobIDs {
			if strings.Contains(name, jobID) {
				return filepath.Join(baseDir, name), nil
			}
		}
	}
	return "", fmt.Errorf("cluster job log not found")
}

func writeSyncLocalRunner(path string, data syncLocalRunnerTemplateData) error {
	tmpl, err := template.New("sync-local-runner").Parse(syncLocalRunnerTemplate)
	if err != nil {
		return err
	}
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, data); err != nil {
		return err
	}
	return os.WriteFile(path, buffer.Bytes(), 0o755)
}

func shellQuote(raw string) string {
	return strconv.Quote(raw)
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(value string) string {
	return ansiEscapePattern.ReplaceAllString(value, "")
}
