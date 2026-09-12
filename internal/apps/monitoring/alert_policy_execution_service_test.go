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
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/monitor"
)

func TestDispatchAlertPolicyEvent_sendsUnifiedPolicyNotification(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	var hitCount atomic.Int32
	var lastBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(string(body))
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "policy-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	policy, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "Unified restart failed policy",
		Description:            "send webhook directly from unified policy",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyProcessRestartFailed,
		LegacyRuleKey:          AlertRuleKeyProcessRestartFailed,
		ClusterID:              strconv.FormatUint(uint64(cluster.ID), 10),
		Severity:               AlertSeverityCritical,
		NotificationChannelIDs: []uint{channel.ID},
	})
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	event := &monitor.ProcessEvent{
		ID:          1001,
		ClusterID:   cluster.ID,
		NodeID:      7,
		HostID:      9,
		EventType:   monitor.EventTypeRestartFailed,
		PID:         4321,
		ProcessName: "seatunnel-master",
		InstallDir:  "/tmp/seatunnel",
		Role:        "master",
		Details:     `{"reason":"restart command failed"}`,
		CreatedAt:   time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC),
	}

	if err := service.DispatchAlertPolicyEvent(ctx, event); err != nil {
		t.Fatalf("expected dispatch success, got %v", err)
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected 1 webhook call, got %d", hitCount.Load())
	}

	delivery, err := repo.GetNotificationDeliveryByDedupKey(
		ctx,
		buildLocalAlertSourceKey(event.ID),
		channel.ID,
		string(NotificationDeliveryEventTypeFiring),
	)
	if err != nil {
		t.Fatalf("failed to query delivery: %v", err)
	}
	if delivery == nil {
		t.Fatalf("expected delivery record to be created")
	}
	if NotificationDeliveryStatus(delivery.Status) != NotificationDeliveryStatusSent {
		t.Fatalf("expected sent delivery status, got %s", delivery.Status)
	}
	if delivery.AlertName != policy.Name {
		t.Fatalf("expected alert name %q, got %q", policy.Name, delivery.AlertName)
	}

	storedPolicy, err := repo.GetAlertPolicyByID(ctx, policy.ID)
	if err != nil {
		t.Fatalf("failed to reload policy: %v", err)
	}
	if storedPolicy.MatchCount != 1 {
		t.Fatalf("expected match_count=1, got %d", storedPolicy.MatchCount)
	}
	if storedPolicy.DeliveryCount != 1 {
		t.Fatalf("expected delivery_count=1, got %d", storedPolicy.DeliveryCount)
	}
	if storedPolicy.LastMatchedAt == nil {
		t.Fatalf("expected last_matched_at to be set")
	}
	if !storedPolicy.LastMatchedAt.Equal(event.CreatedAt.UTC()) {
		t.Fatalf("expected last_matched_at=%s, got %s", event.CreatedAt.UTC(), storedPolicy.LastMatchedAt.UTC())
	}
	if storedPolicy.LastDeliveredAt == nil {
		t.Fatalf("expected last_delivered_at to be set")
	}
	if storedPolicy.LastExecutionStatus != AlertPolicyExecutionStatusSent {
		t.Fatalf("expected last_execution_status=sent, got %s", storedPolicy.LastExecutionStatus)
	}
	if storedPolicy.LastExecutionError != "" {
		t.Fatalf("expected empty last_execution_error, got %q", storedPolicy.LastExecutionError)
	}

	body, _ := lastBody.Load().(string)
	if !strings.Contains(body, `"policy_name":"Unified restart failed policy"`) {
		t.Fatalf("expected payload to include policy_name, got %s", body)
	}
	if !strings.Contains(body, `"event_type":"restart_failed"`) {
		t.Fatalf("expected payload to include event_type, got %s", body)
	}
}

func TestResolveLocalAlertLifecycle_marksManualOperationEventsResolved(t *testing.T) {
	service := &Service{}
	createdAt := time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC)

	for _, eventType := range []monitor.ProcessEventType{
		monitor.EventTypeClusterRestartRequested,
		monitor.EventTypeNodeRestartRequested,
		monitor.EventTypeNodeStopRequested,
	} {
		status, resolvedAt, err := service.resolveLocalAlertLifecycle(context.Background(), &AlertEventSource{
			EventType: eventType,
			CreatedAt: createdAt,
		})
		if err != nil {
			t.Fatalf("expected no error for %s, got %v", eventType, err)
		}
		if status != AlertLifecycleStatusResolved {
			t.Fatalf("expected %s to resolve immediately, got %s", eventType, status)
		}
		if resolvedAt == nil || !resolvedAt.Equal(createdAt) {
			t.Fatalf("expected resolved_at=%s for %s, got %+v", createdAt, eventType, resolvedAt)
		}
	}
}

func TestBuildLocalAlertPolicyMessage_formatsNodeStopEvent(t *testing.T) {
	originalLocal := time.Local
	time.Local = time.FixedZone("UTC+8", 8*3600)
	t.Cleanup(func() {
		time.Local = originalLocal
	})

	event := &monitor.ProcessEvent{
		ClusterID:   6,
		NodeID:      4,
		HostID:      9,
		EventType:   monitor.EventTypeNodeStopRequested,
		ProcessName: "node 4",
		Role:        "master/worker",
		CreatedAt:   time.Date(2026, 3, 10, 9, 40, 0, 0, time.UTC),
		Details:     `{"scope":"node","operation":"stop","operator":"alice","trigger":"manual_api","success":"true","message":"command accepted","host_name":"host-4","host_ip":"10.0.0.4","role":"master/worker"}`,
	}
	policy := &AlertPolicy{
		ID:          11,
		Name:        "节点停止通知",
		Description: "记录节点被人工停止的运维动作",
		Severity:    AlertSeverityWarning,
	}

	text := buildLocalAlertPolicyMessageText(event, nil, "t3", policy, NotificationDeliveryEventTypeFiring)
	if !strings.Contains(text, "范围：节点") {
		t.Fatalf("expected node scope in text, got %s", text)
	}
	if !strings.Contains(text, "操作：停止") {
		t.Fatalf("expected stop operation in text, got %s", text)
	}
	if !strings.Contains(text, "主机：host-4") {
		t.Fatalf("expected host information in text, got %s", text)
	}
	if !strings.Contains(text, "2026-03-10 17:40:00 +08:00 (server local)") {
		t.Fatalf("expected server-local timestamp in text, got %s", text)
	}
	if !strings.Contains(text, "2026-03-10T09:40:00Z (UTC)") {
		t.Fatalf("expected utc timestamp in text, got %s", text)
	}
	if strings.Contains(text, "EventDetails:") {
		t.Fatalf("expected formatted fields without raw EventDetails dump, got %s", text)
	}

	html := buildLocalAlertPolicyMessageHTML(event, nil, "t3", policy, NotificationDeliveryEventTypeFiring)
	if !strings.Contains(html, "<table") {
		t.Fatalf("expected html table layout, got %s", html)
	}
	if !strings.Contains(html, "host-4") {
		t.Fatalf("expected host information in html, got %s", html)
	}
	if !strings.Contains(html, "告警中") {
		t.Fatalf("expected firing badge in html, got %s", html)
	}
}

func TestBuildLocalAlertPolicyMessage_formatsResolvedNotificationDifferently(t *testing.T) {
	event := &monitor.ProcessEvent{
		ClusterID:   6,
		NodeID:      4,
		HostID:      9,
		EventType:   monitor.EventTypeNodeOffline,
		ProcessName: "seatunnel",
		Role:        "master/worker",
		CreatedAt:   time.Date(2026, 3, 10, 9, 40, 0, 0, time.UTC),
		Details:     `{"reason":"process_stopped","host_name":"host-4","host_ip":"10.0.0.4","node_status":"stopped","observed_since":"2026-03-10T09:39:30Z","grace_seconds":"20"}`,
	}
	recoveryEvent := &monitor.ProcessEvent{
		ClusterID: event.ClusterID,
		NodeID:    event.NodeID,
		HostID:    event.HostID,
		EventType: monitor.EventTypeNodeRecovered,
		CreatedAt: time.Date(2026, 3, 10, 9, 41, 0, 0, time.UTC),
		Details:   `{"recovered_at":"2026-03-10T09:41:00Z","node_status":"running"}`,
	}
	policy := &AlertPolicy{
		ID:       12,
		Name:     "节点离线",
		Severity: AlertSeverityCritical,
	}

	title := buildLocalAlertPolicyMessageTitle(event, policy, NotificationDeliveryEventTypeResolved)
	if !strings.Contains(title, "[恢复]") {
		t.Fatalf("expected recovered marker in title, got %s", title)
	}
	text := buildLocalAlertPolicyMessageText(event, recoveryEvent, "t3", policy, NotificationDeliveryEventTypeResolved)
	if !strings.Contains(text, "状态：已恢复") {
		t.Fatalf("expected recovered state in text, got %s", text)
	}
	if !strings.Contains(text, "恢复事件：node_recovered") {
		t.Fatalf("expected recovery event in text, got %s", text)
	}

	html := buildLocalAlertPolicyMessageHTML(event, recoveryEvent, "t3", policy, NotificationDeliveryEventTypeResolved)
	if !strings.Contains(html, "已恢复") {
		t.Fatalf("expected recovered badge in html, got %s", html)
	}
	if !strings.Contains(html, "该告警已恢复") {
		t.Fatalf("expected resolved summary in html, got %s", html)
	}
}

func TestDispatchAlertPolicyEvent_deduplicatesSentDelivery(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	var hitCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "policy-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	policy, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "Unified crash policy",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyProcessCrashed,
		LegacyRuleKey:          AlertRuleKeyProcessCrashed,
		ClusterID:              strconv.FormatUint(uint64(cluster.ID), 10),
		Severity:               AlertSeverityCritical,
		NotificationChannelIDs: []uint{channel.ID},
	})
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	event := &monitor.ProcessEvent{
		ID:          2002,
		ClusterID:   cluster.ID,
		NodeID:      3,
		HostID:      4,
		EventType:   monitor.EventTypeCrashed,
		PID:         9988,
		ProcessName: "seatunnel-worker",
		Role:        "worker",
		CreatedAt:   time.Now().UTC(),
	}

	for i := 0; i < 2; i++ {
		if err := service.DispatchAlertPolicyEvent(context.Background(), event); err != nil {
			t.Fatalf("dispatch #%d failed: %v", i+1, err)
		}
	}

	if hitCount.Load() != 1 {
		t.Fatalf("expected deduplicated single webhook call, got %d", hitCount.Load())
	}

	storedPolicy, err := repo.GetAlertPolicyByID(ctx, policy.ID)
	if err != nil {
		t.Fatalf("failed to reload policy: %v", err)
	}
	if storedPolicy.MatchCount != 2 {
		t.Fatalf("expected match_count=2 after duplicate dispatch, got %d", storedPolicy.MatchCount)
	}
	if storedPolicy.DeliveryCount != 1 {
		t.Fatalf("expected delivery_count=1 after duplicate dispatch, got %d", storedPolicy.DeliveryCount)
	}
	if storedPolicy.LastExecutionStatus != AlertPolicyExecutionStatusSent {
		t.Fatalf("expected last_execution_status=sent, got %s", storedPolicy.LastExecutionStatus)
	}
}

func TestDispatchAlertPolicyEvent_recordsFailedExecutionState(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"downstream failed"}`))
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "policy-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	policy, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "Unified crash policy",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyProcessCrashed,
		LegacyRuleKey:          AlertRuleKeyProcessCrashed,
		ClusterID:              strconv.FormatUint(uint64(cluster.ID), 10),
		Severity:               AlertSeverityCritical,
		NotificationChannelIDs: []uint{channel.ID},
	})
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	event := &monitor.ProcessEvent{
		ID:          3003,
		ClusterID:   cluster.ID,
		NodeID:      5,
		HostID:      6,
		EventType:   monitor.EventTypeCrashed,
		PID:         5566,
		ProcessName: "seatunnel-master",
		Role:        "master",
		CreatedAt:   time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC),
	}

	err = service.DispatchAlertPolicyEvent(ctx, event)
	if err == nil {
		t.Fatalf("expected dispatch error, got nil")
	}
	if !strings.Contains(err.Error(), "http status 500") {
		t.Fatalf("expected http status 500 error, got %v", err)
	}

	storedPolicy, err := repo.GetAlertPolicyByID(ctx, policy.ID)
	if err != nil {
		t.Fatalf("failed to reload policy: %v", err)
	}
	if storedPolicy.MatchCount != 1 {
		t.Fatalf("expected match_count=1, got %d", storedPolicy.MatchCount)
	}
	if storedPolicy.DeliveryCount != 0 {
		t.Fatalf("expected delivery_count=0, got %d", storedPolicy.DeliveryCount)
	}
	if storedPolicy.LastMatchedAt == nil {
		t.Fatalf("expected last_matched_at to be set")
	}
	if storedPolicy.LastDeliveredAt != nil {
		t.Fatalf("expected last_delivered_at to remain nil on failed delivery")
	}
	if storedPolicy.LastExecutionStatus != AlertPolicyExecutionStatusFailed {
		t.Fatalf("expected last_execution_status=failed, got %s", storedPolicy.LastExecutionStatus)
	}
	if !strings.Contains(storedPolicy.LastExecutionError, "http status 500") {
		t.Fatalf("expected last_execution_error to record downstream failure, got %q", storedPolicy.LastExecutionError)
	}
}

func TestDispatchAlertPolicyEvent_sendsResolvedNotificationForNodeRecovered(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	var hitCount atomic.Int32
	var lastBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody.Store(string(body))
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "resolved-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	sendRecovery := true
	policy, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "Node offline policy",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyNodeOffline,
		LegacyRuleKey:          AlertRuleKeyNodeOffline,
		ClusterID:              strconv.FormatUint(uint64(cluster.ID), 10),
		Severity:               AlertSeverityCritical,
		SendRecovery:           &sendRecovery,
		NotificationChannelIDs: []uint{channel.ID},
	})
	if err != nil {
		t.Fatalf("failed to create node_offline policy: %v", err)
	}

	offlineEvent := &monitor.ProcessEvent{
		ClusterID:   cluster.ID,
		NodeID:      21,
		HostID:      31,
		EventType:   monitor.EventTypeNodeOffline,
		PID:         4455,
		ProcessName: "seatunnel-master",
		Role:        "master",
		Details:     `{"reason":"process_stopped","host_name":"host-31","host_ip":"10.0.0.31","node_status":"stopped","observed_since":"2026-03-09T10:00:00Z","grace_seconds":"20"}`,
		CreatedAt:   time.Date(2026, 3, 9, 10, 0, 20, 0, time.UTC),
	}
	if err := service.monitorService.RecordEvent(ctx, offlineEvent); err != nil {
		t.Fatalf("failed to persist node_offline event: %v", err)
	}
	if offlineEvent.ID == 0 {
		t.Fatal("expected persisted node_offline event id")
	}

	if err := service.DispatchAlertPolicyEvent(ctx, offlineEvent); err != nil {
		t.Fatalf("failed to dispatch node_offline event: %v", err)
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected one firing notification, got %d", hitCount.Load())
	}

	recoveryEvent := &monitor.ProcessEvent{
		ClusterID:   cluster.ID,
		NodeID:      offlineEvent.NodeID,
		HostID:      offlineEvent.HostID,
		EventType:   monitor.EventTypeNodeRecovered,
		PID:         offlineEvent.PID,
		ProcessName: offlineEvent.ProcessName,
		Role:        offlineEvent.Role,
		Details: fmt.Sprintf(
			`{"node_status":"running","recovered_at":"2026-03-09T10:00:35Z","offline_event_id":"%d"}`,
			offlineEvent.ID,
		),
		CreatedAt: time.Date(2026, 3, 9, 10, 0, 35, 0, time.UTC),
	}
	if err := service.monitorService.RecordEvent(ctx, recoveryEvent); err != nil {
		t.Fatalf("failed to persist node_recovered event: %v", err)
	}

	if err := service.DispatchAlertPolicyEvent(ctx, recoveryEvent); err != nil {
		t.Fatalf("failed to dispatch node_recovered event: %v", err)
	}
	if hitCount.Load() != 2 {
		t.Fatalf("expected firing + resolved notifications, got %d", hitCount.Load())
	}

	resolvedDelivery, err := repo.GetNotificationDeliveryByDedupKey(
		ctx,
		buildLocalAlertSourceKey(offlineEvent.ID),
		channel.ID,
		string(NotificationDeliveryEventTypeResolved),
	)
	if err != nil {
		t.Fatalf("failed to load resolved delivery: %v", err)
	}
	if resolvedDelivery == nil {
		t.Fatal("expected resolved delivery record to be created")
	}
	if NotificationDeliveryStatus(resolvedDelivery.Status) != NotificationDeliveryStatusSent {
		t.Fatalf("expected resolved delivery status sent, got %s", resolvedDelivery.Status)
	}

	storedPolicy, err := repo.GetAlertPolicyByID(ctx, policy.ID)
	if err != nil {
		t.Fatalf("failed to reload policy: %v", err)
	}
	if storedPolicy.MatchCount != 1 {
		t.Fatalf("expected match_count to remain 1 after recovery delivery, got %d", storedPolicy.MatchCount)
	}
	if storedPolicy.DeliveryCount != 2 {
		t.Fatalf("expected delivery_count=2 after firing+resolved, got %d", storedPolicy.DeliveryCount)
	}
	if storedPolicy.LastExecutionStatus != AlertPolicyExecutionStatusSent {
		t.Fatalf("expected last_execution_status=sent, got %s", storedPolicy.LastExecutionStatus)
	}
	if storedPolicy.LastDeliveredAt == nil {
		t.Fatal("expected last_delivered_at to be updated by resolved notification")
	}

	body, _ := lastBody.Load().(string)
	if !strings.Contains(body, `"status":"resolved"`) {
		t.Fatalf("expected resolved payload status, got %s", body)
	}
	if !strings.Contains(body, `"resolution_event_type":"node_recovered"`) {
		t.Fatalf("expected resolved payload to include node_recovered signal, got %s", body)
	}
}

func TestDispatchAlertPolicyEvent_skipsResolvedNotificationWhenSendRecoveryDisabled(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	var hitCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "no-recovery-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	sendRecovery := false
	policy, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "Node offline policy",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyNodeOffline,
		LegacyRuleKey:          AlertRuleKeyNodeOffline,
		ClusterID:              strconv.FormatUint(uint64(cluster.ID), 10),
		Severity:               AlertSeverityCritical,
		SendRecovery:           &sendRecovery,
		NotificationChannelIDs: []uint{channel.ID},
	})
	if err != nil {
		t.Fatalf("failed to create node_offline policy: %v", err)
	}

	offlineEvent := &monitor.ProcessEvent{
		ClusterID:   cluster.ID,
		NodeID:      41,
		HostID:      51,
		EventType:   monitor.EventTypeNodeOffline,
		PID:         7788,
		ProcessName: "seatunnel-worker",
		Role:        "worker",
		Details:     `{"reason":"host_offline","host_name":"host-51","host_ip":"10.0.0.51","node_status":"offline","observed_since":"2026-03-09T11:00:00Z","grace_seconds":"0"}`,
		CreatedAt:   time.Date(2026, 3, 9, 11, 0, 10, 0, time.UTC),
	}
	if err := service.monitorService.RecordEvent(ctx, offlineEvent); err != nil {
		t.Fatalf("failed to persist node_offline event: %v", err)
	}
	if err := service.DispatchAlertPolicyEvent(ctx, offlineEvent); err != nil {
		t.Fatalf("failed to dispatch node_offline event: %v", err)
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected one firing notification, got %d", hitCount.Load())
	}

	recoveryEvent := &monitor.ProcessEvent{
		ClusterID:   cluster.ID,
		NodeID:      offlineEvent.NodeID,
		HostID:      offlineEvent.HostID,
		EventType:   monitor.EventTypeNodeRecovered,
		PID:         offlineEvent.PID,
		ProcessName: offlineEvent.ProcessName,
		Role:        offlineEvent.Role,
		Details: fmt.Sprintf(
			`{"node_status":"running","recovered_at":"2026-03-09T11:00:20Z","offline_event_id":"%d"}`,
			offlineEvent.ID,
		),
		CreatedAt: time.Date(2026, 3, 9, 11, 0, 20, 0, time.UTC),
	}
	if err := service.monitorService.RecordEvent(ctx, recoveryEvent); err != nil {
		t.Fatalf("failed to persist node_recovered event: %v", err)
	}

	if err := service.DispatchAlertPolicyEvent(ctx, recoveryEvent); err != nil {
		t.Fatalf("failed to dispatch node_recovered event: %v", err)
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected resolved notification to be skipped, got %d total sends", hitCount.Load())
	}

	resolvedDelivery, err := repo.GetNotificationDeliveryByDedupKey(
		ctx,
		buildLocalAlertSourceKey(offlineEvent.ID),
		channel.ID,
		string(NotificationDeliveryEventTypeResolved),
	)
	if err != nil {
		t.Fatalf("failed to query resolved delivery: %v", err)
	}
	if resolvedDelivery != nil {
		t.Fatalf("expected no resolved delivery row when send_recovery=false, got %+v", resolvedDelivery)
	}

	storedPolicy, err := repo.GetAlertPolicyByID(ctx, policy.ID)
	if err != nil {
		t.Fatalf("failed to reload policy: %v", err)
	}
	if storedPolicy.MatchCount != 1 {
		t.Fatalf("expected match_count=1, got %d", storedPolicy.MatchCount)
	}
	if storedPolicy.DeliveryCount != 1 {
		t.Fatalf("expected delivery_count=1 with only firing delivery, got %d", storedPolicy.DeliveryCount)
	}
}

func TestListAlertPolicyExecutions_returnsPolicyScopedHistory(t *testing.T) {
	service, _, cluster, ctx := setupMonitoringAlertPolicyService(t)

	var hitCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	channel, err := service.CreateNotificationChannel(ctx, &UpsertNotificationChannelRequest{
		Name:     "history-webhook",
		Type:     NotificationChannelTypeWebhook,
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	policy, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:                   "History policy",
		PolicyType:             AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:            AlertRuleKeyProcessRestartFailed,
		LegacyRuleKey:          AlertRuleKeyProcessRestartFailed,
		ClusterID:              strconv.FormatUint(uint64(cluster.ID), 10),
		Severity:               AlertSeverityCritical,
		NotificationChannelIDs: []uint{channel.ID},
	})
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	event := &monitor.ProcessEvent{
		ID:          4004,
		ClusterID:   cluster.ID,
		NodeID:      8,
		HostID:      10,
		EventType:   monitor.EventTypeRestartFailed,
		PID:         7788,
		ProcessName: "seatunnel-worker",
		Role:        "worker",
		CreatedAt:   time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC),
	}
	if err := service.DispatchAlertPolicyEvent(ctx, event); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if hitCount.Load() != 1 {
		t.Fatalf("expected one webhook call, got %d", hitCount.Load())
	}

	history, err := service.ListAlertPolicyExecutions(ctx, policy.ID, &NotificationDeliveryFilter{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("failed to list policy execution history: %v", err)
	}
	if history.Total != 1 {
		t.Fatalf("expected history total=1, got %d", history.Total)
	}
	if len(history.Deliveries) != 1 {
		t.Fatalf("expected 1 delivery item, got %d", len(history.Deliveries))
	}
	if history.Deliveries[0].PolicyID != policy.ID {
		t.Fatalf("expected delivery policy_id=%d, got %d", policy.ID, history.Deliveries[0].PolicyID)
	}
	if history.Deliveries[0].ChannelID != channel.ID {
		t.Fatalf("expected delivery channel_id=%d, got %d", channel.ID, history.Deliveries[0].ChannelID)
	}
}
