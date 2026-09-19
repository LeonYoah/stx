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

// Package installer provides SeaTunnel installation management for the Agent.
// installer 包提供 Agent 的 SeaTunnel 安装管理功能。
package installer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	processmanager "github.com/LeonYoah/stx/agent/internal/process"
)

// NodePrecheckResult represents the result of a node precheck
// NodePrecheckResult 表示节点预检查的结果
type NodePrecheckResult struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// CheckPortListening checks if a port is listening (service is running)
// CheckPortListening 检查端口是否正在监听（服务正在运行）
func CheckPortListening(port int) *NodePrecheckResult {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Port %d is not listening: %v", port, err),
		}
	}
	conn.Close()
	return &NodePrecheckResult{
		Success: true,
		Message: fmt.Sprintf("Port %d is listening", port),
	}
}

// CheckTCPConnection checks whether a remote TCP endpoint is reachable.
// CheckTCPConnection 检查远程 TCP 端点是否可达。
func CheckTCPConnection(host string, port int, timeout time.Duration) *NodePrecheckResult {
	if strings.TrimSpace(host) == "" || port <= 0 {
		return &NodePrecheckResult{
			Success: false,
			Message: "host and port are required",
		}
	}
	addr := net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("TCP endpoint %s is not reachable: %v", addr, err),
			Details: map[string]string{
				"host": host,
				"port": strconv.Itoa(port),
			},
		}
	}
	_ = conn.Close()
	return &NodePrecheckResult{
		Success: true,
		Message: fmt.Sprintf("TCP endpoint %s is reachable", addr),
		Details: map[string]string{
			"host": host,
			"port": strconv.Itoa(port),
		},
	}
}

// CheckDirectoryExists checks if a directory exists and is writable
// CheckDirectoryExists 检查目录是否存在且可写
func CheckDirectoryExists(path string) *NodePrecheckResult {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Directory %s does not exist", path),
		}
	}
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Failed to check directory %s: %v", path, err),
		}
	}

	if !info.IsDir() {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Path %s is not a directory", path),
		}
	}

	// Check if writable by creating a temp file
	testFile := fmt.Sprintf("%s/.seatunnel_write_test_%d", path, time.Now().UnixNano())
	f, err := os.Create(testFile)
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Directory %s is not writable: %v", path, err),
		}
	}
	f.Close()
	os.Remove(testFile)

	return &NodePrecheckResult{
		Success: true,
		Message: fmt.Sprintf("Directory %s exists and is writable", path),
	}
}

// CheckPathReady checks whether a local directory path exists and is writable,
// or can be created because its parent directory is writable.
// CheckPathReady 检查本地目录路径是否已存在可写，或其父目录是否可写从而可以创建。
func CheckPathReady(path string) *NodePrecheckResult {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return &NodePrecheckResult{
			Success: false,
			Message: "path parameter is required",
		}
	}
	normalized := filepath.Clean(trimmed)
	info, err := os.Stat(normalized)
	if err == nil {
		if !info.IsDir() {
			return &NodePrecheckResult{
				Success: false,
				Message: fmt.Sprintf("Path %s is not a directory", trimmed),
			}
		}
		return CheckDirectoryExists(normalized)
	}
	if !os.IsNotExist(err) {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Failed to check path %s: %v", trimmed, err),
		}
	}

	parent := filepath.Dir(normalized)
	if parent == "." || parent == "" {
		parent = "/"
	}
	parentResult := CheckDirectoryExists(parent)
	if !parentResult.Success {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Path %s does not exist and parent %s is not writable", trimmed, parent),
			Details: map[string]string{
				"path":   trimmed,
				"parent": parent,
			},
		}
	}

	return &NodePrecheckResult{
		Success: true,
		Message: fmt.Sprintf("Path %s can be created (parent %s is writable)", trimmed, parent),
		Details: map[string]string{
			"path":   trimmed,
			"parent": parent,
		},
	}
}

// PathStatResult describes basic local path status and size.
// PathStatResult 描述本地路径的基本状态和大小。
type PathStatResult struct {
	Exists    bool   `json:"exists"`
	IsDir     bool   `json:"is_dir"`
	SizeBytes int64  `json:"size_bytes"`
	Path      string `json:"path"`
}

// StatPath returns path existence and recursive size information.
// StatPath 返回路径存在性及递归大小信息。
func StatPath(path string) (*PathStatResult, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("path parameter is required")
	}
	info, err := os.Stat(trimmed)
	if os.IsNotExist(err) {
		return &PathStatResult{Exists: false, Path: trimmed}, nil
	}
	if err != nil {
		return nil, err
	}

	result := &PathStatResult{
		Exists: true,
		IsDir:  info.IsDir(),
		Path:   trimmed,
	}
	if !info.IsDir() {
		result.SizeBytes = info.Size()
		return result, nil
	}

	var total int64
	err = filepath.Walk(trimmed, func(_ string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi != nil && !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.SizeBytes = total
	return result, nil
}

// CleanupDirectoryContents removes all children under a directory but keeps the directory itself.
// CleanupDirectoryContents 清理目录下所有子项，但保留目录本身。
func CleanupDirectoryContents(path string) *NodePrecheckResult {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return &NodePrecheckResult{Success: false, Message: "path parameter is required"}
	}
	cleaned := filepath.Clean(trimmed)
	switch cleaned {
	case "/", ".", "":
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("refusing to clean unsafe path: %s", cleaned),
		}
	}

	info, err := os.Stat(cleaned)
	if os.IsNotExist(err) {
		return &NodePrecheckResult{
			Success: true,
			Message: fmt.Sprintf("Directory %s does not exist, nothing to clean", cleaned),
			Details: map[string]string{"path": cleaned},
		}
	}
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Failed to inspect directory %s: %v", cleaned, err),
		}
	}
	if !info.IsDir() {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Path %s is not a directory", cleaned),
		}
	}

	entries, err := os.ReadDir(cleaned)
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Failed to list directory %s: %v", cleaned, err),
		}
	}
	removed := 0
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(cleaned, entry.Name())); err != nil {
			return &NodePrecheckResult{
				Success: false,
				Message: fmt.Sprintf("Failed to remove %s: %v", entry.Name(), err),
			}
		}
		removed++
	}
	return &NodePrecheckResult{
		Success: true,
		Message: fmt.Sprintf("Cleaned %d entries under %s", removed, cleaned),
		Details: map[string]string{
			"path":          cleaned,
			"removed_count": strconv.Itoa(removed),
		},
	}
}

// CheckHTTPEndpoint checks if an HTTP endpoint is accessible
// CheckHTTPEndpoint 检查 HTTP 端点是否可访问
func CheckHTTPEndpoint(url string) *NodePrecheckResult {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("HTTP endpoint %s is not accessible: %v", url, err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return &NodePrecheckResult{
			Success: true,
			Message: fmt.Sprintf("HTTP endpoint %s is accessible (status: %d)", url, resp.StatusCode),
		}
	}

	return &NodePrecheckResult{
		Success: false,
		Message: fmt.Sprintf("HTTP endpoint %s returned error status: %d", url, resp.StatusCode),
	}
}

// SeaTunnelProcessInfo represents information about a SeaTunnel process
// SeaTunnelProcessInfo 表示 SeaTunnel 进程的信息
type SeaTunnelProcessInfo struct {
	PID       int    `json:"pid"`
	Role      string `json:"role"` // "hybrid", "master", "worker"
	CmdLine   string `json:"cmd_line"`
	StartTime string `json:"start_time"`
}

// CheckSeaTunnelProcess 按安装目录和角色检查正在运行的 SeaTunnel 进程。
// CheckSeaTunnelProcess checks a running SeaTunnel process by install directory and role.
func CheckSeaTunnelProcess(ctx context.Context, installDir, role string) (*SeaTunnelProcessInfo, error) {
	pid, cmdLine, err := processmanager.FindSeaTunnelProcess(ctx, installDir, role)
	if errors.Is(err, processmanager.ErrProcessNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	startTime := ""
	if runtime.GOOS != "windows" {
		if output, startErr := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output(); startErr == nil {
			startTime = strings.TrimSpace(string(output))
		}
	}

	return &SeaTunnelProcessInfo{
		PID:       pid,
		Role:      processRoleFromCommandLine(cmdLine),
		CmdLine:   cmdLine,
		StartTime: startTime,
	}, nil
}

func processRoleFromCommandLine(cmdLine string) string {
	if strings.Contains(cmdLine, "-r master") {
		return "master"
	}
	if strings.Contains(cmdLine, "-r worker") {
		return "worker"
	}
	return "hybrid"
}

// GetSeaTunnelProcessPID returns the PID of a running SeaTunnel process
// GetSeaTunnelProcessPID 返回正在运行的 SeaTunnel 进程的 PID
func GetSeaTunnelProcessPID(ctx context.Context, installDir, role string) (int, error) {
	info, err := CheckSeaTunnelProcess(ctx, installDir, role)
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, nil
	}
	return info.PID, nil
}

// CheckSeaTunnelRunning checks if SeaTunnel is running
// CheckSeaTunnelRunning 检查 SeaTunnel 是否正在运行
func CheckSeaTunnelRunning(ctx context.Context, installDir, role string) *NodePrecheckResult {
	info, err := CheckSeaTunnelProcess(ctx, installDir, role)
	if err != nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("Failed to check SeaTunnel process: %v", err),
		}
	}

	if info == nil {
		return &NodePrecheckResult{
			Success: false,
			Message: fmt.Sprintf("SeaTunnel process (role: %s) is not running", role),
		}
	}

	return &NodePrecheckResult{
		Success: true,
		Message: fmt.Sprintf("SeaTunnel process found: PID=%d, role=%s", info.PID, info.Role),
	}
}

// GetAllSeaTunnelProcesses returns all running SeaTunnel processes
// GetAllSeaTunnelProcesses 返回所有正在运行的 SeaTunnel 进程
func GetAllSeaTunnelProcesses(ctx context.Context) ([]*SeaTunnelProcessInfo, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c",
		`ps -ef | grep "org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer" | grep -v grep`)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return nil, nil
	}

	lines := strings.Split(outputStr, "\n")
	processes := make([]*SeaTunnelProcessInfo, 0, len(lines))

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		cmdLine := strings.Join(fields[7:], " ")
		role := "hybrid"
		if strings.Contains(cmdLine, "-r master") {
			role = "master"
		} else if strings.Contains(cmdLine, "-r worker") {
			role = "worker"
		}

		processes = append(processes, &SeaTunnelProcessInfo{
			PID:       pid,
			Role:      role,
			CmdLine:   cmdLine,
			StartTime: fields[4],
		})
	}

	return processes, nil
}

// ExtractPortFromConfig extracts port configuration from SeaTunnel config file
// ExtractPortFromConfig 从 SeaTunnel 配置文件中提取端口配置
func ExtractPortFromConfig(configPath string, portName string) (int, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read config file: %w", err)
	}

	patterns := []string{
		fmt.Sprintf(`%s\s*[=:]\s*(\d+)`, regexp.QuoteMeta(portName)),
		fmt.Sprintf(`"%s"\s*[=:]\s*(\d+)`, regexp.QuoteMeta(portName)),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(string(content))
		if len(matches) > 1 {
			port, err := strconv.Atoi(matches[1])
			if err == nil {
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("port %s not found in config", portName)
}
