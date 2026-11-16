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
		state := &AlertState{SourceType: AlertSourceTypeRemoteAlertmanager, SourceKey: buildRemoteAlertSourceKey(key, map[string]int64{"closed": 1, "acked": 2}[key]), ClusterID: clusterID, HandlingStatus: status}
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
