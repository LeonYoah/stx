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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// clustersHealthCacheTTL 是集群健康 / 首页 KPI 的缓存时长；测试可设为 0 以关闭缓存。
// clustersHealthCacheTTL is the cluster-health / home KPI cache TTL; tests may set it to 0 to disable caching.
var clustersHealthCacheTTL = 15 * time.Second

// clustersHealthAlertLookback 限制本地进程告警计入 KPI 的时间窗，避免扫全部历史事件。
// clustersHealthAlertLookback caps how far back local process alerts count toward KPIs, avoiding a full history scan.
const clustersHealthAlertLookback = 31 * 24 * time.Hour

// ingestRemoteAlertRecord 是「标准化告警记录 → 持久化 → 投递」的统一入口，供内嵌告警评估器使用：
// lookup previous → upsert → managed 策略投递或路由匹配投递。
// ingestRemoteAlertRecord is the "normalized record → persist → deliver" entry used by the embedded
// alert evaluator: lookup previous → upsert → managed policy delivery or route-match delivery.
func (s *Service) ingestRemoteAlertRecord(ctx context.Context, record *RemoteAlertRecord) error {
	previousRecord, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, record.Fingerprint, record.StartsAt)
	if err != nil {
		return fmt.Errorf("lookup failed: %w", err)
	}
	if err := s.repo.UpsertRemoteAlert(ctx, record); err != nil {
		return fmt.Errorf("upsert failed: %w", err)
	}
	// 同指纹只允许一个未恢复实例：评估器重启/规则重载后相同告警会以新 starts_at 重新上报，
	// 把旧实例静默收编为 resolved（不触发恢复通知——这只是账面归并，不是真实恢复）。
	// Only one unresolved instance per fingerprint: after evaluator restarts / rule reloads the same
	// alert is re-reported with a new starts_at, so fold older instances into resolved silently (no
	// recovery notification — this is bookkeeping, not a real recovery).
	if staleErr := s.repo.ResolveStaleRemoteAlertsByFingerprint(ctx, record.Fingerprint, record.StartsAt, remoteAlertSupersedeTime(record)); staleErr != nil {
		logger.WarnF(ctx, "远程告警收编同指纹旧实例失败，继续投递: %v / superseding stale instances of remote alert fingerprint failed, still delivering: %v", staleErr, staleErr)
	}
	return s.deliverRemoteAlertRecord(ctx, record, previousRecord)
}

// remoteAlertSupersedeTime 给被收编的旧实例选恢复时刻：恢复记录用自身的 ResolvedAt，
// 触发记录用其 StartsAt（旧实例应在新实例开始前结束）；兜底 now。
// remoteAlertSupersedeTime picks the resolve moment for superseded instances: a resolved record
// uses its own ResolvedAt, a firing record uses its StartsAt (the old instance should end before
// the new one begins); falls back to now.
func remoteAlertSupersedeTime(record *RemoteAlertRecord) time.Time {
	if record == nil {
		return time.Now().UTC()
	}
	if record.ResolvedAt != nil && !record.ResolvedAt.IsZero() {
		return record.ResolvedAt.UTC()
	}
	if record.StartsAt > 0 {
		return time.Unix(record.StartsAt, 0).UTC()
	}
	return time.Now().UTC()
}

// deliverRemoteAlertRecord 走 managed 规则投递路径（去重/冷却/静默/恢复语义所在地）；
// 非 managed 记录不再回落到路由匹配投递——告警模型收拢后只有 managed 规则会产生记录。
// deliverRemoteAlertRecord uses the managed rule delivery path (home of
// dedupe/cooldown/silence/recovery semantics); non-managed records no longer fall back to
// route-match delivery — after the alert-model consolidation only managed rules produce records.
func (s *Service) deliverRemoteAlertRecord(ctx context.Context, record *RemoteAlertRecord, previousRecord *RemoteAlertRecord) error {
	_, err := s.deliverManagedRemoteAlertPolicyNotifications(ctx, record, previousRecord)
	return err
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func mustMarshalJSON(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// ListRemoteAlerts returns remote alert records with paging filters.
// ListRemoteAlerts 返回远程告警记录（支持分页过滤）。
func (s *Service) ListRemoteAlerts(ctx context.Context, filter *RemoteAlertFilter) (*RemoteAlertListData, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if filter == nil {
		filter = &RemoteAlertFilter{}
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	filter.ClusterID = strings.TrimSpace(filter.ClusterID)
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))

	rows, total, err := s.repo.ListRemoteAlerts(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*RemoteAlertItem, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		items = append(items, &RemoteAlertItem{
			ID:             row.ID,
			Fingerprint:    row.Fingerprint,
			Status:         row.Status,
			Receiver:       row.Receiver,
			AlertName:      row.AlertName,
			Severity:       row.Severity,
			ClusterID:      row.ClusterID,
			ClusterName:    row.ClusterName,
			Env:            row.Env,
			Summary:        row.Summary,
			Description:    row.Description,
			StartsAt:       row.StartsAt,
			EndsAt:         row.EndsAt,
			ResolvedAt:     row.ResolvedAt,
			LastReceivedAt: row.LastReceivedAt,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		})
	}

	return &RemoteAlertListData{
		GeneratedAt: timex.Now(),
		Page:        filter.Page,
		PageSize:    filter.PageSize,
		Total:       total,
		Alerts:      items,
	}, nil
}

// GetClustersHealth returns cluster-level health summary for monitoring center.
// GetClustersHealth 返回监控中心集群级健康摘要。
func (s *Service) GetClustersHealth(ctx context.Context) (*ClusterHealthData, error) {
	return s.GetClustersHealthFor(ctx, 0)
}

// GetClustersHealthFor 返回集群健康摘要。clusterID=0 统计全部集群；大于 0 时只查该集群的告警。
// GetClustersHealthFor returns cluster health. clusterID 0 counts every cluster; a positive id only that cluster's alerts.
func (s *Service) GetClustersHealthFor(ctx context.Context, clusterID uint) (*ClusterHealthData, error) {
	if clustersHealthCacheTTL > 0 {
		s.clustersHealthMu.Lock()
		if s.clustersHealthCache == nil {
			s.clustersHealthCache = make(map[uint]*ClusterHealthData)
			s.clustersHealthUntil = make(map[uint]time.Time)
		}
		if cached := s.clustersHealthCache[clusterID]; cached != nil && time.Now().Before(s.clustersHealthUntil[clusterID]) {
			s.clustersHealthMu.Unlock()
			return cached, nil
		}
		s.clustersHealthMu.Unlock()
	}

	data, err := s.loadClustersHealth(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if clustersHealthCacheTTL > 0 {
		s.clustersHealthMu.Lock()
		if s.clustersHealthCache == nil {
			s.clustersHealthCache = make(map[uint]*ClusterHealthData)
			s.clustersHealthUntil = make(map[uint]time.Time)
		}
		s.clustersHealthCache[clusterID] = data
		s.clustersHealthUntil[clusterID] = time.Now().Add(clustersHealthCacheTTL)
		s.clustersHealthMu.Unlock()
	}
	return data, nil
}

// invalidateClustersHealthCache 丢掉全部范围的集群健康缓存，使首页 / 大盘告警数在确认或关闭后立刻对齐。
// invalidateClustersHealthCache drops every clusters-health cache entry so home / dashboard alert counts
// match immediately after an ack or close.
func (s *Service) invalidateClustersHealthCache() {
	if s == nil {
		return
	}
	s.clustersHealthMu.Lock()
	s.clustersHealthCache = make(map[uint]*ClusterHealthData)
	s.clustersHealthUntil = make(map[uint]time.Time)
	s.clustersHealthMu.Unlock()
}

// loadClustersHealth 计算集群健康摘要（不读缓存）。clusterID=0 表示全部集群。
// loadClustersHealth computes the cluster health summary (no cache). clusterID 0 means every cluster.
func (s *Service) loadClustersHealth(ctx context.Context, clusterID uint) (*ClusterHealthData, error) {
	if s.clusterService == nil {
		return nil, fmt.Errorf("cluster service is not configured")
	}

	var clusters []*cluster.Cluster
	var err error
	if clusterID > 0 {
		item, getErr := s.clusterService.Get(ctx, clusterID)
		if getErr != nil {
			return nil, getErr
		}
		if item != nil {
			clusters = []*cluster.Cluster{item}
		}
	} else {
		clusters, _, err = s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
		if err != nil {
			return nil, err
		}
	}

	clusterIDs := make([]string, 0, len(clusters))
	for _, c := range clusters {
		if c == nil {
			continue
		}
		clusterIDs = append(clusterIDs, strconv.FormatUint(uint64(c.ID), 10))
	}

	statsMap := make(map[string]*RemoteAlertClusterStat)
	if s.repo != nil {
		now := timex.Now()
		remoteStats, statsErr := s.activeRemoteAlertStats(ctx, clusterIDs, now)
		if statsErr != nil {
			logger.WarnF(ctx, "集群健康远程告警计数失败，继续合并本地/同步告警: %v / clusters health remote alert stats failed, still merging local/sync alerts: %v", statsErr, statsErr)
		} else {
			statsMap = remoteStats
		}
		// 集群健康 / 首页 KPI 与告警中心同一口径：远程指标告警 + 同步作业告警 + 本地进程事件。
		// 本地/同步合并失败时降级为仅远程计数，避免整页 KPI 空白。
		// Cluster health / home KPIs match the alert center: remote metric alerts + sync job alerts + local process events.
		// If merging local/sync alerts fails, keep remote counts so the page still has KPIs.
		if err := s.mergeFiringAlertInstanceStats(ctx, statsMap, now, clusterID); err != nil {
			logger.WarnF(ctx, "集群健康合并本地/同步告警失败，仅保留远程告警计数: %v / clusters health merge of local/sync alerts failed, keeping remote counts: %v", err, err)
		}
	}

	items := make([]*ClusterHealthItem, 0, len(clusters))
	for _, c := range clusters {
		if c == nil {
			continue
		}
		status, statusErr := s.clusterService.GetStatus(ctx, c.ID)
		item := &ClusterHealthItem{
			ClusterID:    c.ID,
			ClusterName:  strings.TrimSpace(c.Name),
			HealthStatus: "unknown",
		}

		if statusErr == nil && status != nil {
			item.Status = string(status.Status)
			item.HealthStatus = strings.ToLower(strings.TrimSpace(string(status.HealthStatus)))
			item.TotalNodes = status.TotalNodes
			item.OnlineNodes = status.OnlineNodes
			item.OfflineNodes = status.OfflineNodes
		}

		idKey := strconv.FormatUint(uint64(c.ID), 10)
		if stat := statsMap[idKey]; stat != nil {
			item.ActiveAlerts = stat.ActiveCount
			item.CriticalAlerts = stat.CriticalCount
		}

		switch {
		case item.CriticalAlerts > 0:
			item.HealthStatus = "unhealthy"
		case item.ActiveAlerts > 0 && item.HealthStatus != "unhealthy":
			item.HealthStatus = "degraded"
		case item.HealthStatus == "":
			item.HealthStatus = "unknown"
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ClusterID < items[j].ClusterID
	})
	return &ClusterHealthData{
		GeneratedAt: timex.Now(),
		Total:       len(items),
		Clusters:    items,
	}, nil
}

// GetPlatformHealth returns platform-level health summary.
// GetPlatformHealth 返回平台级健康摘要。
func (s *Service) GetPlatformHealth(ctx context.Context) (*PlatformHealthData, error) {
	clusterHealth, err := s.GetClustersHealth(ctx)
	if err != nil {
		return nil, err
	}

	result := &PlatformHealthData{
		GeneratedAt:   timex.Now(),
		HealthStatus:  "unknown",
		TotalClusters: clusterHealth.Total,
	}

	for _, item := range clusterHealth.Clusters {
		if item == nil {
			continue
		}
		result.ActiveAlerts += item.ActiveAlerts
		result.CriticalAlerts += item.CriticalAlerts

		switch strings.ToLower(strings.TrimSpace(item.HealthStatus)) {
		case "healthy":
			result.HealthyClusters++
		case "degraded":
			result.DegradedClusters++
		case "unhealthy":
			result.UnhealthyClusters++
		default:
			result.UnknownClusters++
		}
	}

	switch {
	case result.UnhealthyClusters > 0:
		result.HealthStatus = "unhealthy"
	case result.DegradedClusters > 0:
		result.HealthStatus = "degraded"
	case result.TotalClusters > 0 && result.HealthyClusters == result.TotalClusters:
		result.HealthStatus = "healthy"
	default:
		result.HealthStatus = "unknown"
	}

	return result, nil
}

// activeRemoteAlertStats 按集群统计活跃远程告警：未恢复且没有被人工关闭，与告警中心「触发中」的口径一致。
// 只看 SQL 里的 status 会把已在告警中心关闭的告警继续算作活跃，首页与集群健康就会和告警列表对不上。
// activeRemoteAlertStats counts active remote alerts per cluster: unresolved and not closed by an operator, matching
// the alert center's "firing" view. Counting only the SQL status would keep alerts closed in the alert center
// "active", so the home page and cluster health would disagree with the alert list.
func (s *Service) activeRemoteAlertStats(ctx context.Context, clusterIDs []string, now time.Time) (map[string]*RemoteAlertClusterStat, error) {
	rows, err := s.repo.ListUnresolvedRemoteAlerts(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, buildRemoteAlertSourceKey(row.Fingerprint, row.StartsAt))
	}
	states, err := s.repo.ListAlertStatesBySourceKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*RemoteAlertClusterStat)
	for _, row := range rows {
		if resolveAlertHandlingStatus(states[buildRemoteAlertSourceKey(row.Fingerprint, row.StartsAt)], now) == AlertHandlingStatusClosed {
			continue
		}
		stat := result[row.ClusterID]
		if stat == nil {
			stat = &RemoteAlertClusterStat{ClusterID: row.ClusterID}
			result[row.ClusterID] = stat
		}
		stat.ActiveCount++
		if strings.EqualFold(strings.TrimSpace(row.Severity), "critical") {
			stat.CriticalCount++
		}
	}
	return result, nil
}

// mergeFiringAlertInstanceStats 把同步作业告警和本地进程事件并入集群健康计数；远程指标告警已由 activeRemoteAlertStats 计入。
// mergeFiringAlertInstanceStats adds sync job alerts and local process events to cluster health counts; remote metric
// alerts are already counted by activeRemoteAlertStats.
func (s *Service) mergeFiringAlertInstanceStats(ctx context.Context, stats map[string]*RemoteAlertClusterStat, now time.Time, clusterID uint) error {
	if stats == nil {
		return nil
	}
	since := now.Add(-clustersHealthAlertLookback)
	syncFilter := &AlertInstanceFilter{
		Status:    AlertDisplayStatusFiring,
		Unbounded: true,
	}
	localFilter := &AlertInstanceFilter{
		Status:    AlertDisplayStatusFiring,
		StartTime: &since,
	}
	if clusterID > 0 {
		id := strconv.FormatUint(uint64(clusterID), 10)
		syncFilter.ClusterID = id
		localFilter.ClusterID = id
	}
	syncItems, err := s.buildSyncAlertInstances(ctx, syncFilter, now)
	if err != nil {
		return err
	}
	addFiringAlertInstanceCounts(stats, syncItems)
	localItems, err := s.buildLocalAlertInstances(ctx, localFilter, now)
	if err != nil {
		return err
	}
	addFiringAlertInstanceCounts(stats, localItems)
	return nil
}

func addFiringAlertInstanceCounts(stats map[string]*RemoteAlertClusterStat, items []*AlertInstance) {
	for _, item := range items {
		if item == nil || item.Status != AlertDisplayStatusFiring {
			continue
		}
		clusterID := strings.TrimSpace(item.ClusterID)
		if clusterID == "" {
			continue
		}
		stat := stats[clusterID]
		if stat == nil {
			stat = &RemoteAlertClusterStat{ClusterID: clusterID}
			stats[clusterID] = stat
		}
		stat.ActiveCount++
		if strings.EqualFold(strings.TrimSpace(string(item.Severity)), string(AlertSeverityCritical)) {
			stat.CriticalCount++
		}
	}
}
