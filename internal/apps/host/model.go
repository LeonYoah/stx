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

/*
 * @Author: Leon Yoah 1733839298@qq.com
 * @Date: 2025-12-17 17:31:49
 * @LastEditTime: 2026-02-07 17:32:33
 * @FilePath: \STX\internal\apps\host\model.go
 */
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
	"net"
	"net/url"
	"strings"
	"time"
)

// HostType represents the type of host environment.
// HostType 表示主机环境类型。
type HostType string

const (
	// HostTypeBareMetal indicates a physical machine or VM managed by Agent.
	// HostTypeBareMetal 表示由 Agent 管理的物理机或虚拟机。
	HostTypeBareMetal HostType = "bare_metal"
	// HostTypeDocker indicates a Docker host managed via Docker API.
	// HostTypeDocker 表示通过 Docker API 管理的 Docker 主机。
	HostTypeDocker HostType = "docker"
	// HostTypeKubernetes indicates a Kubernetes cluster managed via K8s API.
	// HostTypeKubernetes 表示通过 K8s API 管理的 Kubernetes 集群。
	HostTypeKubernetes HostType = "kubernetes"
)

// ValidHostTypes contains all valid host types.
// ValidHostTypes 包含所有有效的主机类型。
var ValidHostTypes = []HostType{HostTypeBareMetal, HostTypeDocker, HostTypeKubernetes}

// IsValid checks if the host type is valid.
// IsValid 检查主机类型是否有效。
func (t HostType) IsValid() bool {
	for _, valid := range ValidHostTypes {
		if t == valid {
			return true
		}
	}
	return false
}

// HostStatus represents the connection status of a host.
// HostStatus 表示主机的连接状态。
type HostStatus string

const (
	// HostStatusPending indicates the host is waiting for connection.
	// HostStatusPending 表示主机正在等待连接。
	HostStatusPending HostStatus = "pending"
	// HostStatusConnected indicates the host is connected and operational.
	// HostStatusConnected 表示主机已连接并可操作。
	HostStatusConnected HostStatus = "connected"
	// HostStatusOffline indicates the host is offline.
	// HostStatusOffline 表示主机离线。
	HostStatusOffline HostStatus = "offline"
	// HostStatusError indicates the host has an error.
	// HostStatusError 表示主机出现错误。
	HostStatusError HostStatus = "error"
)

// AgentStatus represents the installation status of an Agent on a host.
// AgentStatus 表示主机上 Agent 的安装状态。
type AgentStatus string

const (
	// AgentStatusNotInstalled indicates the Agent has not been installed on the host.
	// AgentStatusNotInstalled 表示 Agent 尚未安装在主机上。
	AgentStatusNotInstalled AgentStatus = "not_installed"
	// AgentStatusInstalled indicates the Agent is installed and connected.
	// AgentStatusInstalled 表示 Agent 已安装并已连接。
	AgentStatusInstalled AgentStatus = "installed"
	// AgentStatusOffline indicates the Agent was installed but is currently offline.
	// AgentStatusOffline 表示 Agent 已安装但当前离线。
	AgentStatusOffline AgentStatus = "offline"
)

// Host represents a physical machine, VM, Docker host, or Kubernetes cluster that runs SeaTunnel services.
// Host 表示运行 SeaTunnel 服务的物理机、虚拟机、Docker 主机或 Kubernetes 集群。
type Host struct {
	ID          uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string     `json:"name" gorm:"size:100;uniqueIndex;not null"`
	HostType    HostType   `json:"host_type" gorm:"size:20;not null;default:bare_metal;index"`
	Description string     `json:"description" gorm:"type:text"`
	Status      HostStatus `json:"status" gorm:"size:20;default:pending;index"`

	// Common resource usage fields / 通用资源使用率字段
	CPUUsage    float64    `json:"cpu_usage" gorm:"type:decimal(5,2)"`
	MemoryUsage float64    `json:"memory_usage" gorm:"type:decimal(5,2)"`
	DiskUsage   float64    `json:"disk_usage" gorm:"type:decimal(5,2)"`
	LastCheck   *time.Time `json:"last_check"`

	// bare_metal specific fields / 物理机/VM 专用字段
	IPAddress     string      `json:"ip_address" gorm:"size:45"`
	SSHPort       int         `json:"ssh_port" gorm:"default:22"`
	AgentID       string      `json:"agent_id" gorm:"size:100;index"`
	AgentStatus   AgentStatus `json:"agent_status" gorm:"size:20;default:not_installed;index"`
	AgentVersion  string      `json:"agent_version" gorm:"size:20"`
	OSType        string      `json:"os_type" gorm:"size:20"`
	Arch          string      `json:"arch" gorm:"size:20"`
	CPUCores      int         `json:"cpu_cores"`
	TotalMemory   int64       `json:"total_memory"`
	TotalDisk     int64       `json:"total_disk"`
	LastHeartbeat *time.Time  `json:"last_heartbeat"`

	// docker specific fields (Phase 2) / Docker 专用字段（第二阶段）
	DockerAPIURL     string `json:"docker_api_url" gorm:"size:255"`
	DockerTLSEnabled bool   `json:"docker_tls_enabled" gorm:"default:false"`
	DockerCertPath   string `json:"docker_cert_path" gorm:"size:255"`
	DockerVersion    string `json:"docker_version" gorm:"size:20"`

	// kubernetes specific fields (Phase 2) / Kubernetes 专用字段（第二阶段）
	K8sAPIURL     string `json:"k8s_api_url" gorm:"size:255"`
	K8sNamespace  string `json:"k8s_namespace" gorm:"size:100;default:default"`
	K8sKubeconfig string `json:"k8s_kubeconfig" gorm:"type:text"`
	K8sToken      string `json:"k8s_token" gorm:"type:text"`
	K8sVersion    string `json:"k8s_version" gorm:"size:20"`

	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	CreatedBy uint      `json:"created_by"`
}

// TableName specifies the table name for the Host model.
func (Host) TableName() string {
	return "hosts"
}

// IsOnline returns true if the host has received a heartbeat within the timeout period.
// The default timeout is 30 seconds as per Requirements 3.4.
// For backward compatibility this uses no "since" cutoff; use IsOnlineWithSince for process-start awareness.
func (h *Host) IsOnline(timeout time.Duration) bool {
	return h.IsOnlineWithSince(timeout, time.Time{})
}

// IsOnlineWithSince returns true if the host has received a heartbeat within the timeout period
// and (when since is non-zero) the last heartbeat is after since (e.g. process start time).
// This avoids showing hosts as online after platform restart before any heartbeat is received.
func (h *Host) IsOnlineWithSince(timeout time.Duration, since time.Time) bool {
	if h.LastHeartbeat == nil {
		return false
	}
	if !since.IsZero() && !h.LastHeartbeat.After(since) {
		return false
	}
	return time.Since(*h.LastHeartbeat) <= timeout
}

// ValidateIPAddress validates that the IP address is a valid IPv4 or IPv6 address.
// Returns true if the IP address is valid, false otherwise.
func ValidateIPAddress(ip string) bool {
	if ip == "" {
		return false
	}
	return net.ParseIP(ip) != nil
}

// HostFilter represents filter criteria for querying hosts.
// HostFilter 表示查询主机的过滤条件。
type HostFilter struct {
	Name        string      `json:"name"`
	HostType    HostType    `json:"host_type"`
	IPAddress   string      `json:"ip_address"`
	Status      HostStatus  `json:"status"`
	AgentStatus AgentStatus `json:"agent_status"`
	IsOnline    *bool       `json:"is_online"`
	Page        int         `json:"page"`
	PageSize    int         `json:"page_size"`
}

// HostInfo represents host information for API responses.
// HostInfo 表示 API 响应的主机信息。
type HostInfo struct {
	ID          uint       `json:"id"`
	Name        string     `json:"name"`
	HostType    HostType   `json:"host_type"`
	Description string     `json:"description"`
	Status      HostStatus `json:"status"`

	// Common fields / 通用字段
	CPUUsage    float64    `json:"cpu_usage"`
	MemoryUsage float64    `json:"memory_usage"`
	DiskUsage   float64    `json:"disk_usage"`
	IsOnline    bool       `json:"is_online"`
	LastCheck   *time.Time `json:"last_check"`

	// bare_metal fields / 物理机字段
	IPAddress     string      `json:"ip_address,omitempty"`
	SSHPort       int         `json:"ssh_port,omitempty"`
	AgentID       string      `json:"agent_id,omitempty"`
	AgentStatus   AgentStatus `json:"agent_status,omitempty"`
	AgentVersion  string      `json:"agent_version,omitempty"`
	OSType        string      `json:"os_type,omitempty"`
	Arch          string      `json:"arch,omitempty"`
	CPUCores      int         `json:"cpu_cores,omitempty"`
	TotalMemory   int64       `json:"total_memory,omitempty"`
	TotalDisk     int64       `json:"total_disk,omitempty"`
	LastHeartbeat *time.Time  `json:"last_heartbeat,omitempty"`

	// docker fields / Docker 字段
	DockerAPIURL     string `json:"docker_api_url,omitempty"`
	DockerTLSEnabled bool   `json:"docker_tls_enabled,omitempty"`
	DockerVersion    string `json:"docker_version,omitempty"`

	// kubernetes fields / Kubernetes 字段
	K8sAPIURL    string `json:"k8s_api_url,omitempty"`
	K8sNamespace string `json:"k8s_namespace,omitempty"`
	K8sVersion   string `json:"k8s_version,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToHostInfo converts a Host to HostInfo with online status calculated.
// ToHostInfo 将 Host 转换为 HostInfo，并计算在线状态。
// since: if non-zero, host is considered online only if last_heartbeat is after since (e.g. process start).
// For bare_metal: when !IsOnline the displayed status is forced to offline for consistency.
func (h *Host) ToHostInfo(heartbeatTimeout time.Duration, since time.Time) *HostInfo {
	info := &HostInfo{
		ID:          h.ID,
		Name:        h.Name,
		HostType:    h.HostType,
		Description: h.Description,
		Status:      h.Status,
		CPUUsage:    h.CPUUsage,
		MemoryUsage: h.MemoryUsage,
		DiskUsage:   h.DiskUsage,
		LastCheck:   h.LastCheck,
		CreatedAt:   h.CreatedAt,
		UpdatedAt:   h.UpdatedAt,
	}

	// Set online status and unified status based on host type
	switch h.HostType {
	case HostTypeBareMetal:
		info.IsOnline = h.IsOnlineWithSince(heartbeatTimeout, since)
		info.IPAddress = h.IPAddress
		info.SSHPort = h.SSHPort
		info.AgentID = h.AgentID
		info.AgentStatus = h.AgentStatus
		info.AgentVersion = h.AgentVersion
		info.OSType = h.OSType
		info.Arch = h.Arch
		info.CPUCores = h.CPUCores
		info.TotalMemory = h.TotalMemory
		info.TotalDisk = h.TotalDisk
		info.LastHeartbeat = h.LastHeartbeat
		// Display status: offline when not online for consistency after platform restart
		if info.IsOnline {
			info.Status = agentStatusToHostStatus(h.AgentStatus)
		} else {
			info.Status = HostStatusOffline
		}
	case HostTypeDocker:
		info.IsOnline = h.Status == HostStatusConnected
		info.DockerAPIURL = h.DockerAPIURL
		info.DockerTLSEnabled = h.DockerTLSEnabled
		info.DockerVersion = h.DockerVersion
	case HostTypeKubernetes:
		info.IsOnline = h.Status == HostStatusConnected
		info.K8sAPIURL = h.K8sAPIURL
		info.K8sNamespace = h.K8sNamespace
		info.K8sVersion = h.K8sVersion
	default:
		info.IsOnline = h.IsOnlineWithSince(heartbeatTimeout, since)
		info.IPAddress = h.IPAddress
		info.SSHPort = h.SSHPort
		info.AgentID = h.AgentID
		info.AgentStatus = h.AgentStatus
		info.AgentVersion = h.AgentVersion
		if info.IsOnline {
			info.Status = agentStatusToHostStatus(h.AgentStatus)
		} else {
			info.Status = HostStatusOffline
		}
	}

	return info
}

// agentStatusToHostStatus maps AgentStatus to HostStatus for unified display.
// agentStatusToHostStatus 将 AgentStatus 映射为 HostStatus 用于统一展示。
func agentStatusToHostStatus(s AgentStatus) HostStatus {
	switch s {
	case AgentStatusNotInstalled:
		return HostStatusPending
	case AgentStatusInstalled:
		return HostStatusConnected
	case AgentStatusOffline:
		return HostStatusOffline
	default:
		return HostStatusPending
	}
}

// CreateHostRequest represents a request to create a new host.
// CreateHostRequest 表示创建新主机的请求。
type CreateHostRequest struct {
	Name        string   `json:"name" binding:"required,max=100"`
	HostType    HostType `json:"host_type"`
	Description string   `json:"description"`

	// bare_metal fields / 物理机字段
	IPAddress string `json:"ip_address"`
	SSHPort   int    `json:"ssh_port"`

	// docker fields / Docker 字段
	DockerAPIURL     string `json:"docker_api_url"`
	DockerTLSEnabled bool   `json:"docker_tls_enabled"`
	DockerCertPath   string `json:"docker_cert_path"`

	// kubernetes fields / Kubernetes 字段
	K8sAPIURL     string `json:"k8s_api_url"`
	K8sNamespace  string `json:"k8s_namespace"`
	K8sKubeconfig string `json:"k8s_kubeconfig"`
	K8sToken      string `json:"k8s_token"`
}

// UpdateHostRequest represents a request to update an existing host.
// UpdateHostRequest 表示更新现有主机的请求。
type UpdateHostRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`

	// bare_metal fields / 物理机字段
	IPAddress *string `json:"ip_address"`
	SSHPort   *int    `json:"ssh_port"`

	// docker fields / Docker 字段
	DockerAPIURL     *string `json:"docker_api_url"`
	DockerTLSEnabled *bool   `json:"docker_tls_enabled"`
	DockerCertPath   *string `json:"docker_cert_path"`

	// kubernetes fields / Kubernetes 字段
	K8sAPIURL     *string `json:"k8s_api_url"`
	K8sNamespace  *string `json:"k8s_namespace"`
	K8sKubeconfig *string `json:"k8s_kubeconfig"`
	K8sToken      *string `json:"k8s_token"`
}

// ValidateDockerAPIURL validates the Docker API URL format.
// ValidateDockerAPIURL 验证 Docker API URL 格式。
// Valid formats: tcp://host:port, unix:///path/to/socket
func ValidateDockerAPIURL(apiURL string) bool {
	if apiURL == "" {
		return false
	}

	// Check for unix socket
	// 检查 Unix socket
	if strings.HasPrefix(apiURL, "unix://") {
		return len(apiURL) > 7 // Must have path after unix://
	}

	// Check for tcp URL
	// 检查 TCP URL
	if strings.HasPrefix(apiURL, "tcp://") {
		u, err := url.Parse(apiURL)
		if err != nil {
			return false
		}
		return u.Host != ""
	}

	return false
}

// ValidateK8sAPIURL validates the Kubernetes API URL format.
// ValidateK8sAPIURL 验证 Kubernetes API URL 格式。
// Valid format: https://host:port
func ValidateK8sAPIURL(apiURL string) bool {
	if apiURL == "" {
		return false
	}

	u, err := url.Parse(apiURL)
	if err != nil {
		return false
	}

	// Must be https scheme
	// 必须是 https 协议
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}

	return u.Host != ""
}
