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
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/auth"
	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupMonitoringAlertPolicyTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "monitoring_alert_policy_test_*")
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
		&auth.User{},
		&clusterapp.Cluster{},
		&clusterapp.ClusterNode{},
		&hostapp.Host{},
		&monitor.ProcessEvent{},
		&AlertRule{},
		&AlertPolicy{},
		&AlertState{},
		&AlertEventState{},
		&NotificationChannel{},
		&NotificationDelivery{},
		&SyncAlertEpisode{},
		&ClusterRuleConfig{},
		&ClusterRuleOverride{},
		&AlertNotificationDefaults{},
		&RemoteAlertRecord{},
	); err != nil {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("failed to migrate alert policy test tables: %v", err)
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

func setupMonitoringAlertPolicyService(t *testing.T) (*Service, *Repository, *clusterapp.Cluster, context.Context) {
	t.Helper()

	database, cleanup := setupMonitoringAlertPolicyTestDB(t)
	t.Cleanup(cleanup)

	clusterRepo := clusterapp.NewRepository(database)
	clusterService := clusterapp.NewService(clusterRepo, nil, nil)
	repo := NewRepository(database)
	monitorRepo := monitor.NewRepository(database)
	monitorService := monitor.NewService(monitorRepo)
	service := NewService(clusterService, monitorService, repo)
	oldTTL := clustersHealthCacheTTL
	clustersHealthCacheTTL = 0
	t.Cleanup(func() { clustersHealthCacheTTL = oldTTL })
	ctx := context.Background()

	testCluster := &clusterapp.Cluster{
		Name:           "policy-bridge-cluster",
		Description:    "test cluster",
		DeploymentMode: clusterapp.DeploymentModeHybrid,
		Version:        "2.3.0",
		Status:         clusterapp.ClusterStatusCreated,
		InstallDir:     "/tmp/seatunnel",
		CreatedBy:      1,
	}
	if err := database.WithContext(ctx).Create(testCluster).Error; err != nil {
		t.Fatalf("failed to create test cluster: %v", err)
	}

	return service, repo, testCluster, ctx
}

func TestUpsertAlertPolicyRequestValidate(t *testing.T) {
	// 非 sync_job 的策略类型已被集群级规则目录取代，写接口一律拒绝并提示改用 cluster-rules。
	// Non-sync_job policy types were replaced by the cluster rule catalog; the write API rejects
	// them all with guidance to use cluster-rules.
	for _, policyType := range []AlertPolicyBuilderKind{
		AlertPolicyBuilderKindPlatformHealth,
		AlertPolicyBuilderKindMetricsTemplate,
		AlertPolicyBuilderKind("custom_promql"),
	} {
		req := &UpsertAlertPolicyRequest{
			Name:       "legacy policy",
			PolicyType: policyType,
			Severity:   AlertSeverityCritical,
		}
		if err := req.Validate(); !errors.Is(err, ErrAlertPolicyUseClusterRules) {
			t.Fatalf("expected ErrAlertPolicyUseClusterRules for %s, got %v", policyType, err)
		}
	}
}

func TestNormalizeAlertPolicyChannelIDs(t *testing.T) {
	ids := normalizeAlertPolicyChannelIDs([]uint{3, 2, 3, 0, 1})
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(ids))
	}
	expected := []uint{1, 2, 3}
	for idx, value := range expected {
		if ids[idx] != value {
			t.Fatalf("expected ids[%d]=%d, got %d", idx, value, ids[idx])
		}
	}
}

func TestNormalizeReceiverUserIDs(t *testing.T) {
	ids := normalizeReceiverUserIDs([]uint64{5, 2, 5, 0, 3})
	expected := []uint64{2, 3, 5}
	if len(ids) != len(expected) {
		t.Fatalf("expected %d ids, got %d", len(expected), len(ids))
	}
	for idx, value := range expected {
		if ids[idx] != value {
			t.Fatalf("expected ids[%d]=%d, got %d", idx, value, ids[idx])
		}
	}
}

func TestCreateAlertPolicy_persistsReceiverUserIDs(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	adminUser := &auth.User{
		Username: "admin-alert",
		Nickname: "Admin Alert",
		Email:    "admin-alert@example.com",
		IsActive: true,
		IsAdmin:  true,
	}
	if err := adminUser.SetPassword("admin123", auth.DefaultBcryptCost); err != nil {
		t.Fatalf("failed to set password: %v", err)
	}
	if err := service.repo.db.WithContext(ctx).Create(adminUser).Error; err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}

	clusterID := "all"
	if cluster != nil {
		clusterID = strconv.FormatUint(uint64(cluster.ID), 10)
	}

	channel := &NotificationChannel{
		Name:       "persist-receivers-webhook",
		Type:       NotificationChannelTypeWebhook,
		Enabled:    true,
		ConfigJSON: `{"webhook":{"url":"http://127.0.0.1:1/hook"}}`,
	}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatalf("failed to create notification channel: %v", err)
	}

	created, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:       "Job failed notify admins",
		PolicyType: AlertPolicyBuilderKindSyncJob,
		ClusterID:  clusterID,
		Severity:   AlertSeverityCritical,
		TaskID:     7,
		Conditions: []*AlertPolicyConditionDTO{
			{MetricKey: syncalert.RuleJobFailed, Operator: ">=", Threshold: "1"},
		},
		NotificationChannelIDs: []uint{channel.ID},
		ReceiverUserIDs:        []uint64{adminUser.ID},
	})
	if err != nil {
		t.Fatalf("expected create success, got %v", err)
	}
	if len(created.ReceiverUserIDs) != 1 || created.ReceiverUserIDs[0] != adminUser.ID {
		t.Fatalf("unexpected receiver user ids: %+v", created.ReceiverUserIDs)
	}

	stored, err := repo.GetAlertPolicyByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to reload policy: %v", err)
	}
	if got := unmarshalAlertPolicyReceiverUserIDs(stored.ReceiverUserIDsJSON); len(got) != 1 || got[0] != adminUser.ID {
		t.Fatalf("unexpected stored receiver user ids: %+v", got)
	}
}

func TestResolveNotificationChannelTestRecipients(t *testing.T) {
	service, _, _, ctx := setupMonitoringAlertPolicyService(t)

	channel := &NotificationChannel{Type: NotificationChannelTypeEmail}
	validUser := &auth.User{
		Username: "notify-user",
		Nickname: "Notify User",
		Email:    "notify-user@example.com",
		IsActive: true,
	}
	if err := validUser.SetPassword("notify123", auth.DefaultBcryptCost); err != nil {
		t.Fatalf("failed to set valid user password: %v", err)
	}
	if err := service.repo.db.WithContext(ctx).Create(validUser).Error; err != nil {
		t.Fatalf("failed to create valid user: %v", err)
	}

	t.Run("returns selected user email", func(t *testing.T) {
		recipients, receiver, err := service.resolveNotificationChannelTestRecipients(ctx, channel, &NotificationChannelTestRequest{
			ReceiverUserID: validUser.ID,
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if len(recipients) != 1 || recipients[0] != validUser.Email {
			t.Fatalf("unexpected recipients: %+v", recipients)
		}
		if receiver != validUser.Email {
			t.Fatalf("expected receiver %q, got %q", validUser.Email, receiver)
		}
	})

	t.Run("rejects user without valid email", func(t *testing.T) {
		invalidUser := &auth.User{
			Username: "notify-user-no-email",
			Nickname: "Notify User Missing Email",
			Email:    "",
			IsActive: true,
		}
		if err := invalidUser.SetPassword("notify123", auth.DefaultBcryptCost); err != nil {
			t.Fatalf("failed to set invalid user password: %v", err)
		}
		if err := service.repo.db.WithContext(ctx).Create(invalidUser).Error; err != nil {
			t.Fatalf("failed to create invalid user: %v", err)
		}

		_, _, err := service.resolveNotificationChannelTestRecipients(ctx, channel, &NotificationChannelTestRequest{
			ReceiverUserID: invalidUser.ID,
		})
		if err != ErrAlertPolicyReceiverUserEmailMissing {
			t.Fatalf("expected ErrAlertPolicyReceiverUserEmailMissing, got %v", err)
		}
	})
}
