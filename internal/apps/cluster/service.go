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

// Package cluster provides cluster management functionality for the STX Agent system.
// cluster 包提供 STX Agent 系统的集群管理功能。
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/seatunnel"
	"gopkg.in/yaml.v3"
)

// HealthStatus represents the health status of a cluster.
// HealthStatus 表示集群的健康状态。
type HealthStatus string

const (
	// HealthStatusHealthy indicates all nodes are online and running.
	// HealthStatusHealthy 表示所有节点都在线且运行正常。
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusUnhealthy indicates one or more nodes are offline or in error state.
	// HealthStatusUnhealthy 表示一个或多个节点离线或处于错误状态。
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	// HealthStatusUnknown indicates the health status cannot be determined.
	// HealthStatusUnknown 表示无法确定健康状态。
	HealthStatusUnknown HealthStatus = "unknown"
)

// HostInfo represents host information needed by cluster service.
// HostInfo 表示集群服务所需的主机信息。
// This interface decouples cluster from host package to avoid import cycles.
// 此接口将集群与主机包解耦以避免导入循环。
type HostInfo struct {
	ID               uint
	Name             string
	HostType         string
	IPAddress        string
	AgentID          string
	AgentStatus      string
	LastHeartbeat    *time.Time
	ProcessStartedAt *time.Time // when set, online requires heartbeat after this (e.g. API process start)
}

// IsOnline checks if the host is online based on heartbeat timeout.
// When ProcessStartedAt is set, also requires last_heartbeat to be after that time.
func (h *HostInfo) IsOnline(timeout time.Duration) bool {
	if h.LastHeartbeat == nil {
		return false
	}
	if h.ProcessStartedAt != nil && !h.LastHeartbeat.After(*h.ProcessStartedAt) {
		return false
	}
	return time.Since(*h.LastHeartbeat) <= timeout
}

// HostProvider is an interface for retrieving host information.
// HostProvider 是获取主机信息的接口。
// This interface decouples cluster service from host package.
// 此接口将集群服务与主机包解耦。
type HostProvider interface {
	// GetHostByID retrieves host information by ID.
	// GetHostByID 根据 ID 获取主机信息。
	GetHostByID(ctx context.Context, id uint) (*HostInfo, error)
}

// ClusterStatusInfo represents detailed cluster status information.
// ClusterStatusInfo 表示详细的集群状态信息。
type ClusterStatusInfo struct {
	ClusterID    uint              `json:"cluster_id"`
	ClusterName  string            `json:"cluster_name"`
	Status       ClusterStatus     `json:"status"`
	HealthStatus HealthStatus      `json:"health_status"`
	TotalNodes   int               `json:"total_nodes"`
	OnlineNodes  int               `json:"online_nodes"`
	OfflineNodes int               `json:"offline_nodes"`
	Nodes        []*NodeStatusInfo `json:"nodes"`
}

// NodeStatusInfo represents detailed node status information.
// NodeStatusInfo 表示详细的节点状态信息。
type NodeStatusInfo struct {
	NodeID     uint       `json:"node_id"`
	HostID     uint       `json:"host_id"`
	HostName   string     `json:"host_name"`
	HostIP     string     `json:"host_ip"`
	Role       NodeRole   `json:"role"`
	Status     NodeStatus `json:"status"`      // Unified status: pending, installing, running, stopped, error / 统一状态
	IsOnline   bool       `json:"is_online"`   // Whether host is online / 主机是否在线
	ProcessPID int        `json:"process_pid"` // SeaTunnel process PID / SeaTunnel 进程 PID
}

// OperationType represents the type of cluster operation.
// OperationType 表示集群操作类型。
type OperationType string

const (
	// OperationStart starts the cluster.
	// OperationStart 启动集群。
	OperationStart OperationType = "start"
	// OperationStop stops the cluster.
	// OperationStop 停止集群。
	OperationStop OperationType = "stop"
	// OperationRestart restarts the cluster.
	// OperationRestart 重启集群。
	OperationRestart OperationType = "restart"
)

// OperationResult represents the result of a cluster operation.
// OperationResult 表示集群操作的结果。
type OperationResult struct {
	ClusterID   uint                   `json:"cluster_id"`
	Operation   OperationType          `json:"operation"`
	Success     bool                   `json:"success"`
	Message     string                 `json:"message"`
	NodeResults []*NodeOperationResult `json:"node_results"`
}

// NodeOperationResult represents the result of an operation on a single node.
// NodeOperationResult 表示单个节点操作的结果。
type NodeOperationResult struct {
	NodeID   uint   `json:"node_id"`
	HostID   uint   `json:"host_id"`
	HostName string `json:"host_name"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

// AgentCommandSender is an interface for sending commands to agents.
// AgentCommandSender 是向 Agent 发送命令的接口。
// This interface will be implemented by the Agent Manager in Phase 4.
// 此接口将在第 4 阶段由 Agent Manager 实现。
type AgentCommandSender interface {
	// SendCommand sends a command to an agent and returns the result.
	// SendCommand 向 Agent 发送命令并返回结果。
	SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error)
}

// ConfigAgentClient reads/writes node config files via Agent.
// ConfigAgentClient 通过 Agent 读写节点配置文件。
type ConfigAgentClient interface {
	PullConfig(ctx context.Context, hostID uint, installDir string, configType appconfig.ConfigType) (string, error)
	PushConfig(ctx context.Context, hostID uint, installDir string, configType appconfig.ConfigType, content string) error
}

// Service provides business logic for cluster management operations.
// Service 提供集群管理操作的业务逻辑。
type Service struct {
	repo                     *Repository
	hostProvider             HostProvider
	heartbeatTimeout         time.Duration
	agentSender              AgentCommandSender
	configAgentClient        ConfigAgentClient
	onBeforeClusterDelete    func(context.Context, uint) // optional hook for monitor cleanup etc.
	onClusterTopologyChanged func(context.Context, uint) // optional hook for observability sync etc.
}

// ServiceConfig holds configuration for the Cluster Service.
// ServiceConfig 保存 Cluster Service 的配置。
type ServiceConfig struct {
	HeartbeatTimeout time.Duration
}

// NewService creates a new Service instance.
// NewService 创建一个新的 Service 实例。
func NewService(repo *Repository, hostProvider HostProvider, cfg *ServiceConfig) *Service {
	timeout := 30 * time.Second
	if cfg != nil && cfg.HeartbeatTimeout > 0 {
		timeout = cfg.HeartbeatTimeout
	}

	return &Service{
		repo:             repo,
		hostProvider:     hostProvider,
		heartbeatTimeout: timeout,
	}
}

// SetAgentCommandSender sets the agent command sender for cluster operations.
// SetAgentCommandSender 设置用于集群操作的 Agent 命令发送器。
func (s *Service) SetAgentCommandSender(sender AgentCommandSender) {
	s.agentSender = sender
}

// SetConfigAgentClient sets the config agent client used for config file synchronization.
// SetConfigAgentClient 设置用于配置文件同步的 Agent 配置客户端。
func (s *Service) SetConfigAgentClient(client ConfigAgentClient) {
	s.configAgentClient = client
}

// SetOnBeforeClusterDelete sets an optional hook called before cluster DB deletion (e.g. monitor config cleanup).
// SetOnBeforeClusterDelete 设置删除集群前可选钩子（如清理监控配置）。
func (s *Service) SetOnBeforeClusterDelete(fn func(context.Context, uint)) {
	s.onBeforeClusterDelete = fn
}

// SetOnClusterTopologyChanged sets an optional hook called when cluster topology changes
// (create/update/add-node/remove-node/delete), typically used for observability target sync.
// SetOnClusterTopologyChanged 设置集群拓扑变更回调（创建/更新/加减节点/删除），通常用于可观测目标同步。
func (s *Service) SetOnClusterTopologyChanged(fn func(context.Context, uint)) {
	s.onClusterTopologyChanged = fn
}

func (s *Service) notifyClusterTopologyChanged(ctx context.Context, clusterID uint) {
	if s.onClusterTopologyChanged == nil {
		return
	}
	go func(parent context.Context) {
		hookCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				logger.WarnF(parent, "[Cluster] topology change hook panic recovered: cluster=%d, panic=%v", clusterID, r)
			}
		}()
		s.onClusterTopologyChanged(hookCtx, clusterID)
	}(ctx)
}

// Create creates a new cluster with validation.
// Create 创建一个新集群并进行验证。
// Requirements: 7.1 - Validates cluster name uniqueness and stores basic info.
func (s *Service) Create(ctx context.Context, req *CreateClusterRequest) (*Cluster, error) {
	// Validate cluster name is not empty
	// 验证集群名不为空
	if req.Name == "" {
		return nil, ErrClusterNameEmpty
	}

	// Validate deployment mode
	// 验证部署模式
	if !isValidDeploymentMode(req.DeploymentMode) {
		return nil, ErrInvalidDeploymentMode
	}

	// Create cluster
	// 创建集群
	cluster := &Cluster{
		Name:           req.Name,
		Description:    req.Description,
		DeploymentMode: req.DeploymentMode,
		Version:        req.Version,
		Status:         ClusterStatusCreated,
		InstallDir:     req.InstallDir,
		Config:         req.Config,
	}

	if err := s.repo.Create(ctx, cluster); err != nil {
		return nil, err
	}

	// Auto-create nodes from discovery if provided
	// 如果提供了发现的节点，自动创建节点
	if len(req.Nodes) > 0 {
		logger.InfoF(ctx, "[Cluster] Auto-creating %d nodes from discovery for cluster %s / 为集群 %s 自动创建 %d 个发现的节点",
			len(req.Nodes), cluster.Name, cluster.Name, len(req.Nodes))

		for _, nodeReq := range req.Nodes {
			// Convert role string to NodeRole
			// 将角色字符串转换为 NodeRole
			var role NodeRole
			switch nodeReq.Role {
			case "master":
				role = NodeRoleMaster
			case "worker":
				role = NodeRoleWorker
			case "hybrid":
				role = NodeRoleMasterWorker
			default:
				// Default to hybrid if unknown role / 未知角色默认为混合模式
				role = NodeRoleMasterWorker
			}

			// Use discovered ports if available, otherwise use defaults
			// 如果有发现的端口则使用，否则使用默认值
			hazelcastPort, apiPort, workerPort := GetDefaultPorts(role, req.DeploymentMode)
			if nodeReq.HazelcastPort > 0 {
				hazelcastPort = nodeReq.HazelcastPort
			}
			if nodeReq.APIPort > 0 {
				apiPort = nodeReq.APIPort
			}

			addNodeReq := &AddNodeRequest{
				HostID:        nodeReq.HostID,
				Role:          role,
				InstallDir:    nodeReq.InstallDir,
				HazelcastPort: hazelcastPort,
				APIPort:       apiPort,
				WorkerPort:    workerPort,
				SkipPrecheck:  true, // Skip precheck for discovered nodes / 跳过发现节点的预检查
			}

			_, err := s.AddNode(ctx, cluster.ID, addNodeReq)
			if err != nil {
				// Log error but continue with other nodes
				// 记录错误但继续处理其他节点
				logger.ErrorF(ctx, "[Cluster] Failed to auto-create node for host %d: %v / 为主机 %d 自动创建节点失败: %v",
					nodeReq.HostID, err, nodeReq.HostID, err)
			} else {
				logger.InfoF(ctx, "[Cluster] Auto-created node: host_id=%d, role=%s, install_dir=%s, hazelcast_port=%d, api_port=%d / 自动创建节点: host_id=%d, role=%s, install_dir=%s, hazelcast_port=%d, api_port=%d",
					nodeReq.HostID, role, nodeReq.InstallDir, hazelcastPort, apiPort, nodeReq.HostID, role, nodeReq.InstallDir, hazelcastPort, apiPort)
			}
		}

		// Reload cluster with nodes
		// 重新加载集群及其节点
		cluster, _ = s.repo.GetByID(ctx, cluster.ID, true)
	}

	s.notifyClusterTopologyChanged(ctx, cluster.ID)

	return cluster, nil
}

// Get retrieves a cluster by ID with optional node preloading.
// Get 根据 ID 获取集群，可选择预加载节点。
// Requirements: 7.3 - Returns cluster name, status, node list, version info, creation time.
func (s *Service) Get(ctx context.Context, id uint) (*Cluster, error) {
	// Update cluster status based on nodes before returning
	// 返回前根据节点状态更新集群状态
	s.updateClusterStatusFromNodes(ctx, id)

	return s.repo.GetByID(ctx, id, true)
}

// GetClusterVersion retrieves the version of a cluster by ID.
// GetClusterVersion 根据 ID 获取集群的版本。
// This method implements the ClusterGetter interface for plugin version validation.
// 此方法实现 ClusterGetter 接口用于插件版本校验。
func (s *Service) GetClusterVersion(ctx context.Context, clusterID uint) (string, error) {
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return "", err
	}
	return cluster.Version, nil
}

// GetByName retrieves a cluster by name.
// GetByName 根据名称获取集群。
func (s *Service) GetByName(ctx context.Context, name string) (*Cluster, error) {
	return s.repo.GetByName(ctx, name)
}

// List retrieves clusters based on filter criteria.
// List 根据过滤条件获取集群列表。
// Requirements: 7.3 - Returns cluster list with node count.
func (s *Service) List(ctx context.Context, filter *ClusterFilter) ([]*Cluster, int64, error) {
	clusters, _, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	// Update each cluster's status based on nodes
	// 根据节点状态更新每个集群的状态
	for _, c := range clusters {
		s.updateClusterStatusFromNodes(ctx, c.ID)
	}

	// Re-fetch to get updated statuses
	// 重新获取以获得更新后的状态
	return s.repo.List(ctx, filter)
}

// ListWithInfo retrieves clusters and converts them to ClusterInfo with online_nodes and health_status.
// ListWithInfo 获取集群列表并转换为 ClusterInfo，包含 online_nodes 与 health_status。
func (s *Service) ListWithInfo(ctx context.Context, filter *ClusterFilter) ([]*ClusterInfo, int64, error) {
	clusters, total, err := s.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	infos := make([]*ClusterInfo, len(clusters))
	for i, c := range clusters {
		infos[i] = c.ToClusterInfo()
		// Populate online_nodes and health_status so list UI can show "异常" when running but 0 online
		status, err := s.GetStatus(ctx, c.ID)
		if err == nil {
			infos[i].Status = status.Status
			infos[i].OnlineNodes = status.OnlineNodes
			infos[i].HealthStatus = string(status.HealthStatus)
		}
	}

	return infos, total, nil
}

// Update updates an existing cluster with validation.
// Update 更新现有集群并进行验证。
func (s *Service) Update(ctx context.Context, id uint, req *UpdateClusterRequest) (*Cluster, error) {
	// Get existing cluster
	// 获取现有集群
	cluster, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	// 如果提供了字段则更新
	if req.Name != nil {
		if *req.Name == "" {
			return nil, ErrClusterNameEmpty
		}
		cluster.Name = *req.Name
	}

	if req.Description != nil {
		cluster.Description = *req.Description
	}

	if req.Version != nil {
		cluster.Version = *req.Version
	}

	if req.InstallDir != nil {
		cluster.InstallDir = *req.InstallDir
	}

	if req.Config != nil {
		cluster.Config = *req.Config
	}

	if err := s.repo.Update(ctx, cluster); err != nil {
		return nil, err
	}

	s.notifyClusterTopologyChanged(ctx, cluster.ID)

	return cluster, nil
}

// Delete removes a cluster after checking for running tasks.
// Before DB deletion it sends stop to all nodes' agents (best effort). If forceRemoveInstallDir is true, also sends REMOVE_INSTALL_DIR to each agent.
// Delete 在检查运行中的任务后删除集群；删除前向各节点 Agent 发送停止命令；若 forceRemoveInstallDir 为 true 则再发送删除安装目录命令。
// Requirements: 7.5 - Checks if cluster has running tasks before deletion.
func (s *Service) Delete(ctx context.Context, id uint, forceRemoveInstallDir bool) error {
	// Get cluster to check status
	// 获取集群以检查状态
	cluster, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return err
	}

	// Check if cluster has running tasks (deploying or running status)
	// 检查集群是否有运行中的任务（部署中或运行中状态）
	if cluster.Status == ClusterStatusDeploying || cluster.Status == ClusterStatusRunning {
		return ErrClusterHasRunningTask
	}

	// Get cluster with nodes to send stop to each node's agent (best effort)
	// 获取带节点的集群，向各节点 Agent 发送停止命令（尽力而为）
	clusterWithNodes, err := s.repo.GetByID(ctx, id, true)
	if err == nil && len(clusterWithNodes.Nodes) > 0 {
		s.stopProcessesForDeletion(ctx, clusterWithNodes)
		if forceRemoveInstallDir {
			s.removeInstallDirOnAgents(ctx, clusterWithNodes)
		}
	}

	// Optional hook (e.g. delete monitor config and events for this cluster)
	// 可选钩子（如删除该集群的监控配置与事件）
	if s.onBeforeClusterDelete != nil {
		s.onBeforeClusterDelete(ctx, id)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	s.notifyClusterTopologyChanged(ctx, id)
	return nil
}

// stopProcessesForDeletion sends stop command to each node's agent so actual SeaTunnel processes are stopped.
// Best effort: logs errors but does not fail the deletion.
// stopProcessesForDeletion 向各节点 Agent 发送停止命令以停止主机上的 SeaTunnel 进程；尽力而为，不阻断删除。
func (s *Service) stopProcessesForDeletion(ctx context.Context, cluster *Cluster) {
	if s.hostProvider == nil || s.agentSender == nil {
		logger.WarnF(ctx, "[Cluster] Delete: skip notifying agents (hostProvider or agentSender not set) / 删除集群：未通知 Agent（主机提供者或命令发送器未配置）")
		return
	}
	for _, node := range cluster.Nodes {
		hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
		if err != nil || hostInfo.AgentID == "" {
			logger.WarnF(ctx, "[Cluster] Delete: skip node (no host or no agent) / 删除集群：跳过节点: node_id=%d, host_id=%d", node.ID, node.HostID)
			continue
		}
		installDir := node.InstallDir
		if installDir == "" {
			installDir = cluster.InstallDir
		}
		params := map[string]string{
			"cluster_id":  fmt.Sprintf("%d", cluster.ID),
			"node_id":     fmt.Sprintf("%d", node.ID),
			"role":        string(node.Role),
			"install_dir": installDir,
		}
		logger.InfoF(ctx, "[Cluster] Delete: sending stop to agent / 删除集群：向 Agent 发送停止命令: agent_id=%s, node_id=%d", hostInfo.AgentID, node.ID)
		_, _, err = s.agentSender.SendCommand(ctx, hostInfo.AgentID, string(OperationStop), params)
		if err != nil {
			logger.WarnF(ctx, "[Cluster] Delete: stop process on agent failed / 删除集群时向 Agent 发送停止失败: host_id=%d, node_id=%d, err=%v", node.HostID, node.ID, err)
		}
	}
}

// removeInstallDirOnAgents sends REMOVE_INSTALL_DIR to each node's agent so the install directory is removed on the host (force delete).
// removeInstallDirOnAgents 向各节点 Agent 发送删除安装目录命令（强制删除时）。
func (s *Service) removeInstallDirOnAgents(ctx context.Context, cluster *Cluster) {
	if s.hostProvider == nil || s.agentSender == nil {
		return
	}
	for _, node := range cluster.Nodes {
		hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
		if err != nil || hostInfo.AgentID == "" {
			continue
		}
		installDir := node.InstallDir
		if installDir == "" {
			installDir = cluster.InstallDir
		}
		if installDir == "" {
			continue
		}
		params := map[string]string{"install_dir": installDir}
		logger.InfoF(ctx, "[Cluster] Delete: sending remove_install_dir to agent / 删除集群：向 Agent 发送删除安装目录: agent_id=%s, install_dir=%s", hostInfo.AgentID, installDir)
		_, _, err = s.agentSender.SendCommand(ctx, hostInfo.AgentID, "remove_install_dir", params)
		if err != nil {
			logger.WarnF(ctx, "[Cluster] Delete: remove_install_dir on agent failed / 删除集群时向 Agent 发送删除安装目录失败: host_id=%d, err=%v", node.HostID, err)
		}
	}
}

// UpdateStatus updates the status of a cluster.
// UpdateStatus 更新集群的状态。
func (s *Service) UpdateStatus(ctx context.Context, id uint, status ClusterStatus) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

// isValidDeploymentMode checks if the deployment mode is valid.
// isValidDeploymentMode 检查部署模式是否有效。
func isValidDeploymentMode(mode DeploymentMode) bool {
	return mode == DeploymentModeHybrid || mode == DeploymentModeSeparated
}

// isValidNodeRole checks if the node role is valid.
// isValidNodeRole 检查节点角色是否有效。
func isValidNodeRole(role NodeRole) bool {
	return role == NodeRoleMaster || role == NodeRoleWorker || role == NodeRoleMasterWorker
}

func normalizeNodeRoleForDeployment(deploymentMode DeploymentMode, requestedRole NodeRole) (NodeRole, error) {
	if !isValidNodeRole(requestedRole) {
		return "", ErrInvalidNodeRole
	}
	if deploymentMode == DeploymentModeHybrid {
		return NodeRoleMasterWorker, nil
	}
	if requestedRole == NodeRoleMasterWorker {
		return "", ErrInvalidNodeRole
	}
	return requestedRole, nil
}

func resolveNodeInstallDir(requestedInstallDir string, clusterInstallDir string) string {
	if installDir := strings.TrimSpace(requestedInstallDir); installDir != "" {
		return installDir
	}
	if installDir := strings.TrimSpace(clusterInstallDir); installDir != "" {
		return installDir
	}
	return "/opt/seatunnel"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func validateRequiredPort(port int, invalidErr error) error {
	if port <= 0 || port > 65535 {
		return invalidErr
	}
	return nil
}

func validateOptionalPort(port int, invalidErr error) error {
	if port == 0 {
		return nil
	}
	if port < 0 || port > 65535 {
		return invalidErr
	}
	return nil
}

func resolveNodePorts(role NodeRole, deploymentMode DeploymentMode, hazelcastPort, apiPort, workerPort int) (int, int, int, error) {
	switch role {
	case NodeRoleMaster:
		if hazelcastPort == 0 {
			hazelcastPort = DefaultPorts.MasterHazelcast
		}
		if apiPort == 0 {
			apiPort = DefaultPorts.MasterAPI
		}
		workerPort = 0
	case NodeRoleWorker:
		if hazelcastPort == 0 {
			hazelcastPort = DefaultPorts.WorkerHazelcast
		}
		apiPort = 0
		workerPort = 0
	case NodeRoleMasterWorker:
		if hazelcastPort == 0 {
			hazelcastPort = DefaultPorts.MasterHazelcast
		}
		if apiPort == 0 {
			apiPort = DefaultPorts.MasterAPI
		}
		if deploymentMode == DeploymentModeHybrid && workerPort == 0 {
			workerPort = DefaultPorts.WorkerHazelcast
		}
	}

	if err := validateRequiredPort(hazelcastPort, ErrInvalidHazelcastPort); err != nil {
		return 0, 0, 0, err
	}
	if err := validateOptionalPort(apiPort, ErrInvalidAPIPort); err != nil {
		return 0, 0, 0, err
	}
	if err := validateOptionalPort(workerPort, ErrInvalidWorkerPort); err != nil {
		return 0, 0, 0, err
	}
	return hazelcastPort, apiPort, workerPort, nil
}

func validateNodeOverrides(overrides NodeOverrides) error {
	normalized := overrides.Normalize()
	if normalized.JVM == nil {
		return nil
	}

	values := []*int{
		normalized.JVM.HybridHeapSize,
		normalized.JVM.MasterHeapSize,
		normalized.JVM.WorkerHeapSize,
	}
	for _, value := range values {
		if value != nil && *value <= 0 {
			return ErrInvalidNodeJVMOverride
		}
	}
	return nil
}

func (s *Service) ensureHostReady(ctx context.Context, hostID uint) error {
	if s.hostProvider == nil {
		return nil
	}

	hostInfo, err := s.hostProvider.GetHostByID(ctx, hostID)
	if err != nil {
		return err
	}

	if hostInfo.HostType == "bare_metal" || hostInfo.HostType == "" {
		if hostInfo.AgentStatus != "installed" {
			return ErrNodeAgentNotInstalled
		}
	}
	return nil
}

func buildNodeForCreate(clusterID uint, hostID uint, cluster *Cluster, requestedRole NodeRole, requestedInstallDir string, hazelcastPort, apiPort, workerPort int, overrides *NodeOverrides) (*ClusterNode, error) {
	role, err := normalizeNodeRoleForDeployment(cluster.DeploymentMode, requestedRole)
	if err != nil {
		return nil, err
	}

	if ports := cluster.Config.GetPortConfig(); ports != nil {
		// 集群级端口配置作为节点未显式传端口时的默认值，确保一键安装配置和 DB 展示一致。
		// Cluster-level port config is used when node ports are omitted, keeping one-click install config and DB display aligned.
		switch role {
		case NodeRoleMaster:
			if hazelcastPort == 0 {
				hazelcastPort = ports.MasterHazelcastPort
			}
			if apiPort == 0 {
				apiPort = ports.MasterAPIPort
			}
		case NodeRoleWorker:
			if hazelcastPort == 0 {
				hazelcastPort = ports.WorkerPort
			}
		case NodeRoleMasterWorker:
			if hazelcastPort == 0 {
				hazelcastPort = ports.MasterHazelcastPort
			}
			if apiPort == 0 {
				apiPort = ports.MasterAPIPort
			}
			if workerPort == 0 {
				workerPort = ports.WorkerPort
			}
		}
	}

	resolvedHazelcastPort, resolvedAPIPort, resolvedWorkerPort, err := resolveNodePorts(role, cluster.DeploymentMode, hazelcastPort, apiPort, workerPort)
	if err != nil {
		return nil, err
	}

	resolvedOverrides := NodeOverrides{}
	if overrides != nil {
		resolvedOverrides = overrides.Normalize()
		if err := validateNodeOverrides(resolvedOverrides); err != nil {
			return nil, err
		}
	}

	return &ClusterNode{
		ClusterID:     clusterID,
		HostID:        hostID,
		Role:          role,
		InstallDir:    resolveNodeInstallDir(requestedInstallDir, cluster.InstallDir),
		HazelcastPort: resolvedHazelcastPort,
		APIPort:       resolvedAPIPort,
		WorkerPort:    resolvedWorkerPort,
		Overrides:     resolvedOverrides,
		Status:        NodeStatusPending,
	}, nil
}

func parseIntValue(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, false
		}
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// ClusterPortConfig 表示 cluster config 中的集群级端口默认值。
// ClusterPortConfig represents cluster-level port defaults from cluster config.
type ClusterPortConfig struct {
	MasterHazelcastPort int `json:"master_hazelcast_port,omitempty"`
	MasterAPIPort       int `json:"master_api_port,omitempty"`
	WorkerPort          int `json:"worker_port,omitempty"`
	// JavaProxyPort is the managed stx-java-proxy listen port (default 18080).
	// JavaProxyPort 是托管 stx-java-proxy 监听端口（默认 18080）。
	JavaProxyPort int `json:"java_proxy_port,omitempty"`
}

// HasValues 返回端口配置是否包含任意显式值。
// HasValues returns whether the port config contains any explicit value.
func (c *ClusterPortConfig) HasValues() bool {
	return c != nil && (c.MasterHazelcastPort > 0 || c.MasterAPIPort > 0 || c.WorkerPort > 0 || c.JavaProxyPort > 0)
}

// GetPortConfig 返回 cluster config 中的端口默认值。
// GetPortConfig returns cluster-level port defaults from cluster config.
func (c ClusterConfig) GetPortConfig() *ClusterPortConfig {
	if len(c) == 0 {
		return nil
	}

	raw, ok := c["ports"]
	if !ok || raw == nil {
		return nil
	}

	parseMap := func(values map[string]interface{}) *ClusterPortConfig {
		cfg := &ClusterPortConfig{}
		if value, ok := parseIntValue(values["master_hazelcast_port"]); ok {
			cfg.MasterHazelcastPort = value
		}
		if value, ok := parseIntValue(values["master_api_port"]); ok {
			cfg.MasterAPIPort = value
		}
		if value, ok := parseIntValue(values["worker_port"]); ok {
			cfg.WorkerPort = value
		}
		if value, ok := parseIntValue(values["java_proxy_port"]); ok {
			cfg.JavaProxyPort = value
		}
		if !cfg.HasValues() {
			return nil
		}
		return cfg
	}

	switch typed := raw.(type) {
	case ClusterPortConfig:
		cfg := typed
		if !cfg.HasValues() {
			return nil
		}
		return &cfg
	case *ClusterPortConfig:
		if !typed.HasValues() {
			return nil
		}
		cfg := *typed
		return &cfg
	case map[string]interface{}:
		return parseMap(typed)
	default:
		payload, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var cfg ClusterPortConfig
		if err := json.Unmarshal(payload, &cfg); err != nil {
			return nil
		}
		if !cfg.HasValues() {
			return nil
		}
		return &cfg
	}
}

// GetJVMConfig returns cluster-level JVM defaults from cluster config.
// GetJVMConfig 返回 cluster config 中的 JVM 默认值。
func (c ClusterConfig) GetJVMConfig() *JVMConfig {
	if len(c) == 0 {
		return nil
	}

	raw, ok := c["jvm"]
	if !ok || raw == nil {
		return nil
	}

	switch typed := raw.(type) {
	case JVMConfig:
		cfg := typed
		return &cfg
	case *JVMConfig:
		if typed == nil {
			return nil
		}
		cfg := *typed
		return &cfg
	case map[string]interface{}:
		cfg := &JVMConfig{}
		if value, ok := parseIntValue(typed["hybrid_heap_size"]); ok {
			cfg.HybridHeapSize = value
		}
		if value, ok := parseIntValue(typed["master_heap_size"]); ok {
			cfg.MasterHeapSize = value
		}
		if value, ok := parseIntValue(typed["worker_heap_size"]); ok {
			cfg.WorkerHeapSize = value
		}
		if cfg.HybridHeapSize == 0 && cfg.MasterHeapSize == 0 && cfg.WorkerHeapSize == 0 {
			return nil
		}
		return cfg
	default:
		payload, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var cfg JVMConfig
		if err := json.Unmarshal(payload, &cfg); err != nil {
			return nil
		}
		if cfg.HybridHeapSize == 0 && cfg.MasterHeapSize == 0 && cfg.WorkerHeapSize == 0 {
			return nil
		}
		return &cfg
	}
}

// ResolveJVM resolves node-level JVM overrides on top of cluster defaults.
// ResolveJVM 基于集群默认值解析节点级 JVM overrides。
func (n *ClusterNode) ResolveJVM(clusterConfig ClusterConfig) *JVMConfig {
	var resolved JVMConfig
	if defaults := clusterConfig.GetJVMConfig(); defaults != nil {
		resolved = *defaults
	}

	if n.Overrides.JVM != nil {
		if n.Overrides.JVM.HybridHeapSize != nil {
			resolved.HybridHeapSize = *n.Overrides.JVM.HybridHeapSize
		}
		if n.Overrides.JVM.MasterHeapSize != nil {
			resolved.MasterHeapSize = *n.Overrides.JVM.MasterHeapSize
		}
		if n.Overrides.JVM.WorkerHeapSize != nil {
			resolved.WorkerHeapSize = *n.Overrides.JVM.WorkerHeapSize
		}
	}

	if resolved.HybridHeapSize == 0 && resolved.MasterHeapSize == 0 && resolved.WorkerHeapSize == 0 {
		return nil
	}
	return &resolved
}

// AddNode adds a node to a cluster with validation.
// AddNode 向集群添加节点并进行验证。
// Requirements: 7.2 - Validates host Agent status is "installed" before association.
func (s *Service) AddNode(ctx context.Context, clusterID uint, req *AddNodeRequest) (*ClusterNode, error) {
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}

	if err := s.ensureHostReady(ctx, req.HostID); err != nil {
		return nil, err
	}

	node, err := buildNodeForCreate(
		clusterID,
		req.HostID,
		cluster,
		req.Role,
		req.InstallDir,
		req.HazelcastPort,
		req.APIPort,
		req.WorkerPort,
		req.Overrides,
	)
	if err != nil {
		return nil, err
	}

	if err := s.repo.AddNode(ctx, node); err != nil {
		return nil, err
	}

	// After saving, detect SeaTunnel process status via Agent
	// 保存后，通过 Agent 检测 SeaTunnel 进程状态
	s.detectAndUpdateNodeProcess(ctx, node, req.HostID)

	// Update cluster status based on all nodes
	// 根据所有节点状态更新集群状态
	s.updateClusterStatusFromNodes(ctx, clusterID)

	s.notifyClusterTopologyChanged(ctx, clusterID)

	return node, nil
}

// AddNodes adds one or more logical nodes for the same host atomically.
// AddNodes 原子地为同一主机添加一个或多个逻辑节点。
func (s *Service) AddNodes(ctx context.Context, clusterID uint, req *AddNodesRequest) ([]*ClusterNode, error) {
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}

	if len(req.Entries) == 0 {
		return nil, ErrNodeBatchEntriesRequired
	}

	if err := s.ensureHostReady(ctx, req.HostID); err != nil {
		return nil, err
	}

	createdNodes := make([]*ClusterNode, 0, len(req.Entries))
	seenRoles := make(map[NodeRole]struct{}, len(req.Entries))

	err = s.repo.Transaction(ctx, func(tx *Repository) error {
		for _, entry := range req.Entries {
			node, err := buildNodeForCreate(
				clusterID,
				req.HostID,
				cluster,
				entry.Role,
				firstNonEmpty(entry.InstallDir, req.InstallDir),
				entry.HazelcastPort,
				entry.APIPort,
				entry.WorkerPort,
				entry.Overrides,
			)
			if err != nil {
				return err
			}

			if _, exists := seenRoles[node.Role]; exists {
				return ErrNodeAlreadyExists
			}
			seenRoles[node.Role] = struct{}{}

			if err := tx.AddNode(ctx, node); err != nil {
				return err
			}
			createdNodes = append(createdNodes, node)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, node := range createdNodes {
		s.detectAndUpdateNodeProcess(ctx, node, req.HostID)
	}
	s.updateClusterStatusFromNodes(ctx, clusterID)
	s.notifyClusterTopologyChanged(ctx, clusterID)

	return createdNodes, nil
}

// RemoveNode removes a node from a cluster.
// RemoveNode 从集群中移除节点。
// Requirements: 7.4 - Removes node from cluster.
func (s *Service) RemoveNode(ctx context.Context, clusterID uint, nodeID uint) error {
	// Verify node belongs to the cluster
	// 验证节点属于该集群
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return err
	}

	if node.ClusterID != clusterID {
		return ErrNodeNotFound
	}

	if err := s.repo.RemoveNode(ctx, nodeID); err != nil {
		return err
	}

	s.notifyClusterTopologyChanged(ctx, clusterID)
	return nil
}

// RemoveNodeByHostID removes a node from a cluster by host ID.
// RemoveNodeByHostID 根据主机 ID 从集群中移除节点。
func (s *Service) RemoveNodeByHostID(ctx context.Context, clusterID uint, hostID uint) error {
	if err := s.repo.RemoveNodeByClusterAndHost(ctx, clusterID, hostID); err != nil {
		return err
	}
	s.notifyClusterTopologyChanged(ctx, clusterID)
	return nil
}

func buildNodeInfo(node *ClusterNode) *NodeInfo {
	if node == nil {
		return nil
	}
	return &NodeInfo{
		ID:            node.ID,
		ClusterID:     node.ClusterID,
		HostID:        node.HostID,
		Role:          node.Role,
		InstallDir:    node.InstallDir,
		HazelcastPort: node.HazelcastPort,
		APIPort:       node.APIPort,
		WorkerPort:    node.WorkerPort,
		Overrides:     node.Overrides.Normalize(),
		Status:        node.Status,
		ProcessPID:    node.ProcessPID,
		CreatedAt:     node.CreatedAt,
		UpdatedAt:     node.UpdatedAt,
	}
}

// GetNodes retrieves all nodes for a cluster with host information.
// GetNodes 获取集群的所有节点及其主机信息。
// Requirements: 7.4 - Returns each node's host info, role, SeaTunnel process status, resource usage.
func (s *Service) GetNodes(ctx context.Context, clusterID uint) ([]*NodeInfo, error) {
	nodes, err := s.repo.GetNodesByClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	nodeInfos := make([]*NodeInfo, len(nodes))
	for i, node := range nodes {
		nodeInfo := buildNodeInfo(node)

		// Get host information and online status; when host is offline, show node as offline
		if s.hostProvider != nil {
			hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
			if err == nil {
				nodeInfo.HostName = hostInfo.Name
				nodeInfo.HostIP = hostInfo.IPAddress
				nodeInfo.IsOnline = hostInfo.IsOnline(s.heartbeatTimeout)
				if !nodeInfo.IsOnline {
					nodeInfo.Status = NodeStatusOffline
				}
			} else {
				nodeInfo.IsOnline = false
				nodeInfo.Status = NodeStatusOffline
			}
		}

		nodeInfos[i] = nodeInfo
	}

	return nodeInfos, nil
}

// GetNode retrieves a specific node by ID.
// GetNode 根据 ID 获取特定节点。
func (s *Service) GetNode(ctx context.Context, nodeID uint) (*ClusterNode, error) {
	return s.repo.GetNodeByID(ctx, nodeID)
}

// GetClusterNodeDisplayInfo returns cluster name and node display string "主机名 - 角色" for audit log.
// GetClusterNodeDisplayInfo 返回集群名及节点展示串「主机名 - 角色」，用于审计日志。
func (s *Service) GetClusterNodeDisplayInfo(ctx context.Context, clusterID uint, nodeID uint) (clusterName, nodeDisplay string) {
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil || cluster == nil {
		return "", ""
	}
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil || node == nil {
		return cluster.Name, ""
	}
	host, err := s.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil || host == nil {
		return cluster.Name, ""
	}
	roleDisplay := string(node.Role)
	switch node.Role {
	case NodeRoleMaster:
		roleDisplay = "Master"
	case NodeRoleWorker:
		roleDisplay = "Worker"
	case NodeRoleMasterWorker:
		roleDisplay = "Master/Worker"
	}
	hostName := host.Name
	if hostName == "" {
		hostName = host.IPAddress
	}
	if hostName == "" {
		hostName = "node"
	}
	return cluster.Name, hostName + " - " + roleDisplay
}

// GetNodeInstallDir retrieves the install directory for a node by cluster ID and host ID.
// GetNodeInstallDir 根据集群 ID 和主机 ID 获取节点的安装目录。
// This implements the config.NodeInfoProvider interface.
// 这实现了 config.NodeInfoProvider 接口。
func (s *Service) GetNodeInstallDir(ctx context.Context, clusterID uint, hostID uint) (string, error) {
	node, err := s.repo.GetNodeByClusterAndHost(ctx, clusterID, hostID)
	if err != nil {
		return "", err
	}
	if node == nil {
		return "", fmt.Errorf("node not found for cluster %d and host %d / 未找到集群 %d 主机 %d 对应的节点", clusterID, hostID, clusterID, hostID)
	}
	return node.InstallDir, nil
}

// ResolveNodeJVMByClusterAndHostAndRole resolves effective JVM config for one logical node.
// ResolveNodeJVMByClusterAndHostAndRole 解析一个逻辑节点的生效 JVM 配置。
func (s *Service) ResolveNodeJVMByClusterAndHostAndRole(ctx context.Context, clusterID uint, hostID uint, role string) (*installerapp.JVMConfig, error) {
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}

	normalizedRole, err := normalizeNodeRoleForDeployment(cluster.DeploymentMode, NodeRole(role))
	if err != nil {
		return nil, err
	}

	node, err := s.repo.GetNodeByClusterAndHostAndRole(ctx, clusterID, hostID, string(normalizedRole))
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}

	resolved := node.ResolveJVM(cluster.Config)
	if resolved == nil {
		return nil, nil
	}

	return &installerapp.JVMConfig{
		HybridHeapSize: resolved.HybridHeapSize,
		MasterHeapSize: resolved.MasterHeapSize,
		WorkerHeapSize: resolved.WorkerHeapSize,
	}, nil
}

// UpdateNodeStatus updates the status of a cluster node.
// UpdateNodeStatus 更新集群节点的状态。
func (s *Service) UpdateNodeStatus(ctx context.Context, nodeID uint, status NodeStatus) error {
	return s.repo.UpdateNodeStatus(ctx, nodeID, status)
}

// UpdateNodeStatusByClusterAndHost updates the node status by cluster ID and host ID.
// UpdateNodeStatusByClusterAndHost 根据集群 ID 和主机 ID 更新节点状态。
// This implements the installer.NodeStatusUpdater interface.
// 这实现了 installer.NodeStatusUpdater 接口。
func (s *Service) UpdateNodeStatusByClusterAndHost(ctx context.Context, clusterID uint, hostID uint, status string) error {
	// Find node by cluster ID and host ID / 根据集群 ID 和主机 ID 查找节点
	node, err := s.repo.GetNodeByClusterAndHost(ctx, clusterID, hostID)
	if err != nil {
		return fmt.Errorf("failed to find node by cluster %d and host %d: %w / 根据集群 %d 和主机 %d 查找节点失败: %w", clusterID, hostID, err, clusterID, hostID, err)
	}
	if node == nil {
		return fmt.Errorf("node not found for cluster %d and host %d / 未找到集群 %d 主机 %d 对应的节点", clusterID, hostID, clusterID, hostID)
	}

	logger.InfoF(ctx, "[Cluster] UpdateNodeStatusByClusterAndHost: clusterID=%d, hostID=%d, nodeID=%d, oldStatus=%s, newStatus=%s",
		clusterID, hostID, node.ID, node.Status, status)

	err = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatus(status))
	if err != nil {
		logger.ErrorF(ctx, "[Cluster] UpdateNodeStatusByClusterAndHost failed: nodeID=%d, error=%v", node.ID, err)
		return err
	}

	logger.InfoF(ctx, "[Cluster] UpdateNodeStatusByClusterAndHost success: nodeID=%d, newStatus=%s", node.ID, status)
	return nil
}

// UpdateNodeProcess updates the process information for a cluster node.
// UpdateNodeProcess 更新集群节点的进程信息。
func (s *Service) UpdateNodeProcess(ctx context.Context, nodeID uint, pid int, processStatus string) error {
	return s.repo.UpdateNodeProcess(ctx, nodeID, pid, processStatus)
}

// UpdateNode updates a node's configuration (install_dir, ports).
// UpdateNode 更新节点配置（安装目录、端口）。
func (s *Service) UpdateNode(ctx context.Context, clusterID uint, nodeID uint, req *UpdateNodeRequest) (*ClusterNode, error) {
	// Verify node belongs to the cluster
	// 验证节点属于该集群
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	if node.ClusterID != clusterID {
		return nil, ErrNodeNotFound
	}

	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	originalNodeAPIPort := node.APIPort

	// Update fields if provided
	// 如果提供了字段则更新
	if req.InstallDir != nil {
		node.InstallDir = resolveNodeInstallDir(*req.InstallDir, cluster.InstallDir)
	}

	hazelcastPort := node.HazelcastPort
	apiPort := node.APIPort
	workerPort := node.WorkerPort
	if req.HazelcastPort != nil {
		hazelcastPort = *req.HazelcastPort
	}
	if req.APIPort != nil {
		apiPort = *req.APIPort
	}
	if req.WorkerPort != nil {
		workerPort = *req.WorkerPort
	}

	resolvedHazelcastPort, resolvedAPIPort, resolvedWorkerPort, err := resolveNodePorts(node.Role, cluster.DeploymentMode, hazelcastPort, apiPort, workerPort)
	if err != nil {
		return nil, err
	}
	node.HazelcastPort = resolvedHazelcastPort
	node.APIPort = resolvedAPIPort
	node.WorkerPort = resolvedWorkerPort

	if req.Overrides != nil {
		normalizedOverrides := req.Overrides.Normalize()
		if err := validateNodeOverrides(normalizedOverrides); err != nil {
			return nil, err
		}
		node.Overrides = normalizedOverrides
	}

	if req.APIPort != nil && node.Role != NodeRoleWorker && node.APIPort != originalNodeAPIPort {
		if err := s.syncNodeRuntimeAPIPort(ctx, clusterID, node, originalNodeAPIPort); err != nil {
			return nil, err
		}
	}

	if err := s.repo.UpdateNode(ctx, node); err != nil {
		return nil, err
	}

	// After saving, detect SeaTunnel process status via Agent
	// 保存后，通过 Agent 检测 SeaTunnel 进程状态
	s.detectAndUpdateNodeProcess(ctx, node, node.HostID)

	// Update cluster status based on all nodes
	// 根据所有节点状态更新集群状态
	s.updateClusterStatusFromNodes(ctx, clusterID)

	s.notifyClusterTopologyChanged(ctx, clusterID)

	return node, nil
}

func (s *Service) syncNodeRuntimeAPIPort(ctx context.Context, clusterID uint, node *ClusterNode, previousAPIPort int) error {
	if node == nil {
		return fmt.Errorf("node is required / 节点不能为空")
	}
	if s.configAgentClient == nil {
		return fmt.Errorf("config agent client not configured / 配置 Agent 客户端未配置")
	}
	if node.HostID == 0 {
		return fmt.Errorf("node host id is required / 节点 HostID 不能为空")
	}

	installDir := strings.TrimSpace(node.InstallDir)
	if installDir == "" {
		return fmt.Errorf("node install_dir is required / 节点安装目录不能为空")
	}

	content, err := s.configAgentClient.PullConfig(ctx, node.HostID, installDir, appconfig.ConfigTypeSeatunnel)
	if err != nil {
		return fmt.Errorf("pull seatunnel.yaml failed: %w / 拉取 seatunnel.yaml 失败: %w", err, err)
	}
	updatedContent, err := updateSeatunnelHTTPPort(content, node.APIPort)
	if err != nil {
		return err
	}
	if err := s.configAgentClient.PushConfig(ctx, node.HostID, installDir, appconfig.ConfigTypeSeatunnel, updatedContent); err != nil {
		return fmt.Errorf("push seatunnel.yaml failed: %w / 推送 seatunnel.yaml 失败: %w", err, err)
	}

	logger.InfoF(ctx, "[Cluster] synced seatunnel.yaml http.port for node=%d: %d -> %d", node.ID, previousAPIPort, node.APIPort)

	if _, err := s.restartNodeWithResolvedSpec(ctx, clusterID, node); err != nil {
		return fmt.Errorf("restart node after api_port change failed: %w / 修改 api_port 后重启节点失败: %w", err, err)
	}
	return nil
}

func updateSeatunnelHTTPPort(content string, port int) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("seatunnel.yaml content is empty / seatunnel.yaml 内容为空")
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return "", fmt.Errorf("invalid seatunnel.yaml: %w / 非法 seatunnel.yaml: %w", err, err)
	}
	if len(root.Content) == 0 {
		return "", fmt.Errorf("invalid seatunnel.yaml root / 非法 seatunnel.yaml 根节点")
	}

	top := root.Content[0]
	seatunnelNode := ensureYAMLMapChild(top, "seatunnel")
	engineNode := ensureYAMLMapChild(seatunnelNode, "engine")
	httpNode := ensureYAMLMapChild(engineNode, "http")
	setYAMLMapValue(httpNode, "enable-http", "true")
	setYAMLMapValue(httpNode, "enable-dynamic-port", "false")
	setYAMLMapValue(httpNode, "port", strconv.Itoa(port))

	normalized, err := yaml.Marshal(&root)
	if err != nil {
		return "", fmt.Errorf("marshal seatunnel.yaml failed: %w / 序列化 seatunnel.yaml 失败: %w", err, err)
	}
	return string(normalized), nil
}

func ensureYAMLMapChild(parent *yaml.Node, key string) *yaml.Node {
	if parent.Kind != yaml.MappingNode {
		parent.Kind = yaml.MappingNode
		parent.Tag = "!!map"
		parent.Content = []*yaml.Node{}
	}
	for i := 0; i < len(parent.Content)-1; i += 2 {
		if parent.Content[i].Value == key {
			return parent.Content[i+1]
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, keyNode, valueNode)
	return valueNode
}

func setYAMLMapValue(parent *yaml.Node, key, value string) {
	if parent.Kind != yaml.MappingNode {
		parent.Kind = yaml.MappingNode
		parent.Tag = "!!map"
		parent.Content = []*yaml.Node{}
	}
	for i := 0; i < len(parent.Content)-1; i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1].Kind = yaml.ScalarNode
			parent.Content[i+1].Tag = inferYAMLScalarTag(value)
			parent.Content[i+1].Value = value
			return
		}
	}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: inferYAMLScalarTag(value), Value: value},
	)
}

func inferYAMLScalarTag(value string) string {
	switch value {
	case "true", "false":
		return "!!bool"
	default:
		if _, err := strconv.Atoi(value); err == nil {
			return "!!int"
		}
		return "!!str"
	}
}

// GetStatus retrieves the detailed status of a cluster including node health.
// GetStatus 获取集群的详细状态，包括节点健康状况。
// Requirements: 7.6 - Returns cluster health status based on node states.
func (s *Service) GetStatus(ctx context.Context, clusterID uint) (*ClusterStatusInfo, error) {
	// Get cluster with nodes
	// 获取集群及其节点
	cluster, err := s.repo.GetByID(ctx, clusterID, true)
	if err != nil {
		return nil, err
	}

	s.refreshNodeProcessesForStatus(ctx, cluster)

	cluster, err = s.repo.GetByID(ctx, clusterID, true)
	if err != nil {
		return nil, err
	}

	statusInfo := &ClusterStatusInfo{
		ClusterID:   cluster.ID,
		ClusterName: cluster.Name,
		Status:      cluster.Status,
		TotalNodes:  len(cluster.Nodes),
		Nodes:       make([]*NodeStatusInfo, len(cluster.Nodes)),
	}

	onlineCount := 0
	offlineCount := 0

	for i, node := range cluster.Nodes {
		nodeStatus := &NodeStatusInfo{
			NodeID:     node.ID,
			HostID:     node.HostID,
			Role:       node.Role,
			Status:     node.Status,
			ProcessPID: node.ProcessPID,
		}

		// Get host information and online status; when host is offline, show node status as offline
		if s.hostProvider != nil {
			hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
			if err == nil {
				nodeStatus.HostName = hostInfo.Name
				nodeStatus.HostIP = hostInfo.IPAddress
				nodeStatus.IsOnline = hostInfo.IsOnline(s.heartbeatTimeout)
				if !nodeStatus.IsOnline {
					nodeStatus.Status = NodeStatusOffline
				}
				if nodeStatus.IsOnline {
					onlineCount++
				} else {
					offlineCount++
				}
			} else {
				nodeStatus.IsOnline = false
				nodeStatus.Status = NodeStatusOffline
				offlineCount++
			}
		}

		statusInfo.Nodes[i] = nodeStatus
	}

	statusInfo.OnlineNodes = onlineCount
	statusInfo.OfflineNodes = offlineCount

	// Determine health status
	// 确定健康状态
	// Requirements: 7.6 - If any node is offline, cluster health is "unhealthy"
	if statusInfo.TotalNodes == 0 {
		statusInfo.HealthStatus = HealthStatusUnknown
	} else if offlineCount > 0 {
		statusInfo.HealthStatus = HealthStatusUnhealthy
	} else {
		statusInfo.HealthStatus = HealthStatusHealthy
	}

	return statusInfo, nil
}

func (s *Service) refreshNodeProcessesForStatus(ctx context.Context, cluster *Cluster) {
	if cluster == nil || s.hostProvider == nil || s.agentSender == nil {
		return
	}

	shouldRefresh := cluster.Status == ClusterStatusRunning || cluster.Status == ClusterStatusDeploying
	if !shouldRefresh {
		for _, node := range cluster.Nodes {
			if node.Status == NodeStatusRunning || node.ProcessPID > 0 {
				shouldRefresh = true
				break
			}
		}
	}

	if !shouldRefresh {
		return
	}

	for i := range cluster.Nodes {
		node := &cluster.Nodes[i]
		if node.Status != NodeStatusRunning && node.ProcessPID <= 0 {
			continue
		}
		s.detectAndUpdateNodeProcess(ctx, node, node.HostID)
	}

	s.updateClusterStatusFromNodes(ctx, cluster.ID)
}

// IsClusterHealthy checks if all nodes in a cluster are online.
// IsClusterHealthy 检查集群中的所有节点是否都在线。
// Requirements: 7.6 - Returns false if any node is offline.
func (s *Service) IsClusterHealthy(ctx context.Context, clusterID uint) (bool, error) {
	status, err := s.GetStatus(ctx, clusterID)
	if err != nil {
		return false, err
	}
	return status.HealthStatus == HealthStatusHealthy, nil
}

// Start starts all nodes in a cluster.
// Start 启动集群中的所有节点。
// Requirements: 6.1 - Executes SeaTunnel start script, waits for process startup, verifies process alive.
func (s *Service) Start(ctx context.Context, clusterID uint) (*OperationResult, error) {
	return s.executeOperation(ctx, clusterID, OperationStart)
}

// Stop stops all nodes in a cluster.
// Stop 停止集群中的所有节点。
// Requirements: 6.2 - Sends SIGTERM, waits for graceful shutdown (max 30s), sends SIGKILL if timeout.
func (s *Service) Stop(ctx context.Context, clusterID uint) (*OperationResult, error) {
	return s.executeOperation(ctx, clusterID, OperationStop)
}

// Restart restarts all nodes in a cluster.
// Restart 重启集群中的所有节点。
// Requirements: 6.3 - Executes stop first, waits for complete exit, then executes start.
func (s *Service) Restart(ctx context.Context, clusterID uint) (*OperationResult, error) {
	return s.executeOperation(ctx, clusterID, OperationRestart)
}

// executeOperation executes an operation on all nodes in a cluster.
// executeOperation 在集群的所有节点上执行操作。
func (s *Service) executeOperation(ctx context.Context, clusterID uint, operation OperationType) (*OperationResult, error) {
	// Get cluster with nodes
	// 获取集群及其节点
	cluster, err := s.repo.GetByID(ctx, clusterID, true)
	if err != nil {
		return nil, err
	}

	result := &OperationResult{
		ClusterID:   clusterID,
		Operation:   operation,
		Success:     true,
		NodeResults: make([]*NodeOperationResult, 0, len(cluster.Nodes)),
	}

	// Update cluster status based on operation
	// 根据操作更新集群状态
	switch operation {
	case OperationStart, OperationRestart:
		if err := s.repo.UpdateStatus(ctx, clusterID, ClusterStatusDeploying); err != nil {
			return nil, err
		}
	}

	// Execute operation on each node
	// 在每个节点上执行操作
	for _, node := range cluster.Nodes {
		nodeResult := &NodeOperationResult{
			NodeID: node.ID,
			HostID: node.HostID,
		}

		// Get host information
		// 获取主机信息
		if s.hostProvider != nil {
			hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
			if err != nil {
				nodeResult.Success = false
				nodeResult.Message = "Failed to get host information: " + err.Error()
				result.NodeResults = append(result.NodeResults, nodeResult)
				result.Success = false
				continue
			}

			nodeResult.HostName = hostInfo.Name

			// Check if host is online (for bare_metal hosts)
			// 检查主机是否在线（对于物理机/VM 主机）
			if hostInfo.HostType == "bare_metal" || hostInfo.HostType == "" {
				if !hostInfo.IsOnline(s.heartbeatTimeout) {
					nodeResult.Success = false
					nodeResult.Message = "Host is offline"
					result.NodeResults = append(result.NodeResults, nodeResult)
					result.Success = false
					continue
				}

				// Send command to agent if sender is available
				// 如果发送器可用，向 Agent 发送命令
				if s.agentSender != nil && hostInfo.AgentID != "" {
					installDir := node.InstallDir
					if installDir == "" {
						installDir = cluster.InstallDir
					}
					params := map[string]string{
						"cluster_id":  fmt.Sprintf("%d", clusterID),
						"node_id":     fmt.Sprintf("%d", node.ID),
						"role":        string(node.Role),
						"install_dir": installDir,
					}

					success, message, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, string(operation), params)
					if err != nil {
						nodeResult.Success = false
						nodeResult.Message = "Failed to send command: " + err.Error()
						result.Success = false
					} else {
						nodeResult.Success = success
						nodeResult.Message = message
						if !success {
							result.Success = false
						}
					}
				} else {
					// Agent sender not available, mark as pending
					// Agent 发送器不可用，标记为待处理
					nodeResult.Success = true
					nodeResult.Message = "Operation queued (Agent sender not configured)"
				}
			} else {
				// For Docker/K8s hosts, operations will be handled by respective managers
				// 对于 Docker/K8s 主机，操作将由相应的管理器处理
				nodeResult.Success = true
				nodeResult.Message = "Operation queued for " + hostInfo.HostType + " host"
			}
		} else {
			// No host provider, mark as pending
			// 没有主机提供者，标记为待处理
			nodeResult.Success = true
			nodeResult.Message = "Operation queued (host provider not configured)"
		}

		// Update node status based on operation
		// 根据操作更新节点状态
		if nodeResult.Success {
			switch operation {
			case OperationStart:
				_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusRunning)
				s.detectAndUpdateNodeProcess(ctx, &node, node.HostID)
			case OperationStop:
				_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusStopped)
				_ = s.repo.UpdateNodeProcess(ctx, node.ID, 0, "stopped")
			case OperationRestart:
				_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusRunning)
				s.detectAndUpdateNodeProcess(ctx, &node, node.HostID)
			}
		} else {
			_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusError)
		}

		result.NodeResults = append(result.NodeResults, nodeResult)
	}

	// Update cluster status based on overall result
	// 根据整体结果更新集群状态
	if result.Success {
		switch operation {
		case OperationStart, OperationRestart:
			_ = s.repo.UpdateStatus(ctx, clusterID, ClusterStatusRunning)
		case OperationStop:
			_ = s.repo.UpdateStatus(ctx, clusterID, ClusterStatusStopped)
		}
		result.Message = "Operation completed successfully"
	} else {
		_ = s.repo.UpdateStatus(ctx, clusterID, ClusterStatusError)
		result.Message = "Operation completed with errors"
	}

	return result, nil
}

// GetClustersByHostID retrieves all clusters that have a specific host as a node.
// GetClustersByHostID 获取将特定主机作为节点的所有集群。
func (s *Service) GetClustersByHostID(ctx context.Context, hostID uint) ([]*Cluster, error) {
	return s.repo.GetClustersWithHostID(ctx, hostID)
}

// PrecheckNode performs precheck on a node before adding to cluster.
// PrecheckNode 在将节点添加到集群之前执行预检查。
// Checks:
// 1. Port is listening (SeaTunnel service is running) / 端口正在监听（SeaTunnel 服务正在运行）
// 2. Directory exists and is writable / 目录存在且可写
// 3. SeaTunnel REST API connectivity / SeaTunnel REST API 连通性
func (s *Service) PrecheckNode(ctx context.Context, clusterID uint, req *PrecheckRequest) (*PrecheckResult, error) {
	// Validate cluster exists
	// 验证集群存在
	_, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}

	// Validate hazelcast port
	// 验证 Hazelcast 端口
	if req.HazelcastPort <= 0 || req.HazelcastPort > 65535 {
		return nil, ErrInvalidHazelcastPort
	}

	// Get host information
	// 获取主机信息
	if s.hostProvider == nil {
		return &PrecheckResult{
			Success: false,
			Message: "Host provider not configured / 主机提供者未配置",
			Checks:  []*PrecheckCheckItem{},
		}, nil
	}

	hostInfo, err := s.hostProvider.GetHostByID(ctx, req.HostID)
	if err != nil {
		return nil, err
	}

	// Initialize result
	// 初始化结果
	result := &PrecheckResult{
		Success: true,
		Checks:  make([]*PrecheckCheckItem, 0),
	}

	// Check 1: Agent is installed and online
	// 检查 1：Agent 已安装且在线
	agentCheck := &PrecheckCheckItem{
		Name: "agent_status",
	}
	if hostInfo.AgentStatus != "installed" {
		agentCheck.Status = PrecheckStatusFailed
		agentCheck.Message = "Agent is not installed / Agent 未安装"
		result.Success = false
	} else if !hostInfo.IsOnline(s.heartbeatTimeout) {
		agentCheck.Status = PrecheckStatusFailed
		agentCheck.Message = "Agent is offline / Agent 离线"
		result.Success = false
	} else {
		agentCheck.Status = PrecheckStatusPassed
		agentCheck.Message = "Agent is installed and online / Agent 已安装且在线"
	}
	result.Checks = append(result.Checks, agentCheck)

	// If agent is not available, skip remaining checks
	// 如果 Agent 不可用，跳过剩余检查
	if agentCheck.Status == PrecheckStatusFailed {
		result.Message = "Agent is not available, cannot perform precheck / Agent 不可用，无法执行预检查"
		return result, nil
	}

	// Check 2: Port is listening (via Agent command)
	// 检查 2：端口正在监听（通过 Agent 命令）
	// For node registration, port listening means SeaTunnel is running (PASSED)
	// 对于节点注册，端口监听意味着 SeaTunnel 正在运行（通过）
	portCheck := &PrecheckCheckItem{
		Name: "port_listening",
	}
	if s.agentSender != nil && hostInfo.AgentID != "" {
		params := map[string]string{
			"port": fmt.Sprintf("%d", req.HazelcastPort),
		}
		success, _, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_port", params)
		if err != nil {
			portCheck.Status = PrecheckStatusFailed
			portCheck.Message = fmt.Sprintf("Failed to check port: %v / 检查端口失败: %v", err, err)
			result.Success = false
		} else if success {
			// Port is listening = SeaTunnel service is running = PASSED
			// 端口正在监听 = SeaTunnel 服务正在运行 = 通过
			portCheck.Status = PrecheckStatusPassed
			portCheck.Message = fmt.Sprintf("Port %d is listening (SeaTunnel is running) / 端口 %d 正在监听（SeaTunnel 正在运行）", req.HazelcastPort, req.HazelcastPort)
		} else {
			// Port is not listening = SeaTunnel service is not running = FAILED
			// 端口未监听 = SeaTunnel 服务未运行 = 失败
			portCheck.Status = PrecheckStatusFailed
			portCheck.Message = fmt.Sprintf("Port %d is not listening (SeaTunnel is not running) / 端口 %d 未监听（SeaTunnel 未运行）", req.HazelcastPort, req.HazelcastPort)
			result.Success = false
		}
	} else {
		portCheck.Status = PrecheckStatusSkipped
		portCheck.Message = "Agent command sender not configured / Agent 命令发送器未配置"
	}
	result.Checks = append(result.Checks, portCheck)

	// Check 3: Directory exists and is writable (via Agent command)
	// 检查 3：目录存在且可写（通过 Agent 命令）
	installDir := req.InstallDir
	if installDir == "" {
		installDir = "/opt/seatunnel"
	}
	dirCheck := &PrecheckCheckItem{
		Name: "directory_check",
	}
	if s.agentSender != nil && hostInfo.AgentID != "" {
		params := map[string]string{
			"path": installDir,
		}
		success, _, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_directory", params)
		if err != nil {
			dirCheck.Status = PrecheckStatusFailed
			dirCheck.Message = fmt.Sprintf("Failed to check directory: %v / 检查目录失败: %v", err, err)
			result.Success = false
		} else if success {
			dirCheck.Status = PrecheckStatusPassed
			dirCheck.Message = fmt.Sprintf("Directory %s exists and is writable / 目录 %s 存在且可写", installDir, installDir)
		} else {
			dirCheck.Status = PrecheckStatusFailed
			dirCheck.Message = fmt.Sprintf("Directory %s does not exist or is not writable / 目录 %s 不存在或不可写", installDir, installDir)
			result.Success = false
		}
	} else {
		dirCheck.Status = PrecheckStatusSkipped
		dirCheck.Message = "Agent command sender not configured / Agent 命令发送器未配置"
	}
	result.Checks = append(result.Checks, dirCheck)

	// Check 4: SeaTunnel REST API V1 connectivity (via Agent command)
	// 检查 4：SeaTunnel REST API V1 连通性（通过 Agent 命令）
	// REST API V1 on hazelcast port: /hazelcast/rest/maps/overview
	apiV1Check := &PrecheckCheckItem{
		Name: "seatunnel_api_v1",
	}
	if s.agentSender != nil && hostInfo.AgentID != "" {
		params := map[string]string{
			"url": fmt.Sprintf("http://127.0.0.1:%d/hazelcast/rest/maps/overview", req.HazelcastPort),
		}
		success, _, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_http", params)
		if err != nil {
			apiV1Check.Status = PrecheckStatusFailed
			apiV1Check.Message = fmt.Sprintf("Failed to check SeaTunnel API V1: %v / 检查 SeaTunnel API V1 失败: %v", err, err)
			result.Success = false
		} else if success {
			apiV1Check.Status = PrecheckStatusPassed
			apiV1Check.Message = "SeaTunnel REST API V1 is accessible / SeaTunnel REST API V1 可访问"
		} else {
			apiV1Check.Status = PrecheckStatusFailed
			apiV1Check.Message = "SeaTunnel REST API V1 is not accessible / SeaTunnel REST API V1 不可访问"
			result.Success = false
		}
	} else {
		apiV1Check.Status = PrecheckStatusSkipped
		apiV1Check.Message = "Agent command sender not configured / Agent 命令发送器未配置"
	}
	result.Checks = append(result.Checks, apiV1Check)

	// Check 5: SeaTunnel REST API V2 connectivity (if api_port is specified)
	// 检查 5：SeaTunnel REST API V2 连通性（如果指定了 api_port）
	// REST API V2 on api port (8080): /overview
	if req.APIPort > 0 {
		apiV2Check := &PrecheckCheckItem{
			Name: "seatunnel_api_v2",
		}
		if s.agentSender != nil && hostInfo.AgentID != "" {
			params := map[string]string{
				"url": fmt.Sprintf("http://127.0.0.1:%d/overview", req.APIPort),
			}
			success, _, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_http", params)
			if err != nil {
				apiV2Check.Status = PrecheckStatusFailed
				apiV2Check.Message = fmt.Sprintf("Failed to check SeaTunnel API V2: %v / 检查 SeaTunnel API V2 失败: %v", err, err)
				result.Success = false
			} else if success {
				apiV2Check.Status = PrecheckStatusPassed
				apiV2Check.Message = "SeaTunnel REST API V2 is accessible / SeaTunnel REST API V2 可访问"
			} else {
				apiV2Check.Status = PrecheckStatusFailed
				apiV2Check.Message = "SeaTunnel REST API V2 is not accessible / SeaTunnel REST API V2 不可访问"
				result.Success = false
			}
		} else {
			apiV2Check.Status = PrecheckStatusSkipped
			apiV2Check.Message = "Agent command sender not configured / Agent 命令发送器未配置"
		}
		result.Checks = append(result.Checks, apiV2Check)
	}

	// Set overall message
	// 设置总体消息
	if result.Success {
		result.Message = "All precheck passed / 所有预检查通过"
	} else {
		result.Message = "Some precheck failed / 部分预检查失败"
	}

	return result, nil
}

// detectAndUpdateNodeProcess detects SeaTunnel process status via Agent and updates node.
// detectAndUpdateNodeProcess 通过 Agent 检测 SeaTunnel 进程状态并更新节点。
func (s *Service) detectAndUpdateNodeProcess(ctx context.Context, node *ClusterNode, hostID uint) {
	if s.hostProvider == nil || s.agentSender == nil {
		return
	}

	// Get host information
	// 获取主机信息
	hostInfo, err := s.hostProvider.GetHostByID(ctx, hostID)
	if err != nil || hostInfo.AgentID == "" {
		return
	}

	// Check if Agent is online
	// 检查 Agent 是否在线
	if !hostInfo.IsOnline(s.heartbeatTimeout) {
		return
	}

	// Send check_process command to Agent
	// 向 Agent 发送 check_process 命令
	role := "hybrid"
	if node.Role == NodeRoleMaster {
		role = "master"
	} else if node.Role == NodeRoleWorker {
		role = "worker"
	}

	params := map[string]string{
		"role": role,
	}

	success, message, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_process", params)
	if err != nil {
		return
	}

	if success {
		// Process is running, try to extract PID from response
		// 进程正在运行，尝试从响应中提取 PID
		// Response format: {"success":true,"message":"SeaTunnel process found: PID=12345, role=hybrid","details":{"pid":"12345","role":"hybrid"}}
		pid := extractPIDFromMessage(message)
		if pid > 0 {
			_ = s.repo.UpdateNodeProcess(ctx, node.ID, pid, "running")
			_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusRunning)
		} else {
			// Process running but couldn't extract PID
			// 进程运行中但无法提取 PID
			_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusRunning)
		}
		return
	}
	_ = s.repo.UpdateNodeProcess(ctx, node.ID, 0, "stopped")
}

// extractPIDFromMessage extracts PID from Agent response message.
// extractPIDFromMessage 从 Agent 响应消息中提取 PID。
func extractPIDFromMessage(message string) int {
	// Try to parse as JSON first
	// 首先尝试解析为 JSON
	type ProcessResult struct {
		Success bool              `json:"success"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	}

	var result ProcessResult
	if err := json.Unmarshal([]byte(message), &result); err == nil {
		if pidStr, ok := result.Details["pid"]; ok {
			if pid, err := strconv.Atoi(pidStr); err == nil {
				return pid
			}
		}
	}

	// Fallback: try to extract PID from message string "PID=12345"
	// 回退：尝试从消息字符串 "PID=12345" 中提取 PID
	if idx := strings.Index(message, "PID="); idx >= 0 {
		pidStr := message[idx+4:]
		if endIdx := strings.IndexAny(pidStr, ", \t\n"); endIdx > 0 {
			pidStr = pidStr[:endIdx]
		}
		if pid, err := strconv.Atoi(pidStr); err == nil {
			return pid
		}
	}

	return 0
}

// ==================== Node Operation Methods 节点操作方法 ====================

// StartNode starts a single node in a cluster.
// StartNode 启动集群中的单个节点。
func (s *Service) StartNode(ctx context.Context, clusterID uint, nodeID uint) (*OperationResult, error) {
	return s.executeNodeOperation(ctx, clusterID, nodeID, OperationStart)
}

// StartNodeByClusterAndHost starts a node by cluster ID and host ID.
// StartNodeByClusterAndHost 根据集群 ID 和主机 ID 启动节点。
// When a host has multiple nodes (master + worker), use StartNodeByClusterAndHostAndRole to start the specific role.
// 当同一主机有多个节点（master+worker）时，请使用 StartNodeByClusterAndHostAndRole 启动指定角色。
// This implements the installer.NodeStarter interface.
// 这实现了 installer.NodeStarter 接口。
func (s *Service) StartNodeByClusterAndHost(ctx context.Context, clusterID uint, hostID uint) (bool, string, error) {
	node, err := s.repo.GetNodeByClusterAndHost(ctx, clusterID, hostID)
	if err != nil {
		return false, "", fmt.Errorf("failed to find node: %w / 查找节点失败: %w", err, err)
	}
	if node == nil {
		return false, "", fmt.Errorf("node not found for cluster %d and host %d / 未找到集群 %d 主机 %d 对应的节点", clusterID, hostID, clusterID, hostID)
	}
	result, err := s.executeNodeOperation(ctx, clusterID, node.ID, OperationStart)
	if err != nil {
		return false, "", err
	}
	if len(result.NodeResults) > 0 {
		return result.NodeResults[0].Success, result.NodeResults[0].Message, nil
	}
	return result.Success, result.Message, nil
}

// StartNodeByClusterAndHostAndRole starts a node by cluster ID, host ID and role.
// StartNodeByClusterAndHostAndRole 根据集群 ID、主机 ID 和角色启动节点。
// Used after install when one host has both master and worker (separated mode); starts the node that was just installed.
// 安装完成后、同一主机兼有 master 与 worker 时使用，用于启动刚安装完成的那一个节点。
func (s *Service) StartNodeByClusterAndHostAndRole(ctx context.Context, clusterID uint, hostID uint, role string) (bool, string, error) {
	node, err := s.repo.GetNodeByClusterAndHostAndRole(ctx, clusterID, hostID, role)
	if err != nil {
		return false, "", fmt.Errorf("failed to find node: %w / 查找节点失败: %w", err, err)
	}
	if node == nil {
		return false, "", fmt.Errorf("node not found for cluster %d, host %d, role %s / 未找到集群 %d 主机 %d 角色 %s 对应的节点", clusterID, hostID, role, clusterID, hostID, role)
	}
	result, err := s.executeNodeOperation(ctx, clusterID, node.ID, OperationStart)
	if err != nil {
		return false, "", err
	}
	if len(result.NodeResults) > 0 {
		return result.NodeResults[0].Success, result.NodeResults[0].Message, nil
	}
	return result.Success, result.Message, nil
}

// StopNode stops a single node in a cluster.
// StopNode 停止集群中的单个节点。
func (s *Service) StopNode(ctx context.Context, clusterID uint, nodeID uint) (*OperationResult, error) {
	return s.executeNodeOperation(ctx, clusterID, nodeID, OperationStop)
}

// RestartNode restarts a single node in a cluster.
// RestartNode 重启集群中的单个节点。
func (s *Service) RestartNode(ctx context.Context, clusterID uint, nodeID uint) (*OperationResult, error) {
	return s.executeNodeOperation(ctx, clusterID, nodeID, OperationRestart)
}

func (s *Service) restartNodeWithResolvedSpec(ctx context.Context, clusterID uint, node *ClusterNode) (*OperationResult, error) {
	if node == nil {
		return nil, fmt.Errorf("node is required / 节点不能为空")
	}
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	return s.executeNodeOperationWithResolvedNode(ctx, cluster, node, OperationRestart)
}

// executeNodeOperation executes an operation on a single node.
// executeNodeOperation 在单个节点上执行操作。
func (s *Service) executeNodeOperation(ctx context.Context, clusterID uint, nodeID uint, operation OperationType) (*OperationResult, error) {
	// Get node
	// 获取节点
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	if node.ClusterID != clusterID {
		return nil, ErrNodeNotFound
	}

	// Get cluster for install_dir
	// 获取集群以获取安装目录
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	return s.executeNodeOperationWithResolvedNode(ctx, cluster, node, operation)
}

func (s *Service) executeNodeOperationWithResolvedNode(ctx context.Context, cluster *Cluster, node *ClusterNode, operation OperationType) (*OperationResult, error) {
	result := &OperationResult{
		ClusterID:   cluster.ID,
		Operation:   operation,
		Success:     true,
		NodeResults: make([]*NodeOperationResult, 0, 1),
	}

	nodeResult := &NodeOperationResult{
		NodeID: node.ID,
		HostID: node.HostID,
	}

	// Get host information
	// 获取主机信息
	if s.hostProvider != nil {
		hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
		if err != nil {
			return nil, fmt.Errorf("failed to get host information: %w / 获取主机信息失败: %w", err, err)
		}

		nodeResult.HostName = hostInfo.Name

		// Check if host is online - return error immediately if offline
		// 检查主机是否在线 - 如果离线立即返回错误
		if !hostInfo.IsOnline(s.heartbeatTimeout) {
			return nil, fmt.Errorf("host '%s' is offline, cannot execute %s operation / 主机 '%s' 离线，无法执行 %s 操作", hostInfo.Name, operation, hostInfo.Name, operation)
		}

		// Check if agent is connected
		// 检查 Agent 是否已连接
		if hostInfo.AgentID == "" {
			return nil, fmt.Errorf("agent not installed on host '%s' / 主机 '%s' 未安装 Agent", hostInfo.Name, hostInfo.Name)
		}

		// Check if agent sender is available
		// 检查 Agent 发送器是否可用
		if s.agentSender == nil {
			return nil, fmt.Errorf("agent sender not configured / Agent 发送器未配置")
		}

		// Send command to agent
		// 向 Agent 发送命令
		installDir := node.InstallDir
		if installDir == "" {
			installDir = cluster.InstallDir
		}

		params := map[string]string{
			"cluster_id":  fmt.Sprintf("%d", cluster.ID),
			"node_id":     fmt.Sprintf("%d", node.ID),
			"role":        string(node.Role),
			"install_dir": installDir,
		}

		success, message, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, string(operation), params)
		if err != nil {
			return nil, fmt.Errorf("failed to send command to agent: %w / 向 Agent 发送命令失败: %w", err, err)
		}

		nodeResult.Success = success
		nodeResult.Message = message
		if !success {
			result.Success = false
		}
	} else {
		return nil, fmt.Errorf("host provider not configured / 主机提供者未配置")
	}

	// Update node status based on operation
	// 根据操作更新节点状态
	if nodeResult.Success {
		switch operation {
		case OperationStart:
			_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusRunning)
		case OperationStop:
			_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusStopped)
			_ = s.repo.UpdateNodeProcess(ctx, node.ID, 0, "stopped")
		case OperationRestart:
			_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusRunning)
		}
		// Detect process after start/restart with a short delay
		// 启动/重启后延迟检测进程（等待进程完全启动）
		if operation == OperationStart || operation == OperationRestart {
			// Wait 2 seconds for process to fully start
			// 等待 2 秒让进程完全启动
			time.Sleep(2 * time.Second)
			s.detectAndUpdateNodeProcess(ctx, node, node.HostID)
		}
	} else {
		_ = s.repo.UpdateNodeStatus(ctx, node.ID, NodeStatusError)
	}

	result.NodeResults = append(result.NodeResults, nodeResult)
	if result.Success {
		result.Message = "Operation completed successfully / 操作成功完成"
	} else {
		result.Message = nodeResult.Message
	}

	// Update cluster status based on all nodes' status
	// 根据所有节点的状态更新集群状态
	s.updateClusterStatusFromNodes(ctx, cluster.ID)

	return result, nil
}

// updateClusterStatusFromNodes updates cluster status based on all nodes' status
// updateClusterStatusFromNodes 根据所有节点的状态更新集群状态
func (s *Service) updateClusterStatusFromNodes(ctx context.Context, clusterID uint) {
	nodes, err := s.repo.GetNodesByClusterID(ctx, clusterID)
	if err != nil || len(nodes) == 0 {
		return
	}

	// Count node statuses / 统计节点状态
	runningCount := 0
	stoppedCount := 0
	errorCount := 0
	otherCount := 0

	for _, node := range nodes {
		switch node.Status {
		case NodeStatusRunning:
			runningCount++
		case NodeStatusStopped:
			stoppedCount++
		case NodeStatusError:
			errorCount++
		default:
			otherCount++
		}
	}

	// Determine cluster status / 确定集群状态
	var newStatus ClusterStatus
	if errorCount > 0 {
		newStatus = ClusterStatusError
	} else if runningCount == len(nodes) {
		newStatus = ClusterStatusRunning
	} else if stoppedCount == len(nodes) {
		newStatus = ClusterStatusStopped
	} else if runningCount > 0 {
		// Some nodes running, some stopped / 部分节点运行，部分停止
		newStatus = ClusterStatusRunning
	} else {
		newStatus = ClusterStatusCreated
	}

	logger.DebugF(ctx, "[Cluster] updateClusterStatusFromNodes: cluster_id=%d, running=%d, stopped=%d, error=%d, other=%d, newStatus=%s",
		clusterID, runningCount, stoppedCount, errorCount, otherCount, newStatus)

	_ = s.repo.UpdateStatus(ctx, clusterID, newStatus)
}

// GetNodeLogsRequest represents the request for getting node logs.
// GetNodeLogsRequest 表示获取节点日志的请求。
type GetNodeLogsRequest struct {
	Lines  int    `json:"lines" form:"lines"`   // Number of lines / 行数
	Mode   string `json:"mode" form:"mode"`     // "tail" (default), "head", "all" / 模式
	Filter string `json:"filter" form:"filter"` // Filter pattern / 过滤模式
	Date   string `json:"date" form:"date"`     // Date for rolling logs / 滚动日志日期
}

// GetNodeLogs gets the logs of a node.
// GetNodeLogs 获取节点的日志。
func (s *Service) GetNodeLogs(ctx context.Context, clusterID uint, nodeID uint, req *GetNodeLogsRequest) (string, error) {
	// Get node
	// 获取节点
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return "", err
	}

	if node.ClusterID != clusterID {
		return "", ErrNodeNotFound
	}

	// Get cluster to check deployment mode
	// 获取集群以检查部署模式
	cluster, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return "", err
	}

	// Get host information
	// 获取主机信息
	if s.hostProvider == nil {
		return "", fmt.Errorf("host provider not configured / 主机提供者未配置")
	}

	hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil {
		return "", err
	}

	if !hostInfo.IsOnline(s.heartbeatTimeout) {
		return "", fmt.Errorf("host is offline / 主机离线")
	}

	if s.agentSender == nil || hostInfo.AgentID == "" {
		return "", fmt.Errorf("agent sender not configured / Agent 发送器未配置")
	}

	// Determine log file based on deployment mode and role
	// 根据部署模式和角色确定日志文件
	installDir := strings.TrimSpace(node.InstallDir)
	if installDir == "" {
		installDir = strings.TrimSpace(cluster.InstallDir)
	}
	if installDir == "" {
		installDir = seatunnel.DefaultInstallDir(cluster.Version)
	}

	var logFile string
	// In hybrid mode, all nodes use seatunnel-engine-server.log
	// 混合模式下，所有节点使用 seatunnel-engine-server.log
	if cluster.DeploymentMode == DeploymentModeHybrid {
		logFile = fmt.Sprintf("%s/logs/seatunnel-engine-server.log", installDir)
	} else {
		// In separated mode, use role-specific log files
		// 分离模式下，使用角色特定的日志文件
		switch node.Role {
		case NodeRoleMaster:
			logFile = fmt.Sprintf("%s/logs/seatunnel-engine-master.log", installDir)
		case NodeRoleWorker:
			logFile = fmt.Sprintf("%s/logs/seatunnel-engine-worker.log", installDir)
		default:
			logFile = fmt.Sprintf("%s/logs/seatunnel-engine-server.log", installDir)
		}
	}

	// Set default values / 设置默认值
	lines := req.Lines
	if lines <= 0 {
		lines = 100
	}
	mode := req.Mode
	if mode == "" {
		mode = "tail"
	}

	// Send get_logs command to agent
	// 向 Agent 发送 get_logs 命令
	params := map[string]string{
		"log_file": logFile,
		"lines":    fmt.Sprintf("%d", lines),
		"mode":     mode,
	}
	if req.Filter != "" {
		params["filter"] = req.Filter
	}
	if req.Date != "" {
		params["date"] = req.Date
	}

	success, message, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "get_logs", params)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %v / 获取日志失败: %v", err, err)
	}

	if !success {
		return "", fmt.Errorf("failed to get logs: %s / 获取日志失败: %s", message, message)
	}

	return message, nil
}

// ============================================================================
// Monitor Config Push Methods (Task 8.5)
// 监控配置下发方法
// ============================================================================

// GetNodeByHostAndInstallDirAndRole retrieves a cluster node by host ID, install directory and role.
// GetNodeByHostAndInstallDirAndRole 根据主机 ID、安装目录和角色获取集群节点。
// This implements the grpc.ClusterNodeProvider interface.
// 这实现了 grpc.ClusterNodeProvider 接口。
func (s *Service) GetNodeByHostAndInstallDirAndRole(ctx context.Context, hostID uint, installDir, role string) (clusterID, nodeID uint, found bool, err error) {
	return s.repo.GetNodeByHostAndInstallDirAndRole(ctx, hostID, installDir, role)
}

// GetClusterNodesWithAgentInfo retrieves all nodes for a cluster with agent information.
// GetClusterNodesWithAgentInfo 获取集群的所有节点及其 Agent 信息。
// Returns nodes with their associated agent IDs for config push.
// 返回节点及其关联的 Agent ID 用于配置下发。
func (s *Service) GetClusterNodesWithAgentInfo(ctx context.Context, clusterID uint) ([]*NodeInfo, error) {
	nodes, err := s.repo.GetNodesByClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	nodeInfos := make([]*NodeInfo, 0, len(nodes))
	for _, node := range nodes {
		nodeInfo := &NodeInfo{
			ID:            node.ID,
			ClusterID:     node.ClusterID,
			HostID:        node.HostID,
			Role:          node.Role,
			InstallDir:    node.InstallDir,
			HazelcastPort: node.HazelcastPort,
			APIPort:       node.APIPort,
			WorkerPort:    node.WorkerPort,
			Status:        node.Status,
			ProcessPID:    node.ProcessPID,
			CreatedAt:     node.CreatedAt,
			UpdatedAt:     node.UpdatedAt,
		}

		// Get host information; when host is offline, show node as offline
		if s.hostProvider != nil {
			hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
			if err == nil {
				nodeInfo.HostName = hostInfo.Name
				nodeInfo.HostIP = hostInfo.IPAddress
				nodeInfo.IsOnline = hostInfo.IsOnline(s.heartbeatTimeout)
				if !nodeInfo.IsOnline {
					nodeInfo.Status = NodeStatusOffline
				}
			} else {
				nodeInfo.IsOnline = false
				nodeInfo.Status = NodeStatusOffline
			}
		}

		nodeInfos = append(nodeInfos, nodeInfo)
	}

	return nodeInfos, nil
}

// GetNodesByHostID retrieves all cluster nodes for a specific host.
// GetNodesByHostID 获取特定主机上的所有集群节点。
// This is used for pushing monitor config to agent after registration.
// 这用于在 Agent 注册后推送监控配置。
func (s *Service) GetNodesByHostID(ctx context.Context, hostID uint) ([]*ClusterNode, error) {
	return s.repo.GetNodesByHostID(ctx, hostID)
}

// UpdateNodeProcessStatus updates the process PID and status for a node.
// UpdateNodeProcessStatus 更新节点的进程 PID 和状态。
// This is called when agent reports process events (started, stopped, crashed, restarted).
// 当 Agent 上报进程事件（启动、停止、崩溃、重启）时调用。
func (s *Service) UpdateNodeProcessStatus(ctx context.Context, nodeID uint, pid int, status string) error {
	logger.DebugF(ctx, "[Cluster] UpdateNodeProcessStatus: nodeID=%d, pid=%d, status=%s", nodeID, pid, status)
	return s.repo.UpdateNodeProcessStatus(ctx, nodeID, pid, status)
}

// RefreshClusterStatusFromNodes recalculates and updates cluster status from its nodes' status.
// Called after heartbeat updates node process status so cluster status stays in sync.
// RefreshClusterStatusFromNodes 根据节点状态重新计算并更新集群状态；心跳更新节点后调用以保持集群状态一致。
func (s *Service) RefreshClusterStatusFromNodes(ctx context.Context, clusterID uint) {
	s.updateClusterStatusFromNodes(ctx, clusterID)
}
