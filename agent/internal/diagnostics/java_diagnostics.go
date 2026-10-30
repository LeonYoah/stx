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

// Package diagnostics provides agent-side diagnostics evidence collection.
// diagnostics 包提供 Agent 侧的诊断证据采集能力。
package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/LeonYoah/stx/agent/internal/javaenv"
)

const (
	DefaultJVMDumpMinFreeMB = 2048
)

type DumpStatus string

const (
	DumpStatusCreated DumpStatus = "created"
	DumpStatusSkipped DumpStatus = "skipped"
)

type ThreadDumpResult struct {
	Status      DumpStatus `json:"status"`
	PID         int        `json:"pid"`
	Role        string     `json:"role"`
	InstallDir  string     `json:"install_dir"`
	Tool        string     `json:"tool"`
	OutputPath  string     `json:"output_path"`
	SizeBytes   int64      `json:"size_bytes"`
	Message     string     `json:"message"`
	Content     string     `json:"content"`
	CollectedAt time.Time  `json:"collected_at"`
}

type JVMDumpResult struct {
	Status         DumpStatus `json:"status"`
	PID            int        `json:"pid"`
	Role           string     `json:"role"`
	InstallDir     string     `json:"install_dir"`
	Tool           string     `json:"tool"`
	OutputPath     string     `json:"output_path,omitempty"`
	SizeBytes      int64      `json:"size_bytes,omitempty"`
	FreeBytes      int64      `json:"free_bytes"`
	RequiredBytes  int64      `json:"required_bytes"`
	EstimatedBytes int64      `json:"estimated_bytes"`
	Message        string     `json:"message"`
	CollectedAt    time.Time  `json:"collected_at"`
}

// CollectThreadDump 采集线程栈；javaHome 是节点配置的 JDK，非空时只使用其 bin 下的 jcmd/jstack。
// CollectThreadDump collects a thread dump; javaHome is the node's configured JDK and, when set, only its bin
// jcmd/jstack are used.
func CollectThreadDump(ctx context.Context, installDir, role, outputDir, javaHome string) (*ThreadDumpResult, error) {
	pid, err := findSeaTunnelPID(installDir, role)
	if err != nil {
		return nil, err
	}
	if outputDir == "" {
		outputDir = filepath.Join(installDir, "logs", "diagnostics")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create diagnostics output dir: %w", err)
	}

	content, tool, err := runThreadDumpCommand(ctx, pid, javaHome)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(outputDir, buildDumpFileName("thread-dump", role, "txt"))
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return nil, fmt.Errorf("write thread dump file: %w", err)
	}

	return &ThreadDumpResult{
		Status:      DumpStatusCreated,
		PID:         pid,
		Role:        normalizeRole(role),
		InstallDir:  installDir,
		Tool:        tool,
		OutputPath:  filePath,
		SizeBytes:   int64(len(content)),
		Message:     "线程栈采集完成 / thread dump collected",
		Content:     string(content),
		CollectedAt: time.Now().UTC(),
	}, nil
}

// CollectJVMDump 生成 JVM Dump；javaHome 是节点配置的 JDK，非空时只使用其 bin 下的 jcmd/jmap。
// CollectJVMDump creates a JVM dump; javaHome is the node's configured JDK and, when set, only its bin jcmd/jmap
// are used.
func CollectJVMDump(ctx context.Context, installDir, role, outputDir string, minFreeBytes int64, javaHome string) (*JVMDumpResult, error) {
	pid, err := findSeaTunnelPID(installDir, role)
	if err != nil {
		return nil, err
	}
	if outputDir == "" {
		outputDir = filepath.Join(installDir, "logs", "diagnostics")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create diagnostics output dir: %w", err)
	}

	freeBytes, err := getAvailableBytes(outputDir)
	if err != nil {
		return nil, err
	}

	estimatedBytes := estimateJVMDumpBytes(pid)
	requiredBytes := minFreeBytes
	if requiredBytes <= 0 {
		requiredBytes = int64(DefaultJVMDumpMinFreeMB) * 1024 * 1024
	}
	if estimatedBytes > 0 {
		estimatedRequired := estimatedBytes * 2
		if estimatedRequired > requiredBytes {
			requiredBytes = estimatedRequired
		}
	}

	result := &JVMDumpResult{
		PID:            pid,
		Role:           normalizeRole(role),
		InstallDir:     installDir,
		FreeBytes:      freeBytes,
		RequiredBytes:  requiredBytes,
		EstimatedBytes: estimatedBytes,
		CollectedAt:    time.Now().UTC(),
	}
	if freeBytes < requiredBytes {
		result.Status = DumpStatusSkipped
		result.Message = fmt.Sprintf("磁盘空间不足：free=%d required=%d / insufficient disk space: free=%d required=%d", freeBytes, requiredBytes, freeBytes, requiredBytes)
		return result, nil
	}

	filePath := filepath.Join(outputDir, buildDumpFileName("jvm-dump", role, "hprof"))
	tool, err := runJVMDumpCommand(ctx, pid, filePath, javaHome)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat jvm dump file: %w", err)
	}

	result.Status = DumpStatusCreated
	result.Tool = tool
	result.OutputPath = filePath
	result.SizeBytes = stat.Size()
	result.Message = "JVM Dump 采集完成 / jvm dump created"
	return result, nil
}

func MarshalThreadDumpResult(result *ThreadDumpResult) (string, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func MarshalJVMDumpResult(result *JVMDumpResult) (string, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// lookupJDKTool 是诊断工具定位入口，测试可替换。
// lookupJDKTool is the diagnostic tool lookup entry; tests may replace it.
var lookupJDKTool = javaenv.LookupTool

// jdkToolHint 返回工具缺失时的提示：配置了节点 JDK 时指出具体目录，否则保留原有 PATH 提示。
// jdkToolHint returns the missing-tool hint: the concrete directory for a configured node JDK, otherwise the
// original PATH hint.
func jdkToolHint(javaHome, first, second string) error {
	if strings.TrimSpace(javaHome) != "" {
		binDir := filepath.Join(javaenv.NormalizeHome(javaHome), "bin")
		return fmt.Errorf("节点 JDK 目录 %s 下未找到 %s 或 %s，请确认该路径是完整 JDK / %s or %s not found under the node JDK directory %s; make sure the path is a full JDK", binDir, first, second, first, second, binDir)
	}
	return fmt.Errorf("未检测到 %s 或 %s，请安装 JDK 并确保命令在 PATH 中 / %s or %s not found; install a JDK and ensure the tools are in PATH", first, second, first, second)
}

func runThreadDumpCommand(ctx context.Context, pid int, javaHome string) ([]byte, string, error) {
	var failures []string
	if path, err := lookupJDKTool(javaHome, "jcmd"); err == nil {
		output, cmdErr := exec.CommandContext(ctx, path, strconv.Itoa(pid), "Thread.print", "-l").CombinedOutput()
		if cmdErr == nil {
			return output, "jcmd", nil
		}
		failures = append(failures, formatCommandFailure("jcmd", cmdErr, output))
	}
	if path, err := lookupJDKTool(javaHome, "jstack"); err == nil {
		output, cmdErr := exec.CommandContext(ctx, path, "-l", strconv.Itoa(pid)).CombinedOutput()
		if cmdErr == nil {
			return output, "jstack", nil
		}
		failures = append(failures, formatCommandFailure("jstack", cmdErr, output))
	}
	if len(failures) == 0 {
		return nil, "", jdkToolHint(javaHome, "jcmd", "jstack")
	}
	return nil, "", fmt.Errorf("线程栈采集失败 / failed to collect thread dump: %s", strings.Join(failures, "; "))
}

func runJVMDumpCommand(ctx context.Context, pid int, outputPath, javaHome string) (string, error) {
	var failures []string
	if path, err := lookupJDKTool(javaHome, "jcmd"); err == nil {
		output, cmdErr := exec.CommandContext(ctx, path, strconv.Itoa(pid), "GC.heap_dump", outputPath).CombinedOutput()
		if cmdErr == nil {
			return "jcmd", nil
		}
		failures = append(failures, formatCommandFailure("jcmd", cmdErr, output))
	}
	if path, err := lookupJDKTool(javaHome, "jmap"); err == nil {
		output, cmdErr := exec.CommandContext(ctx, path, fmt.Sprintf("-dump:live,format=b,file=%s", outputPath), strconv.Itoa(pid)).CombinedOutput()
		if cmdErr == nil {
			return "jmap", nil
		}
		failures = append(failures, formatCommandFailure("jmap", cmdErr, output))
	}
	if len(failures) == 0 {
		return "", jdkToolHint(javaHome, "jcmd", "jmap")
	}
	return "", fmt.Errorf("JVM Dump 采集失败 / failed to create jvm dump: %s", strings.Join(failures, "; "))
}

func formatCommandFailure(command string, err error, output []byte) string {
	detail := strings.TrimSpace(string(output))
	if detail == "" && err != nil {
		detail = strings.TrimSpace(err.Error())
	}
	if detail == "" {
		detail = "unknown error"
	}
	detail = truncateCommandDetail(detail, 512)
	return fmt.Sprintf("%s => %s", command, detail)
}

func truncateCommandDetail(detail string, limit int) string {
	if limit <= 0 || len(detail) <= limit {
		return detail
	}
	return detail[:limit] + "..."
}

func findSeaTunnelPID(installDir, role string) (int, error) {
	const appMain = "org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer"

	isHybridMode := role == "" || role == "hybrid" || role == "master/worker"
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("wmic", "process", "where", fmt.Sprintf("CommandLine like '%%%s%%' and CommandLine like '%%SeaTunnel%%'", installDir), "get", "ProcessId")
	} else {
		var grepCmd string
		if isHybridMode {
			grepCmd = fmt.Sprintf("ps -ef | grep '%s' | grep '%s' | grep -v '\\-r master' | grep -v '\\-r worker' | grep -v grep | awk '{print $2}'", appMain, installDir)
		} else {
			grepCmd = fmt.Sprintf("ps -ef | grep '%s' | grep '%s' | grep '\\-r %s' | grep -v grep | awk '{print $2}'", appMain, installDir, role)
		}
		cmd = exec.Command("/bin/bash", "-c", grepCmd)
	}

	output, err := cmd.Output()
	if err != nil {
		if runtime.GOOS != "windows" {
			pattern := installDir
			if !isHybridMode {
				pattern = fmt.Sprintf("%s.*-r %s", installDir, role)
			}
			output, err = exec.Command("pgrep", "-f", pattern).Output()
			if err != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "ProcessId" {
			continue
		}
		pid, convErr := strconv.Atoi(line)
		if convErr == nil && pid > 0 {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("no SeaTunnel process found")
}

func estimateJVMDumpBytes(pid int) int64 {
	if pid <= 0 {
		return 0
	}
	if runtime.GOOS != "linux" {
		return 0
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	rssPages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return rssPages * 4096
}

func getAvailableBytes(path string) (int64, error) {
	cleanPath := filepath.Clean(path)
	var stat syscall.Statfs_t
	if err := syscall.Statfs(cleanPath, &stat); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", cleanPath, err)
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

func buildDumpFileName(prefix, role, ext string) string {
	return fmt.Sprintf("%s-%s-%s.%s", prefix, normalizeRole(role), time.Now().UTC().Format("20060102-150405"), ext)
}

func normalizeRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" || role == "master/worker" {
		return "hybrid"
	}
	return strings.ReplaceAll(role, "/", "-")
}
