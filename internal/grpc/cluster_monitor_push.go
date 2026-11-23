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

package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/processidentity"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"go.uber.org/zap"
)

// ErrClusterMonitorPushUnavailable 缺少主机服务、节点提供者或 Agent 管理器，无法按集群下发监控配置。
// ErrClusterMonitorPushUnavailable means the host service, node provider or Agent manager is missing, so a
// cluster-scoped monitor config cannot be pushed.
var ErrClusterMonitorPushUnavailable = errors.New("cluster monitor config push is not configured")

// clusterMonitorPushTimeout 是单次 UPDATE_MONITOR_CONFIG 命令的超时。
// clusterMonitorPushTimeout is the timeout of one UPDATE_MONITOR_CONFIG command.
const clusterMonitorPushTimeout = 30 * time.Second

// PushClusterMonitorConfigToHost 向主机 Agent 下发只针对一个集群的监控配置：跟踪列表只包含该集群在此主机上的节点；
// 该集群在此主机已无节点时下发空列表，Agent 据此只清理该集群的跟踪项，其他集群的跟踪项保持不变（缩容后调用）。
// PushClusterMonitorConfigToHost pushes a monitor config scoped to one cluster to the host's Agent: the
// tracked list holds only this cluster's nodes on the host; when the cluster has no node left there, an empty
// list is sent so the Agent drops only this cluster's tracked processes and keeps other clusters' entries
// (called after scale-in).
func (s *Server) PushClusterMonitorConfigToHost(ctx context.Context, clusterID uint, hostID uint) error {
	if clusterID == 0 || hostID == 0 {
		return fmt.Errorf("cluster id and host id are required / 集群 ID 与主机 ID 必填")
	}
	if s == nil || s.hostService == nil || s.agentManager == nil || clusterNodeProvider == nil {
		return ErrClusterMonitorPushUnavailable
	}

	host, err := s.hostService.Get(ctx, hostID)
	if err != nil {
		return fmt.Errorf("get host %d: %w", hostID, err)
	}
	agentID := strings.TrimSpace(host.AgentID)
	if agentID == "" {
		return fmt.Errorf("agent not installed on host %d", hostID)
	}

	nodes, err := clusterNodeProvider.GetNodesByHostID(ctx, hostID)
	if err != nil {
		return fmt.Errorf("get nodes of host %d: %w", hostID, err)
	}

	// 集群在此主机已无节点时，节点里拿不到监控配置，使用该集群自身的配置。
	// When the cluster has no node on this host, the monitor config cannot come from nodes; use the
	// cluster's own config.
	var fallback *monitor.MonitorConfig
	if monitorService != nil {
		if cfg, cfgErr := monitorService.GetConfig(ctx, clusterID); cfgErr == nil {
			fallback = cfg
		}
	}

	params, trackedCount, err := buildClusterMonitorParams(clusterID, nodes, fallback)
	if err != nil {
		return err
	}
	if _, err := s.agentManager.SendCommand(ctx, agentID, pb.CommandType_UPDATE_MONITOR_CONFIG, params, clusterMonitorPushTimeout); err != nil {
		s.logger.Warn("Failed to push cluster-scoped monitor config",
			zap.Uint("cluster_id", clusterID), zap.Uint("host_id", hostID), zap.String("agent_id", agentID), zap.Error(err))
		return fmt.Errorf("push monitor config to agent %s: %w", agentID, err)
	}
	s.logger.Info("Cluster-scoped monitor config pushed",
		zap.Uint("cluster_id", clusterID), zap.Uint("host_id", hostID), zap.String("agent_id", agentID), zap.Int("process_count", trackedCount))
	return nil
}

// buildClusterMonitorParams 构建按 cluster_id 限定的 UPDATE_MONITOR_CONFIG 参数：只纳入该集群中已运行或可监控的节点，
// tracked_processes 始终存在（无可跟踪节点时为 "[]"），使 Agent 进入“按集群替换”分支而不是跳过。
// buildClusterMonitorParams builds cluster-scoped UPDATE_MONITOR_CONFIG params: only this cluster's running or
// monitorable nodes are included, and tracked_processes is always present ("[]" when empty) so the Agent takes the
// "replace by cluster" branch instead of skipping.
func buildClusterMonitorParams(clusterID uint, nodes []*NodeWithMonitorConfig, fallback *monitor.MonitorConfig) (map[string]string, int, error) {
	tracked := make([]map[string]interface{}, 0)
	var config *monitor.MonitorConfig
	for _, node := range nodes {
		if node == nil || node.ClusterID != clusterID {
			continue
		}
		if config == nil && node.MonitorConfig != nil {
			config = node.MonitorConfig
		}
		if !shouldTrackNodeForMonitor(node) {
			continue
		}
		tracked = append(tracked, map[string]interface{}{
			"pid":         node.ProcessPID,
			"name":        processidentity.ManagedName(node.InstallDir, node.Role),
			"cluster_id":  node.ClusterID,
			"install_dir": node.InstallDir,
			"role":        node.Role,
			"java_home":   node.JavaHome,
		})
	}
	if config == nil {
		config = fallback
	}
	if config == nil {
		config = monitor.DefaultMonitorConfig(clusterID)
	}

	processesJSON, err := json.Marshal(tracked)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal tracked processes: %w", err)
	}
	params := map[string]string{
		"cluster_id":        fmt.Sprintf("%d", clusterID),
		"config_version":    fmt.Sprintf("%d", config.ConfigVersion),
		"auto_monitor":      fmt.Sprintf("%t", config.AutoMonitor),
		"auto_restart":      fmt.Sprintf("%t", config.AutoRestart),
		"monitor_interval":  fmt.Sprintf("%d", config.MonitorInterval),
		"restart_delay":     fmt.Sprintf("%d", config.RestartDelay),
		"max_restarts":      fmt.Sprintf("%d", config.MaxRestarts),
		"time_window":       fmt.Sprintf("%d", config.TimeWindow),
		"cooldown_period":   fmt.Sprintf("%d", config.CooldownPeriod),
		"tracked_processes": string(processesJSON),
	}
	return params, len(tracked), nil
}

// shouldTrackNodeForMonitor 判断节点是否应纳入 Agent 进程监控跟踪列表。
// 尚未完成安装启动（pending / installing）或已停止且无存活 PID（stopped）的节点不能以 PID=0 下发，
// 否则 Agent 在 auto_restart=true 时会把未安装、安装中或手动停止的节点误当崩溃进程自动拉起，
// 甚至与安装器的首次启动竞态导致 "process is already running"。
// shouldTrackNodeForMonitor reports whether a node belongs in the Agent's tracked process list.
// Unstarted (pending / installing) or stopped nodes without a live PID must not be pushed with PID=0,
// or the Agent treats them as crashed when auto_restart=true and auto-starts uninstalled, installing,
// or manually stopped nodes — racing with the installer's first start ("process is already running").
func shouldTrackNodeForMonitor(node *NodeWithMonitorConfig) bool {
	if node == nil || strings.TrimSpace(node.InstallDir) == "" {
		return false
	}
	if node.ProcessPID > 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(node.Status)) {
	case "pending", "installing", "stopped":
		return false
	default:
		return true
	}
}
