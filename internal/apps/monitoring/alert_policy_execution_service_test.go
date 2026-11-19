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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/monitor"
)

// enableClusterRuleForDelivery 把一条集群级规则配成 critical + 自定义渠道，让事件 / 集群
// 信号满足外投条件（critical 且生效通知里至少一个渠道）。
// enableClusterRuleForDelivery configures a cluster rule as critical + custom channels so
// event / cluster signals qualify for external delivery (critical plus at least one channel).
func enableClusterRuleForDelivery(t *testing.T, service *Service, ctx context.Context, ruleKey string, channelIDs []uint) {
	t.Helper()
	severity := string(AlertSeverityCritical)
	idsJSON, err := marshalAlertPolicyChannelIDs(channelIDs)
	if err != nil {
		t.Fatalf("marshal channel ids: %v", err)
	}
	if _, err := service.SaveClusterRuleConfig(ctx, ruleKey, &clusterRuleFields{
		Enabled:        ptrBool(true),
		Severity:       &severity,
		NotifyMode:     clusterRuleNotifyModeCustom,
		ChannelIDsJSON: idsJSON,
	}); err != nil {
		t.Fatalf("save cluster rule config for %s: %v", ruleKey, err)
	}
}

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

	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyProcessRestartFailed, []uint{channel.ID})

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
	expectedName := lookupClusterRuleDefinition(AlertRuleKeyProcessRestartFailed).Name
	if delivery.AlertName != expectedName {
		t.Fatalf("expected alert name %q, got %q", expectedName, delivery.AlertName)
	}

	body, _ := lastBody.Load().(string)
	if !strings.Contains(body, expectedName) {
		t.Fatalf("expected payload to include rule name %q, got %s", expectedName, body)
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
		}, nil)
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
	if !strings.Contains(text, "触发时间：2026-03-10 17:40:00 (UTC+8)\n") {
		t.Fatalf("expected one-line display-timezone timestamp in text, got %s", text)
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
	if !strings.Contains(text, "恢复事件：节点恢复 (node_recovered)") {
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

	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyProcessCrashed, []uint{channel.ID})

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

	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyProcessCrashed, []uint{channel.ID})

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

	err := service.DispatchAlertPolicyEvent(ctx, event)
	if err == nil {
		t.Fatalf("expected dispatch error, got nil")
	}
	if !strings.Contains(err.Error(), "http status 500") {
		t.Fatalf("expected http status 500 error, got %v", err)
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
	if NotificationDeliveryStatus(delivery.Status) != NotificationDeliveryStatusFailed {
		t.Fatalf("expected failed delivery status, got %s", delivery.Status)
	}
	if !strings.Contains(delivery.LastError, "http status 500") {
		t.Fatalf("expected last_error to record downstream failure, got %q", delivery.LastError)
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

	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyNodeOffline, []uint{channel.ID})

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

	if _, err := service.SaveAlertNotificationDefaults(ctx, nil, nil, 0, false); err != nil {
		t.Fatalf("failed to disable recovery notifications: %v", err)
	}
	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyNodeOffline, []uint{channel.ID})

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

}

func TestDispatchAlertPolicyEvent_writesDeliveryWithoutPolicyID(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

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

	enableClusterRuleForDelivery(t, service, ctx, AlertRuleKeyProcessRestartFailed, []uint{channel.ID})

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

	// 集群级规则的投递不再绑定策略行：policy_id 恒为 0。
	// Cluster-rule deliveries are no longer bound to a policy row: policy_id is always 0.
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
	if delivery.PolicyID != 0 {
		t.Fatalf("expected delivery policy_id=0, got %d", delivery.PolicyID)
	}
	if delivery.ChannelID != channel.ID {
		t.Fatalf("expected delivery channel_id=%d, got %d", channel.ID, delivery.ChannelID)
	}
}
