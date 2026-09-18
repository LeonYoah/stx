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

// Package discovery provides simplified SeaTunnel process discovery for the Agent.
// discovery 包提供 Agent 的简化 SeaTunnel 进程发现功能。
//
// Simplified flow (用户先创建集群，再发现进程):
// 1. User creates cluster in frontend (manually fills version, deployment mode)
// 2. User selects hosts and assigns node roles
// 3. User clicks "Discover Process" button
// 4. Agent scans for SeaTunnel processes, returns PID, role, install_dir
// 5. Control Plane associates process info with existing node
package discovery

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/agent/internal/logger"
)

// SeaTunnelMainClass is the main class name for SeaTunnel Server
// SeaTunnelMainClass 是 SeaTunnel Server 的主类名
const SeaTunnelMainClass = "org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer"

// ProcessScanner scans for SeaTunnel processes on the local machine
// ProcessScanner 扫描本机上的 SeaTunnel 进程
type ProcessScanner struct{}

// NewProcessScanner creates a new ProcessScanner instance
// NewProcessScanner 创建一个新的 ProcessScanner 实例
func NewProcessScanner() *ProcessScanner {
	return &ProcessScanner{}
}

// DiscoveredProcess represents a discovered SeaTunnel process (enhanced)
// DiscoveredProcess 表示发现的 SeaTunnel 进程（增强版）
// Contains: PID, role, install_dir, version, hazelcast_port, api_port
// 包含：PID、角色、安装目录、版本、hazelcast端口、api端口
type DiscoveredProcess struct {
	PID           int    `json:"pid"`            // Process ID / 进程 ID
	InstallDir    string `json:"install_dir"`    // SeaTunnel installation directory / SeaTunnel 安装目录
	Role          string `json:"role"`           // master, worker, or hybrid / 角色：master、worker 或 hybrid
	Version       string `json:"version"`        // SeaTunnel version (e.g., 2.3.12) / SeaTunnel 版本
	HazelcastPort int    `json:"hazelcast_port"` // Hazelcast cluster port / Hazelcast 集群端口
	APIPort       int    `json:"api_port"`       // REST API port (from seatunnel.yaml) / REST API 端口
}

// ScanProcesses scans all Java processes and identifies SeaTunnel processes
// ScanProcesses 扫描所有 Java 进程并识别 SeaTunnel 进程
// Returns simplified process info: PID, role, install_dir
// 返回简化的进程信息：PID、角色、安装目录
func (s *ProcessScanner) ScanProcesses() ([]*DiscoveredProcess, error) {
	if runtime.GOOS == "windows" {
		return s.scanProcessesWindows()
	}
	return s.scanProcessesUnix()
}

// scanProcessesUnix scans processes on Unix/Linux
// scanProcessesUnix 在 Unix/Linux 上扫描进程
func (s *ProcessScanner) scanProcessesUnix() ([]*DiscoveredProcess, error) {
	ctx := context.Background()
	var processes []*DiscoveredProcess

	// Use ps command to get all Java processes containing SeaTunnel main class
	// 使用 ps 命令获取所有包含 SeaTunnel 主类的 Java 进程
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf("ps -ef | grep '%s' | grep -v grep", SeaTunnelMainClass))
	output, err := cmd.Output()
	if err != nil {
		// No processes found is not an error / 未找到进程不是错误
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			logger.InfoF(ctx, "[ProcessScanner] No SeaTunnel processes found / 未找到 SeaTunnel 进程")
			return processes, nil
		}
		return nil, fmt.Errorf("failed to scan processes: %w / 扫描进程失败：%w", err, err)
	}

	versionDetector := NewVersionDetector()
	configReader := NewConfigReader()

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		proc, err := s.parseUnixProcessLine(line)
		if err != nil {
			logger.WarnF(ctx, "[ProcessScanner] Warning: failed to parse process line: %v", err)
			continue
		}

		if proc != nil {
			// Enhance with version and port info / 增强版本和端口信息
			if proc.InstallDir != "" {
				// Detect version from connector jars / 从 connector jar 检测版本
				proc.Version = versionDetector.DetectVersion(proc.InstallDir)

				// Read hazelcast port from config / 从配置读取 hazelcast 端口
				proc.HazelcastPort = configReader.ReadHazelcastPort(proc.InstallDir, proc.Role)

				// Read API port from seatunnel.yaml / 从 seatunnel.yaml 读取 API 端口
				proc.APIPort = configReader.ReadAPIPort(proc.InstallDir)
			}

			processes = append(processes, proc)
			logger.InfoF(ctx, "[ProcessScanner] Found: PID=%d, Role=%s, InstallDir=%s, Version=%s, HazelcastPort=%d, APIPort=%d",
				proc.PID, proc.Role, proc.InstallDir, proc.Version, proc.HazelcastPort, proc.APIPort)
		}
	}

	return processes, nil
}

// scanProcessesWindows scans processes on Windows
// scanProcessesWindows 在 Windows 上扫描进程
func (s *ProcessScanner) scanProcessesWindows() ([]*DiscoveredProcess, error) {
	var processes []*DiscoveredProcess

	// Use wmic to get Java processes
	// 使用 wmic 获取 Java 进程
	cmd := exec.Command("wmic", "process", "where", "name like '%java%'", "get", "ProcessId,CommandLine", "/format:csv")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to scan processes on Windows: %w / 在 Windows 上扫描进程失败：%w", err, err)
	}

	versionDetector := NewVersionDetector()
	configReader := NewConfigReader()

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, SeaTunnelMainClass) {
			continue
		}

		// Parse CSV format: Node,CommandLine,ProcessId
		// 解析 CSV 格式：Node,CommandLine,ProcessId
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
		if err != nil {
			continue
		}

		cmdline := strings.Join(parts[1:len(parts)-1], ",")
		installDir, role := s.parseCommandLine(cmdline)

		proc := &DiscoveredProcess{
			PID:        pid,
			InstallDir: installDir,
			Role:       role,
		}

		// Enhance with version and port info / 增强版本和端口信息
		if installDir != "" {
			proc.Version = versionDetector.DetectVersion(installDir)
			proc.HazelcastPort = configReader.ReadHazelcastPort(installDir, role)
			proc.APIPort = configReader.ReadAPIPort(installDir)
		}

		processes = append(processes, proc)
	}

	return processes, nil
}

// parseUnixProcessLine parses a single line from ps output
// parseUnixProcessLine 解析 ps 输出的单行
func (s *ProcessScanner) parseUnixProcessLine(line string) (*DiscoveredProcess, error) {
	// ps -ef format: UID PID PPID C STIME TTY TIME CMD
	// ps -ef 格式：UID PID PPID C STIME TTY TIME CMD
	fields := strings.Fields(line)
	if len(fields) < 8 {
		return nil, fmt.Errorf("invalid ps line format / 无效的 ps 行格式")
	}

	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse PID: %w / 解析 PID 失败：%w", err, err)
	}

	// Get full command line / 获取完整命令行
	cmdline := strings.Join(fields[7:], " ")
	installDir, role := s.parseCommandLine(cmdline)

	return &DiscoveredProcess{
		PID:        pid,
		InstallDir: installDir,
		Role:       role,
	}, nil
}

// parseCommandLine extracts install directory and role from command line
// parseCommandLine 从命令行提取安装目录和角色
func (s *ProcessScanner) parseCommandLine(cmdline string) (installDir, role string) {
	role = "hybrid" // Default to hybrid mode / 默认为混合模式

	// Method 1: Extract from -Dseatunnel.logs.path=/xxx/logs (remove /logs suffix)
	// 方法1：从 -Dseatunnel.logs.path=/xxx/logs 提取（去掉 /logs 后缀）
	logsPathRegex := regexp.MustCompile(`-Dseatunnel\.logs\.path=([^\s]+)`)
	if matches := logsPathRegex.FindStringSubmatch(cmdline); len(matches) > 1 {
		logsPath := matches[1]
		if strings.HasSuffix(logsPath, "/logs") {
			installDir = strings.TrimSuffix(logsPath, "/logs")
		} else if strings.HasSuffix(logsPath, "\\logs") {
			installDir = strings.TrimSuffix(logsPath, "\\logs")
		}
	}

	// Method 2: Extract from -Dseatunnel.config=/xxx/config/seatunnel.yaml
	// 方法2：从 -Dseatunnel.config=/xxx/config/seatunnel.yaml 提取
	if installDir == "" {
		configRegex := regexp.MustCompile(`-Dseatunnel\.config=([^\s]+)`)
		if matches := configRegex.FindStringSubmatch(cmdline); len(matches) > 1 {
			configPath := matches[1]
			// Remove /config/seatunnel.yaml or /config/xxx.yaml suffix
			// 去掉 /config/seatunnel.yaml 或 /config/xxx.yaml 后缀
			if idx := strings.Index(configPath, "/config/"); idx > 0 {
				installDir = configPath[:idx]
			} else if idx := strings.Index(configPath, "\\config\\"); idx > 0 {
				installDir = configPath[:idx]
			}
		}
	}

	// Method 3: Extract from -Dhazelcast.config=/xxx/config/hazelcast-xxx.yaml
	// 方法3：从 -Dhazelcast.config=/xxx/config/hazelcast-xxx.yaml 提取
	if installDir == "" {
		hazelcastConfigRegex := regexp.MustCompile(`-Dhazelcast\.config=([^\s]+)`)
		if matches := hazelcastConfigRegex.FindStringSubmatch(cmdline); len(matches) > 1 {
			configPath := matches[1]
			if idx := strings.Index(configPath, "/config/"); idx > 0 {
				installDir = configPath[:idx]
			} else if idx := strings.Index(configPath, "\\config\\"); idx > 0 {
				installDir = configPath[:idx]
			}
		}
	}

	// Method 4: Extract from -cp or -classpath (find path before /lib/)
	// 方法4：从 -cp 或 -classpath 提取（找 /lib/ 前面的路径）
	if installDir == "" {
		// Try -cp first / 先尝试 -cp
		cpRegex := regexp.MustCompile(`-cp\s+([^\s]+)`)
		if matches := cpRegex.FindStringSubmatch(cmdline); len(matches) > 1 {
			cp := matches[1]
			if idx := strings.Index(cp, "/lib/"); idx > 0 {
				installDir = cp[:idx]
			} else if idx := strings.Index(cp, "\\lib\\"); idx > 0 {
				installDir = cp[:idx]
			}
		}
	}

	if installDir == "" {
		// Try -classpath / 尝试 -classpath
		cpRegex := regexp.MustCompile(`-classpath\s+([^\s]+)`)
		if matches := cpRegex.FindStringSubmatch(cmdline); len(matches) > 1 {
			cp := matches[1]
			if idx := strings.Index(cp, "/lib/"); idx > 0 {
				installDir = cp[:idx]
			} else if idx := strings.Index(cp, "\\lib\\"); idx > 0 {
				installDir = cp[:idx]
			}
		}
	}

	// Method 5: Extract from -DSEATUNNEL_HOME=xxx (legacy)
	// 方法5：从 -DSEATUNNEL_HOME=xxx 提取（旧版）
	if installDir == "" {
		homeRegex := regexp.MustCompile(`-DSEATUNNEL_HOME=([^\s]+)`)
		if matches := homeRegex.FindStringSubmatch(cmdline); len(matches) > 1 {
			installDir = matches[1]
		}
	}

	// Extract role from -r parameter (-r master or -r worker)
	// 从 -r 参数提取角色（-r master 或 -r worker）
	roleRegex := regexp.MustCompile(`\s-r\s+(master|worker)`)
	if matches := roleRegex.FindStringSubmatch(cmdline); len(matches) > 1 {
		role = matches[1]
	}

	return installDir, role
}

// IsSeaTunnelProcess checks if a command line belongs to a SeaTunnel process
// IsSeaTunnelProcess 检查命令行是否属于 SeaTunnel 进程
func (s *ProcessScanner) IsSeaTunnelProcess(cmdline string) bool {
	return strings.Contains(cmdline, SeaTunnelMainClass)
}
