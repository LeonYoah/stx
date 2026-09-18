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
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
)

// BuildPrometheusSDTargets builds Prometheus HTTP SD target groups from managed clusters.
// BuildPrometheusSDTargets 从受管集群构建 Prometheus HTTP SD 目标组。
func (s *Service) BuildPrometheusSDTargets(ctx context.Context) ([]*PrometheusSDTargetGroup, error) {
	// now not do metrics probe in HTTP SD stage, directly build targets based on cluster/node metadata,
	// let Prometheus itself handle the fetch and health check.
	// 不再在 HTTP SD 阶段做指标探活，直接基于集群/节点元数据构造 targets，
	// 由 Prometheus 自己负责抓取与健康判定。
	targets, err := s.collectManagedMetricsTargets(ctx, false)
	if err != nil {
		return nil, err
	}

	groupMap := make(map[string]*PrometheusSDTargetGroup)
	seen := make(map[string]map[string]struct{})
	groupKeys := make([]string, 0, 8)

	for _, item := range targets {
		if item == nil {
			continue
		}

		clusterID := strconv.FormatUint(uint64(item.ClusterID), 10)
		if item.ClusterID == 0 {
			clusterID = "static"
		}
		clusterName := strings.TrimSpace(item.ClusterName)
		if clusterName == "" {
			clusterName = "unknown"
		}
		env := strings.TrimSpace(item.Env)
		if env == "" {
			env = "unknown"
		}

		key := clusterID + "|" + clusterName + "|" + env
		group, ok := groupMap[key]
		if !ok {
			group = &PrometheusSDTargetGroup{
				Targets: make([]string, 0, 2),
				Labels: map[string]string{
					"job":          "seatunnel_engine_http",
					"cluster_id":   clusterID,
					"cluster_name": clusterName,
					"cluster":      clusterName, // Grafana 大盘变量 $cluster 使用此 label
					"env":          env,
				},
			}
			groupMap[key] = group
			groupKeys = append(groupKeys, key)
			seen[key] = make(map[string]struct{})
		}
		if _, ok := seen[key][item.Target]; ok {
			continue
		}
		seen[key][item.Target] = struct{}{}
		group.Targets = append(group.Targets, item.Target)
	}

	sort.Strings(groupKeys)
	result := make([]*PrometheusSDTargetGroup, 0, len(groupKeys))
	for _, key := range groupKeys {
		group := groupMap[key]
		sort.Strings(group.Targets)
		result = append(result, group)
	}
	return result, nil
}

// HandleAlertmanagerWebhook ingests one Alertmanager webhook payload into persistent records.
// HandleAlertmanagerWebhook 将 Alertmanager webhook 请求写入持久化告警记录。
func (s *Service) HandleAlertmanagerWebhook(ctx context.Context, payload *AlertmanagerWebhookPayload) (*AlertmanagerWebhookResult, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("monitoring repository is not configured")
	}
	if payload == nil {
		return nil, fmt.Errorf("empty webhook payload")
	}

	result := &AlertmanagerWebhookResult{
		Received: len(payload.Alerts),
		Stored:   0,
		Errors:   make([]string, 0, 2),
	}
	now := time.Now().UTC()
	for idx, alert := range payload.Alerts {
		if alert == nil {
			continue
		}

		record := normalizeWebhookAlert(payload, alert, now)
		previousRecord, err := s.repo.GetRemoteAlertByFingerprintAndStartsAt(ctx, record.Fingerprint, record.StartsAt)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("alert[%d] lookup failed: %v", idx, err))
			continue
		}
		if err := s.repo.UpsertRemoteAlert(ctx, record); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("alert[%d] upsert failed: %v", idx, err))
			continue
		}
		handled, err := s.deliverManagedRemoteAlertPolicyNotifications(ctx, record, previousRecord)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("alert[%d] delivery failed: %v", idx, err))
		} else if !handled {
			if err := s.deliverRemoteAlertNotifications(ctx, record); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("alert[%d] delivery failed: %v", idx, err))
			}
		}
		result.Stored++
	}
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result, nil
}

func normalizeWebhookAlert(payload *AlertmanagerWebhookPayload, alert *WebhookAlert, now time.Time) *RemoteAlertRecord {
	labels := mergeStringMap(payload.CommonLabels, alert.Labels)
	annotations := mergeStringMap(payload.CommonAnnotations, alert.Annotations)

	startsAt := alert.StartsAt.UTC()
	if startsAt.IsZero() {
		startsAt = now
	}

	status := strings.TrimSpace(alert.Status)
	if status == "" {
		status = strings.TrimSpace(payload.Status)
	}
	if status == "" {
		status = "firing"
	}

	clusterID := strings.TrimSpace(labels["cluster_id"])
	if clusterID == "" {
		clusterID = "unknown"
	}
	clusterName := strings.TrimSpace(labels["cluster_name"])
	if clusterName == "" {
		clusterName = "unknown"
	}
	env := strings.TrimSpace(labels["env"])
	if env == "" {
		env = "unknown"
	}

	fingerprint := strings.TrimSpace(alert.Fingerprint)
	if fingerprint == "" {
		fingerprint = buildFallbackFingerprint(labels, startsAt, alert.GeneratorURL)
	}

	alertName := strings.TrimSpace(firstNonEmpty(
		labels["policy_name"],
		annotations["policy_name"],
		labels["alertname"],
	))
	if alertName == "" {
		alertName = "unknown_alert"
	}

	labelsJSON := mustMarshalJSON(labels)
	annotationsJSON := mustMarshalJSON(annotations)

	var endsAtUnix int64
	var resolvedAt *time.Time
	if !alert.EndsAt.IsZero() {
		endsAt := alert.EndsAt.UTC()
		endsAtUnix = endsAt.Unix()
		if strings.EqualFold(status, "resolved") {
			resolvedAt = &endsAt
		}
	}
	if strings.EqualFold(status, "resolved") && resolvedAt == nil {
		t := now
		resolvedAt = &t
	}

	return &RemoteAlertRecord{
		Fingerprint:     fingerprint,
		StartsAt:        startsAt.Unix(),
		Status:          status,
		Receiver:        strings.TrimSpace(payload.Receiver),
		AlertName:       alertName,
		Severity:        strings.TrimSpace(labels["severity"]),
		ClusterID:       clusterID,
		ClusterName:     clusterName,
		Env:             env,
		GeneratorURL:    strings.TrimSpace(alert.GeneratorURL),
		Summary:         strings.TrimSpace(firstNonEmpty(annotations["summary"], annotations["message"])),
		Description:     strings.TrimSpace(firstNonEmpty(annotations["description"], annotations["details"])),
		LabelsJSON:      labelsJSON,
		AnnotationsJSON: annotationsJSON,
		EndsAt:          endsAtUnix,
		ResolvedAt:      resolvedAt,
		LastReceivedAt:  now,
	}
}

func mergeStringMap(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
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

func buildFallbackFingerprint(labels map[string]string, startsAt time.Time, generatorURL string) string {
	raw := mustMarshalJSON(labels) + "|" + startsAt.UTC().Format(time.RFC3339Nano) + "|" + strings.TrimSpace(generatorURL)
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
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
		GeneratedAt: time.Now().UTC(),
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
		statsMap, err = s.repo.GetRemoteAlertClusterStats(ctx, clusterIDs)
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
		GeneratedAt: time.Now().UTC(),
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
		GeneratedAt:   time.Now().UTC(),
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
