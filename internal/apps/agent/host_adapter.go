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

// Package agent provides Agent connection management for the STX Control Plane.
// agent 包提供 STX Control Plane 的 Agent 连接管理功能。
package agent

import (
	"context"

	"github.com/LeonYoah/stx/internal/apps/host"
)

// HostServiceAdapter adapts the Host Service to the HostStatusUpdater interface.
// HostServiceAdapter 将 Host Service 适配到 HostStatusUpdater 接口。
// This adapter allows the Agent Manager to update host status without direct dependency.
// 此适配器允许 Agent Manager 更新主机状态而无需直接依赖。
type HostServiceAdapter struct {
	hostService *host.Service
}

// NewHostServiceAdapter creates a new HostServiceAdapter.
// NewHostServiceAdapter 创建一个新的 HostServiceAdapter。
func NewHostServiceAdapter(hostService *host.Service) *HostServiceAdapter {
	return &HostServiceAdapter{
		hostService: hostService,
	}
}

// UpdateAgentStatus updates the agent status for a host by pre-bound host ID or IP address.
// UpdateAgentStatus 根据预绑定主机 ID 或 IP 地址更新主机的 Agent 状态。
// Requirements: 3.2 - Matches Agent hostID or IP with registered host and updates status to "installed".
func (a *HostServiceAdapter) UpdateAgentStatus(ctx context.Context, hostID uint, ipAddress string, agentID string, version string, sysInfo *SystemInfo, hostname string, localIPs []string) (uint, error) {
	var hostSysInfo *host.SystemInfo
	if sysInfo != nil {
		hostSysInfo = &host.SystemInfo{
			OSType:      sysInfo.OSType,
			Arch:        sysInfo.Arch,
			CPUCores:    sysInfo.CPUCores,
			TotalMemory: sysInfo.TotalMemory,
			TotalDisk:   sysInfo.TotalDisk,
		}
	}

	updatedHost, err := a.hostService.UpdateAgentStatus(ctx, hostID, ipAddress, agentID, version, hostSysInfo, hostname, localIPs)
	if err != nil {
		return 0, err
	}

	return updatedHost.ID, nil
}

// UpdateHeartbeat updates the heartbeat data for a host.
// UpdateHeartbeat 更新主机的心跳数据。
// Requirements: 3.3 - Updates last heartbeat time, CPU, memory, and disk usage.
func (a *HostServiceAdapter) UpdateHeartbeat(ctx context.Context, agentID string, cpuUsage, memoryUsage, diskUsage float64) error {
	return a.hostService.UpdateHeartbeat(ctx, agentID, cpuUsage, memoryUsage, diskUsage)
}

// MarkHostOffline marks a host as offline by agent ID.
// MarkHostOffline 根据 Agent ID 将主机标记为离线。
// Requirements: 3.4 - Marks hosts as offline if no heartbeat received for 30 seconds.
func (a *HostServiceAdapter) MarkHostOffline(ctx context.Context, agentID string) error {
	// Get host by agent ID
	// 根据 Agent ID 获取主机
	h, err := a.hostService.GetByAgentID(ctx, agentID)
	if err != nil {
		return err
	}

	// Update agent status to offline
	// 更新 Agent 状态为离线
	return a.hostService.UpdateAgentStatusByID(ctx, h.ID, host.AgentStatusOffline, agentID, h.AgentVersion)
}

// Verify that HostServiceAdapter implements HostStatusUpdater interface.
// 验证 HostServiceAdapter 实现了 HostStatusUpdater 接口。
var _ HostStatusUpdater = (*HostServiceAdapter)(nil)
