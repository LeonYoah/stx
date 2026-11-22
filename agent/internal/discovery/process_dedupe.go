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

package discovery

import (
	"path/filepath"
	"strings"
)

// discoveredProcessCandidate 是扫描期去重用的中间结构（含 PPID/命令行）。
// discoveredProcessCandidate is the scan-time intermediate used for parent/child dedupe.
type discoveredProcessCandidate struct {
	PID           int
	PPID          int
	InstallDir    string
	Role          string
	CommandLine   string
	HazelcastCfg  string
	Version       string
	HazelcastPort int
	APIPort       int
	ClusterName   string
	JavaHome      string
}

func (c *discoveredProcessCandidate) toDiscoveredProcess() *DiscoveredProcess {
	if c == nil {
		return nil
	}
	return &DiscoveredProcess{
		PID:           c.PID,
		InstallDir:    c.InstallDir,
		Role:          c.Role,
		Version:       c.Version,
		HazelcastPort: c.HazelcastPort,
		APIPort:       c.APIPort,
		ClusterName:   c.ClusterName,
		JavaHome:      c.JavaHome,
	}
}

// dedupeDiscoveredProcesses 去掉 shell 包装父进程，保留 Java 子进程；不同安装目录的真实实例互不合并。
// dedupeDiscoveredProcesses drops shell wrapper parents, keeps Java children; distinct install dirs stay separate.
func dedupeDiscoveredProcesses(input []*discoveredProcessCandidate) []*discoveredProcessCandidate {
	if len(input) <= 1 {
		return input
	}

	byPID := make(map[int]*discoveredProcessCandidate, len(input))
	for _, item := range input {
		if item == nil || item.PID <= 0 {
			continue
		}
		byPID[item.PID] = item
	}

	drop := make(map[int]struct{})
	for _, item := range byPID {
		// PPID 链：父进程也是 SeaTunnel 候选时，父为包装脚本，子为实际监听进程。
		// PPID chain: when the parent is also a SeaTunnel candidate, keep the child listener.
		if parent, ok := byPID[item.PPID]; ok && parent.PID != item.PID {
			drop[parent.PID] = struct{}{}
		}
	}

	for _, item := range byPID {
		if !isShellWrapperCommand(item.CommandLine) {
			continue
		}
		for _, peer := range byPID {
			if peer.PID == item.PID {
				continue
			}
			if !sameDiscoveredIdentity(item, peer) {
				continue
			}
			if isJavaSeaTunnelCommand(peer.CommandLine) {
				drop[item.PID] = struct{}{}
				break
			}
		}
	}

	out := make([]*discoveredProcessCandidate, 0, len(byPID))
	for _, item := range input {
		if item == nil || item.PID <= 0 {
			continue
		}
		if _, skipped := drop[item.PID]; skipped {
			continue
		}
		// 输入可能重复 PID；按首次出现保留。
		// Input may repeat PIDs; keep the first occurrence.
		if _, seen := byPID[item.PID]; !seen {
			continue
		}
		out = append(out, item)
		delete(byPID, item.PID)
	}
	return out
}

func sameDiscoveredIdentity(a, b *discoveredProcessCandidate) bool {
	if a == nil || b == nil {
		return false
	}
	return normalizeDiscoveredInstallDir(a.InstallDir) == normalizeDiscoveredInstallDir(b.InstallDir) &&
		normalizeDiscoveredRole(a.Role) == normalizeDiscoveredRole(b.Role)
}

func normalizeDiscoveredInstallDir(installDir string) string {
	normalized := strings.Trim(strings.TrimSpace(installDir), "\"'")
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	return strings.TrimRight(normalized, "/")
}

func normalizeDiscoveredRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "master":
		return "master"
	case "worker":
		return "worker"
	default:
		return "hybrid"
	}
}

func isShellWrapperCommand(commandLine string) bool {
	trimmed := strings.TrimSpace(commandLine)
	if trimmed == "" {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	base := strings.ToLower(filepath.Base(fields[0]))
	switch base {
	case "sh", "bash", "dash", "zsh", "ksh":
		return true
	default:
		return false
	}
}

func isJavaSeaTunnelCommand(commandLine string) bool {
	trimmed := strings.TrimSpace(commandLine)
	if trimmed == "" || !strings.Contains(trimmed, SeaTunnelMainClass) {
		return false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	base := strings.ToLower(filepath.Base(fields[0]))
	return base == "java" || strings.HasPrefix(base, "java")
}
