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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupMonitoringNotificationTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "monitoring_notification_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	database, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to open database: %v", err)
	}

	if err := database.AutoMigrate(
		&AlertState{},
		&AlertPolicy{},
		&NotificationChannel{},
		&NotificationRoute{},
		&NotificationDelivery{},
		&RemoteAlertRecord{},
		&auth.User{},
	); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to migrate monitoring notification tables: %v", err)
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

func TestService_HandleAlertmanagerWebhook_dispatchesAndDeduplicatesRemoteNotification(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)
	ctx := context.Background()

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
		Name:     "ops-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	route := &NotificationRoute{
		Name:               "remote critical",
		Enabled:            true,
		SourceType:         string(AlertSourceTypeRemoteAlertmanager),
		Severity:           "critical",
		ChannelID:          channel.ID,
		SendResolved:       false,
		MuteIfAcknowledged: true,
		MuteIfSilenced:     true,
	}
	if err := repo.CreateNotificationRoute(ctx, route); err != nil {
		t.Fatalf("failed to create notification route: %v", err)
	}

	startsAt := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	firingPayload := &AlertmanagerWebhookPayload{
		Receiver: "ops",
		Status:   "firing",
		CommonLabels: map[string]string{
			"alertname":    "HighCpuUsage",
			"severity":     "critical",
			"cluster_id":   "6",
			"cluster_name": "alpha",
		},
		CommonAnnotations: map[string]string{
			"summary":     "CPU usage is high",
			"description": "usage > 90%",
		},
		Alerts: []*WebhookAlert{{
			Status:       "firing",
			Fingerprint:  "fp-1",
			StartsAt:     startsAt,
			GeneratorURL: "http://prom.example/rule",
		}},
	}

	result, err := service.HandleAlertmanagerWebhook(ctx, firingPayload)
	if err != nil {
		t.Fatalf("HandleAlertmanagerWebhook returned error: %v", err)
	}
	if result.Stored != 1 {
		t.Fatalf("expected 1 stored alert, got %d", result.Stored)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no processing errors, got %v", result.Errors)
	}
	if got := hitCount.Load(); got != 1 {
		t.Fatalf("expected webhook channel to be called once, got %d", got)
	}
	body, _ := lastBody.Load().(string)
	if !strings.Contains(body, "HighCpuUsage") {
		t.Fatalf("expected request body to contain alert name, got %s", body)
	}

	sourceKey := buildRemoteAlertSourceKey("fp-1", startsAt.Unix())
	delivery, err := repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
	if err != nil {
		t.Fatalf("failed to load firing delivery: %v", err)
	}
	if delivery == nil {
		t.Fatal("expected firing delivery to be created")
	}
	if delivery.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("expected firing delivery status sent, got %s", delivery.Status)
	}
	if delivery.AttemptCount != 1 {
		t.Fatalf("expected attempt_count=1, got %d", delivery.AttemptCount)
	}
	if delivery.ClusterID != "6" || delivery.ClusterName != "alpha" {
		t.Fatalf("unexpected cluster info in delivery: %+v", delivery)
	}
	if delivery.ChannelName != channel.Name {
		t.Fatalf("expected channel name %s, got %s", channel.Name, delivery.ChannelName)
	}
	if delivery.AlertName != "HighCpuUsage" {
		t.Fatalf("expected alert name HighCpuUsage, got %s", delivery.AlertName)
	}

	result, err = service.HandleAlertmanagerWebhook(ctx, firingPayload)
	if err != nil {
		t.Fatalf("HandleAlertmanagerWebhook duplicate firing returned error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no duplicate firing errors, got %v", result.Errors)
	}
	if got := hitCount.Load(); got != 1 {
		t.Fatalf("expected duplicate firing webhook to be deduplicated, got hit count %d", got)
	}

	resolvedPayload := &AlertmanagerWebhookPayload{
		Receiver:          firingPayload.Receiver,
		Status:            "resolved",
		CommonLabels:      firingPayload.CommonLabels,
		CommonAnnotations: firingPayload.CommonAnnotations,
		Alerts: []*WebhookAlert{{
			Status:       "resolved",
			Fingerprint:  "fp-1",
			StartsAt:     startsAt,
			EndsAt:       startsAt.Add(5 * time.Minute),
			GeneratorURL: "http://prom.example/rule",
		}},
	}

	result, err = service.HandleAlertmanagerWebhook(ctx, resolvedPayload)
	if err != nil {
		t.Fatalf("HandleAlertmanagerWebhook resolved returned error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no resolved processing errors, got %v", result.Errors)
	}
	if got := hitCount.Load(); got != 1 {
		t.Fatalf("expected resolved delivery to be skipped when send_resolved=false, got hit count %d", got)
	}

	route.SendResolved = true
	if err := repo.SaveNotificationRoute(ctx, route); err != nil {
		t.Fatalf("failed to enable send_resolved: %v", err)
	}

	result, err = service.HandleAlertmanagerWebhook(ctx, resolvedPayload)
	if err != nil {
		t.Fatalf("HandleAlertmanagerWebhook resolved after enable returned error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no resolved processing errors after enabling route, got %v", result.Errors)
	}
	if got := hitCount.Load(); got != 2 {
		t.Fatalf("expected resolved notification to be sent once, got hit count %d", got)
	}

	resolvedDelivery, err := repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeResolved))
	if err != nil {
		t.Fatalf("failed to load resolved delivery: %v", err)
	}
	if resolvedDelivery == nil {
		t.Fatal("expected resolved delivery to be created")
	}
	if resolvedDelivery.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("expected resolved delivery status sent, got %s", resolvedDelivery.Status)
	}
}

func TestService_HandleAlertmanagerWebhook_skipsMutedAcknowledgedRemoteAlert(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)
	ctx := context.Background()

	var hitCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	channel := &NotificationChannel{
		Name:     "ack-muted-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	route := &NotificationRoute{
		Name:               "remote all",
		Enabled:            true,
		SourceType:         string(AlertSourceTypeRemoteAlertmanager),
		ChannelID:          channel.ID,
		SendResolved:       true,
		MuteIfAcknowledged: true,
		MuteIfSilenced:     true,
	}
	if err := repo.CreateNotificationRoute(ctx, route); err != nil {
		t.Fatalf("failed to create notification route: %v", err)
	}

	startsAt := time.Date(2026, 3, 8, 11, 0, 0, 0, time.UTC)
	sourceKey := buildRemoteAlertSourceKey("fp-ack", startsAt.Unix())
	if err := repo.SaveAlertState(ctx, &AlertState{
		SourceType:     AlertSourceTypeRemoteAlertmanager,
		SourceKey:      sourceKey,
		ClusterID:      "6",
		HandlingStatus: AlertHandlingStatusAcknowledged,
		AcknowledgedBy: "tester",
		AcknowledgedAt: func() *time.Time { t := time.Now().UTC(); return &t }(),
	}); err != nil {
		t.Fatalf("failed to save acknowledged state: %v", err)
	}

	payload := &AlertmanagerWebhookPayload{
		Receiver: "ops",
		Status:   "firing",
		CommonLabels: map[string]string{
			"alertname":    "DiskFull",
			"severity":     "warning",
			"cluster_id":   "6",
			"cluster_name": "alpha",
		},
		CommonAnnotations: map[string]string{
			"summary": "Disk usage is high",
		},
		Alerts: []*WebhookAlert{{
			Status:      "firing",
			Fingerprint: "fp-ack",
			StartsAt:    startsAt,
		}},
	}

	result, err := service.HandleAlertmanagerWebhook(ctx, payload)
	if err != nil {
		t.Fatalf("HandleAlertmanagerWebhook returned error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no processing errors, got %v", result.Errors)
	}
	if got := hitCount.Load(); got != 0 {
		t.Fatalf("expected acknowledged alert to be muted, got hit count %d", got)
	}

	var deliveryCount int64
	if err := database.Model(&NotificationDelivery{}).Count(&deliveryCount).Error; err != nil {
		t.Fatalf("failed to count deliveries: %v", err)
	}
	if deliveryCount != 0 {
		t.Fatalf("expected no delivery rows for muted alert, got %d", deliveryCount)
	}
}

func TestService_HandleAlertmanagerWebhook_dispatchesManagedMetricPolicyFiringAndResolved(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)
	ctx := context.Background()

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

	user := &auth.User{
		Username: "admin",
		Nickname: "管理员",
		Email:    "admin@example.com",
		IsActive: true,
		IsAdmin:  true,
	}
	if err := database.Create(user).Error; err != nil {
		t.Fatalf("failed to create notification recipient user: %v", err)
	}

	channel := &NotificationChannel{
		Name:     "managed-webhook",
		Type:     NotificationChannelTypeWebhook,
		Enabled:  true,
		Endpoint: server.URL,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	channelIDsJSON, err := marshalAlertPolicyChannelIDs([]uint{channel.ID})
	if err != nil {
		t.Fatalf("failed to marshal channel ids: %v", err)
	}
	receiverUserIDsJSON, err := marshalAlertPolicyReceiverUserIDs([]uint64{user.ID})
	if err != nil {
		t.Fatalf("failed to marshal receiver user ids: %v", err)
	}

	policy := &AlertPolicy{
		Name:                       "内存 0.5",
		Description:                "堆内存持续过高",
		PolicyType:                 AlertPolicyBuilderKindMetricsTemplate,
		TemplateKey:                "memory_usage_high",
		ClusterID:                  "6",
		Severity:                   AlertSeverityCritical,
		Enabled:                    true,
		CooldownMinutes:            10,
		SendRecovery:               true,
		NotificationChannelIDsJSON: channelIDsJSON,
		ReceiverUserIDsJSON:        receiverUserIDsJSON,
	}
	if err := database.Create(policy).Error; err != nil {
		t.Fatalf("failed to create managed alert policy: %v", err)
	}

	startsAt := time.Date(2026, 3, 10, 1, 0, 0, 0, time.UTC)
	firingPayload := &AlertmanagerWebhookPayload{
		Receiver: "stx-webhook",
		Status:   "firing",
		CommonLabels: map[string]string{
			"alertname":    sanitizeManagedPrometheusAlertName(policy.ID),
			"managed_by":   "stx",
			"policy_id":    policyIDLabel(policy),
			"policy_name":  "内存 0.5",
			"severity":     "critical",
			"cluster_id":   "6",
			"cluster_name": "t3",
			"instance":     "38.55.133.202:5801",
		},
		CommonAnnotations: map[string]string{
			"summary":       "t3 / 38.55.133.202:5801 触发指标告警：内存 0.5（内存异常）",
			"description":   "堆内存持续过高；当前值：0.732100；持续 1 分钟，条件：> 0.05。",
			"current_value": "0.732100",
			"condition":     "持续 1 分钟，条件：> 0.05",
		},
		Alerts: []*WebhookAlert{{
			Status:       "firing",
			Fingerprint:  "managed-fp-1",
			StartsAt:     startsAt,
			GeneratorURL: "http://127.0.0.1:9090/graph?g0.expr=memory",
		}},
	}

	result, err := service.HandleAlertmanagerWebhook(ctx, firingPayload)
	if err != nil {
		t.Fatalf("HandleAlertmanagerWebhook firing returned error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no firing processing errors, got %v", result.Errors)
	}
	if got := hitCount.Load(); got != 1 {
		t.Fatalf("expected firing webhook to be delivered once, got %d", got)
	}

	sourceKey := buildRemoteAlertSourceKey("managed-fp-1", startsAt.Unix())
	firingDelivery, err := repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeFiring))
	if err != nil {
		t.Fatalf("failed to load firing delivery: %v", err)
	}
	if firingDelivery == nil {
		t.Fatal("expected firing delivery to be created")
	}
	if firingDelivery.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("expected firing delivery status sent, got %s", firingDelivery.Status)
	}
	if firingDelivery.PolicyID != policy.ID {
		t.Fatalf("expected policy id %d, got %d", policy.ID, firingDelivery.PolicyID)
	}

	body, _ := lastBody.Load().(string)
	if !strings.Contains(body, "内存 0.5") || !strings.Contains(body, "告警") {
		t.Fatalf("expected managed firing payload to contain chinese alert content, got %s", body)
	}

	resolvedPayload := &AlertmanagerWebhookPayload{
		Receiver:          firingPayload.Receiver,
		Status:            "resolved",
		CommonLabels:      firingPayload.CommonLabels,
		CommonAnnotations: firingPayload.CommonAnnotations,
		Alerts: []*WebhookAlert{{
			Status:       "resolved",
			Fingerprint:  "managed-fp-1",
			StartsAt:     startsAt,
			EndsAt:       startsAt.Add(2 * time.Minute),
			GeneratorURL: firingPayload.Alerts[0].GeneratorURL,
		}},
	}

	result, err = service.HandleAlertmanagerWebhook(ctx, resolvedPayload)
	if err != nil {
		t.Fatalf("HandleAlertmanagerWebhook resolved returned error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no resolved processing errors, got %v", result.Errors)
	}
	if got := hitCount.Load(); got != 2 {
		t.Fatalf("expected resolved webhook to be delivered once, got %d", got)
	}

	resolvedDelivery, err := repo.GetNotificationDeliveryByDedupKey(ctx, sourceKey, channel.ID, string(NotificationDeliveryEventTypeResolved))
	if err != nil {
		t.Fatalf("failed to load resolved delivery: %v", err)
	}
	if resolvedDelivery == nil {
		t.Fatal("expected resolved delivery to be created")
	}
	if resolvedDelivery.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("expected resolved delivery status sent, got %s", resolvedDelivery.Status)
	}

	body, _ = lastBody.Load().(string)
	if !strings.Contains(body, "已恢复") && !strings.Contains(body, "恢复") {
		t.Fatalf("expected managed resolved payload to contain chinese recovery content, got %s", body)
	}
}

func TestService_ListNotificationDeliveries_returnsFilteredHistory(t *testing.T) {
	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)
	ctx := context.Background()
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)

	deliveries := []*NotificationDelivery{
		{
			AlertID:      "remote:fp-a:1",
			SourceType:   string(AlertSourceTypeRemoteAlertmanager),
			SourceKey:    "remote:fp-a:1",
			ClusterID:    "6",
			ClusterName:  "alpha",
			AlertName:    "HighCpu",
			ChannelID:    1,
			ChannelName:  "ops-webhook",
			EventType:    string(NotificationDeliveryEventTypeFiring),
			Status:       string(NotificationDeliveryStatusSent),
			AttemptCount: 1,
			SentAt:       func() *time.Time { t := now.Add(-2 * time.Minute); return &t }(),
			CreatedAt:    now.Add(-3 * time.Minute),
			UpdatedAt:    now.Add(-2 * time.Minute),
		},
		{
			AlertID:      "remote:fp-b:2",
			SourceType:   string(AlertSourceTypeRemoteAlertmanager),
			SourceKey:    "remote:fp-b:2",
			ClusterID:    "7",
			ClusterName:  "beta",
			AlertName:    "DiskFull",
			ChannelID:    2,
			ChannelName:  "wechat",
			EventType:    string(NotificationDeliveryEventTypeResolved),
			Status:       string(NotificationDeliveryStatusFailed),
			AttemptCount: 2,
			LastError:    "http status 500",
			CreatedAt:    now.Add(-90 * time.Second),
			UpdatedAt:    now.Add(-60 * time.Second),
		},
	}
	for _, delivery := range deliveries {
		if err := database.Create(delivery).Error; err != nil {
			t.Fatalf("failed to create delivery seed data: %v", err)
		}
	}

	data, err := service.ListNotificationDeliveries(ctx, &NotificationDeliveryFilter{
		ClusterID: "7",
		Status:    NotificationDeliveryStatusFailed,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListNotificationDeliveries returned error: %v", err)
	}
	if data.Total != 1 {
		t.Fatalf("expected total=1, got %d", data.Total)
	}
	if len(data.Deliveries) != 1 {
		t.Fatalf("expected 1 delivery item, got %d", len(data.Deliveries))
	}
	if data.Deliveries[0].ClusterID != "7" || data.Deliveries[0].Status != string(NotificationDeliveryStatusFailed) {
		t.Fatalf("unexpected filtered delivery: %+v", data.Deliveries[0])
	}
}
