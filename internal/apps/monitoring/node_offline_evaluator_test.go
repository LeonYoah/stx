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

func TestEvaluateNodeHealthAlerts_emitsAndResolvesNodeOfflineEpisode(t *testing.T) {
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

	policy, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
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
	if policy.LegacyRuleKey != AlertRuleKeyNodeOffline {
		t.Fatalf("expected node_offline legacy rule key, got %q", policy.LegacyRuleKey)
	}

	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("EvaluateNodeHealthAlerts returned error: %v", err)
	}

	offlineEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{monitor.EventTypeNodeOffline})
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

	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("second EvaluateNodeHealthAlerts returned error: %v", err)
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

	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("third EvaluateNodeHealthAlerts returned error: %v", err)
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

	time.Sleep(100 * time.Millisecond)

	alerts, err := service.ListAlertInstances(ctx, &AlertInstanceFilter{
		ClusterID: strconv.FormatUint(uint64(testCluster.ID), 10),
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("ListAlertInstances returned error: %v", err)
	}
	if alerts.Total != 1 {
		t.Fatalf("expected one local alert instance, got %d", alerts.Total)
	}
	if alerts.Alerts[0].LifecycleStatus != AlertLifecycleStatusResolved {
		t.Fatalf("expected node_offline alert instance to be resolved, got %s", alerts.Alerts[0].LifecycleStatus)
	}
	if alerts.Alerts[0].ResolvedAt == nil {
		t.Fatal("expected resolved_at to be set on node_offline alert instance")
	}
}

func TestEvaluateNodeHealthAlerts_repeatsNodeOfflineReminderAfterCooldown(t *testing.T) {
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

	cooldown := 1
	if _, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "Node offline repeated reminder",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyNodeOffline,
		ClusterID:              strconv.FormatUint(uint64(testCluster.ID), 10),
		Severity:               AlertSeverityCritical,
		CooldownMinutes:        &cooldown,
		NotificationChannelIDs: []uint{channel.ID},
	}); err != nil {
		t.Fatalf("failed to create alert policy: %v", err)
	}

	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("first EvaluateNodeHealthAlerts returned error: %v", err)
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

	delivery, err := repo.GetNotificationDeliveryByDedupKey(
		ctx,
		buildLocalAlertSourceKey(offlineEvent.ID),
		channel.ID,
		string(NotificationDeliveryEventTypeFiring),
	)
	if err != nil {
		t.Fatalf("failed to load notification delivery: %v", err)
	}
	if delivery == nil || delivery.SentAt == nil {
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

	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("second EvaluateNodeHealthAlerts returned error: %v", err)
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

func TestEvaluateNodeHealthAlerts_suppressesTransientRuntimeHostOfflineWithinGraceWindow(t *testing.T) {
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
		Name:           "node-offline-runtime-grace",
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

	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("EvaluateNodeHealthAlerts during runtime grace returned error: %v", err)
	}

	offlineEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{
		monitor.EventTypeNodeOffline,
	})
	if err != nil {
		t.Fatalf("failed to query node_offline event during runtime grace: %v", err)
	}
	if offlineEvent != nil {
		t.Fatalf("expected no node_offline event during runtime grace, got %+v", offlineEvent)
	}

	if observedSince, ok := service.nodeHealthOfflineObservedAt[node.ID]; !ok || observedSince.IsZero() {
		t.Fatal("expected runtime host_offline observation to be recorded")
	}

	recoveredHeartbeat := time.Now().UTC()
	hostProvider.hosts[7].LastHeartbeat = &recoveredHeartbeat
	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("EvaluateNodeHealthAlerts after recovery returned error: %v", err)
	}

	offlineEvent, err = monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{
		monitor.EventTypeNodeOffline,
	})
	if err != nil {
		t.Fatalf("failed to query node_offline event after recovery: %v", err)
	}
	if offlineEvent != nil {
		t.Fatalf("expected no node_offline event after transient recovery, got %+v", offlineEvent)
	}

	recoveredEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{
		monitor.EventTypeNodeRecovered,
	})
	if err != nil {
		t.Fatalf("failed to query node_recovered event after transient recovery: %v", err)
	}
	if recoveredEvent != nil {
		t.Fatalf("expected no node_recovered event for suppressed transient offline, got %+v", recoveredEvent)
	}
	if _, ok := service.nodeHealthOfflineObservedAt[node.ID]; ok {
		t.Fatal("expected runtime host_offline observation to be cleared after recovery")
	}
}

func TestEvaluateNodeHealthAlerts_suppressesColdStartHostOfflineUntilStartupWindowElapses(t *testing.T) {
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
		Name:           "node-offline-startup-suppression",
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

	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("EvaluateNodeHealthAlerts within startup suppression returned error: %v", err)
	}

	offlineEvent, err := monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{
		monitor.EventTypeNodeOffline,
	})
	if err != nil {
		t.Fatalf("failed to query node_offline event during startup suppression: %v", err)
	}
	if offlineEvent != nil {
		t.Fatalf("expected no node_offline event during startup suppression, got %+v", offlineEvent)
	}

	service.nodeHealthEvaluatorStartedAt = processStart.Add(-31 * time.Second)
	service.nodeHealthOfflineObservedAt[node.ID] = processStart.Add(-31 * time.Second)
	if err := service.EvaluateNodeHealthAlerts(ctx); err != nil {
		t.Fatalf("EvaluateNodeHealthAlerts after startup suppression returned error: %v", err)
	}

	offlineEvent, err = monitorService.GetLatestNodeEventByTypes(ctx, node.ID, []monitor.ProcessEventType{
		monitor.EventTypeNodeOffline,
	})
	if err != nil {
		t.Fatalf("failed to query node_offline event after startup suppression: %v", err)
	}
	if offlineEvent == nil {
		t.Fatal("expected node_offline event after startup suppression elapsed")
	}
	if offlineEvent.EventType != monitor.EventTypeNodeOffline {
		t.Fatalf("expected node_offline event type, got %s", offlineEvent.EventType)
	}
}

func TestEvaluateNodeOfflineState_hostOfflineUsesRuntimeGraceAtServiceLayer(t *testing.T) {
	now := time.Now().UTC()
	node := &clusterapp.NodeInfo{
		ID:         9,
		ClusterID:  1,
		HostID:     2,
		HostName:   "host-offline",
		HostIP:     "10.0.0.9",
		Role:       clusterapp.NodeRoleWorker,
		Status:     clusterapp.NodeStatusRunning,
		IsOnline:   false,
		ProcessPID: 2345,
		UpdatedAt:  now,
	}
	cfg := &monitor.MonitorConfig{
		AutoMonitor:     true,
		AutoRestart:     true,
		MonitorInterval: 5,
		RestartDelay:    10,
	}
	service := &Service{
		nodeHealthOfflineObservedAt: make(map[uint]time.Time),
	}

	offline, matured, reason, observedSince, grace := evaluateNodeOfflineState(node, cfg, now)
	if !offline {
		t.Fatal("expected host-offline node to be considered offline")
	}
	if !matured {
		t.Fatal("expected base host-offline state to be offline before service-layer suppression")
	}
	if reason != "host_offline" {
		t.Fatalf("expected host_offline reason, got %q", reason)
	}
	if grace != 0 {
		t.Fatalf("expected base evaluator to report zero direct grace for host_offline, got %s", grace)
	}
	if !observedSince.Equal(now) {
		t.Fatalf("expected base evaluator observedSince to be now, got %s want %s", observedSince, now)
	}

	observedSince, grace, matured = service.applyRuntimeHostOfflineGrace(node, cfg, now, offline)
	if matured {
		t.Fatal("expected runtime host_offline grace to delay alert maturation")
	}
	if grace < minNodeRuntimeOfflineGraceWindow {
		t.Fatalf("expected runtime host_offline grace to be at least %s, got %s", minNodeRuntimeOfflineGraceWindow, grace)
	}
	if observedSince.IsZero() {
		t.Fatal("expected runtime host_offline observation time to be recorded")
	}

	_, _, matured = service.applyRuntimeHostOfflineGrace(node, cfg, observedSince.Add(grace), offline)
	if !matured {
		t.Fatal("expected runtime host_offline grace to mature after grace window elapsed")
	}
}

func ptrTime(value time.Time) *time.Time {
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
