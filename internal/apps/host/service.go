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

// Package host provides host management functionality for the STX Agent system.
// host 包提供 STX Agent 系统的主机管理功能。
package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
)

// DefaultHeartbeatTimeout is the default timeout for considering a host offline.
// DefaultHeartbeatTimeout 是判断主机离线的默认超时时间。
const DefaultHeartbeatTimeout = 30 * time.Second

// Service provides business logic for host management operations.
// Service 提供主机管理操作的业务逻辑。
type Service struct {
	repo             *Repository
	clusterRepo      *cluster.Repository
	heartbeatTimeout time.Duration
	controlPlaneAddr string
	processStartedAt time.Time // process start time; online requires heartbeat after this
}

// ServiceConfig holds configuration for the Host Service.
// ServiceConfig 保存 Host Service 的配置。
type ServiceConfig struct {
	HeartbeatTimeout time.Duration
	ControlPlaneAddr string
}

// NewService creates a new Service instance.
// NewService 创建一个新的 Service 实例。
func NewService(repo *Repository, clusterRepo *cluster.Repository, cfg *ServiceConfig) *Service {
	timeout := DefaultHeartbeatTimeout
	controlPlaneAddr := "localhost:17800"

	if cfg != nil {
		if cfg.HeartbeatTimeout > 0 {
			timeout = cfg.HeartbeatTimeout
		}
		if cfg.ControlPlaneAddr != "" {
			controlPlaneAddr = cfg.ControlPlaneAddr
		}
	}

	return &Service{
		repo:             repo,
		clusterRepo:      clusterRepo,
		heartbeatTimeout: timeout,
		controlPlaneAddr: controlPlaneAddr,
		processStartedAt: time.Now(),
	}
}

// GetProcessStartedAt returns the process start time used for "online since start" checks.
func (s *Service) GetProcessStartedAt() time.Time {
	return s.processStartedAt
}

// Create creates a new host with validation.
// Create 创建一个新主机并进行验证。
// Requirements: 3.1 - Validates host name uniqueness and IP address format.
func (s *Service) Create(ctx context.Context, req *CreateHostRequest) (*Host, error) {
	// Validate host name is not empty
	// 验证主机名不为空
	if req.Name == "" {
		return nil, ErrHostNameEmpty
	}

	// Set default host type if not specified
	// 如果未指定，设置默认主机类型
	hostType := req.HostType
	if hostType == "" {
		hostType = HostTypeBareMetal
	}

	// Validate host type
	// 验证主机类型
	if !hostType.IsValid() {
		return nil, ErrHostTypeInvalid
	}

	// Create host based on type
	// 根据类型创建主机
	host := &Host{
		Name:        req.Name,
		HostType:    hostType,
		Description: req.Description,
		Status:      HostStatusPending,
	}

	// Validate and set type-specific fields
	// 验证并设置类型特定字段
	switch hostType {
	case HostTypeBareMetal:
		if err := s.validateBareMetalHost(req, host); err != nil {
			return nil, err
		}
	case HostTypeDocker:
		if err := s.validateDockerHost(req, host); err != nil {
			return nil, err
		}
	case HostTypeKubernetes:
		if err := s.validateK8sHost(req, host); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Create(ctx, host); err != nil {
		return nil, err
	}

	return host, nil
}

// validateBareMetalHost validates and sets fields for bare_metal host type.
// validateBareMetalHost 验证并设置物理机/VM 类型的字段。
func (s *Service) validateBareMetalHost(req *CreateHostRequest, host *Host) error {
	// Validate IP address format
	// 验证 IP 地址格式
	if !ValidateIPAddress(req.IPAddress) {
		return ErrHostIPInvalid
	}

	// Set default SSH port if not specified
	// 如果未指定，设置默认 SSH 端口
	sshPort := req.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}

	host.IPAddress = req.IPAddress
	host.SSHPort = sshPort
	host.AgentStatus = AgentStatusNotInstalled
	return nil
}

// validateDockerHost validates and sets fields for docker host type.
// validateDockerHost 验证并设置 Docker 类型的字段。
func (s *Service) validateDockerHost(req *CreateHostRequest, host *Host) error {
	// Validate Docker API URL format
	// 验证 Docker API URL 格式
	if !ValidateDockerAPIURL(req.DockerAPIURL) {
		return ErrDockerAPIURLInvalid
	}

	host.DockerAPIURL = req.DockerAPIURL
	host.DockerTLSEnabled = req.DockerTLSEnabled
	host.DockerCertPath = req.DockerCertPath
	// Docker hosts don't need Agent, status will be updated after API connection test
	// Docker 主机不需要 Agent，状态将在 API 连接测试后更新
	return nil
}

// validateK8sHost validates and sets fields for kubernetes host type.
// validateK8sHost 验证并设置 Kubernetes 类型的字段。
func (s *Service) validateK8sHost(req *CreateHostRequest, host *Host) error {
	// Validate K8s API URL format
	// 验证 K8s API URL 格式
	if !ValidateK8sAPIURL(req.K8sAPIURL) {
		return ErrK8sAPIURLInvalid
	}

	// Must have either kubeconfig or token
	// 必须有 kubeconfig 或 token
	if req.K8sKubeconfig == "" && req.K8sToken == "" {
		return ErrK8sCredentialsRequired
	}

	host.K8sAPIURL = req.K8sAPIURL
	host.K8sNamespace = req.K8sNamespace
	if host.K8sNamespace == "" {
		host.K8sNamespace = "default"
	}
	host.K8sKubeconfig = req.K8sKubeconfig
	host.K8sToken = req.K8sToken
	// K8s hosts don't need Agent, status will be updated after API connection test
	// K8s 主机不需要 Agent，状态将在 API 连接测试后更新
	return nil
}

// Get retrieves a host by ID.
// Get 根据 ID 获取主机。
// Requirements: 3.5 - Returns host details including agent status and resource usage.
func (s *Service) Get(ctx context.Context, id uint) (*Host, error) {
	return s.repo.GetByID(ctx, id)
}

// GetByIP retrieves a host by IP address.
// GetByIP 根据 IP 地址获取主机。
func (s *Service) GetByIP(ctx context.Context, ip string) (*Host, error) {
	return s.repo.GetByIP(ctx, ip)
}

// GetByAgentID retrieves a host by Agent ID.
// GetByAgentID 根据 Agent ID 获取主机。
func (s *Service) GetByAgentID(ctx context.Context, agentID string) (*Host, error) {
	return s.repo.GetByAgentID(ctx, agentID)
}

// List retrieves hosts based on filter criteria.
// List 根据过滤条件获取主机列表。
// Requirements: 3.5 - Returns host name, IP, agent status, online status, resource usage, last heartbeat.
func (s *Service) List(ctx context.Context, filter *HostFilter) ([]*Host, int64, error) {
	return s.repo.List(ctx, filter, s.heartbeatTimeout, s.processStartedAt)
}

// ListWithInfo retrieves hosts and converts them to HostInfo with online status.
// ListWithInfo 获取主机列表并转换为包含在线状态的 HostInfo。
func (s *Service) ListWithInfo(ctx context.Context, filter *HostFilter) ([]*HostInfo, int64, error) {
	hosts, total, err := s.repo.List(ctx, filter, s.heartbeatTimeout, s.processStartedAt)
	if err != nil {
		return nil, 0, err
	}

	infos := make([]*HostInfo, len(hosts))
	for i, h := range hosts {
		infos[i] = h.ToHostInfo(s.heartbeatTimeout, s.processStartedAt)
	}

	return infos, total, nil
}

// Update updates an existing host with validation.
// Update 更新现有主机并进行验证。
func (s *Service) Update(ctx context.Context, id uint, req *UpdateHostRequest) (*Host, error) {
	// Get existing host
	// 获取现有主机
	host, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update common fields if provided
	// 如果提供了通用字段则更新
	if req.Name != nil {
		if *req.Name == "" {
			return nil, ErrHostNameEmpty
		}
		host.Name = *req.Name
	}

	if req.Description != nil {
		host.Description = *req.Description
	}

	// Update type-specific fields based on host type
	// 根据主机类型更新特定字段
	switch host.HostType {
	case HostTypeBareMetal, "":
		if err := s.updateBareMetalFields(req, host); err != nil {
			return nil, err
		}
	case HostTypeDocker:
		if err := s.updateDockerFields(req, host); err != nil {
			return nil, err
		}
	case HostTypeKubernetes:
		if err := s.updateK8sFields(req, host); err != nil {
			return nil, err
		}
	}

	if err := s.repo.Update(ctx, host); err != nil {
		return nil, err
	}

	return host, nil
}

// updateBareMetalFields updates bare_metal specific fields.
// updateBareMetalFields 更新物理机/VM 特定字段。
func (s *Service) updateBareMetalFields(req *UpdateHostRequest, host *Host) error {
	if req.IPAddress != nil {
		if !ValidateIPAddress(*req.IPAddress) {
			return ErrHostIPInvalid
		}
		host.IPAddress = *req.IPAddress
	}

	if req.SSHPort != nil {
		host.SSHPort = *req.SSHPort
	}
	return nil
}

// updateDockerFields updates docker specific fields.
// updateDockerFields 更新 Docker 特定字段。
func (s *Service) updateDockerFields(req *UpdateHostRequest, host *Host) error {
	if req.DockerAPIURL != nil {
		if !ValidateDockerAPIURL(*req.DockerAPIURL) {
			return ErrDockerAPIURLInvalid
		}
		host.DockerAPIURL = *req.DockerAPIURL
	}

	if req.DockerTLSEnabled != nil {
		host.DockerTLSEnabled = *req.DockerTLSEnabled
	}

	if req.DockerCertPath != nil {
		host.DockerCertPath = *req.DockerCertPath
	}
	return nil
}

// updateK8sFields updates kubernetes specific fields.
// updateK8sFields 更新 Kubernetes 特定字段。
func (s *Service) updateK8sFields(req *UpdateHostRequest, host *Host) error {
	if req.K8sAPIURL != nil {
		if !ValidateK8sAPIURL(*req.K8sAPIURL) {
			return ErrK8sAPIURLInvalid
		}
		host.K8sAPIURL = *req.K8sAPIURL
	}

	if req.K8sNamespace != nil {
		host.K8sNamespace = *req.K8sNamespace
	}

	if req.K8sKubeconfig != nil {
		host.K8sKubeconfig = *req.K8sKubeconfig
	}

	if req.K8sToken != nil {
		host.K8sToken = *req.K8sToken
	}
	return nil
}

// Delete removes a host after checking cluster associations.
// Delete 在检查集群关联后删除主机。
// Requirements: 3.6 - Checks if host is associated with clusters before deletion.
func (s *Service) Delete(ctx context.Context, id uint) error {
	// Check if host exists
	// 检查主机是否存在
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Check if host is associated with any clusters
	// 检查主机是否关联任何集群
	if s.clusterRepo != nil {
		clusters, err := s.clusterRepo.GetClustersWithHostID(ctx, id)
		if err != nil {
			return err
		}
		if len(clusters) > 0 {
			return ErrHostHasCluster
		}
	}

	return s.repo.Delete(ctx, id)
}

// GetAssociatedClusters returns the list of clusters associated with a host.
// GetAssociatedClusters 返回与主机关联的集群列表。
func (s *Service) GetAssociatedClusters(ctx context.Context, hostID uint) ([]*cluster.Cluster, error) {
	if s.clusterRepo == nil {
		return nil, nil
	}
	return s.clusterRepo.GetClustersWithHostID(ctx, hostID)
}

// UpdateAgentStatus updates the agent status when an Agent registers.
// UpdateAgentStatus 在 Agent 注册时更新 Agent 状态。
// Requirements: 3.2 - Matches Agent hostID or IP with registered host and updates status to "installed".
// If hostID > 0, attempts to find and bind to the specified host first.
// 若 hostID > 0，则优先查找并绑定指定的主机 ID。
// If no host matches, falls back to matching by IP, then by uninstalled host name, or auto-creates a bare_metal host.
// 若未匹配到主机，则依次回退到按 IP 查找、按未安装主机名匹配或自动创建物理机。
// localIPs is the Agent's local address set; when the stored host IP is not among them, it is treated as wrong and corrected.
// localIPs 为本机地址集合；若主机已存 IP 不在该集合中，视为用户填错并校正为上报主 IP。
func (s *Service) UpdateAgentStatus(ctx context.Context, hostID uint, ipAddress string, agentID string, version string, systemInfo *SystemInfo, hostname string, localIPs []string) (*Host, error) {
	var host *Host
	var err error
	created := false

	// 1. Try matching by explicit hostID if provided
	// 1. 若提供了显式 hostID，优先按 hostID 匹配
	if hostID > 0 {
		host, err = s.repo.GetByID(ctx, hostID)
		if err != nil && !errors.Is(err, ErrHostNotFound) {
			return nil, err
		}
	}

	// 2. Fall back to finding host by IP address
	// 2. 若未指定 hostID 或未找到，按 IP 地址查找主机
	if host == nil && ipAddress != "" {
		host, err = s.repo.GetByIP(ctx, ipAddress)
		if err != nil && !errors.Is(err, ErrHostNotFound) {
			return nil, err
		}
	}

	// 3. Fall back to matching an uninstalled host by hostname
	// 3. 若按 IP 未找到，尝试匹配同名且未安装的主机
	if host == nil && hostname != "" {
		namedHost, err := s.repo.GetByName(ctx, hostname)
		if err == nil && namedHost != nil && (namedHost.AgentStatus == AgentStatusNotInstalled || namedHost.AgentStatus == "") {
			host = namedHost
		}
	}

	// 4. Auto-create host when still no matching host exists
	// 4. 当仍无匹配主机时自动创建（如主服务重启后首次上报）
	if host == nil {
		host, err = s.autoCreateHostForAgent(ctx, ipAddress, agentID, hostname)
		if err != nil {
			return nil, err
		}
		created = true
	}

	// 已有主机：用户填写的 IP 优先保留；仅当为空/回环，或不在 Agent 本机地址集合中时，校正为上报主 IP。
	// Existing host: keep user-entered IP unless empty/loopback, or not present in Agent local address set.
	if !created && shouldCorrectHostIP(host.IPAddress, ipAddress, localIPs) {
		host.IPAddress = ipAddress
		if err := s.repo.Update(ctx, host); err != nil {
			return nil, err
		}
	}

	// Update agent status to installed
	// 更新 Agent 状态为已安装
	if err := s.repo.UpdateAgentStatus(ctx, host.ID, AgentStatusInstalled, agentID, version); err != nil {
		return nil, err
	}

	// Update system info if provided
	// 如果提供了系统信息则更新
	if systemInfo != nil {
		if err := s.repo.UpdateSystemInfo(ctx, host.ID, systemInfo.OSType, systemInfo.Arch, systemInfo.CPUCores, systemInfo.TotalMemory, systemInfo.TotalDisk); err != nil {
			return nil, err
		}
	}

	// Return updated host
	// 返回更新后的主机
	return s.repo.GetByID(ctx, host.ID)
}

// shouldCorrectHostIP 判断是否应用 Agent 上报的主 IP 覆盖主机已存 IP。
// 规则：上报 IP 无效则不改；已存为空/回环则改；已存等于上报则不改；
// 若 Agent 提供了本机地址集合且已存 IP 不在集合中，视为用户填错则改。
// shouldCorrectHostIP decides whether the reported primary IP should replace the stored host IP.
// Rules: skip when reported IP is invalid; correct empty/loopback stored IP; skip when equal;
// when Agent provides local IPs and stored IP is absent, treat as wrong user input and correct.
func shouldCorrectHostIP(storedIP, reportedIP string, localIPs []string) bool {
	reported := strings.TrimSpace(reportedIP)
	if reported == "" || isLoopbackOrLocalhost(reported) {
		return false
	}

	stored := strings.TrimSpace(storedIP)
	if stored == "" || isLoopbackOrLocalhost(stored) {
		return true
	}
	if stored == reported {
		return false
	}

	if len(localIPs) == 0 {
		// 无本机地址集合时无法判定用户 IP 是否属于该机器，保留用户填写。
		// Without a local address set we cannot tell if the stored IP belongs to this machine; keep it.
		return false
	}
	return !ipInList(stored, localIPs)
}

// isLoopbackOrLocalhost 判断是否为回环或 localhost 占位地址。
// isLoopbackOrLocalhost reports whether the IP is loopback or a localhost placeholder.
func isLoopbackOrLocalhost(ip string) bool {
	switch strings.TrimSpace(ip) {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

// ipInList 判断目标 IP 是否出现在地址列表中（忽略首尾空白）。
// ipInList reports whether target appears in the IP list (trim space).
func ipInList(target string, ips []string) bool {
	target = strings.TrimSpace(target)
	for _, ip := range ips {
		if strings.TrimSpace(ip) == target {
			return true
		}
	}
	return false
}

// autoCreateHostForAgent creates a bare_metal host record when an agent registers
// but no host matches by IP (e.g. after Control Plane restart or first-time agent install).
func (s *Service) autoCreateHostForAgent(ctx context.Context, ipAddress string, agentID string, hostname string) (*Host, error) {
	name := strings.TrimSpace(hostname)
	if name == "" {
		name = "agent-" + agentID
	}
	if len(name) > 100 {
		name = name[:97] + "..."
	}

	createReq := &CreateHostRequest{
		Name:        name,
		HostType:    HostTypeBareMetal,
		IPAddress:   ipAddress,
		Description: "Auto-created from agent registration",
	}
	h, err := s.Create(ctx, createReq)
	if err != nil && errors.Is(err, ErrHostNameDuplicate) {
		// Retry with agent ID suffix for uniqueness
		createReq.Name = name + "-" + agentID
		if len(createReq.Name) > 100 {
			createReq.Name = createReq.Name[:97] + "..."
		}
		h, err = s.Create(ctx, createReq)
	}
	if err != nil {
		return nil, err
	}
	return h, nil
}

// UpdateAgentStatusByID updates the agent status for a specific host ID.
// UpdateAgentStatusByID 更新指定主机 ID 的 Agent 状态。
func (s *Service) UpdateAgentStatusByID(ctx context.Context, hostID uint, status AgentStatus, agentID string, version string) error {
	return s.repo.UpdateAgentStatus(ctx, hostID, status, agentID, version)
}

// UpdateHeartbeat updates the heartbeat timestamp and resource usage for a host.
// UpdateHeartbeat 更新主机的心跳时间戳和资源使用率。
// Requirements: 3.3 - Updates last heartbeat time, CPU, memory, and disk usage.
// If the host was marked offline, it is set back to installed so dashboard and host management show the same state.
// 若主机此前被标为离线，会恢复为已安装，使首页与主机管理状态一致。
func (s *Service) UpdateHeartbeat(ctx context.Context, agentID string, cpuUsage, memoryUsage, diskUsage float64) error {
	// Find host by agent ID
	// 根据 Agent ID 查找主机
	host, err := s.repo.GetByAgentID(ctx, agentID)
	if err != nil {
		return err
	}

	// Update heartbeat data
	// 更新心跳数据
	if err := s.repo.UpdateHeartbeat(ctx, host.ID, cpuUsage, memoryUsage, diskUsage); err != nil {
		return err
	}

	// 若当前为离线，收到心跳后恢复为已安装，与首页“在线”一致
	if host.AgentStatus == AgentStatusOffline {
		_ = s.repo.UpdateAgentStatus(ctx, host.ID, AgentStatusInstalled, host.AgentID, host.AgentVersion)
	}
	return nil
}

// UpdateHeartbeatByID updates the heartbeat for a specific host ID.
// UpdateHeartbeatByID 更新指定主机 ID 的心跳。
func (s *Service) UpdateHeartbeatByID(ctx context.Context, hostID uint, cpuUsage, memoryUsage, diskUsage float64) error {
	return s.repo.UpdateHeartbeat(ctx, hostID, cpuUsage, memoryUsage, diskUsage)
}

// MarkOfflineHosts marks all hosts as offline if their last heartbeat exceeds the timeout.
// MarkOfflineHosts 如果主机的最后心跳超过超时时间，则将其标记为离线。
// Requirements: 3.4 - Marks hosts as offline if no heartbeat received for 30 seconds.
func (s *Service) MarkOfflineHosts(ctx context.Context) (int64, error) {
	return s.repo.MarkOfflineHosts(ctx, s.heartbeatTimeout)
}

// CheckAndMarkOffline checks a specific host and marks it offline if heartbeat timeout exceeded.
// CheckAndMarkOffline 检查特定主机，如果心跳超时则标记为离线。
func (s *Service) CheckAndMarkOffline(ctx context.Context, hostID uint) (bool, error) {
	host, err := s.repo.GetByID(ctx, hostID)
	if err != nil {
		return false, err
	}

	// Only check hosts that are currently installed (online)
	// 只检查当前已安装（在线）的主机
	if host.AgentStatus != AgentStatusInstalled {
		return false, nil
	}

	// Check if heartbeat timeout exceeded
	// 检查心跳是否超时
	if !host.IsOnline(s.heartbeatTimeout) {
		if err := s.repo.UpdateAgentStatus(ctx, hostID, AgentStatusOffline, host.AgentID, host.AgentVersion); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// IsHostOnline checks if a host is currently online based on heartbeat.
// IsHostOnline 根据心跳检查主机是否当前在线。
func (s *Service) IsHostOnline(ctx context.Context, hostID uint) (bool, error) {
	host, err := s.repo.GetByID(ctx, hostID)
	if err != nil {
		return false, err
	}
	return host.IsOnline(s.heartbeatTimeout), nil
}

// GetInstallCommand generates the Agent installation command for a host.
// GetInstallCommand 为主机生成 Agent 安装命令。
// Requirements: 2.1 - Returns shell script with auto-detection logic and Control Plane address.
func (s *Service) GetInstallCommand(ctx context.Context, hostID uint) (string, error) {
	// Verify host exists
	// 验证主机存在
	_, err := s.repo.GetByID(ctx, hostID)
	if err != nil {
		return "", err
	}

	// Generate installation command with explicit default --install-dir (user-customizable) and bound host_id.
	// 生成带显式默认 --install-dir 的安装命令（用户可自行改目录）并携带预绑定 host_id。
	// controlPlaneAddr should be a full URL like "http://192.168.1.100:17800"
	// controlPlaneAddr 应该是完整的 URL，如 "http://192.168.1.100:17800"
	addr := strings.TrimRight(strings.TrimSpace(s.controlPlaneAddr), "/")
	if addr == "" {
		addr = "http://localhost:17800"
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	// Keep in sync with agent.DefaultAgentHomePath / 与 agent.DefaultAgentHomePath 保持一致
	return fmt.Sprintf(
		"curl -sSL %s/api/v1/agent/install.sh | bash -s -- --install-dir=%s/ --host-id %d",
		addr,
		"$HOME/.stx/agent",
		hostID,
	), nil
}

// SystemInfo represents system information reported by an Agent.
// SystemInfo 表示 Agent 上报的系统信息。
type SystemInfo struct {
	OSType      string
	Arch        string
	CPUCores    int
	TotalMemory int64
	TotalDisk   int64
}

// GetHeartbeatTimeout returns the configured heartbeat timeout.
// GetHeartbeatTimeout 返回配置的心跳超时时间。
func (s *Service) GetHeartbeatTimeout() time.Duration {
	return s.heartbeatTimeout
}

// GetHostByID implements cluster.HostProvider interface.
// GetHostByID 实现 cluster.HostProvider 接口。
// This method is used by cluster service to get host information.
// 此方法由集群服务用于获取主机信息。
func (s *Service) GetHostByID(ctx context.Context, id uint) (*cluster.HostInfo, error) {
	host, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	startedAt := s.processStartedAt
	return &cluster.HostInfo{
		ID:               host.ID,
		Name:             host.Name,
		HostType:         string(host.HostType),
		IPAddress:        host.IPAddress,
		AgentID:          host.AgentID,
		AgentStatus:      string(host.AgentStatus),
		LastHeartbeat:    host.LastHeartbeat,
		ProcessStartedAt: &startedAt,
	}, nil
}

// UpdateHostStatus updates the connection status for a host (used for Docker/K8s hosts).
// UpdateHostStatus 更新主机的连接状态（用于 Docker/K8s 主机）。
// This method is called after API connection test for Docker/K8s hosts.
// 此方法在 Docker/K8s 主机的 API 连接测试后调用。
func (s *Service) UpdateHostStatus(ctx context.Context, hostID uint, status HostStatus) error {
	host, err := s.repo.GetByID(ctx, hostID)
	if err != nil {
		return err
	}

	host.Status = status
	return s.repo.Update(ctx, host)
}

// UpdateDockerVersion updates the Docker version for a Docker host.
// UpdateDockerVersion 更新 Docker 主机的 Docker 版本。
func (s *Service) UpdateDockerVersion(ctx context.Context, hostID uint, version string) error {
	host, err := s.repo.GetByID(ctx, hostID)
	if err != nil {
		return err
	}

	if host.HostType != HostTypeDocker {
		return ErrHostTypeInvalid
	}

	host.DockerVersion = version
	host.Status = HostStatusConnected
	return s.repo.Update(ctx, host)
}

// UpdateK8sVersion updates the Kubernetes version for a K8s host.
// UpdateK8sVersion 更新 K8s 主机的 Kubernetes 版本。
func (s *Service) UpdateK8sVersion(ctx context.Context, hostID uint, version string) error {
	host, err := s.repo.GetByID(ctx, hostID)
	if err != nil {
		return err
	}

	if host.HostType != HostTypeKubernetes {
		return ErrHostTypeInvalid
	}

	host.K8sVersion = version
	host.Status = HostStatusConnected
	return s.repo.Update(ctx, host)
}

// UpdateResourceUsage updates the resource usage for any host type.
// UpdateResourceUsage 更新任何主机类型的资源使用率。
// This is used for Docker/K8s hosts where resource info is obtained via API.
// 用于通过 API 获取资源信息的 Docker/K8s 主机。
func (s *Service) UpdateResourceUsage(ctx context.Context, hostID uint, cpuUsage, memoryUsage, diskUsage float64) error {
	host, err := s.repo.GetByID(ctx, hostID)
	if err != nil {
		return err
	}

	host.CPUUsage = cpuUsage
	host.MemoryUsage = memoryUsage
	host.DiskUsage = diskUsage
	now := time.Now()
	host.LastCheck = &now

	return s.repo.Update(ctx, host)
}
