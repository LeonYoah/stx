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
	"sync"

	"github.com/LeonYoah/stx/agent/internal/logger"
)

// ProcessDiscovery provides on-demand SeaTunnel process discovery
// ProcessDiscovery 提供按需的 SeaTunnel 进程发现功能
// No longer auto-scans or parses config files - just finds running processes
// 不再自动扫描或解析配置文件 - 只查找运行中的进程
type ProcessDiscovery struct {
	scanner *ProcessScanner
	mu      sync.RWMutex
}

// NewProcessDiscovery creates a new ProcessDiscovery instance
// NewProcessDiscovery 创建一个新的 ProcessDiscovery 实例
func NewProcessDiscovery() *ProcessDiscovery {
	return &ProcessDiscovery{
		scanner: NewProcessScanner(),
	}
}

// DiscoverProcesses scans for SeaTunnel processes on this host
// DiscoverProcesses 扫描本机上的 SeaTunnel 进程
// Returns simplified process info: PID, role, install_dir
// 返回简化的进程信息：PID、角色、安装目录
func (d *ProcessDiscovery) DiscoverProcesses() ([]*DiscoveredProcess, error) {
	ctx := context.Background()
	d.mu.Lock()
	defer d.mu.Unlock()

	logger.InfoF(ctx, "[ProcessDiscovery] Scanning for SeaTunnel processes... / 正在扫描 SeaTunnel 进程...")

	processes, err := d.scanner.ScanProcesses()
	if err != nil {
		return nil, fmt.Errorf("failed to scan processes: %w / 扫描进程失败：%w", err, err)
	}

	if len(processes) == 0 {
		logger.InfoF(ctx, "[ProcessDiscovery] No SeaTunnel processes found / 未找到 SeaTunnel 进程")
	} else {
		logger.InfoF(ctx, "[ProcessDiscovery] Found %d process(es) / 发现 %d 个进程", len(processes), len(processes))
	}

	return processes, nil
}

// DiscoverProcessByInstallDir finds a SeaTunnel process by install directory
// DiscoverProcessByInstallDir 根据安装目录查找 SeaTunnel 进程
// Useful when user wants to find process for a specific node
// 当用户想要查找特定节点的进程时很有用
func (d *ProcessDiscovery) DiscoverProcessByInstallDir(installDir string) (*DiscoveredProcess, error) {
	processes, err := d.DiscoverProcesses()
	if err != nil {
		return nil, err
	}

	for _, proc := range processes {
		if proc.InstallDir == installDir {
			return proc, nil
		}
	}

	return nil, fmt.Errorf("no process found for install dir: %s / 未找到安装目录 %s 的进程", installDir, installDir)
}

// DiscoverProcessByRole finds SeaTunnel processes by role
// DiscoverProcessByRole 根据角色查找 SeaTunnel 进程
func (d *ProcessDiscovery) DiscoverProcessByRole(role string) ([]*DiscoveredProcess, error) {
	processes, err := d.DiscoverProcesses()
	if err != nil {
		return nil, err
	}

	var matched []*DiscoveredProcess
	for _, proc := range processes {
		if proc.Role == role {
			matched = append(matched, proc)
		}
	}

	return matched, nil
}

// =============================================================================
// Legacy types for backward compatibility with existing code
// 为了与现有代码向后兼容的遗留类型
// =============================================================================

// DiscoveredCluster is kept for backward compatibility
// DiscoveredCluster 保留用于向后兼容
// In the new simplified flow, clusters are created manually by user
// 在新的简化流程中，集群由用户手动创建
type DiscoveredCluster struct {
	Name           string            `json:"name"`
	InstallDir     string            `json:"install_dir"`
	Version        string            `json:"version"`
	DeploymentMode string            `json:"deployment_mode"`
	Nodes          []*DiscoveredNode `json:"nodes"`
	Config         map[string]any    `json:"config"`
}

// DiscoveredNode is kept for backward compatibility
// DiscoveredNode 保留用于向后兼容
type DiscoveredNode struct {
	PID           int    `json:"pid"`
	Role          string `json:"role"`
	HazelcastPort int    `json:"hazelcast_port"`
	APIPort       int    `json:"api_port"`
}

// ClusterDiscovery is kept for backward compatibility
// ClusterDiscovery 保留用于向后兼容
// Deprecated: Use ProcessDiscovery instead
// 已弃用：请使用 ProcessDiscovery
type ClusterDiscovery struct {
	*ProcessDiscovery
}

// NewClusterDiscovery creates a new ClusterDiscovery (deprecated)
// NewClusterDiscovery 创建一个新的 ClusterDiscovery（已弃用）
func NewClusterDiscovery() *ClusterDiscovery {
	return &ClusterDiscovery{
		ProcessDiscovery: NewProcessDiscovery(),
	}
}

// DiscoverNow is kept for backward compatibility
// DiscoverNow 保留用于向后兼容
// Returns empty cluster list - clusters should be created manually now
// 返回空的集群列表 - 现在集群应该手动创建
func (d *ClusterDiscovery) DiscoverNow() ([]*DiscoveredCluster, error) {
	ctx := context.Background()
	logger.WarnF(ctx, "[ClusterDiscovery] DiscoverNow is deprecated. Use ProcessDiscovery.DiscoverProcesses instead.")
	logger.WarnF(ctx, "[ClusterDiscovery] DiscoverNow 已弃用。请使用 ProcessDiscovery.DiscoverProcesses。")
	return nil, nil
}

// GetLastDiscovery is kept for backward compatibility
// GetLastDiscovery 保留用于向后兼容
func (d *ClusterDiscovery) GetLastDiscovery() []*DiscoveredCluster {
	return nil
}
