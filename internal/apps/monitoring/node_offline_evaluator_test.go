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
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type stubNodeOfflineHostProvider struct {
	hosts map[uint]*clusterapp.HostInfo
}

func (s *stubNodeOfflineHostProvider) GetHostByID(_ context.Context, id uint) (*clusterapp.HostInfo, error) {
	if host, ok := s.hosts[id]; ok {
		copyHost := *host
		return &copyHost, nil
	}
	return nil, clusterapp.ErrNodeNotFound
}

func setupMonitoringNodeOfflineTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "monitoring_node_offline_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	database, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("failed to open database: %v", err)
	}

	if err := database.AutoMigrate(
		&clusterapp.Cluster{},
		&clusterapp.ClusterNode{},
		&hostapp.Host{},
		&monitor.MonitorConfig{},
		&monitor.ProcessEvent{},
		&AlertRule{},
		&AlertPolicy{},
		&NotificationChannel{},
		&NotificationDelivery{},
		&AlertEventState{},
		&AlertState{},
		&RemoteAlertRecord{},
		&ClusterRuleConfig{},
		&ClusterRuleOverride{},
		&AlertNotificationDefaults{},
		&SyncAlertEpisode{},
	); err != nil {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("failed to migrate node offline test tables: %v", err)
	}

	cleanup := func() {
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.RemoveAll(tempDir)
	}
	return database, cleanup
}

func TestEvaluateAvailabilityAlerts_emitsAndResolvesNodeOfflineEpisode(t *testing.T) {
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
		Name:           "node-offline-test",
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
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

	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyNodeOffline, []uint{channel.ID})

	// 第一次评估只记 pending 起点，未持续满 for 不触发。
	// The first evaluation only records the pending start; nothing fires before for elapses.
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	offlineEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{monitor.EventTypeNodeOffline})
	if err != nil {
		t.Fatalf("failed to load node_offline event: %v", err)
	}
	if offlineEvent != nil {
		t.Fatalf("expected no node_offline event before the for window matured, got %+v", offlineEvent)
	}

	ageAvailabilityPending(service,
		availabilityPendingKey{RuleKey: AlertRuleKeyNodeOffline, ClusterID: testCluster.ID, NodeID: node.ID, HostID: node.HostID},
		time.Hour)

	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}

	offlineEvent, err = monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{monitor.EventTypeNodeOffline})
	if err != nil {
		t.Fatalf("failed to load node_offline event: %v", err)
	}
	if offlineEvent == nil {
		t.Fatal("expected node_offline event to be created")
	}
	if !waitForCondition(2*time.Second, func() bool {
		return hitCount.Load() == 1
	}) {
		t.Fatalf("expected exactly one notification delivery, got %d", hitCount.Load())
	}

	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("second EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected no duplicate delivery for active offline episode, got %d", hitCount.Load())
	}

	var offlineCount int64
	if err := database.WithContext(ctx).
		Model(&monitor.ProcessEvent{}).
		Where("node_id = ? AND event_type = ?", node.ID, monitor.EventTypeNodeOffline).
		Count(&offlineCount).Error; err != nil {
		t.Fatalf("failed to count node_offline events: %v", err)
	}
	if offlineCount != 1 {
		t.Fatalf("expected one node_offline event, got %d", offlineCount)
	}

	recoveredAt := time.Now().UTC()
	if err := database.WithContext(ctx).
		Model(&clusterapp.ClusterNode{}).
		Where("id = ?", node.ID).
		Updates(map[string]interface{}{
			"status":      clusterapp.NodeStatusRunning,
			"process_pid": 4242,
			"updated_at":  recoveredAt,
		}).Error; err != nil {
		t.Fatalf("failed to update node back to running: %v", err)
	}

	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("third EvaluateAvailabilityAlerts returned error: %v", err)
	}

	recoveredEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{monitor.EventTypeNodeRecovered})
	if err != nil {
		t.Fatalf("failed to load node_recovered event: %v", err)
	}
	if recoveredEvent == nil {
		t.Fatal("expected node_recovered event to be created")
	}
	if !recoveredEvent.CreatedAt.After(offlineEvent.CreatedAt) {
		t.Fatalf("expected recovery event after offline event, got offline=%s recovered=%s", offlineEvent.CreatedAt, recoveredEvent.CreatedAt)
	}

	// 等待告警实例状态异步更新为 resolved
	// Wait for alert instance lifecycle status to be asynchronously updated to resolved
	var alerts *AlertInstanceListData
	if !waitForCondition(3*time.Second, func() bool {
		var err error
		alerts, err = service.ListAlertInstances(ctx, &AlertInstanceFilter{
			ClusterID: strconv.FormatUint(uint64(testCluster.ID), 10),
			Page:      1,
			PageSize:  20,
		})
		return err == nil && alerts != nil && alerts.Total == 1 &&
			alerts.Alerts[0].LifecycleStatus == AlertLifecycleStatusResolved &&
			alerts.Alerts[0].ResolvedAt != nil
	}) {
		t.Fatalf("expected resolved node_offline alert instance, got %+v", alerts)
	}
}

func TestEvaluateAvailabilityAlerts_repeatsNodeOfflineReminderAfterCooldown(t *testing.T) {
	database, cleanup := setupMonitoringNodeOfflineTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	now := time.Now().UTC()

	clusterRepo := clusterapp.NewRepository(database)
	hostProvider := &stubNodeOfflineHostProvider{
		hosts: map[uint]*clusterapp.HostInfo{
			5: {
				ID:            5,
				Name:          "host-5",
				IPAddress:     "10.0.0.5",
				AgentID:       "agent-5",
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
		Name:           "node-offline-reminder-test",
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
		HostID:        5,
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "node-offline-repeat-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyNodeOffline, []uint{channel.ID})
	if _, err := service.SaveAlertNotificationDefaults(ctx, nil, nil, 1, true); err != nil {
		t.Fatalf("failed to save default notification cooldown: %v", err)
	}

	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("first EvaluateAvailabilityAlerts returned error: %v", err)
	}
	ageAvailabilityPending(service,
		availabilityPendingKey{RuleKey: AlertRuleKeyNodeOffline, ClusterID: testCluster.ID, NodeID: node.ID, HostID: node.HostID},
		time.Hour)
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("second EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if !waitForCondition(2*time.Second, func() bool {
		return hitCount.Load() == 1
	}) {
		t.Fatalf("expected initial notification delivery, got %d", hitCount.Load())
	}

	offlineEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{monitor.EventTypeNodeOffline})
	if err != nil {
		t.Fatalf("failed to load node_offline event: %v", err)
	}
	if offlineEvent == nil {
		t.Fatal("expected node_offline event to exist")
	}

	// 等待投递记录落库并更新为 sent 状态（由于事件回调通过后台异步协程处理，需要等待落库完成）
	// Wait for delivery record to be saved and updated to sent status (since event callback runs in background goroutine)
	var delivery *NotificationDelivery
	if !waitForCondition(3*time.Second, func() bool {
		var err error
		delivery, err = repo.GetNotificationDeliveryByDedupKey(
			ctx,
			buildLocalAlertSourceKey(offlineEvent.ID),
			channel.ID,
			string(NotificationDeliveryEventTypeFiring),
		)
		return err == nil && delivery != nil && delivery.SentAt != nil && NotificationDeliveryStatus(delivery.Status) == NotificationDeliveryStatusSent
	}) {
		t.Fatalf("expected sent delivery record, got %+v", delivery)
	}

	expiredSentAt := delivery.SentAt.UTC().Add(-2 * time.Minute)
	if err := database.WithContext(ctx).
		Model(&NotificationDelivery{}).
		Where("id = ?", delivery.ID).
		Update("sent_at", expiredSentAt).
		Error; err != nil {
		t.Fatalf("failed to backdate delivery sent_at: %v", err)
	}

	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("second EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if !waitForCondition(2*time.Second, func() bool {
		return hitCount.Load() == 2
	}) {
		t.Fatalf("expected repeated reminder delivery after cooldown, got %d", hitCount.Load())
	}

	var offlineCount int64
	if err := database.WithContext(ctx).
		Model(&monitor.ProcessEvent{}).
		Where("node_id = ? AND event_type = ?", node.ID, monitor.EventTypeNodeOffline).
		Count(&offlineCount).Error; err != nil {
		t.Fatalf("failed to count node_offline events: %v", err)
	}
	if offlineCount != 1 {
		t.Fatalf("expected still one node_offline event, got %d", offlineCount)
	}
}

func TestEvaluateAvailabilityAlerts_keepsTransientAgentOfflinePendingUntilForElapses(t *testing.T) {
	database, cleanup := setupMonitoringNodeOfflineTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	now := time.Now().UTC()
	staleHeartbeat := now.Add(-35 * time.Second)

	clusterRepo := clusterapp.NewRepository(database)
	hostProvider := &stubNodeOfflineHostProvider{
		hosts: map[uint]*clusterapp.HostInfo{
			7: {
				ID:            7,
				Name:          "host-7",
				IPAddress:     "10.0.0.7",
				AgentID:       "agent-7",
				AgentStatus:   "installed",
				LastHeartbeat: &staleHeartbeat,
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

	testCluster := &clusterapp.Cluster{
		Name:           "agent-offline-pending",
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
		HostID:        7,
		Role:          clusterapp.NodeRoleWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5802,
		WorkerPort:    5802,
		Status:        clusterapp.NodeStatusRunning,
		ProcessPID:    4242,
	}
	if err := database.WithContext(ctx).Create(node).Error; err != nil {
		t.Fatalf("failed to create cluster node: %v", err)
	}

	if err := monitorRepo.CreateConfig(ctx, &monitor.MonitorConfig{
		ClusterID:       testCluster.ID,
		AutoMonitor:     true,
		AutoRestart:     true,
		MonitorInterval: 5,
		RestartDelay:    10,
		MaxRestarts:     3,
		TimeWindow:      300,
		CooldownPeriod:  1800,
		ConfigVersion:   1,
	}); err != nil {
		t.Fatalf("failed to create monitor config: %v", err)
	}

	service.nodeHealthEvaluatorStartedAt = now.Add(-2 * time.Minute)
	service.nodeHealthStartupSuppression = 30 * time.Second

	// Agent 掉线但未持续满 for：只记 pending，不发任何事件。
	// Agent down but the for window not yet covered: pending is recorded, no event is emitted.
	agentSubject := availabilityPendingKey{RuleKey: clusterRuleKeyAgentOffline, ClusterID: testCluster.ID, HostID: 7}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts during pending returned error: %v", err)
	}
	if !pendingSubjectExists(service, agentSubject) {
		t.Fatal("expected agent_offline pending to be recorded")
	}
	for _, eventType := range []monitor.ProcessEventType{monitor.EventTypeAgentOffline, monitor.EventTypeNodeOffline} {
		var count int64
		if err := database.WithContext(ctx).Model(&monitor.ProcessEvent{}).
			Where("cluster_id = ? AND event_type = ?", testCluster.ID, eventType).Count(&count).Error; err != nil {
			t.Fatalf("failed to count %s events: %v", eventType, err)
		}
		if count != 0 {
			t.Fatalf("expected no %s event while pending, got %d", eventType, count)
		}
	}

	// 心跳中途恢复：pending 清零，不再触发。
	// Heartbeat recovers midway: the pending start is cleared and nothing ever fires.
	recoveredHeartbeat := time.Now().UTC()
	hostProvider.hosts[7].LastHeartbeat = &recoveredHeartbeat
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts after recovery returned error: %v", err)
	}
	if pendingSubjectExists(service, agentSubject) {
		t.Fatal("expected agent_offline pending to be cleared after recovery")
	}
	var total int64
	if err := database.WithContext(ctx).Model(&monitor.ProcessEvent{}).
		Where("cluster_id = ? AND event_type IN ?", testCluster.ID, []monitor.ProcessEventType{
			monitor.EventTypeAgentOffline, monitor.EventTypeAgentRecovered,
			monitor.EventTypeNodeOffline, monitor.EventTypeNodeRecovered,
		}).Count(&total).Error; err != nil {
		t.Fatalf("failed to count availability events: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected no availability events for a transient agent blip, got %d", total)
	}
}

func TestEvaluateAvailabilityAlerts_suppressesColdStartAgentOfflineUntilStartupWindowElapses(t *testing.T) {
	database, cleanup := setupMonitoringNodeOfflineTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	processStart := time.Now().UTC()
	lastHeartbeat := processStart.Add(-1 * time.Second)

	clusterRepo := clusterapp.NewRepository(database)
	hostProvider := &stubNodeOfflineHostProvider{
		hosts: map[uint]*clusterapp.HostInfo{
			8: {
				ID:               8,
				Name:             "host-8",
				IPAddress:        "10.0.0.8",
				AgentID:          "agent-8",
				AgentStatus:      "installed",
				LastHeartbeat:    &lastHeartbeat,
				ProcessStartedAt: &processStart,
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

	testCluster := &clusterapp.Cluster{
		Name:           "agent-offline-startup-suppression",
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
		HostID:        8,
		Role:          clusterapp.NodeRoleWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5802,
		WorkerPort:    5802,
		Status:        clusterapp.NodeStatusRunning,
		ProcessPID:    4242,
	}
	if err := database.WithContext(ctx).Create(node).Error; err != nil {
		t.Fatalf("failed to create cluster node: %v", err)
	}

	if err := monitorRepo.CreateConfig(ctx, &monitor.MonitorConfig{
		ClusterID:       testCluster.ID,
		AutoMonitor:     true,
		AutoRestart:     true,
		MonitorInterval: 5,
		RestartDelay:    10,
		MaxRestarts:     3,
		TimeWindow:      300,
		CooldownPeriod:  1800,
		ConfigVersion:   1,
	}); err != nil {
		t.Fatalf("failed to create monitor config: %v", err)
	}

	service.nodeHealthEvaluatorStartedAt = processStart
	service.nodeHealthStartupSuppression = 30 * time.Second

	agentSubject := availabilityPendingKey{RuleKey: clusterRuleKeyAgentOffline, ClusterID: testCluster.ID, HostID: 8}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts within startup suppression returned error: %v", err)
	}
	// pending 已满也要被启动窗口压住。
	// Even a fully matured pending is held back inside the startup window.
	ageAvailabilityPending(service, agentSubject, time.Hour)
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts within startup suppression returned error: %v", err)
	}

	event, err := monitorService.GetLatestSubjectEventByTypes(ctx, testCluster.ID, 0, 8, []monitor.ProcessEventType{
		monitor.EventTypeAgentOffline,
	})
	if err != nil {
		t.Fatalf("failed to query agent_offline event during startup suppression: %v", err)
	}
	if event != nil {
		t.Fatalf("expected no agent_offline event during startup suppression, got %+v", event)
	}

	service.nodeHealthEvaluatorStartedAt = processStart.Add(-31 * time.Second)
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts after startup suppression returned error: %v", err)
	}

	event, err = monitorService.GetLatestSubjectEventByTypes(ctx, testCluster.ID, 0, 8, []monitor.ProcessEventType{
		monitor.EventTypeAgentOffline,
	})
	if err != nil {
		t.Fatalf("failed to query agent_offline event after startup suppression: %v", err)
	}
	if event == nil {
		t.Fatal("expected agent_offline event after startup suppression elapsed")
	}
}

func TestAvailabilityNodeOfflineReasons_mergesProcessAndMetricReasons(t *testing.T) {
	node := &clusterapp.NodeInfo{
		ID:            9,
		ClusterID:     1,
		HostID:        2,
		HostName:      "host-9",
		HostIP:        "10.0.0.9",
		Role:          clusterapp.NodeRoleWorker,
		Status:        clusterapp.NodeStatusStopped,
		IsOnline:      true,
		ProcessPID:    0,
		HazelcastPort: 5802,
		UpdatedAt:     time.Now().UTC(),
	}
	signals := &availabilityMetricSignals{
		endpointDown:  map[string]struct{}{availabilitySignalKey(1, "10.0.0.9:5802"): {}},
		nodeStateDown: map[string]struct{}{availabilitySignalKey(1, "10.0.0.9:5802"): {}},
	}

	reasons := availabilityNodeOfflineReasons(node, signals)
	want := []string{
		availabilityReasonProcessStopped,
		availabilityReasonMetricsEndpointDown,
		availabilityReasonNodeStateDown,
	}
	if len(reasons) != len(want) {
		t.Fatalf("expected %d merged reasons, got %v", len(want), reasons)
	}
	for i, reason := range want {
		if reasons[i] != reason {
			t.Fatalf("reason %d mismatch: got %q want %q", i, reasons[i], reason)
		}
	}

	// Agent 掉线时进程侧与指标原因都不再产生（up==0 / node_state==0 的指标证据除外——
	// 这里把指标信号也置空验证进程侧原因被抑制）。
	// While the agent is down process-side reasons are suppressed (metric evidence would
	// still count; here the signal maps are emptied to prove the process side is gated).
	node.IsOnline = false
	if reasons := availabilityNodeOfflineReasons(node, nil); len(reasons) != 0 {
		t.Fatalf("expected no reasons while agent is offline, got %v", reasons)
	}
}

// setupAvailabilityCluster 是可用性评估测试的公共装配：一个集群 + 若干节点 + AutoMonitor 配置。
// setupAvailabilityCluster is the shared harness of the availability tests: one cluster with
// the given nodes plus an AutoMonitor config.
func setupAvailabilityCluster(t *testing.T, nodes []*clusterapp.ClusterNode, hosts map[uint]*clusterapp.HostInfo) (*gorm.DB, *Service, *monitor.Service, *clusterapp.Cluster, context.Context) {
	t.Helper()
	database, cleanup := setupMonitoringNodeOfflineTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	now := time.Now().UTC()
	clusterRepo := clusterapp.NewRepository(database)
	if hosts == nil {
		hosts = map[uint]*clusterapp.HostInfo{}
	}
	for _, node := range nodes {
		if _, ok := hosts[node.HostID]; !ok {
			hosts[node.HostID] = &clusterapp.HostInfo{
				ID:            node.HostID,
				Name:          "host-" + strconv.FormatUint(uint64(node.HostID), 10),
				IPAddress:     "10.0.0." + strconv.FormatUint(uint64(node.HostID), 10),
				AgentID:       "agent-" + strconv.FormatUint(uint64(node.HostID), 10),
				AgentStatus:   "installed",
				LastHeartbeat: ptrTime(now),
			}
		}
	}
	clusterService := clusterapp.NewService(clusterRepo, &stubNodeOfflineHostProvider{hosts: hosts}, &clusterapp.ServiceConfig{
		HeartbeatTimeout: 30 * time.Second,
	})
	monitorRepo := monitor.NewRepository(database)
	monitorService := monitor.NewService(monitorRepo)
	service := NewService(clusterService, monitorService, NewRepository(database))

	testCluster := &clusterapp.Cluster{
		Name:           "availability-cluster",
		DeploymentMode: clusterapp.DeploymentModeHybrid,
		Version:        "2.3.11",
		Status:         clusterapp.ClusterStatusRunning,
		InstallDir:     "/opt/seatunnel",
		CreatedBy:      1,
	}
	if err := database.WithContext(ctx).Create(testCluster).Error; err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}
	for _, node := range nodes {
		node.ClusterID = testCluster.ID
		if node.InstallDir == "" {
			node.InstallDir = "/opt/seatunnel"
		}
		if err := database.WithContext(ctx).Create(node).Error; err != nil {
			t.Fatalf("failed to create node: %v", err)
		}
	}
	if err := monitorRepo.CreateConfig(ctx, &monitor.MonitorConfig{
		ClusterID: testCluster.ID, AutoMonitor: true, MonitorInterval: 5,
		RestartDelay: 10, MaxRestarts: 3, TimeWindow: 300, CooldownPeriod: 1800, ConfigVersion: 1,
	}); err != nil {
		t.Fatalf("failed to create monitor config: %v", err)
	}
	return database, service, monitorService, testCluster, ctx
}

func countAvailabilityEvents(t *testing.T, database *gorm.DB, clusterID uint, eventType monitor.ProcessEventType) int64 {
	t.Helper()
	var count int64
	if err := database.Model(&monitor.ProcessEvent{}).
		Where("cluster_id = ? AND event_type = ?", clusterID, eventType).Count(&count).Error; err != nil {
		t.Fatalf("failed to count %s events: %v", eventType, err)
	}
	return count
}

func TestEvaluateAvailabilityAlerts_agentOfflineAloneWhenEngineMetricsUp(t *testing.T) {
	node := &clusterapp.ClusterNode{
		HostID: 9, Role: clusterapp.NodeRoleMasterWorker, HazelcastPort: 5801,
		Status: clusterapp.NodeStatusRunning, ProcessPID: 4242,
	}
	database, service, monitorService, testCluster, ctx := setupAvailabilityCluster(t, []*clusterapp.ClusterNode{node}, nil)

	// Agent 心跳超时：主机离线；引擎指标没有 down 证据时只出 agent_offline，不出 node_offline。
	// Agent heartbeat timed out so the host is offline; without engine-metric down evidence
	// only agent_offline fires, never node_offline.
	stale := time.Now().UTC().Add(-time.Hour)
	providerHosts := map[uint]*clusterapp.HostInfo{
		9: {ID: 9, Name: "host-9", IPAddress: "10.0.0.9", AgentID: "agent-9", AgentStatus: "installed", LastHeartbeat: &stale},
	}
	clusterRepo := clusterapp.NewRepository(database)
	clusterService := clusterapp.NewService(clusterRepo, &stubNodeOfflineHostProvider{hosts: providerHosts}, &clusterapp.ServiceConfig{HeartbeatTimeout: 30 * time.Second})
	service = NewService(clusterService, monitorService, NewRepository(database))

	agentSubject := availabilityPendingKey{RuleKey: clusterRuleKeyAgentOffline, ClusterID: testCluster.ID, HostID: 9}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	ageAvailabilityPending(service, agentSubject, time.Hour)
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}

	if got := countAvailabilityEvents(t, database, testCluster.ID, monitor.EventTypeAgentOffline); got != 1 {
		t.Fatalf("expected one agent_offline event, got %d", got)
	}
	if got := countAvailabilityEvents(t, database, testCluster.ID, monitor.EventTypeNodeOffline); got != 0 {
		t.Fatalf("expected no node_offline event while only the agent is down, got %d", got)
	}
	// master_unavailable 也应触发：唯一的 master/worker 节点不健康。
	// master_unavailable should fire too: the only master/worker node is unhealthy.
	if !pendingSubjectExists(service, availabilityPendingKey{RuleKey: clusterRuleKeyMasterUnavailable, ClusterID: testCluster.ID}) {
		t.Fatal("expected master_unavailable pending while the only master is unhealthy")
	}
}

func TestEvaluateAvailabilityAlerts_masterUnavailableFiresAndRecovers(t *testing.T) {
	master := &clusterapp.ClusterNode{
		HostID: 10, Role: clusterapp.NodeRoleMaster, HazelcastPort: 5801,
		Status: clusterapp.NodeStatusStopped, ProcessPID: 0,
	}
	worker := &clusterapp.ClusterNode{
		HostID: 11, Role: clusterapp.NodeRoleWorker, HazelcastPort: 5802,
		Status: clusterapp.NodeStatusRunning, ProcessPID: 1111,
	}
	database, service, monitorService, testCluster, ctx := setupAvailabilityCluster(t,
		[]*clusterapp.ClusterNode{master, worker}, nil)

	subject := availabilityPendingKey{RuleKey: clusterRuleKeyMasterUnavailable, ClusterID: testCluster.ID}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if got := countAvailabilityEvents(t, database, testCluster.ID, monitor.EventTypeMasterUnavailable); got != 0 {
		t.Fatalf("expected no master_unavailable event before for elapses, got %d", got)
	}
	ageAvailabilityPending(service, subject, time.Hour)
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if got := countAvailabilityEvents(t, database, testCluster.ID, monitor.EventTypeMasterUnavailable); got != 1 {
		t.Fatalf("expected one master_unavailable event, got %d", got)
	}

	// Master 恢复后写 master_recovered，阶段关闭。
	// After the master recovers a master_recovered event closes the episode.
	if err := database.WithContext(ctx).Model(&clusterapp.ClusterNode{}).Where("id = ?", master.ID).
		Updates(map[string]interface{}{"status": clusterapp.NodeStatusRunning, "process_pid": 4242}).Error; err != nil {
		t.Fatalf("failed to revive master node: %v", err)
	}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if got := countAvailabilityEvents(t, database, testCluster.ID, monitor.EventTypeMasterRecovered); got != 1 {
		t.Fatalf("expected one master_recovered event, got %d", got)
	}
	_ = monitorService
}

func TestEvaluateAvailabilityAlerts_workerInsufficientFiresOnShortfall(t *testing.T) {
	master := &clusterapp.ClusterNode{
		HostID: 12, Role: clusterapp.NodeRoleMaster, HazelcastPort: 5801,
		Status: clusterapp.NodeStatusRunning, ProcessPID: 1,
	}
	workerDown := &clusterapp.ClusterNode{
		HostID: 13, Role: clusterapp.NodeRoleWorker, HazelcastPort: 5802,
		Status: clusterapp.NodeStatusStopped, ProcessPID: 0,
	}
	workerUp := &clusterapp.ClusterNode{
		HostID: 14, Role: clusterapp.NodeRoleWorker, HazelcastPort: 5803,
		Status: clusterapp.NodeStatusRunning, ProcessPID: 2,
	}
	database, service, _, testCluster, ctx := setupAvailabilityCluster(t,
		[]*clusterapp.ClusterNode{master, workerDown, workerUp}, nil)

	// 阈值缺省 = 部署数 2，健康 1 → 触发。
	// The default threshold is the deployed count (2); 1 healthy worker fires the alert.
	subject := availabilityPendingKey{RuleKey: clusterRuleKeyWorkerInsufficient, ClusterID: testCluster.ID}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	ageAvailabilityPending(service, subject, time.Hour)
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if got := countAvailabilityEvents(t, database, testCluster.ID, monitor.EventTypeWorkerInsufficient); got != 1 {
		t.Fatalf("expected one worker_insufficient event, got %d", got)
	}
}

func TestEvaluateAvailabilityAlerts_pendingResetsWhenSubjectRecovers(t *testing.T) {
	node := &clusterapp.ClusterNode{
		HostID: 15, Role: clusterapp.NodeRoleMasterWorker, HazelcastPort: 5801,
		Status: clusterapp.NodeStatusStopped, ProcessPID: 0,
	}
	database, service, _, testCluster, ctx := setupAvailabilityCluster(t, []*clusterapp.ClusterNode{node}, nil)

	subject := availabilityPendingKey{RuleKey: AlertRuleKeyNodeOffline, ClusterID: testCluster.ID, NodeID: node.ID, HostID: node.HostID}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if !pendingSubjectExists(service, subject) {
		t.Fatal("expected pending after the first unhealthy observation")
	}

	// 中途恢复一次：pending 清零；再次停止后计时从头开始。
	// One recovery in between clears pending; after the node stops again the timer restarts.
	if err := database.WithContext(ctx).Model(&clusterapp.ClusterNode{}).Where("id = ?", node.ID).
		Updates(map[string]interface{}{"status": clusterapp.NodeStatusRunning, "process_pid": 4242}).Error; err != nil {
		t.Fatalf("failed to revive node: %v", err)
	}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if pendingSubjectExists(service, subject) {
		t.Fatal("expected pending cleared after the midway recovery")
	}
	if err := database.WithContext(ctx).Model(&clusterapp.ClusterNode{}).Where("id = ?", node.ID).
		Updates(map[string]interface{}{"status": clusterapp.NodeStatusStopped, "process_pid": 0}).Error; err != nil {
		t.Fatalf("failed to stop node again: %v", err)
	}
	if err := service.EvaluateAvailabilityAlerts(ctx); err != nil {
		t.Fatalf("EvaluateAvailabilityAlerts returned error: %v", err)
	}
	if got := countAvailabilityEvents(t, database, testCluster.ID, monitor.EventTypeNodeOffline); got != 0 {
		t.Fatalf("expected no node_offline event because the for timer restarted, got %d", got)
	}
}

// ageAvailabilityPending 把主体的 pending 起点回拨，让 for 窗口立即满足，避免测试真等持续时间。
// ageAvailabilityPending backdates a subject's pending start so the for window is already
// covered, avoiding real waits in tests.
func ageAvailabilityPending(service *Service, subject availabilityPendingKey, age time.Duration) {
	service.availabilityEvaluator.mu.Lock()
	defer service.availabilityEvaluator.mu.Unlock()
	service.availabilityEvaluator.pending[subject] = time.Now().UTC().Add(-age)
}

func pendingSubjectExists(service *Service, subject availabilityPendingKey) bool {
	service.availabilityEvaluator.mu.Lock()
	defer service.availabilityEvaluator.mu.Unlock()
	_, ok := service.availabilityEvaluator.pending[subject]
	return ok
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func ptrBool(value bool) *bool {
	return &value
}

func waitForCondition(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return cond()
}
