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

package monitor

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/LeonYoah/stx/internal/processidentity"
)

// AgentConfigSender defines the interface for sending config to agents.
// AgentConfigSender 定义向 Agent 发送配置的接口。
type AgentConfigSender interface {
	// SendMonitorConfig sends monitor config and tracked processes to agent.
	// SendMonitorConfig 向 Agent 发送监控配置和跟踪的进程列表。
	SendMonitorConfig(ctx context.Context, agentID string, config *MonitorConfig, processes []*TrackedProcessInfo) error
}

// TrackedProcessInfo represents a process to be tracked by the agent.
// TrackedProcessInfo 表示需要被 Agent 跟踪的进程信息。
type TrackedProcessInfo struct {
	PID        int    `json:"pid"`         // 进程 PID / Process PID
	Name       string `json:"name"`        // 进程名称 / Process name (e.g., "seatunnel-master")
	ClusterID  uint   `json:"cluster_id"`  // 集群 ID / Cluster ID
	InstallDir string `json:"install_dir"` // 安装目录 / Install directory
	Role       string `json:"role"`        // 节点角色 / Node role
}

// ClusterNodeProvider defines the interface for getting cluster node information.
// ClusterNodeProvider 定义获取集群节点信息的接口。
type ClusterNodeProvider interface {
	// GetNodesByClusterID returns all nodes for a cluster.
	// GetNodesByClusterID 返回集群的所有节点。
	GetNodesByClusterID(ctx context.Context, clusterID uint) ([]*NodeInfoForMonitor, error)
}

// NodeInfoForMonitor represents node info needed for monitoring.
// NodeInfoForMonitor 表示监控所需的节点信息。
type NodeInfoForMonitor struct {
	HostID     uint   `json:"host_id"`
	AgentID    string `json:"agent_id"`
	InstallDir string `json:"install_dir"`
	Role       string `json:"role"`
	ProcessPID int    `json:"process_pid"`
}

// Service provides monitor configuration and event management.
// Service 提供监控配置和事件管理。
type Service struct {
	repo            *Repository
	configSender    AgentConfigSender
	nodeProvider    ClusterNodeProvider
	onEventRecorded func(context.Context, *ProcessEvent) error
	// k8sChecker 解耦判断集群是否 K8s 基础设施（由 router 接线，避免反向依赖 cluster 包）。
	// k8sChecker reports whether a cluster runs on Kubernetes (wired in router to avoid
	// a reverse dependency on the cluster package).
	k8sChecker func(ctx context.Context, clusterID uint) bool
}

// NewService creates a new monitor service.
// NewService 创建新的监控服务。
func NewService(repo *Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// SetConfigSender sets the agent config sender.
// SetConfigSender 设置 Agent 配置发送器。
func (s *Service) SetConfigSender(sender AgentConfigSender) {
	s.configSender = sender
}

// SetKubernetesChecker injects the Kubernetes infrastructure checker.
// SetKubernetesChecker 注入 K8s 基础设施检查器。
func (s *Service) SetKubernetesChecker(checker func(ctx context.Context, clusterID uint) bool) {
	s.k8sChecker = checker
}

// SetNodeProvider sets the cluster node provider.
// SetNodeProvider 设置集群节点提供者。
func (s *Service) SetNodeProvider(provider ClusterNodeProvider) {
	s.nodeProvider = provider
}

// SetOnEventRecorded sets an optional hook invoked after a process event is persisted.
// SetOnEventRecorded 设置进程事件落库后的可选回调。
func (s *Service) SetOnEventRecorded(fn func(context.Context, *ProcessEvent) error) {
	s.onEventRecorded = fn
}

// ==================== MonitorConfig Operations 监控配置操作 ====================

// GetConfig retrieves monitor config for a cluster.
// GetConfig 获取集群的监控配置。
// Requirements: 5.2 - Get monitor config
func (s *Service) GetConfig(ctx context.Context, clusterID uint) (*MonitorConfig, error) {
	return s.repo.GetConfigByClusterID(ctx, clusterID)
}

// GetOrCreateConfig retrieves or creates default monitor config for a cluster.
// GetOrCreateConfig 获取或创建集群的默认监控配置。
// Requirements: 5.2, 5.7 - Get or create default config
// **Feature: seatunnel-process-monitor, Property 14: 新集群默认配置**
// **Validates: Requirements 5.2**
func (s *Service) GetOrCreateConfig(ctx context.Context, clusterID uint) (*MonitorConfig, error) {
	config, err := s.repo.GetConfigByClusterID(ctx, clusterID)
	if err == ErrConfigNotFound {
		// Create default config / 创建默认配置
		config = DefaultMonitorConfig(clusterID)
		if err := s.repo.CreateConfig(ctx, config); err != nil {
			return nil, err
		}
		log.Printf("[Monitor] Created default config for cluster %d", clusterID)
		return config, nil
	}
	if err != nil {
		return nil, err
	}

	// Fix legacy records with zero values / 修复旧记录的零值
	// 如果配置存在但关键字段为零值，则应用默认值
	needsUpdate := false
	defaults := DefaultMonitorConfig(clusterID)

	if config.MonitorInterval <= 0 {
		config.MonitorInterval = defaults.MonitorInterval
		needsUpdate = true
	}
	if config.RestartDelay <= 0 {
		config.RestartDelay = defaults.RestartDelay
		needsUpdate = true
	}
	if config.MaxRestarts <= 0 {
		config.MaxRestarts = defaults.MaxRestarts
		needsUpdate = true
	}
	if config.TimeWindow <= 0 {
		config.TimeWindow = defaults.TimeWindow
		needsUpdate = true
	}
	if config.CooldownPeriod <= 0 {
		config.CooldownPeriod = defaults.CooldownPeriod
		needsUpdate = true
	}
	if !config.AutoMonitor {
		config.AutoMonitor = true
		needsUpdate = true
	}

	// Update database if defaults were applied / 如果应用了默认值则更新数据库
	if needsUpdate {
		if err := s.repo.UpdateConfig(ctx, config); err != nil {
			log.Printf("[Monitor] Failed to update legacy config for cluster %d: %v", clusterID, err)
		} else {
			log.Printf("[Monitor] Applied default values to legacy config for cluster %d", clusterID)
		}
	}

	return config, nil
}

// UpdateConfig updates monitor config for a cluster.
// UpdateConfig 更新集群的监控配置。
// Requirements: 5.4 - Update monitor config
// **Feature: seatunnel-process-monitor, Property 13: 配置热更新**
// **Validates: Requirements 5.5**
func (s *Service) UpdateConfig(ctx context.Context, clusterID uint, req *UpdateMonitorConfigRequest) (*MonitorConfig, error) {
	// K8s 集群无主机 Agent：进程自愈由 kubelet RestartPolicy 与 Liveness/Readiness Probe
	// 原生托管，Agent 自愈参数无意义，必须拒绝保存防止空转后误导用户"已同步"。
	// Kubernetes clusters have no host agent: process self-healing is natively managed by
	// the kubelet RestartPolicy and Liveness/Readiness probes, so agent self-healing
	// parameters are meaningless and must be rejected instead of idling misleadingly.
	if s.k8sChecker != nil && s.k8sChecker(ctx, clusterID) {
		return nil, ErrKubernetesMonitorConfigUnsupported
	}

	// Validate request / 验证请求
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Get existing config or create default / 获取现有配置或创建默认配置
	config, err := s.GetOrCreateConfig(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	// Apply updates / 应用更新
	// AutoMonitor is always on for managed clusters.
	// AutoMonitor 对受管集群始终保持开启。
	config.AutoMonitor = true
	if req.AutoRestart != nil {
		config.AutoRestart = *req.AutoRestart
	}
	if req.MonitorInterval != nil {
		config.MonitorInterval = *req.MonitorInterval
	}
	if req.RestartDelay != nil {
		config.RestartDelay = *req.RestartDelay
	}
	if req.MaxRestarts != nil {
		config.MaxRestarts = *req.MaxRestarts
	}
	if req.TimeWindow != nil {
		config.TimeWindow = *req.TimeWindow
	}
	if req.CooldownPeriod != nil {
		config.CooldownPeriod = *req.CooldownPeriod
	}

	// Increment version / 递增版本号
	config.ConfigVersion++

	// Save config / 保存配置
	if err := s.repo.UpdateConfig(ctx, config); err != nil {
		return nil, err
	}

	log.Printf("[Monitor] Updated config for cluster %d, version %d", clusterID, config.ConfigVersion)

	// Push config to agents / 推送配置到 Agent
	// This is done asynchronously to not block the API response
	// Use a background context since the HTTP request context will be canceled after response
	// 异步执行以不阻塞 API 响应
	// 使用后台 context，因为 HTTP 请求 context 在响应后会被取消
	go s.pushConfigToAgents(context.Background(), clusterID, config)

	return config, nil
}

// pushConfigToAgents pushes monitor config to all agents in the cluster.
// pushConfigToAgents 将监控配置推送到集群中的所有 Agent。
func (s *Service) pushConfigToAgents(ctx context.Context, clusterID uint, config *MonitorConfig) {
	if s.configSender == nil || s.nodeProvider == nil {
		log.Printf("[Monitor] Config sender or node provider not configured, skipping push")
		return
	}

	// Get all nodes in the cluster / 获取集群中的所有节点
	nodes, err := s.nodeProvider.GetNodesByClusterID(ctx, clusterID)
	if err != nil {
		log.Printf("[Monitor] Failed to get nodes for cluster %d: %v", clusterID, err)
		return
	}

	// Group nodes by agent ID / 按 Agent ID 分组节点
	agentProcesses := make(map[string][]*TrackedProcessInfo)
	for _, node := range nodes {
		if node.AgentID == "" {
			continue
		}
		// When auto-restart is enabled, track all processes (including PID=0) so agent can restart them
		// When auto-restart is disabled, only track running processes (PID > 0)
		// 当启用自动重启时，跟踪所有进程（包括 PID=0），以便 Agent 可以重启它们
		// 当禁用自动重启时，只跟踪运行中的进程（PID > 0）
		if config.AutoRestart || node.ProcessPID > 0 {
			agentProcesses[node.AgentID] = append(agentProcesses[node.AgentID], &TrackedProcessInfo{
				PID:        node.ProcessPID,
				Name:       processidentity.ManagedName(node.InstallDir, node.Role),
				ClusterID:  clusterID,
				InstallDir: node.InstallDir,
				Role:       node.Role,
			})
		}
	}

	// Send config to each agent / 向每个 Agent 发送配置
	for agentID, processes := range agentProcesses {
		if err := s.configSender.SendMonitorConfig(ctx, agentID, config, processes); err != nil {
			log.Printf("[Monitor] Failed to send config to agent %s: %v", agentID, err)
		} else {
			log.Printf("[Monitor] Sent config to agent %s with %d processes", agentID, len(processes))
		}
	}

	// Mark config as synced / 标记配置已同步
	s.MarkConfigSynced(ctx, clusterID)
}

// PushDisableConfigForCluster pushes a config with auto_monitor=false and auto_restart=false
// to all agents that have nodes in this cluster, so they stop monitoring and won't restart processes.
// Call before deleting the cluster so agents are notified.
// PushDisableConfigForCluster 向该集群下所有 Agent 推送关闭监控的配置，使 Agent 停止监控且不再重启进程；删除集群前调用以通知 Agent。
func (s *Service) PushDisableConfigForCluster(ctx context.Context, clusterID uint) {
	if s.configSender == nil || s.nodeProvider == nil {
		log.Printf("[Monitor] PushDisableConfigForCluster: config sender or node provider not set, skip")
		return
	}
	nodes, err := s.nodeProvider.GetNodesByClusterID(ctx, clusterID)
	if err != nil || len(nodes) == 0 {
		return
	}
	config, _ := s.repo.GetConfigByClusterID(ctx, clusterID)
	if config == nil {
		config = DefaultMonitorConfig(clusterID)
	}
	config.AutoMonitor = false
	config.AutoRestart = false
	config.ConfigVersion++
	agentIDs := make(map[string]struct{})
	for _, node := range nodes {
		if node.AgentID != "" {
			agentIDs[node.AgentID] = struct{}{}
		}
	}
	for agentID := range agentIDs {
		if err := s.configSender.SendMonitorConfig(ctx, agentID, config, nil); err != nil {
			log.Printf("[Monitor] PushDisableConfigForCluster: send to agent %s failed: %v", agentID, err)
		} else {
			log.Printf("[Monitor] PushDisableConfigForCluster: sent disable config to agent %s", agentID)
		}
	}
}

// DeleteConfig deletes monitor config for a cluster.
// DeleteConfig 删除集群的监控配置。
func (s *Service) DeleteConfig(ctx context.Context, clusterID uint) error {
	return s.repo.DeleteConfigByClusterID(ctx, clusterID)
}

// MarkConfigSynced marks the config as synced to agents.
// MarkConfigSynced 标记配置已同步到 Agent。
func (s *Service) MarkConfigSynced(ctx context.Context, clusterID uint) error {
	config, err := s.repo.GetConfigByClusterID(ctx, clusterID)
	if err != nil {
		return err
	}
	now := time.Now()
	config.LastSyncAt = &now
	return s.repo.UpdateConfig(ctx, config)
}

// ==================== ProcessEvent Operations 进程事件操作 ====================

// RecordEvent records a new process event.
// RecordEvent 记录新的进程事件。
// Requirements: 6.1 - Record process events
func (s *Service) RecordEvent(ctx context.Context, event *ProcessEvent) error {
	if err := s.repo.CreateEvent(ctx, event); err != nil {
		return err
	}
	log.Printf("[Monitor] Recorded event: type=%s, cluster=%d, node=%d, pid=%d", event.EventType, event.ClusterID, event.NodeID, event.PID)
	s.notifyEventRecorded(ctx, event)
	return nil
}

// RecordEventFromReport records an event from agent report.
// RecordEventFromReport 从 Agent 上报记录事件。
// Requirements: 3.4, 3.5 - Process event from agent
func (s *Service) RecordEventFromReport(ctx context.Context, clusterID, nodeID, hostID uint, eventType ProcessEventType, pid int, processName, installDir, role string, details map[string]string) error {
	detailsJSON, _ := json.Marshal(details)
	event := &ProcessEvent{
		ClusterID:   clusterID,
		NodeID:      nodeID,
		HostID:      hostID,
		EventType:   eventType,
		PID:         pid,
		ProcessName: processName,
		InstallDir:  installDir,
		Role:        role,
		Details:     string(detailsJSON),
	}
	return s.RecordEvent(ctx, event)
}

func (s *Service) notifyEventRecorded(ctx context.Context, event *ProcessEvent) {
	if s.onEventRecorded == nil || event == nil {
		return
	}

	eventCopy := *event
	go func(parent context.Context, copied ProcessEvent) {
		hookCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.onEventRecorded(hookCtx, &copied); err != nil {
			log.Printf("[Monitor] onEventRecorded failed: cluster=%d event=%d type=%s err=%v / 事件回调失败: cluster=%d event=%d type=%s err=%v",
				copied.ClusterID, copied.ID, copied.EventType, err,
				copied.ClusterID, copied.ID, copied.EventType, err)
		}
		_ = parent
	}(ctx, eventCopy)
}

// GetEvent retrieves a process event by ID.
// GetEvent 根据 ID 获取进程事件。
func (s *Service) GetEvent(ctx context.Context, id uint) (*ProcessEvent, error) {
	return s.repo.GetEventByID(ctx, id)
}

// ListEvents retrieves process events with filtering.
// ListEvents 获取带过滤的进程事件列表。
// Requirements: 6.4 - List process events
func (s *Service) ListEvents(ctx context.Context, filter *ProcessEventFilter) ([]*ProcessEventWithHost, int64, error) {
	// Set default pagination / 设置默认分页
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	return s.repo.ListEventsWithHost(ctx, filter)
}

// ListClusterEvents retrieves recent events for a cluster.
// ListClusterEvents 获取集群的最近事件。
func (s *Service) ListClusterEvents(ctx context.Context, clusterID uint, limit int) ([]*ProcessEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListEventsByClusterID(ctx, clusterID, limit)
}

// DeleteClusterEvents deletes all events for a cluster.
// DeleteClusterEvents 删除集群的所有事件。
func (s *Service) DeleteClusterEvents(ctx context.Context, clusterID uint) error {
	return s.repo.DeleteEventsByClusterID(ctx, clusterID)
}

// GetLatestNodeEvent retrieves the latest event for a node.
// GetLatestNodeEvent 获取节点的最新事件。
func (s *Service) GetLatestNodeEvent(ctx context.Context, nodeID uint) (*ProcessEvent, error) {
	return s.repo.GetLatestEventByNodeID(ctx, nodeID)
}

// GetLatestNodeEventByTypes retrieves the latest node event within the given event types.
// GetLatestNodeEventByTypes 获取某节点在指定事件类型集合中的最新事件。
func (s *Service) GetLatestNodeEventByTypes(ctx context.Context, nodeID uint, eventTypes []ProcessEventType) (*ProcessEvent, error) {
	return s.repo.GetLatestEventByNodeIDAndTypes(ctx, nodeID, eventTypes)
}

// HasNodeEventAfter returns whether one node has any matching event after the given time.
// HasNodeEventAfter 返回某节点在给定时间之后是否存在匹配事件。
func (s *Service) HasNodeEventAfter(ctx context.Context, nodeID uint, after time.Time, eventTypes []ProcessEventType) (bool, *ProcessEvent, error) {
	return s.repo.HasNodeEventAfter(ctx, nodeID, after, eventTypes)
}

// ListNodeEventsAfter 批量读取若干节点在指定时间之后的匹配事件，供告警生命周期一次判定，避免逐条 HasNodeEventAfter。
// ListNodeEventsAfter batch-loads matching events after a time so alert lifecycle can be resolved without N+1 HasNodeEventAfter calls.
func (s *Service) ListNodeEventsAfter(ctx context.Context, nodeIDs []uint, after time.Time, eventTypes []ProcessEventType) ([]ProcessEvent, error) {
	return s.repo.ListNodeEventsAfter(ctx, nodeIDs, after, eventTypes)
}

// GetLatestSubjectEventByTypes 返回某告警主体（cluster+node+host 元组）在指定类型集合中的最新事件。
// GetLatestSubjectEventByTypes returns the latest event of the given types for one alert
// subject (cluster+node+host tuple).
func (s *Service) GetLatestSubjectEventByTypes(ctx context.Context, clusterID, nodeID, hostID uint, eventTypes []ProcessEventType) (*ProcessEvent, error) {
	return s.repo.GetLatestEventBySubject(ctx, clusterID, nodeID, hostID, eventTypes)
}

// ListClusterEventsAfter 批量读取若干集群在指定时间之后的匹配事件，供告警生命周期判定。
// ListClusterEventsAfter batch-loads matching cluster events after a time for alert
// lifecycle resolution.
func (s *Service) ListClusterEventsAfter(ctx context.Context, clusterIDs []uint, after time.Time, eventTypes []ProcessEventType) ([]ProcessEvent, error) {
	return s.repo.ListClusterEventsAfter(ctx, clusterIDs, after, eventTypes)
}

// GetEventStats retrieves event statistics for a cluster.
// GetEventStats 获取集群的事件统计。
func (s *Service) GetEventStats(ctx context.Context, clusterID uint, since *time.Time) (map[ProcessEventType]int64, error) {
	stats := make(map[ProcessEventType]int64)
	eventTypes := []ProcessEventType{
		EventTypeStarted,
		EventTypeStopped,
		EventTypeCrashed,
		EventTypeRestarted,
		EventTypeRestartFailed,
		EventTypeRestartLimitReached,
		EventTypeNodeOffline,
		EventTypeNodeRecovered,
	}
	for _, eventType := range eventTypes {
		count, err := s.repo.CountEventsByType(ctx, clusterID, eventType, since)
		if err != nil {
			return nil, err
		}
		stats[eventType] = count
	}
	return stats, nil
}
