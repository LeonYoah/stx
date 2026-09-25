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

package dashboard

import (
	"context"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/host"
)

// OverviewService provides dashboard overview statistics.
type OverviewService struct {
	hostRepo         *host.Repository
	clusterRepo      *cluster.Repository
	auditRepo        *audit.Repository
	heartbeatTimeout time.Duration
	processStartedAt time.Time // online requires heartbeat after this (e.g. API process start)
}

// NewOverviewService creates a new dashboard overview service.
// processStartedAt is used so hosts are not shown online until at least one heartbeat is received after platform start.
func NewOverviewService(hostRepo *host.Repository, clusterRepo *cluster.Repository, auditRepo *audit.Repository, heartbeatTimeout time.Duration, processStartedAt time.Time) *OverviewService {
	return &OverviewService{
		hostRepo:         hostRepo,
		clusterRepo:      clusterRepo,
		auditRepo:        auditRepo,
		heartbeatTimeout: heartbeatTimeout,
		processStartedAt: processStartedAt,
	}
}

// GetOverviewStats returns dashboard overview statistics.
func (s *OverviewService) GetOverviewStats(ctx context.Context) (*OverviewStats, error) {
	stats := &OverviewStats{}

	hosts, _, err := s.hostRepo.List(ctx, &host.HostFilter{PageSize: 1000}, s.heartbeatTimeout, s.processStartedAt)
	if err != nil {
		return nil, err
	}
	stats.TotalHosts = len(hosts)
	for _, h := range hosts {
		if h.IsOnlineWithSince(s.heartbeatTimeout, s.processStartedAt) {
			stats.OnlineHosts++
		}
		if h.AgentStatus == host.AgentStatusInstalled {
			stats.TotalAgents++
			if h.IsOnlineWithSince(s.heartbeatTimeout, s.processStartedAt) {
				stats.OnlineAgents++
			}
		}
	}

	clusters, _, err := s.clusterRepo.List(ctx, &cluster.ClusterFilter{PageSize: 1000})
	if err != nil {
		return nil, err
	}
	stats.TotalClusters = len(clusters)

	// Count running clusters/nodes only when host is online (consistent with host/agent offline)
	for _, c := range clusters {
		clusterWithNodes, err := s.clusterRepo.GetByID(ctx, c.ID, true)
		if err != nil {
			continue
		}
		stats.TotalNodes += len(clusterWithNodes.Nodes)
		clusterHasOnlineNode := false
		for _, n := range clusterWithNodes.Nodes {
			h, err := s.hostRepo.GetByID(ctx, n.HostID)
			online := err == nil && h.IsOnlineWithSince(s.heartbeatTimeout, s.processStartedAt)
			if online {
				clusterHasOnlineNode = true
			}
			switch n.Status {
			case cluster.NodeStatusRunning:
				if online {
					stats.RunningNodes++
				}
				// stopped/error count unchanged (by DB status only)
			case cluster.NodeStatusStopped:
				stats.StoppedNodes++
			case cluster.NodeStatusError:
				stats.ErrorNodes++
			}
		}
		switch c.Status {
		case cluster.ClusterStatusRunning:
			if clusterHasOnlineNode {
				stats.RunningClusters++
			}
		case cluster.ClusterStatusStopped:
			stats.StoppedClusters++
		case cluster.ClusterStatusError:
			stats.ErrorClusters++
		}
	}

	return stats, nil
}

// GetClusterSummaries returns cluster summaries for dashboard.
func (s *OverviewService) GetClusterSummaries(ctx context.Context, limit int) ([]*ClusterSummary, error) {
	if limit <= 0 {
		limit = 5
	}

	clusters, _, err := s.clusterRepo.List(ctx, &cluster.ClusterFilter{PageSize: limit})
	if err != nil {
		return nil, err
	}

	summaries := make([]*ClusterSummary, 0, len(clusters))
	for _, c := range clusters {
		clusterWithNodes, err := s.clusterRepo.GetByID(ctx, c.ID, true)
		if err != nil {
			continue
		}

		summary := &ClusterSummary{
			ID:             c.ID,
			Name:           c.Name,
			Status:         string(c.Status),
			DeploymentMode: string(c.DeploymentMode),
			TotalNodes:     len(clusterWithNodes.Nodes),
		}

		for _, n := range clusterWithNodes.Nodes {
			if n.Role == cluster.NodeRoleMaster {
				summary.MasterNodes++
			} else {
				summary.WorkerNodes++
			}
			h, err := s.hostRepo.GetByID(ctx, n.HostID)
			online := err == nil && h.IsOnlineWithSince(s.heartbeatTimeout, s.processStartedAt)
			if online {
				summary.OnlineNodes++
			}
			if n.Status == cluster.NodeStatusRunning && online {
				summary.RunningNodes++
			}
		}
		// When DB status is running but 0 nodes online, show as unhealthy on dashboard
		if c.Status == cluster.ClusterStatusRunning && summary.OnlineNodes == 0 {
			summary.Status = "unhealthy"
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetHostSummaries returns host summaries for dashboard.
func (s *OverviewService) GetHostSummaries(ctx context.Context, limit int) ([]*HostSummary, error) {
	if limit <= 0 {
		limit = 5
	}

	hosts, _, err := s.hostRepo.List(ctx, &host.HostFilter{PageSize: limit}, s.heartbeatTimeout, s.processStartedAt)
	if err != nil {
		return nil, err
	}

	clusters, _, _ := s.clusterRepo.List(ctx, &cluster.ClusterFilter{PageSize: 1000})
	hostNodeCount := make(map[uint]int)
	for _, c := range clusters {
		clusterWithNodes, err := s.clusterRepo.GetByID(ctx, c.ID, true)
		if err != nil {
			continue
		}
		for _, n := range clusterWithNodes.Nodes {
			hostNodeCount[n.HostID]++
		}
	}

	summaries := make([]*HostSummary, 0, len(hosts))
	for _, h := range hosts {
		summaries = append(summaries, &HostSummary{
			ID:          h.ID,
			Name:        h.Name,
			IPAddress:   h.IPAddress,
			IsOnline:    h.IsOnlineWithSince(s.heartbeatTimeout, s.processStartedAt),
			AgentStatus: string(h.AgentStatus),
			NodeCount:   hostNodeCount[h.ID],
		})
	}

	return summaries, nil
}

// GetRecentActivities returns recent audit log activities.
func (s *OverviewService) GetRecentActivities(ctx context.Context, limit int) ([]*RecentActivity, error) {
	if limit <= 0 {
		limit = 10
	}

	if s.auditRepo == nil {
		return []*RecentActivity{}, nil
	}

	logs, _, err := s.auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{PageSize: limit})
	if err != nil {
		return nil, err
	}

	activities := make([]*RecentActivity, 0, len(logs))
	for _, log := range logs {
		activityType := "info"
		if log.Action == "create" || log.Action == "start" {
			activityType = "success"
		} else if log.Action == "delete" || log.Action == "stop" {
			activityType = "warning"
		}

		// 展示操作人 + 来源 + 操作名，不拼资源名以免噪音
		// Prefer operator + source + action; skip resource name to reduce noise
		operator := strings.TrimSpace(log.Username)
		if operator == "" {
			operator = "system"
		}
		source := strings.TrimSpace(log.ClientType)
		if source == "" {
			source = "system"
		}
		action := strings.TrimSpace(log.Action)
		if action == "" {
			action = "-"
		}

		activities = append(activities, &RecentActivity{
			ID:        log.ID,
			Type:      activityType,
			Operator:  operator,
			Source:    source,
			Action:    action,
			Message:   operator + " · " + source + " · " + action,
			Timestamp: log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return activities, nil
}

// GetOverviewData returns complete dashboard overview data.
func (s *OverviewService) GetOverviewData(ctx context.Context) (*OverviewData, error) {
	stats, err := s.GetOverviewStats(ctx)
	if err != nil {
		return nil, err
	}

	clusterSummaries, err := s.GetClusterSummaries(ctx, 5)
	if err != nil {
		return nil, err
	}

	hostSummaries, err := s.GetHostSummaries(ctx, 5)
	if err != nil {
		return nil, err
	}

	recentActivities, err := s.GetRecentActivities(ctx, 10)
	if err != nil {
		return nil, err
	}

	return &OverviewData{
		Stats:            stats,
		ClusterSummaries: clusterSummaries,
		HostSummaries:    hostSummaries,
		RecentActivities: recentActivities,
	}, nil
}
