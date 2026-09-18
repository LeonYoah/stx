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

package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/config"
	"gorm.io/gorm"
)

const (
	defaultNodeHealthEvaluationInterval = 5 * time.Second
	minNodeRuntimeOfflineGraceWindow    = 20 * time.Second
)

// Service provides monitoring center data for UI.
// Service 为 UI 提供监控中心数据。
type Service struct {
	clusterService *cluster.Service
	monitorService *monitor.Service
	repo           *Repository

	nodeHealthEvaluatorStartedAt time.Time
	nodeHealthStartupSuppression time.Duration
	nodeHealthOfflineObservedAt  map[uint]time.Time
	nodeHealthStateMu            sync.Mutex
}

// NewService creates a monitoring service.
// NewService 创建监控中心服务。
func NewService(clusterService *cluster.Service, monitorService *monitor.Service, repo *Repository) *Service {
	return &Service{
		clusterService:               clusterService,
		monitorService:               monitorService,
		repo:                         repo,
		nodeHealthStartupSuppression: defaultNodeHealthStartupSuppressionWindow(),
		nodeHealthOfflineObservedAt:  make(map[uint]time.Time),
	}
}

// GetOverview returns global monitoring overview data.
// GetOverview 返回全局监控总览数据。
func (s *Service) GetOverview(ctx context.Context) (*OverviewData, error) {
	now := time.Now().UTC()
	since24h := now.Add(-24 * time.Hour)
	since1h := now.Add(-1 * time.Hour)

	clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
	}

	result := &OverviewData{
		GeneratedAt: now,
		Stats: &OverviewStats{
			TotalClusters: len(clusters),
		},
		EventStats24h: &EventStats{},
		Clusters:      make([]*ClusterMonitoringSummary, 0, len(clusters)),
	}

	for _, c := range clusters {
		status, err := s.clusterService.GetStatus(ctx, c.ID)
		if err != nil {
			continue
		}

		eventStats24h, err := s.monitorService.GetEventStats(ctx, c.ID, &since24h)
		if err != nil {
			eventStats24h = map[monitor.ProcessEventType]int64{}
		}
		eventStats1h, err := s.monitorService.GetEventStats(ctx, c.ID, &since1h)
		if err != nil {
			eventStats1h = map[monitor.ProcessEventType]int64{}
		}

		e24 := toEventStats(eventStats24h)
		e1 := toEventStats(eventStats1h)

		var lastEventAt *time.Time
		recentEvents, err := s.monitorService.ListClusterEvents(ctx, c.ID, 1)
		if err == nil && len(recentEvents) > 0 {
			t := recentEvents[0].CreatedAt
			lastEventAt = &t
		}

		summary := &ClusterMonitoringSummary{
			ClusterID:              c.ID,
			ClusterName:            c.Name,
			Status:                 string(status.Status),
			HealthStatus:           string(status.HealthStatus),
			TotalNodes:             status.TotalNodes,
			OnlineNodes:            status.OnlineNodes,
			OfflineNodes:           status.OfflineNodes,
			CrashedEvents24h:       e24.Crashed,
			RestartFailedEvents24h: e24.RestartFailed + e24.RestartLimitReached,
			ActiveAlerts1h:         e1.CriticalCount(),
			LastEventAt:            lastEventAt,
		}
		result.Clusters = append(result.Clusters, summary)

		// Aggregate global stats / 聚合全局统计
		result.Stats.TotalNodes += status.TotalNodes
		result.Stats.OnlineNodes += status.OnlineNodes
		result.Stats.OfflineNodes += status.OfflineNodes

		switch status.HealthStatus {
		case cluster.HealthStatusHealthy:
			result.Stats.HealthyClusters++
		case cluster.HealthStatusUnhealthy:
			result.Stats.UnhealthyClusters++
		default:
			result.Stats.UnknownClusters++
		}

		result.Stats.CrashedEvents24h += e24.Crashed
		result.Stats.RestartFailed24h += e24.RestartFailed + e24.RestartLimitReached
		result.Stats.ActiveAlerts1h += e1.CriticalCount()

		mergeEventStats(result.EventStats24h, e24)
	}

	// Sort: unhealthy first, then active alerts, then latest events
	// 排序：异常集群优先，其次活跃告警更多，再按最近事件倒序
	sort.Slice(result.Clusters, func(i, j int) bool {
		left := result.Clusters[i]
		right := result.Clusters[j]

		leftUnhealthy := left.HealthStatus == string(cluster.HealthStatusUnhealthy)
		rightUnhealthy := right.HealthStatus == string(cluster.HealthStatusUnhealthy)
		if leftUnhealthy != rightUnhealthy {
			return leftUnhealthy
		}
		if left.ActiveAlerts1h != right.ActiveAlerts1h {
			return left.ActiveAlerts1h > right.ActiveAlerts1h
		}
		if left.LastEventAt == nil {
			return false
		}
		if right.LastEventAt == nil {
			return true
		}
		return left.LastEventAt.After(*right.LastEventAt)
	})

	return result, nil
}

// GetClusterOverview returns monitoring detail data for one cluster.
// GetClusterOverview 返回单集群监控详情数据。
func (s *Service) GetClusterOverview(ctx context.Context, clusterID uint) (*ClusterOverviewData, error) {
	now := time.Now().UTC()
	since24h := now.Add(-24 * time.Hour)
	since1h := now.Add(-1 * time.Hour)

	status, err := s.clusterService.GetStatus(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	config, err := s.monitorService.GetOrCreateConfig(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	eventStats24hRaw, err := s.monitorService.GetEventStats(ctx, clusterID, &since24h)
	if err != nil {
		return nil, err
	}
	eventStats1hRaw, err := s.monitorService.GetEventStats(ctx, clusterID, &since1h)
	if err != nil {
		return nil, err
	}

	e24 := toEventStats(eventStats24hRaw)
	e1 := toEventStats(eventStats1hRaw)
	activeAlertCount, err := s.countFiringAlertsForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	events, _, err := s.monitorService.ListEvents(ctx, &monitor.ProcessEventFilter{
		ClusterID: clusterID,
		Page:      1,
		PageSize:  50,
	})
	if err != nil {
		return nil, err
	}

	nodes := make([]*NodeSnapshot, 0, len(status.Nodes))
	for _, node := range status.Nodes {
		nodes = append(nodes, &NodeSnapshot{
			NodeID:     node.NodeID,
			HostID:     node.HostID,
			HostName:   node.HostName,
			HostIP:     node.HostIP,
			Role:       string(node.Role),
			Status:     string(node.Status),
			IsOnline:   node.IsOnline,
			ProcessPID: node.ProcessPID,
		})
	}

	return &ClusterOverviewData{
		GeneratedAt: now,
		Cluster: &ClusterBaseInfo{
			ClusterID:    status.ClusterID,
			ClusterName:  status.ClusterName,
			Status:       string(status.Status),
			HealthStatus: string(status.HealthStatus),
		},
		Stats: &ClusterDetailStats{
			TotalNodes:             status.TotalNodes,
			OnlineNodes:            status.OnlineNodes,
			OfflineNodes:           status.OfflineNodes,
			CrashedEvents24h:       e24.Crashed,
			RestartFailedEvents24h: e24.RestartFailed + e24.RestartLimitReached,
			ActiveAlerts1h:         activeAlertCount,
		},
		EventStats24h: e24,
		EventStats1h:  e1,
		MonitorConfig: config,
		Nodes:         nodes,
		RecentEvents:  events,
	}, nil
}

func (s *Service) countFiringAlertsForCluster(ctx context.Context, clusterID uint) (int64, error) {
	alerts, err := s.ListAlertInstances(ctx, &AlertInstanceFilter{
		ClusterID: strconv.FormatUint(uint64(clusterID), 10),
		Status:    AlertDisplayStatusFiring,
		Page:      1,
		PageSize:  1,
	})
	if err != nil {
		return 0, err
	}
	if alerts == nil {
		return 0, nil
	}
	return alerts.Stats.Firing, nil
}

// ListAlerts returns alert center data.
// ListAlerts 返回告警中心数据。
func (s *Service) ListAlerts(ctx context.Context, filter *AlertFilter) (*AlertListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if filter == nil {
		filter = &AlertFilter{}
	}
	if filter.Status != "" && filter.Status != AlertStatusFiring && filter.Status != AlertStatusAcknowledged && filter.Status != AlertStatusSilenced {
		return nil, fmt.Errorf("invalid alert status")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	rows, total, err := s.repo.ListCriticalEvents(ctx, &AlertEventQueryFilter{
		ClusterID: filter.ClusterID,
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
		Page:      filter.Page,
		PageSize:  filter.PageSize,
	})
	if err != nil {
		return nil, err
	}

	eventIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		eventIDs = append(eventIDs, row.ID)
	}
	stateMap, err := s.repo.ListEventStatesByEventIDs(ctx, eventIDs)
	if err != nil {
		return nil, err
	}

	result := &AlertListData{
		GeneratedAt: time.Now().UTC(),
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Total:       total,
		Stats:       &AlertStats{},
		Alerts:      make([]*AlertEvent, 0, len(rows)),
	}

	ruleCache := make(map[uint]map[string]*AlertRule)
	for _, row := range rows {
		rules, err := s.getClusterRuleMap(ctx, row.ClusterID, ruleCache)
		if err != nil {
			return nil, err
		}

		ruleKey := eventTypeToRuleKey(row.EventType)
		rule := rules[ruleKey]
		if rule == nil {
			rule = defaultRuleByKey(row.ClusterID, ruleKey)
		}
		if rule == nil || !rule.Enabled {
			continue
		}

		state := stateMap[row.ID]
		status := resolveAlertStatus(state, result.GeneratedAt)
		if filter.Status != "" && status != filter.Status {
			continue
		}

		alert := &AlertEvent{
			AlertID:          fmt.Sprintf("event-%d", row.ID),
			EventID:          row.ID,
			ClusterID:        row.ClusterID,
			ClusterName:      row.ClusterName,
			NodeID:           row.NodeID,
			HostID:           row.HostID,
			Hostname:         row.Hostname,
			IP:               row.IP,
			EventType:        row.EventType,
			Severity:         rule.Severity,
			Status:           status,
			RuleKey:          rule.RuleKey,
			RuleName:         rule.RuleName,
			ProcessName:      row.ProcessName,
			PID:              row.PID,
			Role:             row.Role,
			Details:          row.Details,
			CreatedAt:        row.CreatedAt,
			LatestActionNote: "",
		}
		if state != nil {
			alert.AcknowledgedBy = state.AcknowledgedBy
			alert.AcknowledgedAt = state.AcknowledgedAt
			alert.SilencedBy = state.SilencedBy
			alert.SilencedUntil = state.SilencedUntil
			alert.LatestActionNote = state.Note
		}

		result.Alerts = append(result.Alerts, alert)
		switch status {
		case AlertStatusAcknowledged:
			result.Stats.Acknowledged++
		case AlertStatusSilenced:
			result.Stats.Silenced++
		default:
			result.Stats.Firing++
		}
	}

	// 当前实现中，状态过滤在服务层执行，因此 total 以返回结果为准。
	// Status filtering is applied in service layer, so total follows returned rows.
	if filter.Status != "" {
		result.Total = int64(len(result.Alerts))
	}

	return result, nil
}

// AcknowledgeAlert marks one alert event as acknowledged.
// AcknowledgeAlert 将单条告警标记为已确认。
func (s *Service) AcknowledgeAlert(ctx context.Context, eventID uint, operator, note string) (*AlertActionResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if eventID == 0 {
		return nil, fmt.Errorf("invalid event id")
	}
	if strings.TrimSpace(operator) == "" {
		operator = "unknown"
	}

	event, err := s.monitorService.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if !isAlertableEventType(event.EventType) {
		return nil, fmt.Errorf("event is not alertable")
	}

	_, state, err := s.persistLocalAlertHandlingState(
		ctx,
		event,
		AlertHandlingStatusAcknowledged,
		operator,
		nil,
		note,
	)
	if err != nil {
		return nil, err
	}
	return toAlertActionResult(state), nil
}

// SilenceAlert silences one alert event for a duration in minutes.
// SilenceAlert 将单条告警静默一段分钟数。
func (s *Service) SilenceAlert(ctx context.Context, eventID uint, operator string, durationMinutes int, note string) (*AlertActionResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if eventID == 0 {
		return nil, fmt.Errorf("invalid event id")
	}
	if durationMinutes < 1 || durationMinutes > 7*24*60 {
		return nil, fmt.Errorf("duration_minutes must be between 1 and 10080")
	}
	if strings.TrimSpace(operator) == "" {
		operator = "unknown"
	}

	event, err := s.monitorService.GetEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if !isAlertableEventType(event.EventType) {
		return nil, fmt.Errorf("event is not alertable")
	}

	now := time.Now().UTC()
	silencedUntil := now.Add(time.Duration(durationMinutes) * time.Minute)
	_, state, err := s.persistLocalAlertHandlingState(
		ctx,
		event,
		AlertHandlingStatusSilenced,
		operator,
		&silencedUntil,
		note,
	)
	if err != nil {
		return nil, err
	}
	return toAlertActionResult(state), nil
}

// ListClusterRules returns rules for one cluster.
// ListClusterRules 返回单集群告警规则列表。
func (s *Service) ListClusterRules(ctx context.Context, clusterID uint) (*AlertRuleListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if clusterID == 0 {
		return nil, fmt.Errorf("invalid cluster id")
	}
	if _, err := s.clusterService.Get(ctx, clusterID); err != nil {
		return nil, err
	}

	if err := s.ensureDefaultRules(ctx, clusterID); err != nil {
		return nil, err
	}

	rules, err := s.repo.ListRulesByClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	items := make([]*AlertRuleDTO, 0, len(rules))
	for _, rule := range rules {
		items = append(items, toAlertRuleDTO(rule))
	}

	return &AlertRuleListData{
		GeneratedAt: time.Now().UTC(),
		ClusterID:   clusterID,
		Rules:       items,
	}, nil
}

// UpdateClusterRule updates one rule in cluster scope.
// UpdateClusterRule 更新集群维度的一条告警规则。
func (s *Service) UpdateClusterRule(ctx context.Context, clusterID, ruleID uint, req *UpdateAlertRuleRequest) (*AlertRuleDTO, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if clusterID == 0 || ruleID == 0 {
		return nil, fmt.Errorf("invalid cluster id or rule id")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.clusterService.Get(ctx, clusterID); err != nil {
		return nil, err
	}

	rule, err := s.repo.GetRuleByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}
	if rule.ClusterID != clusterID {
		return nil, fmt.Errorf("rule does not belong to cluster")
	}

	if req.RuleName != nil {
		rule.RuleName = strings.TrimSpace(*req.RuleName)
	}
	if req.Description != nil {
		rule.Description = strings.TrimSpace(*req.Description)
	}
	if req.Severity != nil {
		rule.Severity = *req.Severity
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.Threshold != nil {
		rule.Threshold = *req.Threshold
	}
	if req.WindowSeconds != nil {
		rule.WindowSeconds = *req.WindowSeconds
	}

	if err := s.repo.SaveRule(ctx, rule); err != nil {
		return nil, err
	}

	return toAlertRuleDTO(rule), nil
}

// GetIntegrationStatus returns integration health status of monitoring stack.
// GetIntegrationStatus 返回监控栈集成状态。
func (s *Service) GetIntegrationStatus(ctx context.Context) (*IntegrationStatusData, error) {
	obsCfg := config.Config.Observability

	components := []*IntegrationComponentStatus{
		{Name: "prometheus", URL: joinURL(obsCfg.Prometheus.URL, "/-/healthy")},
		{Name: "alertmanager", URL: joinURL(obsCfg.Alertmanager.URL, "/-/healthy")},
		{Name: "grafana", URL: joinURL(obsCfg.Grafana.URL, "/api/health")},
	}

	if !obsCfg.Enabled {
		for _, component := range components {
			component.Healthy = false
			component.Error = "disabled by config: observability.enabled=false"
		}

		metricsURL := obsCfg.SeatunnelMetric.Path
		if !strings.HasPrefix(metricsURL, "/") {
			metricsURL = "/" + metricsURL
		}
		components = append(components, &IntegrationComponentStatus{
			Name:    "seatunnel_metrics",
			URL:     metricsURL,
			Healthy: false,
			Error:   "disabled by config: observability.enabled=false",
		})

		return &IntegrationStatusData{
			GeneratedAt: time.Now().UTC(),
			Components:  components,
		}, nil
	}

	for _, component := range components {
		component.Healthy, component.StatusCode, component.Error = checkHTTPComponent(ctx, component.URL, 3*time.Second)
	}

	targets, err := s.collectManagedMetricsTargets(ctx, true)
	metricsComponent := &IntegrationComponentStatus{
		Name: "seatunnel_metrics",
		URL:  joinURL(obsCfg.Prometheus.URL, "/targets"),
	}
	if err != nil {
		metricsComponent.Healthy = false
		metricsComponent.Error = err.Error()
	} else {
		total := len(targets)
		healthy := 0
		for _, target := range targets {
			if metricsComponent.URL == "" {
				metricsComponent.URL = target.ProbeURL
				metricsComponent.StatusCode = target.StatusCode
			}
			if target.Healthy {
				healthy++
				if metricsComponent.StatusCode == 0 {
					metricsComponent.StatusCode = target.StatusCode
				}
				if metricsComponent.URL == "" {
					metricsComponent.URL = target.ProbeURL
				}
			}
		}

		switch {
		case total == 0:
			metricsComponent.Healthy = false
			metricsComponent.Error = "no managed cluster node exposes api_port for metrics probing"
			if metricsComponent.URL == "" {
				metricsComponent.URL = obsCfg.SeatunnelMetric.Path
			}
		case healthy == 0:
			metricsComponent.Healthy = false
			metricsComponent.Error = fmt.Sprintf("metrics endpoint unreachable or metrics disabled on %d/%d targets", total, total)
		case healthy < total:
			metricsComponent.Healthy = true
			metricsComponent.Error = fmt.Sprintf("partial healthy: %d/%d targets expose metrics", healthy, total)
		default:
			metricsComponent.Healthy = true
		}
	}
	components = append(components, metricsComponent)

	return &IntegrationStatusData{
		GeneratedAt: time.Now().UTC(),
		Components:  components,
	}, nil
}

type managedMetricsTarget struct {
	ClusterID   uint
	ClusterName string
	Env         string
	Target      string
	ProbeURL    string
	StatusCode  int
	Healthy     bool
	ProbeError  string
}

type prometheusActiveTargetsResponse struct {
	Status string `json:"status"`
	Data   struct {
		ActiveTargets []*prometheusActiveTarget `json:"activeTargets"`
	} `json:"data"`
}

type prometheusActiveTarget struct {
	ScrapeURL        string            `json:"scrapeUrl"`
	Health           string            `json:"health"`
	LastError        string            `json:"lastError"`
	Labels           map[string]string `json:"labels"`
	DiscoveredLabels map[string]string `json:"discoveredLabels"`
}

func (s *Service) collectManagedMetricsTargets(ctx context.Context, doProbe bool) ([]*managedMetricsTarget, error) {
	clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
	}

	metricsPath := ensureLeadingSlash(config.Config.Observability.SeatunnelMetric.Path)
	results := make([]*managedMetricsTarget, 0, 16)
	seen := make(map[string]struct{})

	for _, c := range clusters {
		nodes, err := s.clusterService.GetNodes(ctx, c.ID)
		if err != nil {
			continue
		}
		for _, node := range nodes {
			// 仅对在线节点进行探测，且必须配置 Hazelcast 端口（用于暴露 /hazelcast/rest/instance/metrics）
			if node == nil || node.HazelcastPort <= 0 {
				continue
			}
			if !node.IsOnline {
				continue
			}
			host := strings.TrimSpace(node.HostIP)
			if host == "" {
				continue
			}

			// 使用 Hazelcast 端口作为 Prometheus 抓取目标端口（支持 master / worker 多节点）
			target := net.JoinHostPort(host, strconv.Itoa(node.HazelcastPort))
			if _, ok := seen[target]; ok {
				continue
			}
			seen[target] = struct{}{}

			item := &managedMetricsTarget{
				ClusterID:   c.ID,
				ClusterName: strings.TrimSpace(c.Name),
				Env:         resolveClusterEnvLabel(c),
				Target:      target,
				ProbeURL:    "http://" + target + metricsPath,
			}
			// 目标健康探测由 Prometheus 负责；控制面仅读取 Prometheus 视角的抓取结果。
			// Prometheus is the source of truth for target health; the control plane
			// only reads Prometheus scrape status here.
			results = append(results, item)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].ClusterID != results[j].ClusterID {
			return results[i].ClusterID < results[j].ClusterID
		}
		return results[i].Target < results[j].Target
	})
	if doProbe && len(results) > 0 {
		if err := s.decorateManagedMetricsTargetsFromPrometheus(ctx, results); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (s *Service) decorateManagedMetricsTargetsFromPrometheus(ctx context.Context, targets []*managedMetricsTarget) error {
	if len(targets) == 0 {
		return nil
	}

	activeTargets, err := s.listPrometheusActiveTargets(ctx)
	if err != nil {
		return err
	}

	metricsPath := ensureLeadingSlash(config.Config.Observability.SeatunnelMetric.Path)
	activeByAddress := make(map[string]*prometheusActiveTarget, len(activeTargets))
	for _, target := range activeTargets {
		if target == nil {
			continue
		}

		jobName := strings.TrimSpace(firstNonEmpty(
			target.Labels["job"],
			target.DiscoveredLabels["job"],
		))
		discoveredPath := ensureLeadingSlash(firstNonEmpty(
			target.DiscoveredLabels["__metrics_path__"],
			target.Labels["__metrics_path__"],
		))
		clusterIDLabel := strings.TrimSpace(firstNonEmpty(
			target.Labels["cluster_id"],
			target.DiscoveredLabels["cluster_id"],
		))
		if jobName != "seatunnel_engine_http" &&
			(clusterIDLabel == "" || discoveredPath != metricsPath) {
			continue
		}

		for _, address := range candidatePrometheusTargetAddresses(target) {
			if current, exists := activeByAddress[address]; exists && strings.EqualFold(current.Health, "up") {
				continue
			}
			activeByAddress[address] = target
		}
	}

	if len(activeByAddress) == 0 {
		return fmt.Errorf("Prometheus has not discovered any managed SeaTunnel metrics targets yet")
	}

	for _, target := range targets {
		if target == nil {
			continue
		}

		activeTarget, exists := activeByAddress[target.Target]
		if !exists {
			target.Healthy = false
			target.StatusCode = 0
			target.ProbeError = "Prometheus has not discovered this managed target yet"
			continue
		}

		if strings.TrimSpace(activeTarget.ScrapeURL) != "" {
			target.ProbeURL = strings.TrimSpace(activeTarget.ScrapeURL)
		}
		target.Healthy = strings.EqualFold(strings.TrimSpace(activeTarget.Health), "up")
		if target.Healthy {
			target.StatusCode = http.StatusOK
			target.ProbeError = ""
			continue
		}
		target.StatusCode = http.StatusServiceUnavailable
		target.ProbeError = strings.TrimSpace(firstNonEmpty(
			activeTarget.LastError,
			"Prometheus reports the target as down",
		))
	}
	return nil
}

func (s *Service) listPrometheusActiveTargets(ctx context.Context) ([]*prometheusActiveTarget, error) {
	apiURL := joinURL(config.Config.Observability.Prometheus.URL, "/api/v1/targets?state=active")
	if apiURL == "" {
		return nil, fmt.Errorf("prometheus url is empty")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		message := strings.TrimSpace(string(payload))
		if message == "" {
			message = fmt.Sprintf("http status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("prometheus targets api is not ready: %s", message)
	}

	var payload prometheusActiveTargetsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Status), "success") {
		return nil, fmt.Errorf("prometheus targets api returned status %q", strings.TrimSpace(payload.Status))
	}
	return payload.Data.ActiveTargets, nil
}

func candidatePrometheusTargetAddresses(target *prometheusActiveTarget) []string {
	if target == nil {
		return nil
	}

	candidates := []string{
		strings.TrimSpace(target.Labels["instance"]),
		strings.TrimSpace(target.DiscoveredLabels["__address__"]),
		extractAddressFromScrapeURL(target.ScrapeURL),
	}
	result := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func extractAddressFromScrapeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := neturl.Parse(trimmed)
	if err != nil || parsed == nil {
		return ""
	}
	return strings.TrimSpace(parsed.Host)
}

func (s *Service) probeMetricsEndpoint(ctx context.Context, target string) (probeURL string, statusCode int, healthy bool, probeErr string) {
	obsCfg := config.Config.Observability
	path := ensureLeadingSlash(obsCfg.SeatunnelMetric.Path)
	probeURL = "http://" + target + path

	timeout := time.Duration(obsCfg.SeatunnelMetric.ProbeTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return probeURL, 0, false, err.Error()
	}
	resp, err := client.Do(req)
	if err != nil {
		return probeURL, 0, false, err.Error()
	}
	defer resp.Body.Close()

	statusCode = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return probeURL, statusCode, false, fmt.Sprintf("http status %d", resp.StatusCode)
	}

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := string(payload)
	if strings.Contains(body, "# HELP") ||
		strings.Contains(body, "# TYPE") ||
		strings.Contains(body, "node_state") ||
		strings.Contains(body, "jvm_") {
		return probeURL, statusCode, true, ""
	}
	return probeURL, statusCode, false, "metrics payload signature not detected"
}

func checkHTTPComponent(ctx context.Context, targetURL string, timeout time.Duration) (bool, int, string) {
	targetURL = normalizeBaseURL(targetURL)
	if targetURL == "" {
		return false, 0, "url is empty"
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return false, 0, err.Error()
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, 0, err.Error()
	}
	defer resp.Body.Close()

	code := resp.StatusCode
	if code >= 200 && code < 400 {
		return true, code, ""
	}
	return false, code, fmt.Sprintf("http status %d", code)
}

func joinURL(baseURL, suffix string) string {
	base := normalizeBaseURL(baseURL)
	if base == "" {
		return ""
	}
	if suffix == "" {
		return base
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return base + suffix
}

func normalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	return strings.TrimRight(value, "/")
}

func ensureLeadingSlash(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/metrics"
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	return "/" + trimmed
}

// ListNotificationChannels returns all notification channels.
// ListNotificationChannels 返回全部通知渠道。
func (s *Service) ListNotificationChannels(ctx context.Context) (*NotificationChannelListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	channels, err := s.repo.ListNotificationChannels(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]*NotificationChannelDTO, 0, len(channels))
	for _, channel := range channels {
		items = append(items, toNotificationChannelDTO(channel))
	}

	return &NotificationChannelListData{
		GeneratedAt: time.Now().UTC(),
		Total:       len(items),
		Channels:    items,
	}, nil
}

// CreateNotificationChannel creates one notification channel.
// CreateNotificationChannel 创建一条通知渠道。
func (s *Service) CreateNotificationChannel(ctx context.Context, req *UpsertNotificationChannelRequest) (*NotificationChannelDTO, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	channel, err := buildNotificationChannelFromUpsertRequest(req)
	if err != nil {
		return nil, err
	}

	if err := s.repo.CreateNotificationChannel(ctx, channel); err != nil {
		return nil, err
	}
	return toNotificationChannelDTO(channel), nil
}

// UpdateNotificationChannel updates one notification channel.
// UpdateNotificationChannel 更新一条通知渠道。
func (s *Service) UpdateNotificationChannel(ctx context.Context, id uint, req *UpsertNotificationChannelRequest) (*NotificationChannelDTO, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if id == 0 {
		return nil, fmt.Errorf("invalid channel id")
	}

	channel, err := s.repo.GetNotificationChannelByID(ctx, id)
	if err != nil {
		return nil, err
	}

	nextChannel, err := buildNotificationChannelFromUpsertRequest(req)
	if err != nil {
		return nil, err
	}
	channel.Name = nextChannel.Name
	channel.Type = nextChannel.Type
	channel.Enabled = nextChannel.Enabled
	channel.Description = nextChannel.Description
	channel.ConfigJSON = nextChannel.ConfigJSON
	channel.Endpoint = nextChannel.Endpoint
	channel.Secret = nextChannel.Secret

	if err := s.repo.SaveNotificationChannel(ctx, channel); err != nil {
		return nil, err
	}
	return toNotificationChannelDTO(channel), nil
}

// DeleteNotificationChannel deletes one notification channel.
// DeleteNotificationChannel 删除一条通知渠道。
func (s *Service) DeleteNotificationChannel(ctx context.Context, id uint) error {
	if s.repo == nil {
		return fmt.Errorf("monitoring repository is not configured")
	}
	if id == 0 {
		return fmt.Errorf("invalid channel id")
	}
	return s.repo.DeleteNotificationChannel(ctx, id)
}

// StartNodeHealthEvaluator starts a background loop that converts sustained node-unavailable
// states into local alertable events.
// StartNodeHealthEvaluator 启动后台循环，将持续性的节点不可用状态转换为本地可告警事件。
func (s *Service) StartNodeHealthEvaluator(ctx context.Context) {
	if s == nil || s.clusterService == nil || s.monitorService == nil {
		return
	}
	s.nodeHealthEvaluatorStartedAt = timeNowUTC()
	if s.nodeHealthStartupSuppression <= 0 {
		s.nodeHealthStartupSuppression = defaultNodeHealthStartupSuppressionWindow()
	}

	go func() {
		ticker := time.NewTicker(defaultNodeHealthEvaluationInterval)
		defer ticker.Stop()

		for {
			if err := s.EvaluateNodeHealthAlerts(ctx); err != nil {
				log.Printf("[Monitoring] evaluate node health alerts failed: %v / 评估节点健康告警失败: %v", err, err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// EvaluateNodeHealthAlerts scans managed clusters and emits node_offline / node_recovered
// events when one node crosses the sustained-health boundary.
// EvaluateNodeHealthAlerts 扫描受管集群，并在节点跨越持续状态边界时发出 node_offline / node_recovered 事件。
func (s *Service) EvaluateNodeHealthAlerts(ctx context.Context) error {
	if s == nil || s.clusterService == nil || s.monitorService == nil {
		return nil
	}

	clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return err
	}

	now := timeNowUTC()
	var evalErrs []error
	for _, item := range clusters {
		if item == nil {
			continue
		}

		cfg, err := s.monitorService.GetOrCreateConfig(ctx, item.ID)
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}
		if cfg == nil || !cfg.AutoMonitor {
			continue
		}

		nodes, err := s.clusterService.GetNodes(ctx, item.ID)
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}

		for _, node := range nodes {
			if node == nil || shouldSkipNodeOfflineEvaluation(node) {
				continue
			}

			offline, matured, reason, observedSince, graceWindow := evaluateNodeOfflineState(node, cfg, now)
			if reason == "host_offline" {
				observedSince, graceWindow, matured = s.applyRuntimeHostOfflineGrace(node, cfg, now, offline)
			} else {
				s.clearRuntimeHostOfflineObservation(node.ID)
			}
			switch {
			case offline && matured:
				if s.shouldSuppressNodeOfflineDuringStartup(now, reason) {
					continue
				}
				if err := s.recordNodeOfflineEpisode(ctx, item.ID, strings.TrimSpace(item.Name), node, reason, observedSince, graceWindow); err != nil {
					evalErrs = append(evalErrs, err)
				}
			case !offline:
				s.clearRuntimeHostOfflineObservation(node.ID)
				if err := s.recordNodeRecoveredEpisode(ctx, item.ID, node, now); err != nil {
					evalErrs = append(evalErrs, err)
				}
			}
		}
	}

	return errors.Join(evalErrs...)
}

func (s *Service) shouldSuppressNodeOfflineDuringStartup(now time.Time, reason string) bool {
	if s == nil || s.nodeHealthEvaluatorStartedAt.IsZero() || s.nodeHealthStartupSuppression <= 0 {
		return false
	}
	if strings.TrimSpace(reason) != "host_offline" {
		return false
	}
	return now.Before(s.nodeHealthEvaluatorStartedAt.UTC().Add(s.nodeHealthStartupSuppression))
}

func defaultNodeHealthStartupSuppressionWindow() time.Duration {
	window := time.Duration(config.Config.GRPC.HeartbeatTimeout) * time.Second
	if window <= 0 {
		window = 30 * time.Second
	}
	return window
}

func (s *Service) applyRuntimeHostOfflineGrace(node *cluster.NodeInfo, cfg *monitor.MonitorConfig, now time.Time, offline bool) (time.Time, time.Duration, bool) {
	if node == nil {
		return time.Time{}, 0, false
	}
	if !offline {
		s.clearRuntimeHostOfflineObservation(node.ID)
		return time.Time{}, 0, false
	}

	graceWindow := nodeRuntimeOfflineGrace(cfg)
	observedSince := s.getOrInitRuntimeHostOfflineObservation(node.ID, now)
	return observedSince, graceWindow, !now.Before(observedSince.Add(graceWindow))
}

func (s *Service) getOrInitRuntimeHostOfflineObservation(nodeID uint, now time.Time) time.Time {
	if s == nil {
		return now.UTC()
	}

	s.nodeHealthStateMu.Lock()
	defer s.nodeHealthStateMu.Unlock()

	if s.nodeHealthOfflineObservedAt == nil {
		s.nodeHealthOfflineObservedAt = make(map[uint]time.Time)
	}
	if observedSince, ok := s.nodeHealthOfflineObservedAt[nodeID]; ok && !observedSince.IsZero() {
		return observedSince.UTC()
	}

	observedSince := now.UTC()
	s.nodeHealthOfflineObservedAt[nodeID] = observedSince
	return observedSince
}

func (s *Service) clearRuntimeHostOfflineObservation(nodeID uint) {
	if s == nil || nodeID == 0 {
		return
	}

	s.nodeHealthStateMu.Lock()
	defer s.nodeHealthStateMu.Unlock()
	delete(s.nodeHealthOfflineObservedAt, nodeID)
}

func shouldSkipNodeOfflineEvaluation(node *cluster.NodeInfo) bool {
	if node == nil {
		return true
	}
	switch node.Status {
	case cluster.NodeStatusPending, cluster.NodeStatusInstalling:
		return true
	default:
		return false
	}
}

func evaluateNodeOfflineState(
	node *cluster.NodeInfo,
	cfg *monitor.MonitorConfig,
	now time.Time,
) (offline bool, matured bool, reason string, observedSince time.Time, graceWindow time.Duration) {
	if node == nil {
		return false, false, "", time.Time{}, 0
	}

	if !node.IsOnline {
		return true, true, "host_offline", now, 0
	}

	graceWindow = nodeRuntimeOfflineGrace(cfg)
	switch {
	case node.Status == cluster.NodeStatusOffline:
		observedSince = node.UpdatedAt.UTC()
		return true, !now.Before(observedSince.Add(graceWindow)), "host_visibility_lost", observedSince, graceWindow
	case node.Status == cluster.NodeStatusError:
		observedSince = node.UpdatedAt.UTC()
		return true, !now.Before(observedSince.Add(graceWindow)), "node_error", observedSince, graceWindow
	case node.Status == cluster.NodeStatusStopped:
		observedSince = node.UpdatedAt.UTC()
		return true, !now.Before(observedSince.Add(graceWindow)), "process_stopped", observedSince, graceWindow
	case node.ProcessPID <= 0:
		observedSince = node.UpdatedAt.UTC()
		return true, !now.Before(observedSince.Add(graceWindow)), "process_missing", observedSince, graceWindow
	default:
		return false, false, "", time.Time{}, 0
	}
}

func nodeRuntimeOfflineGrace(cfg *monitor.MonitorConfig) time.Duration {
	if cfg == nil {
		return minNodeRuntimeOfflineGraceWindow
	}

	intervalSeconds := cfg.MonitorInterval
	if intervalSeconds <= 0 {
		intervalSeconds = 5
	}

	restartDelaySeconds := cfg.RestartDelay
	if restartDelaySeconds <= 0 {
		restartDelaySeconds = 10
	}

	grace := time.Duration(intervalSeconds*2) * time.Second
	if cfg.AutoRestart {
		grace = time.Duration(restartDelaySeconds+intervalSeconds*2) * time.Second
	}
	if grace < minNodeRuntimeOfflineGraceWindow {
		grace = minNodeRuntimeOfflineGraceWindow
	}
	return grace
}

func (s *Service) recordNodeOfflineEpisode(
	ctx context.Context,
	clusterID uint,
	clusterName string,
	node *cluster.NodeInfo,
	reason string,
	observedSince time.Time,
	graceWindow time.Duration,
) error {
	if node == nil {
		return nil
	}

	active, err := s.isNodeOfflineEpisodeActive(ctx, node.ID)
	if err != nil {
		return err
	}
	if active {
		return s.DispatchActiveNodeOfflineReminder(ctx, clusterID, strings.TrimSpace(clusterName), node.ID)
	}

	details, err := json.Marshal(map[string]string{
		"reason":          strings.TrimSpace(reason),
		"host_name":       strings.TrimSpace(node.HostName),
		"host_ip":         strings.TrimSpace(node.HostIP),
		"node_status":     string(node.Status),
		"is_online":       fmt.Sprintf("%t", node.IsOnline),
		"process_pid":     strconv.Itoa(node.ProcessPID),
		"observed_since":  observedSince.UTC().Format(time.RFC3339),
		"grace_seconds":   strconv.FormatInt(int64(graceWindow/time.Second), 10),
		"evaluation_mode": "sustained_node_health",
	})
	if err != nil {
		return err
	}

	return s.monitorService.RecordEvent(ctx, &monitor.ProcessEvent{
		ClusterID:   clusterID,
		NodeID:      node.ID,
		HostID:      node.HostID,
		EventType:   monitor.EventTypeNodeOffline,
		PID:         node.ProcessPID,
		ProcessName: monitoringProcessNameForRole(string(node.Role)),
		InstallDir:  node.InstallDir,
		Role:        string(node.Role),
		Details:     string(details),
	})
}

func (s *Service) recordNodeRecoveredEpisode(ctx context.Context, clusterID uint, node *cluster.NodeInfo, recoveredAt time.Time) error {
	if node == nil {
		return nil
	}

	active, err := s.isNodeOfflineEpisodeActive(ctx, node.ID)
	if err != nil {
		return err
	}
	if !active {
		return nil
	}

	var offlineEventID uint
	if s.monitorService != nil {
		offlineEvent, err := s.monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{
			monitor.EventTypeNodeOffline,
		})
		if err != nil {
			return err
		}
		if offlineEvent != nil && !offlineEvent.CreatedAt.After(recoveredAt) {
			offlineEventID = offlineEvent.ID
		}
	}

	detailMap := map[string]string{
		"host_name":       strings.TrimSpace(node.HostName),
		"host_ip":         strings.TrimSpace(node.HostIP),
		"node_status":     string(node.Status),
		"is_online":       fmt.Sprintf("%t", node.IsOnline),
		"process_pid":     strconv.Itoa(node.ProcessPID),
		"recovered_at":    recoveredAt.UTC().Format(time.RFC3339),
		"evaluation_mode": "sustained_node_health",
	}
	if offlineEventID > 0 {
		detailMap["offline_event_id"] = strconv.FormatUint(uint64(offlineEventID), 10)
	}

	details, err := json.Marshal(detailMap)
	if err != nil {
		return err
	}

	return s.monitorService.RecordEvent(ctx, &monitor.ProcessEvent{
		ClusterID:   clusterID,
		NodeID:      node.ID,
		HostID:      node.HostID,
		EventType:   monitor.EventTypeNodeRecovered,
		PID:         node.ProcessPID,
		ProcessName: monitoringProcessNameForRole(string(node.Role)),
		InstallDir:  node.InstallDir,
		Role:        string(node.Role),
		Details:     string(details),
	})
}

func (s *Service) isNodeOfflineEpisodeActive(ctx context.Context, nodeID uint) (bool, error) {
	lastMarker, err := s.monitorService.GetLatestNodeEventByTypes(ctx, nodeID, []monitor.ProcessEventType{
		monitor.EventTypeNodeOffline,
		monitor.EventTypeNodeRecovered,
	})
	if err != nil {
		return false, err
	}
	if lastMarker == nil {
		return false, nil
	}
	return lastMarker.EventType == monitor.EventTypeNodeOffline, nil
}

func monitoringProcessNameForRole(role string) string {
	processName := "seatunnel"
	switch strings.TrimSpace(role) {
	case "", "hybrid", "master/worker":
		return processName
	default:
		return processName + "-" + strings.TrimSpace(role)
	}
}

func toEventStats(m map[monitor.ProcessEventType]int64) *EventStats {
	if m == nil {
		m = map[monitor.ProcessEventType]int64{}
	}
	return &EventStats{
		Started:             m[monitor.EventTypeStarted],
		Stopped:             m[monitor.EventTypeStopped],
		Crashed:             m[monitor.EventTypeCrashed],
		Restarted:           m[monitor.EventTypeRestarted],
		RestartFailed:       m[monitor.EventTypeRestartFailed],
		RestartLimitReached: m[monitor.EventTypeRestartLimitReached],
		NodeOffline:         m[monitor.EventTypeNodeOffline],
		NodeRecovered:       m[monitor.EventTypeNodeRecovered],
	}
}

func mergeEventStats(dst, src *EventStats) {
	if dst == nil || src == nil {
		return
	}
	dst.Started += src.Started
	dst.Stopped += src.Stopped
	dst.Crashed += src.Crashed
	dst.Restarted += src.Restarted
	dst.RestartFailed += src.RestartFailed
	dst.RestartLimitReached += src.RestartLimitReached
	dst.NodeOffline += src.NodeOffline
	dst.NodeRecovered += src.NodeRecovered
}

func alertableProcessEventTypes() []monitor.ProcessEventType {
	return []monitor.ProcessEventType{
		monitor.EventTypeCrashed,
		monitor.EventTypeRestartFailed,
		monitor.EventTypeRestartLimitReached,
		monitor.EventTypeClusterRestartRequested,
		monitor.EventTypeNodeRestartRequested,
		monitor.EventTypeNodeStopRequested,
		monitor.EventTypeNodeOffline,
	}
}

func isAlertableEventType(eventType monitor.ProcessEventType) bool {
	for _, candidate := range alertableProcessEventTypes() {
		if candidate == eventType {
			return true
		}
	}
	return false
}

func eventTypeToRuleKey(eventType monitor.ProcessEventType) string {
	switch eventType {
	case monitor.EventTypeCrashed:
		return AlertRuleKeyProcessCrashed
	case monitor.EventTypeRestartFailed:
		return AlertRuleKeyProcessRestartFailed
	case monitor.EventTypeRestartLimitReached:
		return AlertRuleKeyProcessRestartLimitReached
	case monitor.EventTypeClusterRestartRequested:
		return AlertRuleKeyClusterRestartRequested
	case monitor.EventTypeNodeRestartRequested:
		return AlertRuleKeyClusterRestartRequested
	case monitor.EventTypeNodeStopRequested:
		return AlertRuleKeyNodeStopRequested
	case monitor.EventTypeNodeOffline:
		return AlertRuleKeyNodeOffline
	default:
		return ""
	}
}

func resolveAlertStatus(state *AlertEventState, now time.Time) AlertStatus {
	if state == nil {
		return AlertStatusFiring
	}

	switch state.Status {
	case AlertStatusSilenced:
		if state.SilencedUntil != nil && state.SilencedUntil.After(now) {
			return AlertStatusSilenced
		}
		return AlertStatusFiring
	case AlertStatusAcknowledged:
		return AlertStatusAcknowledged
	default:
		return AlertStatusFiring
	}
}

func toAlertActionResult(state *AlertEventState) *AlertActionResult {
	if state == nil {
		return nil
	}
	return &AlertActionResult{
		EventID:          state.EventID,
		Status:           state.Status,
		AcknowledgedBy:   state.AcknowledgedBy,
		AcknowledgedAt:   state.AcknowledgedAt,
		SilencedBy:       state.SilencedBy,
		SilencedUntil:    state.SilencedUntil,
		LatestActionNote: state.Note,
	}
}

func toAlertRuleDTO(rule *AlertRule) *AlertRuleDTO {
	if rule == nil {
		return nil
	}
	return &AlertRuleDTO{
		ID:            rule.ID,
		ClusterID:     rule.ClusterID,
		RuleKey:       rule.RuleKey,
		RuleName:      rule.RuleName,
		Description:   rule.Description,
		Severity:      rule.Severity,
		Enabled:       rule.Enabled,
		Threshold:     rule.Threshold,
		WindowSeconds: rule.WindowSeconds,
		CreatedAt:     rule.CreatedAt,
		UpdatedAt:     rule.UpdatedAt,
	}
}

func toNotificationChannelDTO(channel *NotificationChannel) *NotificationChannelDTO {
	if channel == nil {
		return nil
	}
	return &NotificationChannelDTO{
		ID:          channel.ID,
		Name:        channel.Name,
		Type:        channel.Type,
		Enabled:     channel.Enabled,
		Endpoint:    deriveNotificationChannelEndpoint(channel.Type, channel.Endpoint, unmarshalNotificationChannelConfig(channel.Type, channel.ConfigJSON)),
		Secret:      channel.Secret,
		Config:      unmarshalNotificationChannelConfig(channel.Type, channel.ConfigJSON),
		Description: channel.Description,
		CreatedAt:   channel.CreatedAt,
		UpdatedAt:   channel.UpdatedAt,
	}
}

func (s *Service) ensureDefaultRules(ctx context.Context, clusterID uint) error {
	for _, tpl := range defaultClusterRules(clusterID) {
		_, err := s.repo.GetRuleByClusterAndKey(ctx, clusterID, tpl.RuleKey)
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if createErr := s.repo.CreateRule(ctx, tpl); createErr != nil {
			if isDuplicateKeyError(createErr) {
				continue
			}
			return createErr
		}
	}
	return nil
}

func (s *Service) getClusterRuleMap(ctx context.Context, clusterID uint, cache map[uint]map[string]*AlertRule) (map[string]*AlertRule, error) {
	if ruleMap, ok := cache[clusterID]; ok {
		return ruleMap, nil
	}
	if err := s.ensureDefaultRules(ctx, clusterID); err != nil {
		return nil, err
	}

	rules, err := s.repo.ListRulesByClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	ruleMap := make(map[string]*AlertRule, len(rules))
	for _, rule := range rules {
		ruleMap[rule.RuleKey] = rule
	}
	cache[clusterID] = ruleMap
	return ruleMap, nil
}

func defaultClusterRules(clusterID uint) []*AlertRule {
	return []*AlertRule{
		{
			ClusterID:     clusterID,
			RuleKey:       AlertRuleKeyProcessCrashed,
			RuleName:      "进程崩溃告警",
			Description:   "当检测到进程崩溃事件时触发告警",
			Severity:      AlertSeverityCritical,
			Enabled:       true,
			Threshold:     1,
			WindowSeconds: 300,
		},
		{
			ClusterID:     clusterID,
			RuleKey:       AlertRuleKeyProcessRestartFailed,
			RuleName:      "进程重启失败告警",
			Description:   "当自动重启失败时触发告警",
			Severity:      AlertSeverityCritical,
			Enabled:       true,
			Threshold:     1,
			WindowSeconds: 300,
		},
		{
			ClusterID:     clusterID,
			RuleKey:       AlertRuleKeyProcessRestartLimitReached,
			RuleName:      "达到重启上限告警",
			Description:   "当进程达到重启上限时触发告警",
			Severity:      AlertSeverityCritical,
			Enabled:       true,
			Threshold:     1,
			WindowSeconds: 300,
		},
		{
			ClusterID:     clusterID,
			RuleKey:       AlertRuleKeyClusterRestartRequested,
			RuleName:      "重启事件通知",
			Description:   "当通过控制面发起集群或节点重启时触发通知，用于邮件联动与变更确认",
			Severity:      AlertSeverityWarning,
			Enabled:       true,
			Threshold:     1,
			WindowSeconds: 60,
		},
		{
			ClusterID:     clusterID,
			RuleKey:       AlertRuleKeyNodeStopRequested,
			RuleName:      "节点停止事件通知",
			Description:   "当通过控制面手动停止某个节点时立即记录并触发通知",
			Severity:      AlertSeverityWarning,
			Enabled:       true,
			Threshold:     1,
			WindowSeconds: 60,
		},
		{
			ClusterID:     clusterID,
			RuleKey:       AlertRuleKeyNodeOffline,
			RuleName:      "节点离线告警",
			Description:   "当节点在宽限窗口后仍不可用时触发告警，覆盖主机离线和进程持续不可见两类场景",
			Severity:      AlertSeverityCritical,
			Enabled:       true,
			Threshold:     1,
			WindowSeconds: 60,
		},
	}
}

func defaultRuleByKey(clusterID uint, ruleKey string) *AlertRule {
	for _, rule := range defaultClusterRules(clusterID) {
		if rule.RuleKey == ruleKey {
			return rule
		}
	}
	return nil
}

func resolveClusterEnvLabel(c *cluster.Cluster) string {
	if c == nil || c.Config == nil {
		return "unknown"
	}
	for _, key := range []string{"env", "environment"} {
		if raw, ok := c.Config[key]; ok && raw != nil {
			value := strings.TrimSpace(fmt.Sprintf("%v", raw))
			if value != "" {
				return value
			}
		}
	}
	return "unknown"
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique constraint")
}
