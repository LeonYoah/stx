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
	"fmt"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/common/model"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

const (
	defaultNodeHealthEvaluationInterval = 5 * time.Second
	minNodeRuntimeOfflineGraceWindow    = 20 * time.Second
)

// Service provides monitoring center data for UI.
// Service 为 UI 提供监控中心数据。
type Service struct {
	k8sManager     *k8s.ClientManager
	clusterService *cluster.Service
	monitorService *monitor.Service
	repo           *Repository
	// channelHealth 记录渠道最近的投递结果（notification_channel_health.go）。
	// channelHealth tracks recent per-channel delivery outcomes (notification_channel_health.go).
	channelHealth notificationChannelHealth

	nodeHealthEvaluatorStartedAt time.Time
	nodeHealthStartupSuppression time.Duration

	// availabilityEvaluator 持有可用性规则的内存 pending 状态（availability_evaluator.go）。
	// availabilityEvaluator keeps the in-memory pending state of the availability rules
	// (availability_evaluator.go).
	availabilityEvaluator *availabilityEvaluator

	// syncTaskNames 由 sync 注入，用于策略列表显示任务名；未注入时不显示。
	// syncTaskNames is injected by sync so the policy list can show task names; nothing is shown without it.
	syncTaskNames SyncTaskNameResolver

	// syncAlertEpisodeMu 让"保存 / 删除策略时关闭阶段"和"冷却提醒的检查"互斥（不覆盖网络投递）；syncAlertPolicyVersion 在
	// 每次保存 / 删除策略时加一（受 syncAlertEpisodeMu 保护），提醒投递前核对它，策略停用的那一刻不会再发出旧提醒。
	// syncAlertEpisodeMu serialises closing episodes on policy save / delete with the reminder check (never the network
	// delivery); syncAlertPolicyVersion is bumped on every policy save / delete (guarded by syncAlertEpisodeMu) and
	// checked again right before a reminder is delivered, so no stale reminder goes out once a policy is disabled.
	syncAlertEpisodeMu     sync.Mutex
	syncAlertPolicyVersion uint64

	// metricEvaluator 是内嵌告警评估器（rules.Manager 包装），由 router 组装区在
	// observability.enabled && embedts.Ready() 时经 StartMetricEvaluator 注入；策略 CRUD 读它
	// 触发规则组重建，用 atomic.Pointer 保证读路径无锁。
	// metricEvaluator is the embedded alert evaluator (a rules.Manager wrapper), injected by the
	// router assembly via StartMetricEvaluator when observability.enabled && embedts.Ready();
	// policy CRUD reads it to trigger rule-group rebuilds, and atomic.Pointer keeps that read
	// path lock-free.
	metricEvaluator atomic.Pointer[MetricEvaluator]

	// clustersHealthCache 按集群范围缓存 GetClustersHealth：0 表示全部集群，其它键为单个 cluster_id。
	// clustersHealthCache keys GetClustersHealth by scope: 0 = every cluster, otherwise one cluster_id.
	clustersHealthMu    sync.Mutex
	clustersHealthCache map[uint]*ClusterHealthData
	clustersHealthUntil map[uint]time.Time

	// clusterRuleCache 是 ResolveClusterRule 的短 TTL 进程内缓存（键为 ruleKey|clusterID）；
	// 配置写入后由 invalidateClusterRuleResolution 清空并触发 TSDB 规则组重建。
	// clusterRuleCache is the short-TTL in-process cache of ResolveClusterRule (keyed
	// ruleKey|clusterID); config writes clear it via invalidateClusterRuleResolution which also
	// triggers a tsdb rule-group rebuild.
	clusterRuleCacheMu sync.Mutex
	clusterRuleCache   map[string]*clusterRuleCacheEntry

	// platformHosts 是平台快照读取 hosts 表的可选依赖（platform_snapshot.go）；
	// 未注入时快照的 Agent 列表为空。
	// platformHosts is the optional hosts-table dependency for the platform snapshot
	// (platform_snapshot.go); the agent list stays empty without it.
	platformHosts platformHostLister
}

// NewService creates a monitoring service.
// NewService 创建监控中心服务。
func NewService(clusterService *cluster.Service, monitorService *monitor.Service, repo *Repository) *Service {
	service := &Service{
		clusterService:               clusterService,
		monitorService:               monitorService,
		repo:                         repo,
		nodeHealthStartupSuppression: defaultNodeHealthStartupSuppressionWindow(),
		availabilityEvaluator:        newAvailabilityEvaluator(),
		clustersHealthCache:          make(map[uint]*ClusterHealthData),
		clustersHealthUntil:          make(map[uint]time.Time),
	}
	service.ensureClusterRuleCache()
	return service
}

// GetOverview returns global monitoring overview data.
// GetOverview 返回全局监控总览数据。
func (s *Service) GetOverview(ctx context.Context) (*OverviewData, error) {
	now := timex.Now()
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
	now := timex.Now()
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

// GetIntegrationStatus returns integration health status of monitoring stack.
// GetIntegrationStatus 返回监控栈集成状态。
//
// 组件集（内嵌可观测终态）：embedded_tsdb（内嵌 tsdb 就绪探测）+ seatunnel_metrics（自研抓取循环
// 目标状态）；原 prometheus/alertmanager/grafana 外部健康检查与 remote_ingest 外部告警接入
// 端点均已退役。
// Component set (embedded observability final state): embedded_tsdb (embedded tsdb readiness) +
// seatunnel_metrics (in-process scraper target states); the old prometheus/alertmanager/grafana
// external health checks and the remote_ingest external alert endpoint were retired.
func (s *Service) GetIntegrationStatus(ctx context.Context) (*IntegrationStatusData, error) {
	obsCfg := config.Config.Observability

	if !obsCfg.Enabled {
		return &IntegrationStatusData{
			GeneratedAt: timex.Now(),
			Components: []*IntegrationComponentStatus{
				disabledIntegrationComponent("embedded_tsdb"),
				disabledIntegrationComponent("seatunnel_metrics"),
			},
		}, nil
	}

	return &IntegrationStatusData{
		GeneratedAt: timex.Now(),
		Components: []*IntegrationComponentStatus{
			s.embeddedTSDBComponent(obsCfg),
			s.seatunnelMetricsComponent(ctx, obsCfg),
		},
	}, nil
}

// disabledIntegrationComponent 构造「observability.enabled=false」态的组件状态。
// disabledIntegrationComponent builds the component state for observability.enabled=false.
func disabledIntegrationComponent(name string) *IntegrationComponentStatus {
	return &IntegrationComponentStatus{
		Name:    name,
		Healthy: false,
		Error:   "disabled by config: observability.enabled=false",
	}
}

// embeddedTSDBComponent 返回内嵌 tsdb 组件状态：就绪探测 + 目录/保留期元信息。
// embeddedTSDBComponent returns the embedded tsdb component: readiness plus dir/retention meta.
func (s *Service) embeddedTSDBComponent(obsCfg config.ObservabilityConfig) *IntegrationComponentStatus {
	// 保留期与容量上限优先展示运行实例的实际生效值（自适应计算后）；未启动时退回配置原文，
	// 空 retention 统一显示 "unlimited"。
	// Retention/size meta prefer the running instance's effective values (after auto derivation);
	// while not started they fall back to the raw config, and an empty retention reads "unlimited".
	retention := strings.TrimSpace(obsCfg.Storage.Retention)
	if retention == "" {
		retention = "unlimited"
	} else if d, err := model.ParseDuration(retention); err == nil && d <= 0 {
		retention = "unlimited"
	}
	maxSize := strconv.FormatInt(obsCfg.Storage.MaxSize, 10)
	maxSizeSource := embedts.MaxBytesSourceConfig
	if !obsCfg.Storage.MaxSizeSet {
		maxSizeSource = "auto"
	}
	if opts, ok := embedts.CurrentOptions(); ok {
		if opts.RetentionDuration > 0 {
			retention = model.Duration(opts.RetentionDuration).String()
		} else {
			retention = "unlimited"
		}
		maxSize = strconv.FormatInt(opts.MaxBytes, 10)
		if opts.MaxBytesSource != "" {
			maxSizeSource = opts.MaxBytesSource
		}
	}
	component := &IntegrationComponentStatus{
		Name:    "embedded_tsdb",
		URL:     strings.TrimSpace(obsCfg.Storage.Dir),
		Healthy: embedts.Ready(),
		Meta: map[string]string{
			"dir":             strings.TrimSpace(obsCfg.Storage.Dir),
			"retention":       retention,
			"max_size":        maxSize,
			"max_size_source": maxSizeSource,
		},
	}
	if !component.Healthy {
		component.Error = "monitoring storage is not running (open failed or not started yet)"
		return component
	}
	// 就绪时附带存储快照：head 序列、块字节、最早样本、compaction 失败次数（AC2）。
	// When ready, attach the storage snapshot: head series, block bytes, oldest sample,
	// compaction failures (AC2). CLI `stx monitoring integration status` already dumps Meta.
	if status, ok := embedts.GetStorageStatus(); ok {
		component.Meta["head_series"] = strconv.FormatUint(status.HeadSeries, 10)
		component.Meta["head_stale_series"] = strconv.FormatUint(status.HeadStaleSeries, 10)
		component.Meta["head_bytes"] = strconv.FormatInt(status.HeadBytes, 10)
		component.Meta["blocks_bytes"] = strconv.FormatInt(status.BlocksBytes, 10)
		component.Meta["compactions_failed"] = strconv.FormatFloat(status.CompactionsFailed, 'f', -1, 64)
		component.Meta["size_retentions"] = strconv.FormatFloat(status.SizeRetentions, 'f', -1, 64)
		if status.HasOldest {
			component.Meta["oldest_sample_ms"] = strconv.FormatInt(status.OldestSampleMs, 10)
		}
	}
	component.Meta["max_samples"] = strconv.FormatInt(obsCfg.MaxSamples, 10)
	component.Meta["sample_limit"] = strconv.FormatInt(obsCfg.SampleLimit, 10)
	return component
}

// seatunnelMetricsComponent 返回 SeaTunnel 抓取目标组件状态：期望目标集与抓取循环内存状态合并，
// 保留原「目标不可达 / 部分健康」语义。
// seatunnelMetricsComponent returns the SeaTunnel scrape-target component: the expected target set
// merged with the scrape loop's in-memory states, preserving the old unreachable / partially
// healthy semantics.
func (s *Service) seatunnelMetricsComponent(ctx context.Context, obsCfg config.ObservabilityConfig) *IntegrationComponentStatus {
	component := &IntegrationComponentStatus{
		Name: "seatunnel_metrics",
		URL:  ensureLeadingSlash(obsCfg.SeatunnelMetric.Path),
	}

	statesByURL := make(map[string]embedts.TargetState)
	for _, state := range embedts.TargetStates() {
		statesByURL[state.URL] = state
	}

	targets, err := s.collectManagedMetricsTargets(ctx)
	if err != nil {
		component.Error = err.Error()
		return component
	}

	total := len(targets)
	up, down, pending := 0, 0, 0
	firstErr := ""
	for _, target := range targets {
		if target == nil {
			continue
		}
		state, seen := statesByURL[target.ProbeURL]
		switch {
		case seen && state.Healthy:
			up++
			if component.URL == "" || component.StatusCode == 0 {
				component.URL = target.ProbeURL
				component.StatusCode = state.StatusCode
			}
		case seen:
			down++
			if firstErr == "" {
				firstErr = strings.TrimSpace(state.LastError)
			}
		default:
			// 目标已在期望集合但抓取循环尚未覆盖到（首轮未跑/刷新未生效）。
			// Expected but not yet covered by the scrape loop (first round pending / refresh lag).
			pending++
		}
	}
	component.Meta = map[string]string{
		"total":   strconv.Itoa(total),
		"up":      strconv.Itoa(up),
		"down":    strconv.Itoa(down),
		"pending": strconv.Itoa(pending),
	}
	if firstErr != "" {
		component.Meta["last_error"] = firstErr
	}

	switch {
	case total == 0:
		component.Error = "no managed cluster node exposes api_port for metrics probing"
	case up == 0 && pending > 0:
		component.Error = fmt.Sprintf("waiting for first scrape round on %d/%d targets", pending, total)
	case up == 0:
		component.Error = fmt.Sprintf("metrics endpoint unreachable or metrics disabled on %d/%d targets", total, total)
		if firstErr != "" {
			component.Error += ": " + firstErr
		}
	case up < total:
		component.Healthy = true
		component.Error = fmt.Sprintf("partial healthy: %d/%d targets expose metrics", up, total)
	default:
		component.Healthy = true
	}
	return component
}

type managedMetricsTarget struct {
	ClusterID   uint
	ClusterName string
	Env         string
	Target      string
	ProbeURL    string
	// Transport 可选的按目标定制 RoundTripper；裸金属为空。
	// Transport is the optional custom RoundTripper per target; empty for bare metal.
	Transport http.RoundTripper
}

// collectManagedMetricsTargets 计算受管 SeaTunnel 抓取目标集（cluster → 在线节点 → host:hazelcastPort + metrics path），
// 供抓取循环 Provider 与 integration status 共用；目标健康由抓取循环状态判定。
// collectManagedMetricsTargets computes the managed SeaTunnel scrape target set (cluster → online
// nodes → host:hazelcastPort + metrics path), shared by the scraper provider and integration
// status; target health is decided by the scrape loop state.
func (s *Service) collectManagedMetricsTargets(ctx context.Context) ([]*managedMetricsTarget, error) {
	clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
	}

	metricsPath := ensureLeadingSlash(config.Config.Observability.SeatunnelMetric.Path)
	results := make([]*managedMetricsTarget, 0, 16)
	seen := make(map[string]struct{})

	for _, c := range clusters {
		if c.IsKubernetes() {
			results = append(results, s.collectK8sMetricsTargets(ctx, c, metricsPath)...)
			continue
		}
		nodes, err := s.clusterService.GetNodes(ctx, c.ID)
		if err != nil {
			continue
		}
		for _, node := range nodes {
			// 仅对在线节点生成抓取目标，且必须配置 Hazelcast 端口（用于暴露 /hazelcast/rest/instance/metrics）。
			// Only online nodes with a Hazelcast port (exposing /hazelcast/rest/instance/metrics) become targets.
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

			// 使用 Hazelcast 端口作为抓取目标端口（支持 master / worker 多节点）。
			// The Hazelcast port is the scrape target port (supports multi-node master / worker layouts).
			target := net.JoinHostPort(host, strconv.Itoa(node.HazelcastPort))
			if _, ok := seen[target]; ok {
				continue
			}
			seen[target] = struct{}{}

			results = append(results, &managedMetricsTarget{
				ClusterID:   c.ID,
				ClusterName: strings.TrimSpace(c.Name),
				Env:         resolveClusterEnvLabel(c),
				Target:      target,
				ProbeURL:    "http://" + target + metricsPath,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].ClusterID != results[j].ClusterID {
			return results[i].ClusterID < results[j].ClusterID
		}
		return results[i].Target < results[j].Target
	})
	return results, nil
}

// StartMetricEvaluator 装配并启动内嵌告警评估器：仅在 observability.enabled && embedts.Ready()
// 时生效（router 组装区在启动抓取循环的同分支调用）；重复调用为空操作。
// StartMetricEvaluator assembles and starts the embedded alert evaluator: it only takes effect
// when observability.enabled && embedts.Ready() (the router assembly calls it in the same branch
// that starts the scrape loop); repeated calls are no-ops.
func (s *Service) StartMetricEvaluator(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if s.metricEvaluator.Load() != nil {
		return nil
	}
	if !config.Config.Observability.Enabled || !embedts.Ready() {
		return nil
	}
	evaluator, err := NewMetricEvaluator(s, MetricEvaluatorOptions{Interval: metricEvalIntervalFromConfig()})
	if err != nil {
		return err
	}
	if err := evaluator.Start(ctx); err != nil {
		return err
	}
	if !s.metricEvaluator.CompareAndSwap(nil, evaluator) {
		evaluator.Stop()
		return nil
	}
	logger.InfoF(ctx, "embedded alert evaluator started: eval_interval=%s", evaluator.opts.Interval)
	return nil
}

// StopMetricEvaluator 停止内嵌告警评估器并清空引用；未装配时为空操作。
// StopMetricEvaluator halts the embedded alert evaluator and clears the reference; a no-op when
// none is assembled.
func (s *Service) StopMetricEvaluator() {
	if s == nil {
		return
	}
	if evaluator := s.metricEvaluator.Swap(nil); evaluator != nil {
		evaluator.Stop()
	}
}

// requestMetricEvalReload 请求评估器即时重建规则组（策略 CRUD 提交后调用）；未装配评估器时为空操作。
// requestMetricEvalReload asks the evaluator for an immediate rule-group rebuild (called after
// policy CRUD commits); a no-op when no evaluator is assembled.
func (s *Service) requestMetricEvalReload() {
	if s == nil {
		return
	}
	if evaluator := s.metricEvaluator.Load(); evaluator != nil {
		evaluator.RequestReload()
	}
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
		GeneratedAt: timex.Now(),
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
	restoredReq, err := restoreMaskedNotificationChannelFields(req, nil)
	if err != nil {
		return nil, err
	}
	channel, err := buildNotificationChannelFromUpsertRequest(restoredReq)
	if err != nil {
		return nil, newInvalidNotificationChannelError("%s", err.Error())
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

	// GET 返回的是脱敏值；前端原样传回占位符 / 空密钥时沿用已保存的真实值。
	// GET returns redacted values; a placeholder / empty secret sent back keeps the stored value.
	restoredReq, err := restoreMaskedNotificationChannelFields(req, channel)
	if err != nil {
		return nil, err
	}
	nextChannel, err := buildNotificationChannelFromUpsertRequest(restoredReq)
	if err != nil {
		return nil, newInvalidNotificationChannelError("%s", err.Error())
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
	s.channelHealth.recordChanged(channel.ID)
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

// StartNodeHealthEvaluator starts a background loop that runs the availability evaluator
// (availability_evaluator.go) on the shared eval interval.
// StartNodeHealthEvaluator 启动后台循环，按统一评估周期运行可用性评估器（availability_evaluator.go）。
func (s *Service) StartNodeHealthEvaluator(ctx context.Context) {
	if s == nil || s.clusterService == nil || s.monitorService == nil {
		return
	}
	s.nodeHealthEvaluatorStartedAt = timeNowUTC()
	if s.nodeHealthStartupSuppression <= 0 {
		s.nodeHealthStartupSuppression = defaultNodeHealthStartupSuppressionWindow()
	}

	go func() {
		ticker := time.NewTicker(metricEvalIntervalFromConfig())
		defer ticker.Stop()

		for {
			if err := s.EvaluateAvailabilityAlerts(ctx); err != nil {
				log.Printf("[Monitoring] evaluate availability alerts failed: %v", err)
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func defaultNodeHealthStartupSuppressionWindow() time.Duration {
	window := time.Duration(config.Config.GRPC.HeartbeatTimeout) * time.Second
	if window <= 0 {
		window = 30 * time.Second
	}
	return window
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
		monitor.EventTypeAgentOffline,
		monitor.EventTypeMasterUnavailable,
		monitor.EventTypeWorkerInsufficient,
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
	case monitor.EventTypeAgentOffline:
		return clusterRuleKeyAgentOffline
	case monitor.EventTypeMasterUnavailable:
		return clusterRuleKeyMasterUnavailable
	case monitor.EventTypeWorkerInsufficient:
		return clusterRuleKeyWorkerInsufficient
	default:
		return ""
	}
}

func toNotificationChannelDTO(channel *NotificationChannel) *NotificationChannelDTO {
	if channel == nil {
		return nil
	}
	// 密钥、SMTP 密码和 URL 里的 token 一律脱敏后再返回给前端。
	// Secrets, SMTP passwords and URL tokens are always redacted before reaching the client.
	config := unmarshalNotificationChannelConfig(channel.Type, channel.ConfigJSON)
	endpoint := deriveNotificationChannelEndpoint(channel.Type, channel.Endpoint, config)
	if channel.Type != NotificationChannelTypeEmail {
		endpoint = redactEndpointURL(endpoint)
	}
	if config != nil && config.Email != nil {
		email := *config.Email
		email.Password = maskNotificationSecret(email.Password)
		config.Email = &email
	}
	return &NotificationChannelDTO{
		ID:          channel.ID,
		Name:        channel.Name,
		Type:        channel.Type,
		Enabled:     channel.Enabled,
		Endpoint:    endpoint,
		Secret:      maskNotificationSecret(channel.Secret),
		Config:      config,
		Description: channel.Description,
		CreatedAt:   channel.CreatedAt,
		UpdatedAt:   channel.UpdatedAt,
	}
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

// SetK8sClientManager 注入 K8sClientManager 供 K8s 集群指标抓取。
// SetK8sClientManager injects the K8sClientManager for K8s cluster metrics scraping.
func (s *Service) SetK8sClientManager(mgr *k8s.ClientManager) {
	s.k8sManager = mgr
}

// collectK8sMetricsTargets 为 K8s 集群生成逐 Pod 抓取目标：按连接方式直连 Pod IP 或外部入口；
// 失败静默降级为空集，不拖垮整体抓取面。
// collectK8sMetricsTargets builds per-pod scrape targets for K8s clusters using in-cluster Pod IPs
// or external base URL; failures degrade silently to an empty set.
func (s *Service) collectK8sMetricsTargets(ctx context.Context, c *cluster.Cluster, metricsPath string) []*managedMetricsTarget {
	out := []*managedMetricsTarget{}
	if s.k8sManager == nil {
		return out
	}
	cfg := c.K8sSessionConfig()
	if cfg == nil {
		return out
	}
	transport := k8s.NewTransport(s.k8sManager, cfg.HostID, cfg.Namespace, cfg.AccessMode, cfg.ExternalBaseURL)
	rt, err := transport.RoundTripper(ctx)
	if err != nil {
		return out
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	pods, err := ops.SessionPods(ctx, cfg, c.EffectiveClusterCode())
	if err != nil {
		return out
	}
	port := cfg.MetricsPort
	if port <= 0 {
		port = cfg.MasterHTTPPort
	}
	path := cfg.MetricsPath
	if path == "" {
		path = metricsPath
	}
	for _, pod := range pods {
		if pod == nil || pod.PodIP == "" {
			continue
		}
		base, err := transport.PodBaseURL(ctx, pod.Name, pod.PodIP, port)
		if err != nil {
			continue
		}
		out = append(out, &managedMetricsTarget{
			ClusterID:   c.ID,
			ClusterName: strings.TrimSpace(c.Name),
			Env:         resolveClusterEnvLabel(c),
			Target:      pod.Name,
			ProbeURL:    base + ensureLeadingSlash(path),
			Transport:   rt,
		})
	}
	return out
}
