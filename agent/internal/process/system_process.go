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
package process

import (
	"context"
	"encoding/csv"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const seaTunnelMainClass = "org.apache.seatunnel.core.starter.seatunnel.SeaTunnelServer"

type systemProcess struct {
	PID         int
	CommandLine string
}

// FindSeaTunnelProcess 按安装目录和角色查找唯一的 SeaTunnel 进程。
// FindSeaTunnelProcess finds a SeaTunnel process by install directory and role.
func FindSeaTunnelProcess(ctx context.Context, installDir, role string) (int, string, error) {
	processes, err := listSeaTunnelProcesses(ctx)
	if err != nil {
		return 0, "", err
	}
	for _, proc := range processes {
		if matchesManagedSeaTunnelProcess(proc.CommandLine, installDir, role) {
			return proc.PID, proc.CommandLine, nil
		}
	}
	return 0, "", ErrProcessNotFound
}

// MatchesSeaTunnelProcess 确认指定 PID 是否仍属于目标安装目录和角色。
// MatchesSeaTunnelProcess verifies that a PID still belongs to the target install directory and role.
func MatchesSeaTunnelProcess(ctx context.Context, pid int, installDir, role string) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	processes, err := listSeaTunnelProcesses(ctx)
	if err != nil {
		return false, err
	}
	for _, proc := range processes {
		if proc.PID == pid {
			return matchesManagedSeaTunnelProcess(proc.CommandLine, installDir, role), nil
		}
	}
	return false, nil
}

func listSeaTunnelProcesses(ctx context.Context) ([]systemProcess, error) {
	if runtime.GOOS == "windows" {
		cmd := exec.CommandContext(ctx, "wmic", "process", "get", "ProcessId,CommandLine", "/format:csv")
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("list Windows processes: %w", err)
		}
		return parseWindowsProcessList(string(output))
	}

	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,command=")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list Unix processes: %w", err)
	}
	return parseUnixProcessList(string(output)), nil
}

func parseUnixProcessList(output string) []systemProcess {
	processes := make([]systemProcess, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		commandLine := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		if !strings.Contains(commandLine, seaTunnelMainClass) {
			continue
		}
		processes = append(processes, systemProcess{PID: pid, CommandLine: commandLine})
	}
	return processes
}

func parseWindowsProcessList(output string) ([]systemProcess, error) {
	reader := csv.NewReader(strings.NewReader(output))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse Windows process list: %w", err)
	}

	processes := make([]systemProcess, 0)
	for _, record := range records {
		if len(record) < 3 {
			continue
		}
		commandLine := strings.TrimSpace(record[len(record)-2])
		pid, err := strconv.Atoi(strings.TrimSpace(record[len(record)-1]))
		if err != nil || pid <= 0 || !strings.Contains(commandLine, seaTunnelMainClass) {
			continue
		}
		processes = append(processes, systemProcess{PID: pid, CommandLine: commandLine})
	}
	return processes, nil
}

func matchesManagedSeaTunnelProcess(commandLine, installDir, role string) bool {
	if !strings.Contains(commandLine, seaTunnelMainClass) {
		return false
	}
	if strings.TrimSpace(installDir) != "" && !commandLineMatchesInstallDir(commandLine, installDir) {
		return false
	}
	return seaTunnelRoleFromCommand(commandLine) == normalizeSeaTunnelRole(role)
}

func commandLineMatchesInstallDir(commandLine, installDir string) bool {
	normalizedDir := normalizeInstallDir(installDir)
	if normalizedDir == "" {
		return false
	}
	normalizedCommand := strings.ReplaceAll(commandLine, "\\", "/")
	markers := []string{
		"-Dseatunnel.logs.path=" + normalizedDir + "/logs",
		"-Dseatunnel.config=" + normalizedDir + "/config/",
		"-Dhazelcast.config=" + normalizedDir + "/config/",
		"-DSEATUNNEL_HOME=" + normalizedDir,
		normalizedDir + "/lib/",
		normalizedDir + "/starter/",
	}

	if len(normalizedDir) >= 3 && normalizedDir[1:3] == ":/" {
		normalizedCommand = strings.ToLower(normalizedCommand)
		for i := range markers {
			markers[i] = strings.ToLower(markers[i])
		}
	}
	for _, marker := range markers {
		if strings.Contains(normalizedCommand, marker) {
			return true
		}
	}
	return false
}

func normalizeInstallDir(installDir string) string {
	normalized := strings.Trim(strings.TrimSpace(installDir), "\"'")
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	return strings.TrimRight(normalized, "/")
}

func normalizeSeaTunnelRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "master":
		return "master"
	case "worker":
		return "worker"
	default:
		return "hybrid"
	}
}

func seaTunnelRoleFromCommand(commandLine string) string {
	fields := strings.Fields(commandLine)
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "-r" {
			continue
		}
		switch strings.ToLower(strings.Trim(fields[i+1], "\"'")) {
		case "master":
			return "master"
		case "worker":
			return "worker"
		}
	}
	return "hybrid"
}
