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
	"strconv"
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
)

// TestClustersHealthIgnoresClosedRemoteAlerts 覆盖：告警中心已关闭的远程告警不再计入集群健康的活跃告警，
// 已恢复的也不计；确认 / 静默中的仍算活跃。
// TestClustersHealthIgnoresClosedRemoteAlerts: remote alerts closed in the alert center no longer count as active in
// cluster health, nor do resolved ones; acknowledged / silenced alerts still count.
func TestClustersHealthIgnoresClosedRemoteAlerts(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)
	if err := repo.db.AutoMigrate(&RemoteAlertRecord{}); err != nil {
		t.Fatalf("migrate remote alerts: %v", err)
	}
	clusterID := strconv.FormatUint(uint64(cluster.ID), 10)
	now := time.Now().UTC()
	rows := []*RemoteAlertRecord{
		{Fingerprint: "closed", StartsAt: 1, Status: "firing", Severity: "critical", ClusterID: clusterID, AlertName: "a", LastReceivedAt: now},
		{Fingerprint: "acked", StartsAt: 2, Status: "firing", Severity: "critical", ClusterID: clusterID, AlertName: "b", LastReceivedAt: now},
		{Fingerprint: "warn", StartsAt: 3, Status: "firing", Severity: "warning", ClusterID: clusterID, AlertName: "c", LastReceivedAt: now},
		{Fingerprint: "done", StartsAt: 4, Status: "resolved", Severity: "critical", ClusterID: clusterID, AlertName: "d", LastReceivedAt: now},
	}
	for _, row := range rows {
		if err := repo.db.Create(row).Error; err != nil {
			t.Fatalf("create remote alert: %v", err)
		}
	}
	for key, status := range map[string]AlertHandlingStatus{"closed": AlertHandlingStatusClosed, "acked": AlertHandlingStatusAcknowledged} {
		state := &AlertState{SourceType: AlertSourceTypeMetric, SourceKey: buildRemoteAlertSourceKey(key, map[string]int64{"closed": 1, "acked": 2}[key]), ClusterID: clusterID, HandlingStatus: status}
		if err := repo.db.Create(state).Error; err != nil {
			t.Fatalf("create alert state: %v", err)
		}
	}

	health, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("clusters health: %v", err)
	}
	if len(health.Clusters) != 1 || health.Clusters[0].ActiveAlerts != 2 || health.Clusters[0].CriticalAlerts != 1 {
		t.Fatalf("health = %+v, want active 2 (acked + warn), critical 1 (acked)", health.Clusters[0])
	}
}

func TestClustersHealthCountsFiringSyncAlerts(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)
	now := time.Now().UTC()
	episode := &SyncAlertEpisode{
		Scope:          SyncAlertEpisodeScopeTask,
		ClusterID:      cluster.ID,
		TaskID:         9,
		RuleKey:        "sync_submit_failed",
		Status:         SyncAlertEpisodeFiring,
		Severity:       AlertSeverityCritical,
		ClusterName:    cluster.Name,
		TaskName:       "broken",
		FirstFiredAt:   now,
		LastOccurredAt: now,
	}
	if err := repo.db.Create(episode).Error; err != nil {
		t.Fatalf("create sync episode: %v", err)
	}
	health, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("clusters health: %v", err)
	}
	if len(health.Clusters) != 1 || health.Clusters[0].ActiveAlerts != 1 || health.Clusters[0].CriticalAlerts != 1 {
		t.Fatalf("health = %+v, want active 1 critical 1 from sync_submit_failed", health.Clusters[0])
	}
}

func TestClustersHealthCountsUnrecoveredLocalAlerts(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)
	event := &monitor.ProcessEvent{
		ClusterID: cluster.ID,
		NodeID:    11,
		EventType: monitor.EventTypeNodeOffline,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	if err := repo.db.Create(event).Error; err != nil {
		t.Fatalf("create process event: %v", err)
	}

	health, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("clusters health: %v", err)
	}
	if len(health.Clusters) != 1 || health.Clusters[0].ActiveAlerts != 1 || health.Clusters[0].CriticalAlerts != 1 {
		t.Fatalf("health = %+v, want active 1 critical 1 for unrecovered node_offline", health.Clusters[0])
	}
}

func TestClustersHealthUsesShortCache(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)
	oldTTL := clustersHealthCacheTTL
	clustersHealthCacheTTL = time.Hour
	t.Cleanup(func() { clustersHealthCacheTTL = oldTTL })

	first, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("first health: %v", err)
	}
	if first.Clusters[0].ActiveAlerts != 0 {
		t.Fatalf("want 0 alerts before insert, got %+v", first.Clusters[0])
	}

	episode := &SyncAlertEpisode{
		Scope:          SyncAlertEpisodeScopeTask,
		ClusterID:      cluster.ID,
		TaskID:         3,
		RuleKey:        "sync_submit_failed",
		Status:         SyncAlertEpisodeFiring,
		Severity:       AlertSeverityCritical,
		ClusterName:    cluster.Name,
		TaskName:       "cached",
		FirstFiredAt:   time.Now().UTC(),
		LastOccurredAt: time.Now().UTC(),
	}
	if err := repo.db.Create(episode).Error; err != nil {
		t.Fatalf("create sync episode: %v", err)
	}

	cached, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("cached health: %v", err)
	}
	if cached.Clusters[0].ActiveAlerts != 0 {
		t.Fatalf("cached health should keep first snapshot, got %+v", cached.Clusters[0])
	}

	service.clustersHealthMu.Lock()
	if service.clustersHealthUntil == nil {
		service.clustersHealthUntil = make(map[uint]time.Time)
	}
	service.clustersHealthUntil[0] = time.Now().Add(-time.Second)
	service.clustersHealthMu.Unlock()
	fresh, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("fresh health: %v", err)
	}
	if fresh.Clusters[0].ActiveAlerts != 1 || fresh.Clusters[0].CriticalAlerts != 1 {
		t.Fatalf("health after cache expiry = %+v, want active 1 critical 1", fresh.Clusters[0])
	}
}

func TestClustersHealthCacheClearedAfterClose(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)
	oldTTL := clustersHealthCacheTTL
	clustersHealthCacheTTL = time.Hour
	t.Cleanup(func() { clustersHealthCacheTTL = oldTTL })

	now := time.Now().UTC()
	episode := &SyncAlertEpisode{
		Scope:          SyncAlertEpisodeScopeTask,
		ClusterID:      cluster.ID,
		TaskID:         9,
		RuleKey:        "sync_submit_failed",
		Status:         SyncAlertEpisodeFiring,
		Severity:       AlertSeverityCritical,
		ClusterName:    cluster.Name,
		TaskName:       "close-cache",
		FirstFiredAt:   now,
		LastOccurredAt: now,
	}
	if err := repo.db.Create(episode).Error; err != nil {
		t.Fatalf("create sync episode: %v", err)
	}
	first, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("first health: %v", err)
	}
	if first.Clusters[0].ActiveAlerts != 1 {
		t.Fatalf("want 1 firing alert cached, got %+v", first.Clusters[0])
	}

	if _, err := service.CloseAlertInstance(ctx, buildSyncAlertSourceKey(episode.ID), "ops", "handled"); err != nil {
		t.Fatalf("close instance: %v", err)
	}
	fresh, err := service.GetClustersHealth(ctx)
	if err != nil {
		t.Fatalf("health after close: %v", err)
	}
	if fresh.Clusters[0].ActiveAlerts != 0 {
		t.Fatalf("health after close must drop the closed alert immediately, got %+v", fresh.Clusters[0])
	}
}

func TestClustersHealthForOnlyCountsRequestedCluster(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)
	other := &clusterapp.Cluster{
		Name:           "other-health-cluster",
		Description:    "other cluster",
		DeploymentMode: clusterapp.DeploymentModeHybrid,
		Version:        "2.3.0",
		Status:         clusterapp.ClusterStatusCreated,
		InstallDir:     "/tmp/seatunnel-other",
		CreatedBy:      1,
	}
	if err := repo.db.WithContext(ctx).Create(other).Error; err != nil {
		t.Fatalf("create other cluster: %v", err)
	}
	now := time.Now().UTC()
	for _, item := range []*SyncAlertEpisode{
		{Scope: SyncAlertEpisodeScopeTask, ClusterID: cluster.ID, TaskID: 1, RuleKey: "sync_submit_failed", Status: SyncAlertEpisodeFiring, Severity: AlertSeverityCritical, ClusterName: cluster.Name, TaskName: "a", FirstFiredAt: now, LastOccurredAt: now},
		{Scope: SyncAlertEpisodeScopeTask, ClusterID: other.ID, TaskID: 2, RuleKey: "sync_submit_failed", Status: SyncAlertEpisodeFiring, Severity: AlertSeverityCritical, ClusterName: other.Name, TaskName: "b", FirstFiredAt: now, LastOccurredAt: now},
	} {
		if err := repo.db.Create(item).Error; err != nil {
			t.Fatalf("create sync episode: %v", err)
		}
	}

	all, err := service.GetClustersHealthFor(ctx, 0)
	if err != nil {
		t.Fatalf("all clusters health: %v", err)
	}
	if all.Total != 2 {
		t.Fatalf("want 2 clusters, got %+v", all)
	}
	scoped, err := service.GetClustersHealthFor(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("scoped health: %v", err)
	}
	if scoped.Total != 1 || scoped.Clusters[0].ClusterID != cluster.ID {
		t.Fatalf("scoped health should be the requested cluster, got %+v", scoped)
	}
	if scoped.Clusters[0].ActiveAlerts != 1 || scoped.Clusters[0].CriticalAlerts != 1 {
		t.Fatalf("scoped health = %+v, want only the requested cluster's 1 firing alert", scoped.Clusters[0])
	}
}
