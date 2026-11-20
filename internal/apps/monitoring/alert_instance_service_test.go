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
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
	"github.com/stretchr/testify/require"
)

func TestService_ListAlertInstances_usesDisplayStatusAndCloseFlow(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(nil, nil, repo)

	startsAt := time.Date(2026, 3, 10, 1, 0, 0, 0, time.UTC)
	resolvedAt := startsAt.Add(8 * time.Minute)
	record := &RemoteAlertRecord{
		Fingerprint:    "fp-resolved-demo",
		StartsAt:       startsAt.Unix(),
		Status:         "resolved",
		AlertName:      "CPU usage high",
		Severity:       "critical",
		ClusterID:      "6",
		ClusterName:    "t3",
		Summary:        "CPU usage high on worker node",
		Description:    "cpu > 80% for 5m",
		ResolvedAt:     &resolvedAt,
		LastReceivedAt: resolvedAt,
	}
	if err := database.WithContext(ctx).Create(record).Error; err != nil {
		t.Fatalf("failed to create remote alert record: %v", err)
	}

	alerts, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{
		SourceType: AlertSourceTypeMetric,
	})
	if err != nil {
		t.Fatalf("ListAlertInstances returned error: %v", err)
	}
	if len(alerts.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts.Alerts))
	}
	if alerts.Alerts[0].Status != AlertDisplayStatusResolved {
		t.Fatalf("expected resolved display status, got %s", alerts.Alerts[0].Status)
	}
	if alerts.Stats.Resolved != 1 || alerts.Stats.Closed != 0 || alerts.Stats.Firing != 0 {
		t.Fatalf("unexpected stats before close: %+v", alerts.Stats)
	}

	action, err := service.CloseAlertInstance(ctx, buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt), "admin", "handled")
	if err != nil {
		t.Fatalf("CloseAlertInstance returned error: %v", err)
	}
	if action.Status != AlertDisplayStatusClosed {
		t.Fatalf("expected action status closed, got %s", action.Status)
	}
	if action.ClosedBy != "admin" || action.ClosedAt == nil {
		t.Fatalf("expected close metadata, got %+v", action)
	}

	alerts, err = service.ListAlertInstances(ctx, &AlertInstanceFilter{
		SourceType: AlertSourceTypeMetric,
	})
	if err != nil {
		t.Fatalf("ListAlertInstances after close returned error: %v", err)
	}
	if len(alerts.Alerts) != 1 {
		t.Fatalf("expected 1 alert after close, got %d", len(alerts.Alerts))
	}
	if alerts.Alerts[0].Status != AlertDisplayStatusClosed {
		t.Fatalf("expected closed display status after close, got %s", alerts.Alerts[0].Status)
	}
	if alerts.Alerts[0].ClosedBy != "admin" || alerts.Alerts[0].ClosedAt == nil {
		t.Fatalf("expected closed metadata on alert item, got %+v", alerts.Alerts[0])
	}
	if alerts.Stats.Closed != 1 || alerts.Stats.Resolved != 0 || alerts.Stats.Firing != 0 {
		t.Fatalf("unexpected stats after close: %+v", alerts.Stats)
	}

	filtered, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{
		SourceType: AlertSourceTypeMetric,
		Status:     AlertDisplayStatusClosed,
	})
	if err != nil {
		t.Fatalf("ListAlertInstances closed filter returned error: %v", err)
	}
	if len(filtered.Alerts) != 1 || filtered.Alerts[0].Status != AlertDisplayStatusClosed {
		t.Fatalf("expected one closed alert in closed filter, got %+v", filtered.Alerts)
	}
}

func TestService_CloseAlertInstance_allowsFiringAlert(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(nil, nil, repo)

	startsAt := time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC)
	record := &RemoteAlertRecord{
		Fingerprint:    "fp-firing-demo",
		StartsAt:       startsAt.Unix(),
		Status:         "firing",
		AlertName:      "Memory usage high",
		Severity:       "warning",
		ClusterID:      "6",
		ClusterName:    "t3",
		Summary:        "Memory usage high on worker node",
		Description:    "memory > 80% for 5m",
		LastReceivedAt: startsAt.Add(2 * time.Minute),
	}
	if err := database.WithContext(ctx).Create(record).Error; err != nil {
		t.Fatalf("failed to create firing remote alert record: %v", err)
	}

	action, err := service.CloseAlertInstance(ctx, buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt), "admin", "handled by human")
	if err != nil {
		t.Fatalf("expected firing alert to be manually closable, got %v", err)
	}
	if action.Status != AlertDisplayStatusClosed {
		t.Fatalf("expected closed status, got %s", action.Status)
	}
	if action.ClosedBy != "admin" || action.ClosedAt == nil {
		t.Fatalf("expected close metadata, got %+v", action)
	}
}

func TestParseAlertSourceKeyRejectsNumericEventID(t *testing.T) {
	if _, err := parseAlertSourceKey("9"); err == nil {
		t.Fatal("plain numeric event id must be rejected")
	}

	canonical, err := parseAlertSourceKey("local:event:9")
	if err != nil {
		t.Fatalf("parse canonical alert id: %v", err)
	}
	if canonical.Kind != alertStoreLocalEvent || canonical.SourceKey != "local:event:9" || canonical.EventID != 9 {
		t.Fatalf("canonical = %+v", canonical)
	}
}

// firing 记录在三种情况下按过期判为已恢复：ends_at 已过（内嵌评估器停止重推）、
// ends_at=0 且 last_received_at 超阈值（外部 Alertmanager 时代记录）、ends_at 未过期保持 firing。
// Firing records count as resolved on expiry in three cases: ends_at elapsed (the embedded
// evaluator stopped resending), ends_at=0 with last_received_at past the threshold (records
// written by the external Alertmanager era), and unexpired ends_at staying firing.
func TestResolveRemoteLifecycleStatus_expiresFiringRecords(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name           string
		status         string
		endsAt         int64
		lastReceivedAt time.Time
		want           AlertLifecycleStatus
		wantResolvedAt bool
	}{
		{name: "resolved status stays resolved", status: "resolved", endsAt: 0, want: AlertLifecycleStatusResolved},
		{name: "firing past ends_at resolves", status: "firing", endsAt: now.Add(-time.Minute).Unix(), want: AlertLifecycleStatusResolved, wantResolvedAt: true},
		{name: "firing before ends_at stays firing", status: "firing", endsAt: now.Add(time.Hour).Unix(), want: AlertLifecycleStatusFiring},
		{name: "legacy record without ends_at expires by last_received_at", status: "firing", endsAt: 0, lastReceivedAt: now.Add(-2 * time.Hour), want: AlertLifecycleStatusResolved, wantResolvedAt: true},
		{name: "fresh record without ends_at stays firing", status: "firing", endsAt: 0, lastReceivedAt: now.Add(-time.Minute), want: AlertLifecycleStatusFiring},
		{name: "zero last_received_at stays firing", status: "firing", endsAt: 0, want: AlertLifecycleStatusFiring},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, resolvedAt := resolveRemoteLifecycleStatus(tc.status, tc.endsAt, tc.lastReceivedAt, now)
			if got != tc.want {
				t.Fatalf("status=%s ends_at=%d last=%v => %s, want %s", tc.status, tc.endsAt, tc.lastReceivedAt, got, tc.want)
			}
			if (resolvedAt != nil) != tc.wantResolvedAt {
				t.Fatalf("inferred resolved_at=%v, want present=%v", resolvedAt, tc.wantResolvedAt)
			}
		})
	}
}

// TestService_ListAlertInstances_scopeFilter 覆盖 scope 派生与过滤：metric 远程告警 →
// cluster，任务级 sync 阶段 → job；scope 过滤与 source_type 并存。
// TestService_ListAlertInstances_scopeFilter covers scope derivation and filtering: a metric
// remote alert maps to cluster, a task-level sync episode to job; the scope filter coexists
// with source_type.
func TestService_ListAlertInstances_scopeFilter(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(nil, nil, repo)

	// 本地事件查询 JOIN hosts / clusters，测试只关心 scope 派生，建空表即可。
	// The local-event query JOINs hosts / clusters; the test only cares about scope derivation,
	// so empty stub tables are enough.
	require.NoError(t, database.Exec("CREATE TABLE IF NOT EXISTS hosts (id INTEGER PRIMARY KEY, name TEXT, ip_address TEXT)").Error)
	require.NoError(t, database.Exec("CREATE TABLE IF NOT EXISTS clusters (id INTEGER PRIMARY KEY, name TEXT)").Error)

	startsAt := time.Date(2026, 3, 10, 1, 0, 0, 0, time.UTC)
	record := &RemoteAlertRecord{
		Fingerprint:    "fp-scope-cluster",
		StartsAt:       startsAt.Unix(),
		Status:         "firing",
		AlertName:      "CPU usage high",
		Severity:       "warning",
		ClusterID:      "6",
		LastReceivedAt: startsAt,
	}
	if err := database.WithContext(ctx).Create(record).Error; err != nil {
		t.Fatalf("failed to create remote alert record: %v", err)
	}
	episode := &SyncAlertEpisode{
		Scope:           SyncAlertEpisodeScopeTask,
		ClusterID:       6,
		TaskID:          9,
		RuleKey:         syncalert.RuleJobFailed,
		Status:          SyncAlertEpisodeFiring,
		Severity:        AlertSeverityCritical,
		FirstFiredAt:    startsAt,
		LastOccurredAt:  startsAt,
		OccurrenceCount: 1,
	}
	if err := database.WithContext(ctx).Create(episode).Error; err != nil {
		t.Fatalf("failed to create sync alert episode: %v", err)
	}

	all, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{})
	if err != nil || len(all.Alerts) != 2 {
		t.Fatalf("expected 2 alerts without scope filter, got %+v err=%v", all, err)
	}
	scopeBySourceKey := map[string]string{}
	for _, item := range all.Alerts {
		scopeBySourceKey[item.AlertID] = item.Scope
	}
	if scopeBySourceKey[buildRemoteAlertSourceKey(record.Fingerprint, record.StartsAt)] != "cluster" {
		t.Fatalf("metric alert should derive cluster scope, got %+v", scopeBySourceKey)
	}
	if scopeBySourceKey[buildSyncAlertSourceKey(episode.ID)] != "job" {
		t.Fatalf("task episode should derive job scope, got %+v", scopeBySourceKey)
	}

	clusterOnly, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{Scope: "cluster"})
	if err != nil || len(clusterOnly.Alerts) != 1 || clusterOnly.Alerts[0].Scope != "cluster" {
		t.Fatalf("scope=cluster should keep only the remote alert, got %+v err=%v", clusterOnly, err)
	}
	jobOnly, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{Scope: "job"})
	if err != nil || len(jobOnly.Alerts) != 1 || jobOnly.Alerts[0].Scope != "job" {
		t.Fatalf("scope=job should keep only the episode, got %+v err=%v", jobOnly, err)
	}
}

func TestService_ListAlertInstances_mapsLegacyYamlRuleNames(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(nil, nil, repo)

	// 旧 YAML 告警没有 template_key label，只能靠告警名映射到目录规则。
	// Legacy YAML alerts carry no template_key label; only the stored alert name
	// can map them onto a catalog rule.
	require.NoError(t, database.Exec("CREATE TABLE IF NOT EXISTS hosts (id INTEGER PRIMARY KEY, name TEXT, ip_address TEXT)").Error)
	require.NoError(t, database.Exec("CREATE TABLE IF NOT EXISTS clusters (id INTEGER PRIMARY KEY, name TEXT)").Error)

	startsAt := time.Date(2026, 3, 10, 1, 0, 0, 0, time.UTC)
	legacy := &RemoteAlertRecord{
		Fingerprint:    "fp-legacy-node-offline",
		StartsAt:       startsAt.Unix(),
		Status:         "firing",
		AlertName:      "SeaTunnelMetricsEndpointDown",
		Severity:       "critical",
		ClusterID:      "6",
		LastReceivedAt: startsAt,
	}
	unknown := &RemoteAlertRecord{
		Fingerprint:    "fp-custom-alert",
		StartsAt:       startsAt.Unix(),
		Status:         "firing",
		AlertName:      "MyCustomAlert",
		Severity:       "warning",
		ClusterID:      "6",
		LastReceivedAt: startsAt,
	}
	for _, record := range []*RemoteAlertRecord{legacy, unknown} {
		if err := database.WithContext(ctx).Create(record).Error; err != nil {
			t.Fatalf("failed to create remote alert record: %v", err)
		}
	}

	alerts, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{SourceType: AlertSourceTypeMetric})
	if err != nil {
		t.Fatalf("ListAlertInstances returned error: %v", err)
	}
	if len(alerts.Alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts.Alerts))
	}

	byName := map[string]*AlertInstance{}
	for _, item := range alerts.Alerts {
		byName[item.SourceRef.OriginalAlertName+item.AlertID] = item
	}

	mapped := byName["SeaTunnelMetricsEndpointDown"+buildRemoteAlertSourceKey(legacy.Fingerprint, legacy.StartsAt)]
	if mapped == nil {
		t.Fatalf("legacy alert missing original name: %+v", alerts.Alerts)
	}
	if mapped.RuleKey != AlertRuleKeyNodeOffline {
		t.Fatalf("expected mapped rule key node_offline, got %q", mapped.RuleKey)
	}
	if mapped.AlertName != "节点离线" || mapped.AlertNameEN != "Node offline" {
		t.Fatalf("expected catalog display names, got %q / %q", mapped.AlertName, mapped.AlertNameEN)
	}

	for legacyName, wantKey := range map[string]string{
		"SeaTunnelNodeStateDown":             AlertRuleKeyNodeOffline,
		"SeaTunnelPartitionUnsafe":           "split_brain_risk",
		"SeaTunnelJobThreadPoolQueueHigh":    "job_thread_pool_queue_backlog_high",
		"SeaTunnelCoordinatorBacklogGrowing": "coordinator_backlog_growing",
		"SeaTunnelCoordinatorRejectingTasks": "job_thread_pool_rejection_high",
		"SeaTunnelJVMHeapUsageHigh":          "memory_usage_high",
		"SeaTunnelProcessCpuHigh":            "cpu_usage_high",
		"SeaTunnelGCTimeHigh":                "gc_time_high",
		"SeaTunnelFDUsageHigh":               "fd_usage_high",
		"SeaTunnelThreadDeadlock":            "deadlocked_threads_detected",
	} {
		if got := legacyAlertNameRuleKeys[legacyName]; got != wantKey {
			t.Fatalf("legacy mapping %s = %q, want %q", legacyName, got, wantKey)
		}
	}

	custom := byName[""+buildRemoteAlertSourceKey(unknown.Fingerprint, unknown.StartsAt)]
	if custom == nil {
		t.Fatalf("custom alert missing: %+v", alerts.Alerts)
	}
	if custom.RuleKey != "" || custom.AlertName != "MyCustomAlert" || custom.AlertNameEN != "" {
		t.Fatalf("unmapped alert should keep stored name and empty rule key, got %+v", custom)
	}
}
