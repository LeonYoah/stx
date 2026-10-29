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
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
)

// 复审 #85-1（Codex P2）：本地进程告警（节点离线）首次发送失败后，渠道恢复、节点仍离线时，下一轮巡检要重发首次告警。
// 退避重试只扫同步告警，所以"首次告警未送达就只交给退避重试"的限制只能用于同步告警。
// Re-review #85-1 (Codex P2): when a local process alert (node offline) fails its first send and the channel recovers
// while the node is still offline, the next sweep must resend the first alert. The backoff retry only scans sync
// alerts, so "an undelivered first alert is left to the backoff retry" may only apply to sync alerts.
func TestEvaluateNodeHealthAlerts_resendsFailedFirstNodeOfflineAlert(t *testing.T) {
	database, cleanup := setupMonitoringNodeOfflineTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	now := time.Now().UTC()

	clusterRepo := clusterapp.NewRepository(database)
	hostProvider := &stubNodeOfflineHostProvider{
		hosts: map[uint]*clusterapp.HostInfo{
			4: {
				ID:            4,
				Name:          "host-4",
				IPAddress:     "10.0.0.4",
				AgentID:       "agent-4",
				AgentStatus:   "installed",
				LastHeartbeat: ptrTime(now),
			},
		},
	}
	clusterService := clusterapp.NewService(clusterRepo, hostProvider, &clusterapp.ServiceConfig{
		HeartbeatTimeout: 30 * time.Second,
	})

	monitorRepo := monitor.NewRepository(database)
	monitorService := monitor.NewService(monitorRepo)
	repo := NewRepository(database)
	service := NewService(clusterService, monitorService, repo)
	monitorService.SetOnEventRecorded(service.DispatchAlertPolicyEvent)

	testCluster := &clusterapp.Cluster{
		Name:           "node-offline-first-send-failed",
		Description:    "test cluster",
		DeploymentMode: clusterapp.DeploymentModeHybrid,
		Version:        "2.3.11",
		Status:         clusterapp.ClusterStatusRunning,
		InstallDir:     "/opt/seatunnel",
		CreatedBy:      1,
	}
	if err := database.WithContext(ctx).Create(testCluster).Error; err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}

	node := &clusterapp.ClusterNode{
		ClusterID:     testCluster.ID,
		HostID:        4,
		Role:          clusterapp.NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		WorkerPort:    5802,
		Status:        clusterapp.NodeStatusStopped,
		ProcessPID:    0,
	}
	if err := database.WithContext(ctx).Create(node).Error; err != nil {
		t.Fatalf("failed to create cluster node: %v", err)
	}

	stoppedSince := now.Add(-25 * time.Second)
	if err := database.WithContext(ctx).
		Model(&clusterapp.ClusterNode{}).
		Where("id = ?", node.ID).
		Update("updated_at", stoppedSince).
		Error; err != nil {
		t.Fatalf("failed to backdate node updated_at: %v", err)
	}

	if err := monitorRepo.CreateConfig(ctx, &monitor.MonitorConfig{
		ClusterID:       testCluster.ID,
		AutoMonitor:     true,
		AutoRestart:     false,
		MonitorInterval: 5,
		RestartDelay:    10,
		MaxRestarts:     3,
		TimeWindow:      300,
		CooldownPeriod:  1800,
		ConfigVersion:   1,
	}); err != nil {
		t.Fatalf("failed to create monitor config: %v", err)
	}

	var hitCount atomic.Int32
	var statusCode atomic.Int32
	statusCode.Store(http.StatusInternalServerError)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		hitCount.Add(1)
		w.WriteHeader(int(statusCode.Load()))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "node-offline-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	_, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "Node offline policy",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyNodeOffline,
		ClusterID:              strconv.FormatUint(uint64(testCluster.ID), 10),
		Severity:               AlertSeverityCritical,
		NotificationChannelIDs: []uint{channel.ID},
	})
	if err != nil {
		t.Fatalf("failed to create alert policy: %v", err)
	}
	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("EvaluateNodeHealthAlerts returned error: %v", err)
	}
	if !waitForCondition(2*time.Second, func() bool { return hitCount.Load() == 1 }) {
		t.Fatalf("expected the first (failing) delivery attempt, got %d", hitCount.Load())
	}
	offlineEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{monitor.EventTypeNodeOffline})
	if err != nil || offlineEvent == nil {
		t.Fatalf("expected node_offline event, got %+v err=%v", offlineEvent, err)
	}
	sourceKey := buildLocalAlertSourceKey(offlineEvent.ID)
	if !waitForCondition(2*time.Second, func() bool {
		delivery, err := repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
		return err == nil && delivery != nil && delivery.Status == string(NotificationDeliveryStatusFailed)
	}) {
		t.Fatal("expected the first delivery to be recorded as failed")
	}

	// 渠道恢复，节点仍离线：下一轮巡检重发首次告警，之后不再重复。
	// The channel recovers while the node is still offline: the next sweep resends the first alert, then no duplicates.
	statusCode.Store(http.StatusOK)
	for i := 0; i < 3; i++ {
		if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
			t.Fatalf("sweep %d returned error: %v", i+1, err)
		}
	}
	if hitCount.Load() != 2 {
		t.Fatalf("expected exactly one resend after the channel recovered, got %d requests", hitCount.Load())
	}
	delivery, err := repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
	if err != nil || delivery == nil || delivery.Status != string(NotificationDeliveryStatusSent) || delivery.AttemptCount != 2 {
		t.Fatalf("expected the first alert to be sent on attempt 2, got %+v err=%v", delivery, err)
	}
}
