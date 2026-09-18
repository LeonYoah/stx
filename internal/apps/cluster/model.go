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
package cluster

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// DeploymentMode represents the deployment mode of a SeaTunnel cluster.
type DeploymentMode string

const (
	// DeploymentModeHybrid indicates a hybrid deployment where master and worker run on the same nodes.
	DeploymentModeHybrid DeploymentMode = "hybrid"
	// DeploymentModeSeparated indicates a separated deployment where master and worker run on different nodes.
	DeploymentModeSeparated DeploymentMode = "separated"
)

// ClusterStatus represents the current status of a cluster.
type ClusterStatus string

const (
	// ClusterStatusCreated indicates the cluster has been created but not deployed.
	ClusterStatusCreated ClusterStatus = "created"
	// ClusterStatusDeploying indicates the cluster is being deployed.
	ClusterStatusDeploying ClusterStatus = "deploying"
	// ClusterStatusRunning indicates the cluster is running normally.
	ClusterStatusRunning ClusterStatus = "running"
	// ClusterStatusStopped indicates the cluster has been stopped.
	ClusterStatusStopped ClusterStatus = "stopped"
	// ClusterStatusError indicates the cluster is in an error state.
	ClusterStatusError ClusterStatus = "error"
)

// NodeRole represents the role of a node in a cluster.
type NodeRole string

const (
	// NodeRoleMaster indicates the node is a master node.
	NodeRoleMaster NodeRole = "master"
	// NodeRoleWorker indicates the node is a worker node.
	NodeRoleWorker NodeRole = "worker"
	// NodeRoleMasterWorker indicates the node is both master and worker (hybrid mode).
	// NodeRoleMasterWorker 表示节点同时是 master 和 worker（混合模式）。
	NodeRoleMasterWorker NodeRole = "master/worker"
)

// NodeStatus represents the current status of a cluster node.
type NodeStatus string

const (
	// NodeStatusPending indicates the node is pending deployment.
	NodeStatusPending NodeStatus = "pending"
	// NodeStatusInstalling indicates the node is being installed.
	NodeStatusInstalling NodeStatus = "installing"
	// NodeStatusRunning indicates the node is running normally.
	NodeStatusRunning NodeStatus = "running"
	// NodeStatusStopped indicates the node has been stopped.
	NodeStatusStopped NodeStatus = "stopped"
	// NodeStatusError indicates the node is in an error state.
	NodeStatusError NodeStatus = "error"
	// NodeStatusOffline indicates the host/agent is offline; display-only, not persisted.
	NodeStatusOffline NodeStatus = "offline"
)

// ClusterConfig represents the JSON configuration for a cluster.
type ClusterConfig map[string]interface{}

// Value implements the driver.Valuer interface for database storage.
func (c ClusterConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for database retrieval.
func (c *ClusterConfig) Scan(value interface{}) error {
	if value == nil {
		*c = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("cluster: failed to scan ClusterConfig - expected []byte")
	}
	return json.Unmarshal(bytes, c)
}

// JVMConfig represents cluster-level effective JVM configuration.
// JVMConfig 表示集群级生效后的 JVM 配置。
type JVMConfig struct {
	HybridHeapSize int `json:"hybrid_heap_size,omitempty"`
	MasterHeapSize int `json:"master_heap_size,omitempty"`
	WorkerHeapSize int `json:"worker_heap_size,omitempty"`
}

// NodeJVMOverrides represents node-level JVM override values.
// NodeJVMOverrides 表示节点级 JVM 覆盖值。
type NodeJVMOverrides struct {
	HybridHeapSize *int `json:"hybrid_heap_size,omitempty"`
	MasterHeapSize *int `json:"master_heap_size,omitempty"`
	WorkerHeapSize *int `json:"worker_heap_size,omitempty"`
}

// HasValues returns whether the override contains any explicit field.
// HasValues 返回 override 是否包含任何显式字段。
func (o NodeJVMOverrides) HasValues() bool {
	return o.HybridHeapSize != nil || o.MasterHeapSize != nil || o.WorkerHeapSize != nil
}

// NodeOverrides represents extensible node-level override settings.
// NodeOverrides 表示可扩展的节点级覆盖配置。
type NodeOverrides struct {
	JVM *NodeJVMOverrides `json:"jvm,omitempty"`
}

// Normalize removes empty nested override objects.
// Normalize 会移除空的嵌套 override 对象。
func (o NodeOverrides) Normalize() NodeOverrides {
	if o.JVM != nil && !o.JVM.HasValues() {
		o.JVM = nil
	}
	return o
}

// HasValues returns whether the override contains any effective values.
// HasValues 返回 override 是否包含任何有效值。
func (o NodeOverrides) HasValues() bool {
	return o.Normalize().JVM != nil
}

// Value implements driver.Valuer for database storage.
// Value 实现 driver.Valuer 以便写入数据库。
func (o NodeOverrides) Value() (driver.Value, error) {
	normalized := o.Normalize()
	if !normalized.HasValues() {
		return nil, nil
	}
	return json.Marshal(normalized)
}

// Scan implements sql.Scanner for database retrieval.
// Scan 实现 sql.Scanner 以便从数据库读取。
func (o *NodeOverrides) Scan(value interface{}) error {
	if value == nil {
		*o = NodeOverrides{}
		return nil
	}

	switch v := value.(type) {
	case []byte:
		if len(v) == 0 {
			*o = NodeOverrides{}
			return nil
		}
		return json.Unmarshal(v, o)
	case string:
		if v == "" {
			*o = NodeOverrides{}
			return nil
		}
		return json.Unmarshal([]byte(v), o)
	default:
		return errors.New("cluster: failed to scan NodeOverrides - expected []byte or string")
	}
}

// Cluster represents a SeaTunnel cluster consisting of multiple nodes.
type Cluster struct {
	ID             uint           `json:"id" gorm:"primaryKey;autoIncrement"`
	Name           string         `json:"name" gorm:"size:100;uniqueIndex;not null"`
	Description    string         `json:"description" gorm:"type:text"`
	DeploymentMode DeploymentMode `json:"deployment_mode" gorm:"size:20;not null"`
	Version        string         `json:"version" gorm:"size:20"`
	Status         ClusterStatus  `json:"status" gorm:"size:20;default:created;index"`
	InstallDir     string         `json:"install_dir" gorm:"size:255"`
	Config         ClusterConfig  `json:"config" gorm:"type:json"`
	CreatedAt      time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	CreatedBy      uint           `json:"created_by"`
	Nodes          []ClusterNode  `json:"nodes" gorm:"foreignKey:ClusterID"`
}

// TableName specifies the table name for the Cluster model.
func (Cluster) TableName() string {
	return "clusters"
}

// ClusterNode represents a node within a SeaTunnel cluster.
// 集群节点，每个节点可以有独立的安装目录和端口配置
type ClusterNode struct {
	ID            uint          `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID     uint          `json:"cluster_id" gorm:"index;not null"`
	HostID        uint          `json:"host_id" gorm:"index;not null"`
	Role          NodeRole      `json:"role" gorm:"size:20;not null"`
	InstallDir    string        `json:"install_dir" gorm:"size:255"`           // SeaTunnel installation directory on this node / 此节点上的 SeaTunnel 安装目录
	HazelcastPort int           `json:"hazelcast_port"`                        // Hazelcast cluster port / Hazelcast 集群端口
	APIPort       int           `json:"api_port"`                              // REST API port (Master only) / REST API 端口（仅 Master）
	WorkerPort    int           `json:"worker_port"`                           // Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式）
	Overrides     NodeOverrides `json:"overrides" gorm:"type:json"`            // Node-level JSON overrides / 节点级 JSON 覆盖配置
	Status        NodeStatus    `json:"status" gorm:"size:20;default:pending"` // Unified status field: pending, installing, running, stopped, error / 统一状态字段
	ProcessPID    int           `json:"process_pid" gorm:"column:process_pid"` // SeaTunnel process PID / SeaTunnel 进程 PID
	LastEventAt   *time.Time    `json:"last_event_at"`                         // 最后事件时间 / Last event time
	CreatedAt     time.Time     `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time     `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the table name for the ClusterNode model.
func (ClusterNode) TableName() string {
	return "cluster_nodes"
}

// ClusterFilter represents filter criteria for querying clusters.
type ClusterFilter struct {
	Name           string         `json:"name"`
	Status         ClusterStatus  `json:"status"`
	DeploymentMode DeploymentMode `json:"deployment_mode"`
	Page           int            `json:"page"`
	PageSize       int            `json:"page_size"`
}

// ClusterInfo represents cluster information for API responses.
type ClusterInfo struct {
	ID             uint           `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	DeploymentMode DeploymentMode `json:"deployment_mode"`
	Version        string         `json:"version"`
	Status         ClusterStatus  `json:"status"`
	InstallDir     string         `json:"install_dir"`
	Config         ClusterConfig  `json:"config"`
	NodeCount      int            `json:"node_count"`
	OnlineNodes    int            `json:"online_nodes"`  // number of nodes whose host is online / 主机在线的节点数
	HealthStatus   string         `json:"health_status"` // healthy, unhealthy, unknown / 健康状态
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ToClusterInfo converts a Cluster to ClusterInfo (OnlineNodes and HealthStatus are set by caller).
func (c *Cluster) ToClusterInfo() *ClusterInfo {
	return &ClusterInfo{
		ID:             c.ID,
		Name:           c.Name,
		Description:    c.Description,
		DeploymentMode: c.DeploymentMode,
		Version:        c.Version,
		Status:         c.Status,
		InstallDir:     c.InstallDir,
		Config:         c.Config,
		NodeCount:      len(c.Nodes),
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

// CreateClusterRequest represents a request to create a new cluster.
// CreateClusterRequest 表示创建新集群的请求。
type CreateClusterRequest struct {
	Name           string         `json:"name" binding:"required,max=100"`
	Description    string         `json:"description"`
	DeploymentMode DeploymentMode `json:"deployment_mode" binding:"required"`
	Version        string         `json:"version" binding:"required"`
	InstallDir     string         `json:"install_dir"`
	Config         ClusterConfig  `json:"config"`
	// Nodes to auto-create from discovery (optional)
	// 从发现自动创建的节点（可选）
	Nodes []CreateNodeFromDiscovery `json:"nodes,omitempty"`
}

// CreateNodeFromDiscovery represents a node to be created from process discovery.
// CreateNodeFromDiscovery 表示从进程发现创建的节点。
type CreateNodeFromDiscovery struct {
	HostID        uint   `json:"host_id"`                  // Host ID / 主机 ID
	InstallDir    string `json:"install_dir"`              // SeaTunnel installation directory / SeaTunnel 安装目录
	Role          string `json:"role"`                     // Node role: master, worker, hybrid / 节点角色
	HazelcastPort int    `json:"hazelcast_port,omitempty"` // Hazelcast cluster port (optional) / Hazelcast 集群端口（可选）
	APIPort       int    `json:"api_port,omitempty"`       // REST API port (optional) / REST API 端口（可选）
}

// UpdateClusterRequest represents a request to update an existing cluster.
type UpdateClusterRequest struct {
	Name        *string        `json:"name"`
	Description *string        `json:"description"`
	Version     *string        `json:"version"`
	InstallDir  *string        `json:"install_dir"`
	Config      *ClusterConfig `json:"config"`
}

// AddNodeRequest represents a request to add a node to a cluster.
// 添加节点请求，包含安装目录和端口配置
type AddNodeRequest struct {
	HostID     uint     `json:"host_id" binding:"required"`
	Role       NodeRole `json:"role" binding:"required"`
	InstallDir string   `json:"install_dir"` // SeaTunnel installation directory / SeaTunnel 安装目录

	// Port configuration based on node role / 基于节点角色的端口配置
	// Master: hazelcast_port (default 5801) + api_port (default 8080, optional)
	// Worker: hazelcast_port (default 5802)
	// Hybrid: hazelcast_port (5801) + worker_port (5802)
	HazelcastPort int            `json:"hazelcast_port"`      // Hazelcast cluster port / Hazelcast 集群端口
	APIPort       int            `json:"api_port"`            // REST API port (Master only, optional) / REST API 端口（仅 Master，可选）
	WorkerPort    int            `json:"worker_port"`         // Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式）
	Overrides     *NodeOverrides `json:"overrides,omitempty"` // Node-level JSON overrides / 节点级 JSON 覆盖配置

	// Whether to skip precheck / 是否跳过预检查
	SkipPrecheck bool `json:"skip_precheck"`
}

// AddNodeEntryRequest represents one logical node entry under the same host.
// AddNodeEntryRequest 表示同一主机下的一个逻辑节点条目。
type AddNodeEntryRequest struct {
	Role          NodeRole       `json:"role" binding:"required"`
	InstallDir    string         `json:"install_dir"`
	HazelcastPort int            `json:"hazelcast_port"`
	APIPort       int            `json:"api_port"`
	WorkerPort    int            `json:"worker_port"`
	Overrides     *NodeOverrides `json:"overrides,omitempty"`
}

// AddNodesRequest represents a same-host multi-entry add-node request.
// AddNodesRequest 表示同一主机多条目加节点请求。
type AddNodesRequest struct {
	HostID       uint                  `json:"host_id" binding:"required"`
	InstallDir   string                `json:"install_dir"`
	Entries      []AddNodeEntryRequest `json:"entries" binding:"required"`
	SkipPrecheck bool                  `json:"skip_precheck"`
}

// UpdateNodeRequest represents a request to update a node in a cluster.
// 更新节点请求，包含安装目录和端口配置
type UpdateNodeRequest struct {
	InstallDir    *string        `json:"install_dir"`         // SeaTunnel installation directory / SeaTunnel 安装目录
	HazelcastPort *int           `json:"hazelcast_port"`      // Hazelcast cluster port / Hazelcast 集群端口
	APIPort       *int           `json:"api_port"`            // REST API port (Master only, optional) / REST API 端口（仅 Master，可选）
	WorkerPort    *int           `json:"worker_port"`         // Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式）
	Overrides     *NodeOverrides `json:"overrides,omitempty"` // Node-level JSON overrides / 节点级 JSON 覆盖配置
}

// NodeInfo represents node information for API responses.
// 节点信息，用于 API 响应
type NodeInfo struct {
	ID            uint          `json:"id"`
	ClusterID     uint          `json:"cluster_id"`
	HostID        uint          `json:"host_id"`
	HostName      string        `json:"host_name"`
	HostIP        string        `json:"host_ip"`
	Role          NodeRole      `json:"role"`
	InstallDir    string        `json:"install_dir"`    // SeaTunnel installation directory / SeaTunnel 安装目录
	HazelcastPort int           `json:"hazelcast_port"` // Hazelcast cluster port / Hazelcast 集群端口
	APIPort       int           `json:"api_port"`       // REST API port (Master only) / REST API 端口（仅 Master）
	WorkerPort    int           `json:"worker_port"`    // Worker hazelcast port (Hybrid only) / Worker Hazelcast 端口（仅混合模式）
	Overrides     NodeOverrides `json:"overrides"`      // Node-level JSON overrides / 节点级 JSON 覆盖配置
	Status        NodeStatus    `json:"status"`         // Unified status: pending, installing, running, stopped, error, offline / 统一状态
	IsOnline      bool          `json:"is_online"`      // Whether host is online; when false, status may be shown as offline / 主机是否在线
	ProcessPID    int           `json:"process_pid"`    // SeaTunnel process PID / SeaTunnel 进程 PID
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// DefaultPorts defines default port values for different node roles
// DefaultPorts 定义不同节点角色的默认端口值
var DefaultPorts = struct {
	MasterHazelcast int // Master hazelcast port / Master Hazelcast 端口
	MasterAPI       int // Master REST API port / Master REST API 端口
	WorkerHazelcast int // Worker hazelcast port / Worker Hazelcast 端口
}{
	MasterHazelcast: 5801,
	MasterAPI:       8080,
	WorkerHazelcast: 5802,
}

// GetDefaultPorts returns default ports based on node role and deployment mode
// GetDefaultPorts 根据节点角色和部署模式返回默认端口
func GetDefaultPorts(role NodeRole, deploymentMode DeploymentMode) (hazelcastPort, apiPort, workerPort int) {
	switch role {
	case NodeRoleMaster:
		hazelcastPort = DefaultPorts.MasterHazelcast
		apiPort = DefaultPorts.MasterAPI
		if deploymentMode == DeploymentModeHybrid {
			workerPort = DefaultPorts.WorkerHazelcast
		}
	case NodeRoleWorker:
		hazelcastPort = DefaultPorts.WorkerHazelcast
	case NodeRoleMasterWorker:
		hazelcastPort = DefaultPorts.MasterHazelcast
		apiPort = DefaultPorts.MasterAPI
		workerPort = DefaultPorts.WorkerHazelcast
	}
	return
}

// PrecheckRequest represents a request to precheck a node before adding.
// PrecheckRequest 表示添加节点前的预检查请求。
type PrecheckRequest struct {
	HostID        uint     `json:"host_id" binding:"required"`
	Role          NodeRole `json:"role" binding:"required"`
	InstallDir    string   `json:"install_dir"`
	HazelcastPort int      `json:"hazelcast_port" binding:"required"`
	APIPort       int      `json:"api_port"`
	WorkerPort    int      `json:"worker_port"`
}

// PrecheckResult represents the result of a node precheck.
// PrecheckResult 表示节点预检查的结果。
type PrecheckResult struct {
	Success bool                 `json:"success"`
	Message string               `json:"message"`
	Checks  []*PrecheckCheckItem `json:"checks"`
}

// PrecheckCheckItem represents a single check item in precheck.
// PrecheckCheckItem 表示预检查中的单个检查项。
type PrecheckCheckItem struct {
	Name    string `json:"name"`    // Check name / 检查名称
	Status  string `json:"status"`  // passed, failed, skipped / 通过、失败、跳过
	Message string `json:"message"` // Detail message / 详细信息
}

// PrecheckStatus constants
// 预检查状态常量
const (
	PrecheckStatusPassed  = "passed"
	PrecheckStatusFailed  = "failed"
	PrecheckStatusSkipped = "skipped"
)
