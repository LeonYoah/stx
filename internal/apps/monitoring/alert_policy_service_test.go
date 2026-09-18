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
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/LeonYoah/stx/internal/apps/auth"
	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
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
		&monitor.ProcessEvent{},
		&AlertRule{},
		&AlertPolicy{},
		&AlertState{},
		&NotificationChannel{},
		&NotificationDelivery{},
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
	t.Run("platform health requires template key", func(t *testing.T) {
		req := &UpsertAlertPolicyRequest{
			Name:       "master unavailable",
			PolicyType: AlertPolicyBuilderKindPlatformHealth,
			Severity:   AlertSeverityCritical,
		}
		if err := req.Validate(); err == nil {
			t.Fatalf("expected validation error, got nil")
		}
	})

	t.Run("custom promql requires promql", func(t *testing.T) {
		req := &UpsertAlertPolicyRequest{
			Name:       "failed jobs",
			PolicyType: AlertPolicyBuilderKindCustomPromQL,
			Severity:   AlertSeverityWarning,
		}
		if err := req.Validate(); err == nil {
			t.Fatalf("expected validation error, got nil")
		}
	})

	t.Run("valid metrics template request passes", func(t *testing.T) {
		cooldown := 15
		req := &UpsertAlertPolicyRequest{
			Name:            "cpu high",
			PolicyType:      AlertPolicyBuilderKindMetricsTemplate,
			TemplateKey:     "cpu_usage_high",
			Severity:        AlertSeverityCritical,
			CooldownMinutes: &cooldown,
			Conditions: []*AlertPolicyConditionDTO{
				{
					MetricKey:     "cpu_usage",
					Operator:      ">",
					Threshold:     "80",
					WindowMinutes: 5,
				},
			},
		}
		if err := req.Validate(); err != nil {
			t.Fatalf("expected validation success, got %v", err)
		}
	})
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

	created, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:            "Node offline notify admins",
		PolicyType:      AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:     AlertRuleKeyNodeOffline,
		ClusterID:       clusterID,
		Severity:        AlertSeverityCritical,
		ReceiverUserIDs: []uint64{adminUser.ID},
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

func TestGetAlertPolicyCenterBootstrap_returnsDefaultAdminRecipients(t *testing.T) {
	service, _, _, ctx := setupMonitoringAlertPolicyService(t)

	adminUser := &auth.User{
		Username: "bootstrap-admin",
		Nickname: "Bootstrap Admin",
		Email:    "bootstrap-admin@example.com",
		IsActive: true,
		IsAdmin:  true,
	}
	if err := adminUser.SetPassword("admin123", auth.DefaultBcryptCost); err != nil {
		t.Fatalf("failed to set password: %v", err)
	}
	if err := service.repo.db.WithContext(ctx).Create(adminUser).Error; err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}

	bootstrap, err := service.GetAlertPolicyCenterBootstrap(ctx)
	if err != nil {
		t.Fatalf("GetAlertPolicyCenterBootstrap returned error: %v", err)
	}
	if len(bootstrap.NotifiableUsers) == 0 {
		t.Fatal("expected notifiable users in bootstrap")
	}
	if len(bootstrap.DefaultReceiverUserIDs) == 0 || bootstrap.DefaultReceiverUserIDs[0] != adminUser.ID {
		t.Fatalf("unexpected default receiver user ids: %+v", bootstrap.DefaultReceiverUserIDs)
	}
}

func TestCreateAlertPolicy_syncsLegacyRuntimeRule(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	enabled := false
	req := &UpsertAlertPolicyRequest{
		Name:          "Process restart failed - production",
		Description:   "Escalate restart failures through unified policy center",
		PolicyType:    AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:   AlertRuleKeyProcessRestartFailed,
		LegacyRuleKey: AlertRuleKeyProcessRestartFailed,
		ClusterID:     "0",
		Severity:      AlertSeverityWarning,
		Enabled:       &enabled,
	}
	req.ClusterID = "1"
	if cluster != nil {
		req.ClusterID = strconv.FormatUint(uint64(cluster.ID), 10)
	}

	created, err := service.CreateAlertPolicy(ctx, req)
	if err != nil {
		t.Fatalf("expected create success, got %v", err)
	}
	if created.LegacyRuleKey != AlertRuleKeyProcessRestartFailed {
		t.Fatalf("expected legacy rule key to be canonicalized, got %s", created.LegacyRuleKey)
	}

	rule, err := repo.GetRuleByClusterAndKey(ctx, cluster.ID, AlertRuleKeyProcessRestartFailed)
	if err != nil {
		t.Fatalf("failed to get synced legacy rule: %v", err)
	}
	if rule.RuleName != req.Name {
		t.Fatalf("expected synced rule name %q, got %q", req.Name, rule.RuleName)
	}
	if rule.Description != req.Description {
		t.Fatalf("expected synced description %q, got %q", req.Description, rule.Description)
	}
	if rule.Severity != AlertSeverityWarning {
		t.Fatalf("expected warning severity, got %s", rule.Severity)
	}
	if rule.Enabled {
		t.Fatalf("expected synced rule to be disabled")
	}
	if rule.Threshold != 1 || rule.WindowSeconds != 300 {
		t.Fatalf("expected bridge to keep default threshold/window, got threshold=%d window=%d", rule.Threshold, rule.WindowSeconds)
	}
}

func TestCreateAlertPolicy_legacyBridgeRequiresConcreteCluster(t *testing.T) {
	service, _, _, ctx := setupMonitoringAlertPolicyService(t)

	_, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:          "Restart failed - global",
		Description:   "invalid global bridge",
		PolicyType:    AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:   AlertRuleKeyProcessRestartFailed,
		LegacyRuleKey: AlertRuleKeyProcessRestartFailed,
		ClusterID:     "all",
		Severity:      AlertSeverityCritical,
	})
	if err == nil {
		t.Fatalf("expected bridge scope validation error, got nil")
	}
	if err != ErrAlertPolicyLegacyBridgeRequiresClusterScope {
		t.Fatalf("expected ErrAlertPolicyLegacyBridgeRequiresClusterScope, got %v", err)
	}
}

func TestCreateAlertPolicy_duplicateLegacyBridgeRejected(t *testing.T) {
	service, _, cluster, ctx := setupMonitoringAlertPolicyService(t)

	clusterID := "1"
	if cluster != nil {
		clusterID = strconv.FormatUint(uint64(cluster.ID), 10)
	}

	_, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:          "Restart failed policy A",
		PolicyType:    AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:   AlertRuleKeyProcessRestartFailed,
		LegacyRuleKey: AlertRuleKeyProcessRestartFailed,
		ClusterID:     clusterID,
		Severity:      AlertSeverityCritical,
	})
	if err != nil {
		t.Fatalf("failed to create first legacy-backed policy: %v", err)
	}

	_, err = service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:          "Restart failed policy B",
		PolicyType:    AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:   AlertRuleKeyProcessRestartFailed,
		LegacyRuleKey: AlertRuleKeyProcessRestartFailed,
		ClusterID:     clusterID,
		Severity:      AlertSeverityWarning,
	})
	if err == nil {
		t.Fatalf("expected duplicate bridge conflict, got nil")
	}
	if err != ErrAlertPolicyLegacyBridgeConflict {
		t.Fatalf("expected ErrAlertPolicyLegacyBridgeConflict, got %v", err)
	}
}

func TestDeleteAlertPolicy_restoresDefaultLegacyRuntimeRule(t *testing.T) {
	service, repo, cluster, ctx := setupMonitoringAlertPolicyService(t)

	enabled := false
	created, err := service.CreateAlertPolicy(ctx, &UpsertAlertPolicyRequest{
		Name:          "Restart failed custom",
		Description:   "custom description",
		PolicyType:    AlertPolicyBuilderKindPlatformHealth,
		TemplateKey:   AlertRuleKeyProcessRestartFailed,
		LegacyRuleKey: AlertRuleKeyProcessRestartFailed,
		ClusterID:     strconv.FormatUint(uint64(cluster.ID), 10),
		Severity:      AlertSeverityWarning,
		Enabled:       &enabled,
	})
	if err != nil {
		t.Fatalf("failed to create legacy-backed policy: %v", err)
	}

	if err := service.DeleteAlertPolicy(ctx, created.ID); err != nil {
		t.Fatalf("failed to delete alert policy: %v", err)
	}

	rule, err := repo.GetRuleByClusterAndKey(ctx, cluster.ID, AlertRuleKeyProcessRestartFailed)
	if err != nil {
		t.Fatalf("failed to get restored legacy rule: %v", err)
	}
	defaultRule := defaultRuleByKey(cluster.ID, AlertRuleKeyProcessRestartFailed)
	if defaultRule == nil {
		t.Fatalf("expected default rule definition")
	}
	if rule.RuleName != defaultRule.RuleName {
		t.Fatalf("expected restored rule name %q, got %q", defaultRule.RuleName, rule.RuleName)
	}
	if rule.Description != defaultRule.Description {
		t.Fatalf("expected restored rule description %q, got %q", defaultRule.Description, rule.Description)
	}
	if rule.Severity != defaultRule.Severity {
		t.Fatalf("expected restored severity %s, got %s", defaultRule.Severity, rule.Severity)
	}
	if rule.Enabled != defaultRule.Enabled {
		t.Fatalf("expected restored enabled %v, got %v", defaultRule.Enabled, rule.Enabled)
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
