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

// Package discovery provides cluster discovery functionality.
// Package discovery 提供集群发现功能。
package discovery

import (
	"context"
	"errors"
	"log"
	"time"
)

// Error definitions for discovery package.
// 发现包的错误定义。
var (
	ErrAgentOffline      = errors.New("agent is offline / Agent 离线")
	ErrAgentNotInstalled = errors.New("agent not installed / Agent 未安装")
	ErrDiscoveryFailed   = errors.New("cluster discovery failed / 集群发现失败")
	ErrClusterConflict   = errors.New("cluster already exists / 集群已存在")
	ErrHostNotFound      = errors.New("host not found / 主机未找到")
)

// DiscoveredProcess represents a discovered SeaTunnel process (simplified).
// DiscoveredProcess 表示发现的 SeaTunnel 进程（简化版）。
type DiscoveredProcess struct {
	PID           int    `json:"pid"`
	Role          string `json:"role"`                     // master, worker, or hybrid
	InstallDir    string `json:"install_dir"`              // SeaTunnel installation directory / SeaTunnel 安装目录
	Version       string `json:"version"`                  // SeaTunnel version / SeaTunnel 版本
	HazelcastPort int    `json:"hazelcast_port"`           // Hazelcast cluster port / Hazelcast 集群端口
	APIPort       int    `json:"api_port"`                 // REST API port / REST API 端口
	ClusterName   string `json:"cluster_name,omitempty"`   // Hazelcast cluster-name / Hazelcast 集群标识
	// JavaHome 是进程实际使用的 JDK（由 Agent 从 /proc/<pid>/exe 解析）；解析不到为空（AC1f）。
	// JavaHome is the JDK the process actually uses (resolved by the Agent from /proc/<pid>/exe); empty when unresolved (AC1f).
	JavaHome string `json:"java_home,omitempty"`
}

// ProcessDiscoveryResult represents the result of process discovery.
// ProcessDiscoveryResult 表示进程发现的结果。
type ProcessDiscoveryResult struct {
	Success   bool                 `json:"success"`
	Message   string               `json:"message"`
	Processes []*DiscoveredProcess `json:"processes"`
}

// DiscoveredCluster represents a discovered SeaTunnel cluster.
// DiscoveredCluster 表示发现的 SeaTunnel 集群。
type DiscoveredCluster struct {
	Name           string                 `json:"name"`
	InstallDir     string                 `json:"install_dir"`
	Version        string                 `json:"version"`
	DeploymentMode string                 `json:"deployment_mode"`
	Nodes          []*DiscoveredNode      `json:"nodes"`
	Config         map[string]interface{} `json:"config"`
	DiscoveredAt   time.Time              `json:"discovered_at"`
	IsNew          bool                   `json:"is_new"`           // 是否为新发现的集群 / Whether it's a newly discovered cluster
	ExistingID     uint                   `json:"existing_id"`      // 如果已存在，关联的集群 ID / If exists, the associated cluster ID
	// JavaHome 是纳管时用户选定的节点 JDK；空表示自动探测（AC1e）。实现 CreateClusterFromDiscovery 时应写入节点。
	// JavaHome is the node JDK chosen at import; empty means auto detection (AC1e). CreateClusterFromDiscovery should persist it.
	JavaHome string `json:"java_home,omitempty"`
}

// DiscoveredNode represents a discovered SeaTunnel node.
// DiscoveredNode 表示发现的 SeaTunnel 节点。
type DiscoveredNode struct {
	PID           int       `json:"pid"`
	Role          string    `json:"role"`
	HazelcastPort int       `json:"hazelcast_port"`
	APIPort       int       `json:"api_port"`
	StartTime     time.Time `json:"start_time"`
	// JavaHome 是进程实际 JDK（AC1f）/ Process actual JDK (AC1f)
	JavaHome string `json:"java_home,omitempty"`
}

// DiscoveryResult represents the result of a discovery operation.
// DiscoveryResult 表示发现操作的结果。
type DiscoveryResult struct {
	Success    bool                 `json:"success"`
	Message    string               `json:"message"`
	Clusters   []*DiscoveredCluster `json:"clusters"`
	NewCount   int                  `json:"new_count"`
	ExistCount int                  `json:"exist_count"`
}

// ConfirmDiscoveryRequest represents a request to confirm discovered clusters.
// ConfirmDiscoveryRequest 表示确认发现集群的请求。
type ConfirmDiscoveryRequest struct {
	HostID      uint     `json:"host_id" binding:"required"`
	ClusterIDs  []string `json:"cluster_ids"`  // 要纳管的集群标识 / Cluster identifiers to manage
	InstallDirs []string `json:"install_dirs"` // 要纳管的安装目录 / Install directories to manage
	// JavaHomes 按安装目录指定节点 JDK；空值表示自动探测（AC1e）。
	// JavaHomes maps install directories to node JDKs; empty means auto detection (AC1e).
	JavaHomes map[string]string `json:"java_homes,omitempty"`
}

// AgentDiscoverer defines the interface for triggering agent discovery.
// AgentDiscoverer 定义触发 Agent 发现的接口。
type AgentDiscoverer interface {
	TriggerDiscovery(ctx context.Context, agentID string) ([]*DiscoveredCluster, error)
	DiscoverProcesses(ctx context.Context, agentID string) ([]*DiscoveredProcess, error)
}

// HostProvider defines the interface for getting host information.
// HostProvider 定义获取主机信息的接口。
type HostProvider interface {
	GetHostByID(ctx context.Context, hostID uint) (*HostInfo, error)
	GetHostByAgentID(ctx context.Context, agentID string) (*HostInfo, error)
}

// HostInfo represents host information for discovery.
// HostInfo 表示用于发现的主机信息。
type HostInfo struct {
	ID          uint
	Name        string
	IPAddress   string
	AgentID     string
	AgentStatus string
}

// ClusterMatcher defines the interface for matching discovered clusters.
// ClusterMatcher 定义匹配发现集群的接口。
type ClusterMatcher interface {
	FindClusterByInstallDir(ctx context.Context, hostID uint, installDir string) (clusterID uint, nodeID uint, found bool, err error)
	CreateClusterFromDiscovery(ctx context.Context, hostID uint, cluster *DiscoveredCluster) (uint, error)
	UpdateNodeFromDiscovery(ctx context.Context, nodeID uint, node *DiscoveredNode) error
}

// Service provides cluster discovery functionality.
// Service 提供集群发现功能。
type Service struct {
	agentDiscoverer AgentDiscoverer
	hostProvider    HostProvider
	clusterMatcher  ClusterMatcher
}

// NewService creates a new discovery service.
// NewService 创建新的发现服务。
func NewService() *Service {
	return &Service{}
}

// SetAgentDiscoverer sets the agent discoverer.
// SetAgentDiscoverer 设置 Agent 发现器。
func (s *Service) SetAgentDiscoverer(discoverer AgentDiscoverer) {
	s.agentDiscoverer = discoverer
}

// SetHostProvider sets the host provider.
// SetHostProvider 设置主机提供者。
func (s *Service) SetHostProvider(provider HostProvider) {
	s.hostProvider = provider
}

// SetClusterMatcher sets the cluster matcher.
// SetClusterMatcher 设置集群匹配器。
func (s *Service) SetClusterMatcher(matcher ClusterMatcher) {
	s.clusterMatcher = matcher
}

// DiscoverProcesses discovers SeaTunnel processes on a host (simplified).
// DiscoverProcesses 在主机上发现 SeaTunnel 进程（简化版）。
// Only returns PID, role, and install_dir - no config parsing.
// 只返回 PID、角色和安装目录 - 不解析配置。
func (s *Service) DiscoverProcesses(ctx context.Context, hostID uint) (*ProcessDiscoveryResult, error) {
	if s.hostProvider == nil {
		return nil, errors.New("host provider not configured / 主机提供者未配置")
	}
	if s.agentDiscoverer == nil {
		return nil, errors.New("agent discoverer not configured / Agent 发现器未配置")
	}

	// Get host info / 获取主机信息
	host, err := s.hostProvider.GetHostByID(ctx, hostID)
	if err != nil {
		return nil, ErrHostNotFound
	}

	// Check agent status / 检查 Agent 状态
	if host.AgentID == "" {
		return nil, ErrAgentNotInstalled
	}
	if host.AgentStatus != "installed" && host.AgentStatus != "online" {
		return nil, ErrAgentOffline
	}

	log.Printf("[Discovery] Discovering processes on host %d (%s) / 在主机 %d (%s) 上发现进程",
		hostID, host.Name, hostID, host.Name)

	// Discover processes via agent / 通过 Agent 发现进程
	processes, err := s.agentDiscoverer.DiscoverProcesses(ctx, host.AgentID)
	if err != nil {
		log.Printf("[Discovery] Process discovery failed on host %d: %v / 主机 %d 进程发现失败: %v",
			hostID, err, hostID, err)
		return nil, ErrDiscoveryFailed
	}

	result := &ProcessDiscoveryResult{
		Success:   true,
		Message:   "process discovery completed / 进程发现完成",
		Processes: processes,
	}

	log.Printf("[Discovery] Found %d processes on host %d / 在主机 %d 上发现 %d 个进程",
		len(processes), hostID, hostID, len(processes))

	return result, nil
}

// TriggerDiscovery triggers cluster discovery on a host.
// TriggerDiscovery 在主机上触发集群发现。
// Requirements: 1.3, 1.7 - Trigger agent discovery
func (s *Service) TriggerDiscovery(ctx context.Context, hostID uint) (*DiscoveryResult, error) {
	if s.hostProvider == nil {
		return nil, errors.New("host provider not configured / 主机提供者未配置")
	}
	if s.agentDiscoverer == nil {
		return nil, errors.New("agent discoverer not configured / Agent 发现器未配置")
	}

	// Get host info / 获取主机信息
	host, err := s.hostProvider.GetHostByID(ctx, hostID)
	if err != nil {
		return nil, ErrHostNotFound
	}

	// Check agent status / 检查 Agent 状态
	if host.AgentID == "" {
		return nil, ErrAgentNotInstalled
	}
	if host.AgentStatus != "installed" && host.AgentStatus != "online" {
		return nil, ErrAgentOffline
	}

	log.Printf("[Discovery] Triggering discovery on host %d (%s) / 在主机 %d (%s) 上触发发现",
		hostID, host.Name, hostID, host.Name)

	// Trigger discovery via agent / 通过 Agent 触发发现
	clusters, err := s.agentDiscoverer.TriggerDiscovery(ctx, host.AgentID)
	if err != nil {
		log.Printf("[Discovery] Discovery failed on host %d: %v / 主机 %d 发现失败: %v",
			hostID, err, hostID, err)
		return nil, ErrDiscoveryFailed
	}

	// Process discovery results / 处理发现结果
	result := &DiscoveryResult{
		Success:  true,
		Message:  "discovery completed / 发现完成",
		Clusters: clusters,
	}

	// Match with existing clusters / 与现有集群匹配
	if s.clusterMatcher != nil {
		for _, cluster := range clusters {
			clusterID, _, found, err := s.clusterMatcher.FindClusterByInstallDir(ctx, hostID, cluster.InstallDir)
			if err != nil {
				log.Printf("[Discovery] Error matching cluster: %v / 匹配集群出错: %v", err, err)
				continue
			}
			if found {
				cluster.IsNew = false
				cluster.ExistingID = clusterID
				result.ExistCount++
			} else {
				cluster.IsNew = true
				result.NewCount++
			}
		}
	} else {
		// Without matcher, all clusters are considered new
		// 没有匹配器时，所有集群都视为新发现
		for _, cluster := range clusters {
			cluster.IsNew = true
		}
		result.NewCount = len(clusters)
	}

	log.Printf("[Discovery] Found %d clusters on host %d: %d new, %d existing / 在主机 %d 上发现 %d 个集群：%d 个新的，%d 个已存在",
		len(clusters), hostID, result.NewCount, result.ExistCount,
		hostID, len(clusters), result.NewCount, result.ExistCount)

	return result, nil
}

// ConfirmDiscovery confirms and imports discovered clusters.
// ConfirmDiscovery 确认并导入发现的集群。
// Requirements: 1.9, 9.4 - Confirm cluster import
func (s *Service) ConfirmDiscovery(ctx context.Context, req *ConfirmDiscoveryRequest) ([]uint, error) {
	if s.hostProvider == nil || s.clusterMatcher == nil {
		return nil, errors.New("service not fully configured / 服务未完全配置")
	}

	// Get host info / 获取主机信息
	host, err := s.hostProvider.GetHostByID(ctx, req.HostID)
	if err != nil {
		return nil, ErrHostNotFound
	}

	// Trigger discovery to get current state / 触发发现获取当前状态
	result, err := s.TriggerDiscovery(ctx, req.HostID)
	if err != nil {
		return nil, err
	}

	var createdIDs []uint
	for _, cluster := range result.Clusters {
		// Check if this cluster should be imported / 检查是否应该导入此集群
		shouldImport := false
		for _, installDir := range req.InstallDirs {
			if cluster.InstallDir == installDir {
				shouldImport = true
				break
			}
		}
		if !shouldImport {
			continue
		}

		// Skip if already exists / 如果已存在则跳过
		if !cluster.IsNew {
			log.Printf("[Discovery] Cluster at %s already exists (ID: %d) / 集群 %s 已存在 (ID: %d)",
				cluster.InstallDir, cluster.ExistingID, cluster.InstallDir, cluster.ExistingID)
			continue
		}

		// Apply user-selected JDK for this install dir (AC1e) / 应用该安装目录用户选定的 JDK（AC1e）
		if req.JavaHomes != nil {
			if home, ok := req.JavaHomes[cluster.InstallDir]; ok {
				cluster.JavaHome = home
			}
		}
		// Create cluster / 创建集群
		clusterID, err := s.clusterMatcher.CreateClusterFromDiscovery(ctx, host.ID, cluster)
		if err != nil {
			log.Printf("[Discovery] Failed to create cluster from %s: %v / 从 %s 创建集群失败: %v",
				cluster.InstallDir, err, cluster.InstallDir, err)
			continue
		}

		createdIDs = append(createdIDs, clusterID)
		log.Printf("[Discovery] Created cluster %d from %s / 从 %s 创建集群 %d",
			clusterID, cluster.InstallDir, cluster.InstallDir, clusterID)
	}

	return createdIDs, nil
}

// ProcessDiscoveryResult processes discovery results from agent.
// ProcessDiscoveryResult 处理来自 Agent 的发现结果。
// Requirements: 2.5 - Process matching and status update
// **Feature: seatunnel-process-monitor, Property 15: 进程匹配更新状态**
// **Validates: Requirements 2.5**
func (s *Service) ProcessDiscoveryResult(ctx context.Context, hostID uint, clusters []*DiscoveredCluster) error {
	if s.clusterMatcher == nil {
		return nil
	}

	for _, cluster := range clusters {
		// Find matching cluster/node / 查找匹配的集群/节点
		_, nodeID, found, err := s.clusterMatcher.FindClusterByInstallDir(ctx, hostID, cluster.InstallDir)
		if err != nil {
			log.Printf("[Discovery] Error finding cluster: %v / 查找集群出错: %v", err, err)
			continue
		}

		if !found {
			continue
		}

		// Update node status from discovered nodes / 从发现的节点更新节点状态
		for _, node := range cluster.Nodes {
			if err := s.clusterMatcher.UpdateNodeFromDiscovery(ctx, nodeID, node); err != nil {
				log.Printf("[Discovery] Error updating node %d: %v / 更新节点 %d 出错: %v",
					nodeID, err, nodeID, err)
			}
		}
	}

	return nil
}
