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

// deliverRemoteAlertRecord 按记录归属选择投递路径：managed_by=stx 的策略告警走
// deliverManagedRemoteAlertPolicyNotifications（去重/冷却/静默/恢复语义所在地），
// 其余告警回落到 deliverRemoteAlertNotifications 的路由匹配投递。
// deliverRemoteAlertRecord picks the delivery path by record ownership: managed_by=stx policy
// alerts go through deliverManagedRemoteAlertPolicyNotifications (home of
// dedupe/cooldown/silence/recovery semantics), everything else falls back to route-match
// delivery in deliverRemoteAlertNotifications.
func (s *Service) deliverRemoteAlertRecord(ctx context.Context, record *RemoteAlertRecord, previousRecord *RemoteAlertRecord) error {
	handled, err := s.deliverManagedRemoteAlertPolicyNotifications(ctx, record, previousRecord)
	if err != nil {
		return err
	}
	if !handled {
		return s.deliverRemoteAlertNotifications(ctx, record)
	}
	return nil
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
	if s.clusterService == nil {
		return nil, fmt.Errorf("cluster service is not configured")
	}

	clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
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
		statsMap, err = s.activeRemoteAlertStats(ctx, clusterIDs, timex.Now())
		if err != nil {
			return nil, err
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

		clusterID := strconv.FormatUint(uint64(c.ID), 10)
		if stat := statsMap[clusterID]; stat != nil {
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
