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

// Package dashboard provides dashboard statistics for the STX system.
// dashboard 包提供 STX 系统的仪表盘统计功能。
package dashboard

// DashboardDataResponse is the common API envelope used by dashboard handlers.
// DashboardDataResponse 是 dashboard handler 统一使用的响应结构。
type DashboardDataResponse struct {
	ErrorMsg string      `json:"error_msg"`
	Data     interface{} `json:"data"`
}

// OverviewStats represents the dashboard overview statistics.
// OverviewStats 表示仪表盘概览统计数据。
type OverviewStats struct {
	// Host statistics / 主机统计
	TotalHosts  int `json:"total_hosts"`
	OnlineHosts int `json:"online_hosts"`

	// Cluster statistics / 集群统计
	TotalClusters   int `json:"total_clusters"`
	RunningClusters int `json:"running_clusters"`
	StoppedClusters int `json:"stopped_clusters"`
	ErrorClusters   int `json:"error_clusters"`

	// Node statistics / 节点统计
	TotalNodes   int `json:"total_nodes"`
	RunningNodes int `json:"running_nodes"`
	StoppedNodes int `json:"stopped_nodes"`
	ErrorNodes   int `json:"error_nodes"`

	// Agent statistics / Agent 统计
	TotalAgents  int `json:"total_agents"`
	OnlineAgents int `json:"online_agents"`
}

// ClusterSummary represents a cluster summary for dashboard.
// ClusterSummary 表示仪表盘的集群摘要。
type ClusterSummary struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"` // DB status; display as "unhealthy" when running but 0 online
	DeploymentMode string `json:"deployment_mode"`
	TotalNodes     int    `json:"total_nodes"`
	MasterNodes    int    `json:"master_nodes"`
	WorkerNodes    int    `json:"worker_nodes"`
	RunningNodes   int    `json:"running_nodes"` // nodes with status running AND host online
	OnlineNodes    int    `json:"online_nodes"`  // number of nodes whose host is online
}

// HostSummary represents a host summary for dashboard.
// HostSummary 表示仪表盘的主机摘要。
type HostSummary struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	IPAddress   string `json:"ip_address"`
	IsOnline    bool   `json:"is_online"`
	AgentStatus string `json:"agent_status"`
	NodeCount   int    `json:"node_count"`
}

// RecentActivity represents a recent activity log entry.
// RecentActivity 表示最近活动日志条目。
type RecentActivity struct {
	ID        uint   `json:"id"`
	Type      string `json:"type"` // success, warning, info, error
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// OverviewData represents the complete dashboard overview data.
// OverviewData 表示完整的仪表盘概览数据。
type OverviewData struct {
	Stats            *OverviewStats    `json:"stats"`
	ClusterSummaries []*ClusterSummary `json:"cluster_summaries"`
	HostSummaries    []*HostSummary    `json:"host_summaries"`
	RecentActivities []*RecentActivity `json:"recent_activities"`
}
